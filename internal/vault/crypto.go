package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// --- CONFIGURATION ---
const (
	// The persistent location for the key
	KeyDir  = "/etc/sentinel"
	KeyFile = "master.key"
)

// We use a private variable that defaults to nil
var activeKey []byte

// LoadKey handles the entire lifecycle of the master key
func LoadKey() error {
	fullPath := filepath.Join(KeyDir, KeyFile)

	// 1. Check if key file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		fmt.Println("   [i] Key not found. Generating new Master Key...")
		return generateAndSaveKey(fullPath)
	}

	// 2. Load existing key
	return loadExistingKey(fullPath)
}

// Generate a new random key and saves it securely
func generateAndSaveKey(path string) error {
	// Create directory if it doesn't exist
	if err := os.MkdirAll(KeyDir, 0700); err != nil {
		return fmt.Errorf("failed to create key directory: %w", err)
	} // We use a private variable that defaults to nil

	// Generate 32 bytes of random entropy
	newKey := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, newKey); err != nil {
		return fmt.Errorf("failed to generate random key: %w", err)
	}

	// Write to file with strict permissions (0400 = Read only by owner/root)
	if err := os.WriteFile(path, newKey, 0400); err != nil {
		return fmt.Errorf("failed to save key to disk: %w", err)
	}

	// Set into memory
	activeKey = newKey
	fmt.Println("   [+] New Master Key generated and saved to", path)
	return nil
}

// Reads the key and performs sanity checks
func loadExistingKey(path string) error {
	keyBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read key file (check permissions): %w", err)
	}

	// Tamper Check: AES-256 keys MUST be 32 bytes.
	if len(keyBytes) != 32 {
		return fmt.Errorf("SECURITY ALERT: Key file at %s is corrupted or tampered (Size: %d bytes)", path, len(keyBytes))
	}

	activeKey = keyBytes
	return nil
}

// Uses the activeKey to encrypte data
func Encrypt(plaintext []byte) ([]byte, error) {
	if activeKey == nil {
		return nil, errors.New("VAULT LOCKED: No key loaded")
	}

	block, err := aes.NewCipher(activeKey)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Uses the activeKey to decrypt data
func Decrypt(data []byte) ([]byte, error) {
	if activeKey == nil {
		return nil, errors.New("VAULT LOCKED: No key loaded")
	}

	block, err := aes.NewCipher(activeKey) // Use the RAM key
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("decryption failed or data corrupted")
	}

	return plaintext, nil
}
