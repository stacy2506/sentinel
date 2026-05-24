package config

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	ConfigDir  = "/etc/sentinel"
	ConfigFile = "config.json"
)

type SentinelConfig struct {
	TargetMAC     string `json:"target_mac"`
	WifiInterface string `json:"wifi_interface"`
	MaxMisses     int    `json:"max_misses"`
	MountPoint    string `json:"mount_point"`
	RealDir       string `json:"real_dir"`
	KillInternet  bool   `json:"kill_internet"`
}

func Load() (*SentinelConfig, error) {
	path := filepath.Join(ConfigDir, ConfigFile)

	// --- CHECK EXISTENCE ---
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("\n  CONFIG MISSING: INITIATING FIRST-RUN SETUP")

		if err := createDefault(path); err != nil {
			return nil, fmt.Errorf("setup failed: %w", err)
		}

		fmt.Printf("   [i] Opening %s in nano...\n", path)
		if err := openNano(path); err != nil {
			fmt.Printf("   [!] Warning: Could not open editor (%v). Edit manually.\n", err)
		} else {
			fmt.Println("   [+] Configuration saved.")
		}
	}

	// --- READ & PARSE ---
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("permission denied: %w", err)
	}

	var cfg SentinelConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("config file is corrupted: %w", err)
	}

	return &cfg, nil
}

func Edit() {
	path := filepath.Join(ConfigDir, ConfigFile)

	// 1. Create Default if missing
	if _, err := os.Stat(path); os.IsNotExist(err) {
		fmt.Println("   [+] Config missing. Creating default template...")
		if err := createDefault(path); err != nil {
			fmt.Printf("❌ Error creating default: %v\n", err)
			return
		}
	}

	// 2. Open Nano
	fmt.Printf("   [i] Opening %s in nano...\n", path)
	if err := openNano(path); err != nil {
		fmt.Printf("Error opening nano: %v\n", err)
	} else {
		fmt.Println("   [+] Configuration saved.")
	}
}

func openNano(path string) error {
	cmd := exec.Command("nano", path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// createDefault generates the template with DYNAMIC user paths
func createDefault(path string) error {
	if err := os.MkdirAll(ConfigDir, 0700); err != nil {
		return fmt.Errorf("failed to create dir: %w", err)
	}

	// 1. DETECT SUDO USER
	// Since we run as root, we need to know who the "Real" user is
	realUser := os.Getenv("SUDO_USER")
	var userHome string

	if realUser == "" {
		fmt.Println("   [!] Warning: Running as direct root (no sudo). Defaulting home to /root")
		userHome = "/root"
	} else {
		userHome = filepath.Join("/home", realUser)
	}

	fmt.Printf("   [i] Detected User: %s (Home: %s)\n", realUser, userHome)

	// 2. GENERATE CONFIG
	defaultCfg := SentinelConfig{
		TargetMAC:     "14:99:3E:C0:CE:2B",
		WifiInterface: "wlan0",
		MaxMisses:     4,

		// The Mirror, Goes to the User's Home
		MountPoint: filepath.Join(userHome, "Sentinel"),

		// The DB, Goes to a safe System Directory (Hidden/Persistent)
		RealDir: "/etc/sentinel/DB",

		KillInternet: false,
	}

	data, _ := json.MarshalIndent(defaultCfg, "", "  ")
	return os.WriteFile(path, data, 0600)
}
