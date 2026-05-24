package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	// Imports
	"github.com/As1agi/sentinel/internal/config"
	"github.com/As1agi/sentinel/internal/vault"
)

// --- GLOBAL STATE ---
// etc/sentinel/config.json
var cfg *config.SentinelConfig

// Start as TRUE. This forces the app to try and "Unlock/Mount" the moment it sees your phone for the first time
var isSystemLocked = true
var isVaultMounted = false

func main() {
	// Root Check
	if os.Geteuid() != 0 {
		fmt.Println(" ERROR: Sentinel must be run as ROOT (sudo)")
		os.Exit(1)
	}

	// --- NEW: HANDLE FLAGS ---
	// If user runs with -config, we open the editor and exit
	configMode := flag.Bool("config", false, "Open configuration editor")
	flag.Parse()

	if *configMode {
		config.Edit() // Handles create/open logic
		return        // Exit after editing
	}
	// -------------------------

	// Load Config (Smart Load: Creates & opens nano if missing)
	var err error
	cfg, err = config.Load()
	if err != nil {
		log.Fatal("Config Error: ", err)
	}
	fmt.Println("Config Loaded from Secure Storage")

	err = vault.LoadKey()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(" Key Loaded into RAM")

	// Setup Graceful Shutdown (Ctrl+C / Ctrl+Z Watchdog)
	setupGracefulExit()

	log.SetLevel(log.ErrorLevel)
	fmt.Println("🛡️ SENTINEL: ACTIVE (Ping Mode)")
	fmt.Printf("   [+] Target: %s\n", cfg.TargetMAC)
	fmt.Printf("   [+] Vault:  %s\n", cfg.MountPoint)
	fmt.Println("   [i] Press Ctrl+C to Exit Safely")

	missedPings := 0

	// --- MAIN LOOP ---
	for {
		// Active Ping
		// -c 1: One packet, -t 1: One second timeout
		cmd := exec.Command("l2ping", "-c", "1", "-t", "1", cfg.TargetMAC)
		err := cmd.Run()

		if err == nil {
			// --- PHONE IS HERE ---
			missedPings = 0

			// If we were previously locked (or just started), UNLOCK NOW
			if isSystemLocked {
				fmt.Println("\n>>> 🔓 AUTHENTICATED: TARGET RETURNED <<<")
				performUnlock()
			}
		} else {
			// --- PHONE IS SILENT ---
			missedPings++
			fmt.Printf("⚠️  Target Missing: %d/%d\n", missedPings, cfg.MaxMisses)
		}

		// 2. THE LOCK (Device Gone)
		if !isSystemLocked && missedPings >= cfg.MaxMisses {
			fmt.Println("\n>>> 🔒 THREAT DETECTED: LOCKING DOWN <<<")
			performLock()
		}

		// Wait before next ping
		time.Sleep(2 * time.Second)
	}
}

// GRACEFUL EXIT
func setupGracefulExit() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGTSTP)

	go func() {
		sig := <-sigChan
		fmt.Printf("\n\n🛑 SIGNAL RECEIVED (%s): INITIATING SECURE SHUTDOWN\n", sig)

		// 1. Force Unmount
		if isVaultMounted {
			fmt.Print("   [-] Cleaning up Ghost Drive... ")

			// Try standard unmount
			err := vault.Unmount(cfg.MountPoint)
			if err != nil {
				// If busy, force unmount
				fmt.Print("(Retrying) ")
				time.Sleep(500 * time.Millisecond)
				_ = vault.Unmount(cfg.MountPoint)
			}

			// 2. DELETE THE MOUNT POINT
			// Burning the bridge so no empty folder remains
			if err := os.Remove(cfg.MountPoint); err == nil {
				fmt.Println("DELETED (Trace Removed)")
			} else {
				fmt.Println("UNMOUNTED")
			}
		}

		// 3. Restore Internet
		if cfg.KillInternet && cfg.WifiInterface != "" {
			fmt.Print("   [+] Restoring Network... ")
			exec.Command("ip", "link", "set", cfg.WifiInterface, "up").Run()
			fmt.Println("DONE")
		}

		fmt.Println("Sentinel Exited.")
		os.Exit(0)
	}()
}

// --- ACTIONS ---

func performLock() {
	isSystemLocked = true // Set state immediately to prevent loops

	// Unmount the Vault
	if isVaultMounted {
		fmt.Print("   [-] Collapsing Ghost Drive... ")
		err := vault.Unmount(cfg.MountPoint)
		if err != nil {
			fmt.Printf("FAILED: %v\n", err)
		} else {
			// DELETE THE DIRECTORY
			os.Remove(cfg.MountPoint)
			fmt.Println("DONE (Trace Removed)")
			isVaultMounted = false
		}
	}

	// Kill Internet (Optional)
	if cfg.KillInternet && cfg.WifiInterface != "" {
		exec.Command("ip", "link", "set", cfg.WifiInterface, "down").Run()
		fmt.Println("   [-] Wi-Fi Killswitched")
	}
}

func performUnlock() {
	// Restore Internet
	if cfg.KillInternet && cfg.WifiInterface != "" {
		exec.Command("ip", "link", "set", cfg.WifiInterface, "up").Run()
		fmt.Println("   [+] Wi-Fi Restored")
	}

	//  Mount the Vault
	if !isVaultMounted {
		// Uses the Force Create method in your service.go
		err := vault.Mount(cfg.MountPoint, cfg.RealDir)
		if err != nil {
			fmt.Printf("❌ MOUNT FAILED: %v\n", err)
			// We do NOT set isSystemLocked=false here, so it tries again next loop
			return
		}

		fmt.Println("   [+] Ghost Drive Projecting...")
		fmt.Println("   [+] DONE (Files Decrypted)")
		isVaultMounted = true
	}

	isSystemLocked = false
}
