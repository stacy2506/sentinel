package vault

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
)

// Mount starts the FUSE filesystem in a background goroutine.
func Mount(mountPoint, realDir string) error {

	// --- STEP 0: DIAGNOSTICS ---
	fmt.Println("   [?] Running Pre-Flight Checks...")

	// Check 1: Is Go Sandboxed? (Snap issue)
	if os.Getenv("SNAP_NAME") != "" {
		return fmt.Errorf("CRITICAL: You are running Go via SNAP. Snap prevents writing to /home. \n       Please install Go via apt or standard tarball.")
	}

	// Check 2: Clean up Zombies explicitly
	// We force unmount blindly just to be safe.
	_ = exec.Command("fusermount", "-u", "-z", mountPoint).Run()
	_ = exec.Command("umount", "-l", mountPoint).Run()

	// --- STEP 1: FORCE CREATION ---

	// Setup Real Dir
	if err := assertDirectory(realDir); err != nil {
		return err
	}

	// Setup Mount Point
	if err := assertDirectory(mountPoint); err != nil {
		return err
	}

	// --- STEP 2: MOUNT ---
	fmt.Printf("   [i] Mounting FUSE to: %s\n", mountPoint)

	c, err := fuse.Mount(
		mountPoint,
		fuse.FSName("Sentinel"),
		fuse.Subtype("secure"),
		fuse.AllowOther(), // Required for visibility
	)
	if err != nil {
		return fmt.Errorf("mount syscall failed: %w", err)
	}

	// --- STEP 3: SERVE ---
	go func() {
		defer c.Close()
		err := fs.Serve(c, &FS{RealDir: realDir})
		if err != nil {
			fmt.Printf("❌ FUSE Server Error: %v\n", err)
		}
	}()

	// Wait for stabilization
	time.Sleep(500 * time.Millisecond)

	// Final verification
	if _, err := os.Stat(mountPoint); err != nil {
		return fmt.Errorf("mount failed to stabilize (check /etc/fuse.conf for 'user_allow_other')")
	}

	return nil
}

// assertDirectory forces a directory to exist using system commands
func assertDirectory(path string) error {
	// 1. Check if it already exists
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("CRITICAL: Path '%s' exists but is a FILE, not a directory", path)
		}
		return nil // It exists, we are good.
	}

	// 2. It doesn't exist, create it.
	fmt.Printf("   [+] Creating: %s\n", path)

	cmd := exec.Command("mkdir", "-p", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// return the EXACT error from the OS
		return fmt.Errorf("mkdir failed: %s (Err: %v)", string(output), err)
	}

	// 3. Fix Permissions (chmod 777)
	_ = exec.Command("chmod", "777", path).Run()

	// 4. Double Check
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("CRITICAL: System claimed success, but directory '%s' is STILL missing.\n       Possible causes: Read-only Filesystem, AppArmor, or SELinux.", path)
	}

	return nil
}

// Unmount forces the drive to disconnect
func Unmount(mountPoint string) error {
	// -u = unmount, -z = lazy (detach immediately even if busy)
	cmd := exec.Command("fusermount", "-u", "-z", mountPoint)

	if err := cmd.Run(); err != nil {
		// Fallback for non-FUSE mounts
		return exec.Command("umount", "-l", mountPoint).Run()
	}
	return nil
}
