#!/bin/bash

# Stop on critical errors
set -e

BINARY="./sentinel"
SOURCE_FILE="cmd/sentinel/main.go"  

# Check if the binary exists
if [ ! -f "$BINARY" ]; then
    
    # Check if source code exists 
    if [ -f "$SOURCE_FILE" ]; then
        
        # Check if Go is installed
        if command -v go &> /dev/null; then
             echo "⚠️  Binary not found, but source code detected."
             
             # --- NEW: Download Dependencies ---
             echo "📦 Resolving dependencies (go mod tidy)..."
             go mod tidy
             # ----------------------------------

             echo "🔨 Compiling Sentinel from source..."
             
             # Compile from the subfolder
             go build -o sentinel "$SOURCE_FILE"
             
             echo "✅ Compilation successful."
        else
             echo "❌ Error: Binary not found and Go is not installed."
             echo "   OPTION 1: Install Go (sudo apt install golang)"
             echo "   OPTION 2: Download the 'sentinel' binary from GitHub Releases."
             exit 1
        fi
        
    else
        # User Environment (Script only)
        echo "❌ Error: 'sentinel' binary not found!"
        echo "   Please download the compiled 'sentinel' file from GitHub Releases"
        echo "   and place it in this folder."
        exit 1
    fi
fi

# 4. Ensure executable permissions
chmod +x "$BINARY"

# 5. Handle Config Flag
if [[ "$1" == "--config" ]]; then
    echo "🔧 Opening Configuration..."
    sudo "$BINARY" -config
    exit 0
fi

# 6. Normal Run
echo "🛡️  Starting Sentinel..."
sudo "$BINARY"