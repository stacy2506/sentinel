package vault

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// ==========================================
// 1. THE FILESYSTEM ( The Entry Point )

type FS struct {
	RealDir string // The actual path on disk
}

func (f *FS) Root() (fs.Node, error) {
	return &Dir{Path: f.RealDir}, nil
}

// ==========================================
// 2. THE DIRECTORY ( Handling Folders )

type Dir struct {
	Path string
	mu   sync.Mutex

	nodes map[string]fs.Node
}

// Attr: Update permissions to 0755 (Read/Write/Execute) so we can create files

func (d *Dir) Attr(ctx context.Context, a *fuse.Attr) error {
	info, err := os.Stat(d.Path)
	if err != nil {
		return err
	}
	a.Inode = 1
	a.Mode = os.ModeDir | 0755
	a.Size = uint64(info.Size())
	return nil
}

func (d *Dir) ReadDirAll(ctx context.Context) ([]fuse.Dirent, error) {
	entries, err := os.ReadDir(d.Path)
	if err != nil {
		return nil, err
	}

	var fuseEntries []fuse.Dirent
	for _, entry := range entries {
		var f fuse.Dirent
		f.Name = entry.Name()
		if entry.IsDir() {
			f.Type = fuse.DT_Dir
		} else {
			f.Type = fuse.DT_File
		}
		fuseEntries = append(fuseEntries, f)
	}
	return fuseEntries, nil
}

func (d *Dir) Lookup(ctx context.Context, name string) (fs.Node, error) {
	//lock to prevent race conditions
	d.mu.Lock()
	defer d.mu.Unlock()

	//check if the data in the nodes map and return it
	if node, ok := d.nodes[name]; ok {
		return node, nil
	}

	if d.nodes == nil {
		d.nodes = make(map[string]fs.Node)
	}

	//create the object if not exists
	realPath := filepath.Join(d.Path, name)

	info, err := os.Stat(realPath)
	if err != nil {
		return nil, syscall.ENOENT
	}

	var node fs.Node
	if info.IsDir() {
		node = &Dir{Path: realPath, nodes: make(map[string]fs.Node)}
	} else {
		node = &File{Path: realPath}
	}

	d.nodes[name] = node

	return node, nil
}

// Handles "touch newfile.txt" or saving a new file
func (d *Dir) Create(ctx context.Context, req *fuse.CreateRequest, resp *fuse.CreateResponse) (fs.Node, fs.Handle, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	realPath := filepath.Join(d.Path, req.Name)

	// 1. Create empty file on Real Disk
	f, err := os.Create(realPath)
	if err != nil {
		return nil, nil, err
	}
	f.Close()

	// --- NEW: FIX OWNERSHIP ---
	// Ensure the $USER owns this file, not Root
	_ = os.Chown(realPath, int(req.Uid), int(req.Gid))

	// Create the File Object
	newFile := &File{
		Path: realPath,
		data: []byte{}, // Start empty
	}

	// Add to Cache
	if d.nodes == nil {
		d.nodes = make(map[string]fs.Node)
	}
	d.nodes[req.Name] = newFile

	return newFile, newFile, nil
}

// Remove handles "rm file.txt"
func (d *Dir) Remove(ctx context.Context, req *fuse.RemoveRequest) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	//Calculate the real path
	childPath := filepath.Join(d.Path, req.Name)

	// Delete from Real Disk
	// os.Remove works for both files and empty folders
	err := os.Remove(childPath)
	if err != nil {
		return err
	}

	//Delete from Cache
	delete(d.nodes, req.Name)

	return nil
}

// mkdir , for creating new directories
func (d *Dir) Mkdir(ctx context.Context, req *fuse.MkdirRequest) (fs.Node, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	realPath := filepath.Join(d.Path, req.Name)

	// Perform the actual mkdir on the OS
	err := os.Mkdir(realPath, req.Mode)
	if err != nil {
		return nil, err
	}

	// --- NEW: FIX OWNERSHIP ---
	_ = os.Chown(realPath, int(req.Uid), int(req.Gid))

	newDir := &Dir{
		Path:  realPath,
		nodes: make(map[string]fs.Node),
	}

	d.nodes[req.Name] = newDir
	return newDir, nil
}

//for moving and renaming files

func (d *Dir) Rename(ctx context.Context, req *fuse.RenameRequest, newDir fs.Node) error {
	// 1. Convert the generic fs.Node to our specific *Dir type
	destDir, ok := newDir.(*Dir)
	if !ok {
		return syscall.EIO // Can only move to directories we manage
	}

	// If we are moving within the same folder (Rename), lock once
	// If moving between folders, we need to lock both to prevent race conditions
	if d == destDir {
		d.mu.Lock()
		defer d.mu.Unlock()
	} else {
		// Lock both
		d.mu.Lock()
		defer d.mu.Unlock()

		destDir.mu.Lock()
		defer destDir.mu.Unlock()
	}

	// 3. Perform the Rename on the Real Disk
	oldPath := filepath.Join(d.Path, req.OldName)
	newPath := filepath.Join(destDir.Path, req.NewName)

	err := os.Rename(oldPath, newPath)
	if err != nil {
		return err
	}

	// Check if the file exists in our RAM cache
	if node, ok := d.nodes[req.OldName]; ok {

		// A. Update the Node's internal path
		// We have to check if it's a File or a Dir to update the struct field
		if fileNode, ok := node.(*File); ok {
			fileNode.Path = newPath
		} else if dirNode, ok := node.(*Dir); ok {
			dirNode.Path = newPath
		}

		// Move it to the new map (Destination)
		if destDir.nodes == nil {
			destDir.nodes = make(map[string]fs.Node)
		}
		destDir.nodes[req.NewName] = node

		//Remove from the old map (Source)
		delete(d.nodes, req.OldName)
	}

	return nil
}

//==========================================
// 3. THE FILE ( Handling Files )

type File struct {
	Path string
	mu   sync.RWMutex

	// State for "Load-Edit-Save"
	data  []byte // The decrypted content in RAM
	dirty bool   // Has it been modified?
}

// Attr: Update permissions to 0644 (Read/Write)
func (f *File) Attr(ctx context.Context, a *fuse.Attr) error {
	f.mu.RLock()
	defer f.mu.RUnlock()

	info, err := os.Stat(f.Path)
	if err != nil {
		return err
	}
	a.Mode = 0644

	// If we have data in memory (file is open), report that size
	// Otherwise, report disk size
	if f.data != nil {
		a.Size = uint64(len(f.data))
	} else {
		a.Size = uint64(info.Size())
	}

	return nil
}

// Open: Loads the encrypted file and decrypts it into RAM
func (f *File) Open(ctx context.Context, req *fuse.OpenRequest, resp *fuse.OpenResponse) (fs.Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	//if data is in memory skip the decrpytion and use the in-memory data
	if len(f.data) > 0 {
		return f, nil
	}

	// 1. Read the raw encrypted blob
	encryptedBytes, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}

	// 2. Handle empty/new files
	if len(encryptedBytes) == 0 {
		f.data = []byte{}
		return f, nil
	}

	// 3. Decrypt into RAM
	f.data, err = Decrypt(encryptedBytes)
	if err != nil {
		return nil, syscall.EIO
	}

	return f, nil
}

// read from the RAM buffer (f.data)
func (f *File) Read(ctx context.Context, req *fuse.ReadRequest, resp *fuse.ReadResponse) error {
	f.mu.RLock()
	defer f.mu.RUnlock()
	//checks is all data has been read
	if req.Offset >= int64(len(f.data)) {
		resp.Data = nil
		return nil
	}

	//check if the data we are reading is in bound to prevent Out Of Bounds Read
	end := req.Offset + int64(req.Size)
	if end > int64(len(f.data)) {
		end = int64(len(f.data))
	}

	//respond with the chunk of data
	resp.Data = f.data[req.Offset:end]
	return nil
	//loops until all the data has been read
}

// Write: Updates the RAM buffer
func (f *File) Write(ctx context.Context, req *fuse.WriteRequest, resp *fuse.WriteResponse) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dirty = true

	// Expand buffer if needed
	writeEnd := req.Offset + int64(len(req.Data))
	if writeEnd > int64(len(f.data)) {
		newBuf := make([]byte, writeEnd) //make a new larger buffer
		copy(newBuf, f.data)             // copy the data of the old buffer to the new buffer
		f.data = newBuf                  //replace the old buffer with newer larger buffer
	}

	//append the request data to buffer
	copy(f.data[req.Offset:], req.Data)

	resp.Size = len(req.Data)
	return nil
}

func (f *File) Fsync(ctx context.Context, req *fuse.FsyncRequest) error {
	// Just map Fsync to Flush
	// This ensures that when an app forces a save, we actually save
	return f.Flush(ctx, nil)
}

// Encrypts RAM buffer and saves to disk
func (f *File) Flush(ctx context.Context, req *fuse.FlushRequest) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if !f.dirty {
		return nil
	}

	//Encrypt
	encryptedBlob, err := Encrypt(f.data)
	if err != nil {
		return syscall.EIO
	}

	//Save to Disk
	err = os.WriteFile(f.Path, encryptedBlob, 0644)
	if err != nil {
		return err
	}

	f.dirty = false
	//wipe data from memory
	f.data = nil
	return nil
}
