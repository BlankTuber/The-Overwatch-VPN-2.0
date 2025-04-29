# The Overwatch VPN 2.0

A Tauri-based application that helps Overwatch players control which region servers they connect to by selectively blocking IP addresses.

## Components

This project consists of three main components:

1. **IP Puller** (Go) - Fetches and categorizes Overwatch server IPs by region
2. **Firewall Sidecar** (Go) - Manages Windows Firewall rules for Overwatch
3. **Tauri Application** (Rust + JavaScript) - Provides a user-friendly interface

## How It Works

### Startup Sequence

1. When the application starts, it executes the IP Puller to obtain the latest Overwatch server IPs
2. The IP Puller categorizes IPs by region and saves them to text files
3. The Firewall Sidecar is started (triggering a UAC prompt for admin privileges)
4. The main Tauri interface loads, allowing the user to block/unblock regions

### Runtime Behavior

-   Blocking a region adds Windows Firewall rules specific to the Overwatch executable
-   If Overwatch is running when attempting to block, the app will wait until Overwatch closes
-   Unblocking works immediately whether Overwatch is running or not
-   The app monitors the status and updates the UI accordingly

### Shutdown Sequence

-   When the application is closed, it automatically:
    1. Unblocks all previously blocked IPs
    2. Cleans up all created firewall rules
    3. Gracefully shuts down all components

## Project Structure

```
The-OW-VPN-2.0/
├── Ip-Puller/                    # IP Puller component
│   ├── cmd/puller/main.go        # Entry point
│   ├── internal/api/api.go       # API client for fetching IPs
│   ├── internal/regions/regions.go # Region categorization
│   └── internal/output/output.go # File output handling
│
├── ow-firewall-sidecar/          # Firewall Sidecar component
│   ├── cmd/sidecar/main.go       # Entry point
│   ├── internal/firewall/firewall.go # Windows firewall management
│   ├── internal/process/process.go # Process monitoring
│   └── internal/config/config.go   # Configuration
│
├── src-tauri/                    # Tauri application
│   ├── src/lib.rs                # Application logic
│   ├── src/main.rs               # Entry point
│   ├── Cargo.toml                # Rust dependencies
│   └── tauri.conf.json           # Tauri configuration
│
├── src/                          # Frontend
│   ├── index.html                # UI structure
│   ├── main.js                   # UI logic
│   └── styles.css                # UI styling
│
└── build.bat                     # Build script
```

## Requirements

-   Windows 10 or later
-   Administrator privileges (for firewall management)
-   Overwatch installed

## Building

To build the complete application:

```bash
# Run the build script
build.bat
```

This will:

1. Build the IP Puller
2. Build the Firewall Sidecar with admin manifest
3. Generate initial IP lists
4. Build the Tauri application
5. Package everything into an installer

## Usage

1. Launch the application (requires admin privileges for firewall access)
2. Select a region you want to avoid playing in
3. Click "Block Region"
4. Launch Overwatch and play with better ping!

## Important Notes

-   Blocking won't work if Overwatch is currently running
-   You may need to restart Overwatch after changing settings
-   All blocks will be automatically removed when you close the app

## Technical Details

### IP Puller

-   Uses the BGP View API to fetch Blizzard's IP ranges
-   Categorizes IPs by country code and maps to regions
-   Outputs one text file per region

### Firewall Sidecar

-   Creates inbound and outbound block rules specific to Overwatch.exe
-   Uses program-specific Windows Firewall rules
-   Handles the case where Overwatch is already running
-   Requires administrator privileges via manifest

### Tauri Application

-   Built with Tauri 2.0
-   Uses external binaries feature to include the Go components
-   Manages the lifecycle of both the IP Puller and Firewall Sidecar

## License

GNU General Public License v3.0

## Acknowledgements

-   [Tauri](https://tauri.app/) - For the application framework
-   [BGP View API](https://bgpview.io/) - For IP data
