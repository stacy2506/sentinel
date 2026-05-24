# 🛡️ Sentinel

Sentinel is a Linux security tool that uses Bluetooth proximity to automatically mount and unmount a secure, encrypted FUSE filesystem

When the target bluetooth signal is out of range/deactivated , your **Vault is purged**. When the signal is in range/active , it automatically remounts the vault and decrypts your files, ready for work

## ✨ Features

* **👻 Ghost Drive:** The mount point (`~/Vault`) is strictly purged when the Bluetooth signal is not active/ out of range

* **🔒 AES Encryption:** Files are stored encrypted in a hidden system database and decrypted on-the-fly only when mounted 

* **⚡ Zero-Touch Auth:** No passwords to type. Your physical presence (Bluetooth MAC) is the key

* **🛠️ IDE Ready:** Custom FUSE implementation supports atomic saves, making it fully compatible with **VS Code**, JetBrains, and other editors without hanging

* **📶 Killswitch (Optional):** Can automatically cut internet access when the device is locked

## 📦 Installation

 Build from Source
*Best for developers or if you want to compile it yourself*

Prerequisites: `Go (Golang) 1.20+`

```bash
# 1. Clone the repository
git clone [https://github.com/stacy2506/sentinel.git](https://github.com/stacy2506/sentinel)
cd sentinel

# 2. Make the script executable
chmod +x sentinel.sh

# 3. Run the script
# The script automatically detects the source code and compiles the binary for you
./sentinel.sh

⚠️ Disclaimer
Use at your own risk. This is a security tool that interacts with your filesystem. While it uses standard AES encryption, always keep backups of critical data
