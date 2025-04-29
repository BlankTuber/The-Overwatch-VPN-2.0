# Overwatch Firewall Sidecar

A standalone Windows application that manages firewall rules for Overwatch. This sidecar works in conjunction with the IP puller to block or unblock specific IPs or IP ranges for the Overwatch game.

## Features

-   Block/unblock IPs from specific regions for Overwatch only
-   Automatically waits if Overwatch is running when trying to block IPs
-   Unblocks IPs instantly whether Overwatch is running or not
-   Requires administrator privileges (automatically requests elevation)
-   Cleans up firewall rules on shutdown

## Building

To build the application:

```bash
# Build with Go
go build -o build/ow-firewall-sidecar.exe cmd/sidecar/main.go

# Or use the build script that adds the administrator manifest
build.bat
```

## Usage

The sidecar is designed to be called from the Tauri application, but it can also be used directly from the command line.

```
ow-firewall-sidecar.exe -action <action> -region <region> [options]
```

### Options

-   `-action`: Required. Action to perform: `block`, `unblock`, `unblock-all`, `status`
-   `-region`: Required for `block` and `unblock` actions. Region code (EU, NA, etc.)
-   `-ip-dir`: Optional. Directory containing IP list files. Default: `ips/`
-   `-wait-timeout`: Optional. Timeout in seconds to wait for Overwatch to close (0 = no timeout). Default: 0

### Examples

Block IPs for EU region:

```
ow-firewall-sidecar.exe -action block -region EU
```

Unblock IPs for NA region:

```
ow-firewall-sidecar.exe -action unblock -region NA
```

Unblock all previously blocked IPs:

```
ow-firewall-sidecar.exe -action unblock-all
```

Check if Overwatch is running:

```
ow-firewall-sidecar.exe -action status
```

## Integration with Tauri

To call the sidecar from your Tauri application, you can use the `Command` module:

```javascript
import { Command } from "@tauri-apps/api/shell";

// Example: Block EU region
async function blockEURegion() {
    const command = Command.sidecar("ow-firewall-sidecar", [
        "-action",
        "block",
        "-region",
        "EU",
    ]);
    const output = await command.execute();
    console.log(output);
}

// Example: Unblock all regions
async function unblockAll() {
    const command = Command.sidecar("ow-firewall-sidecar", [
        "-action",
        "unblock-all",
    ]);
    const output = await command.execute();
    console.log(output);
}
```

## Requirements

-   Windows 10 or later
-   Administrator privileges
