package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"quidque.no/ow-firewall-sidecar/internal/config"
	"quidque.no/ow-firewall-sidecar/internal/firewall"
	"quidque.no/ow-firewall-sidecar/internal/process"
)

func main() {
	if !firewall.IsAdminPrivilegesAvailable() {
		fmt.Println("ERROR: This application requires administrator privileges.")
		fmt.Println("Please right-click and select 'Run as administrator'.")
		os.Exit(config.ExitErrorAdminRights)
	}

	action := flag.String("action", "", "Action to perform: block, unblock, unblock-all, status, set-path, get-path")
	region := flag.String("region", "", "Region to block/unblock (EU, NA, AS, etc.)")
	ipDir := flag.String("ip-dir", config.DefaultIPListDir, "Directory containing IP list files")
	waitTimeout := flag.Int("wait-timeout", 0, "Timeout in seconds to wait for Overwatch to close (0 = no timeout)")
	flag.Parse()

	fw := firewall.New()

	// Set up cleanup on exit for any mode
	setupCleanupHandler(fw)

	if flag.Arg(0) == "daemon" {
		runDaemonMode(fw, *ipDir, *waitTimeout)
		return
	}

	if *action == "" {
		fmt.Println("ERROR: Missing required action flag")
		flag.Usage()
		os.Exit(config.ExitErrorInvalidArgs)
	}

	if (*action == config.ActionBlock || *action == config.ActionUnblock) && *region == "" {
		fmt.Println("ERROR: Region is required for block/unblock actions")
		flag.Usage()
		os.Exit(config.ExitErrorInvalidArgs)
	}

	executeAction(fw, *action, *region, *ipDir, *waitTimeout)
}

// setupCleanupHandler ensures firewall rules are cleaned up on program exit
func setupCleanupHandler(fw *firewall.Firewall) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	go func() {
		<-c
		fmt.Println("Shutting down, cleaning up firewall rules...")
		fw.UnblockAll()
		os.Exit(config.ExitSuccess)
	}()
}

func runDaemonMode(fw *firewall.Firewall, ipDir string, waitTimeout int) {
	fmt.Println("Starting firewall sidecar")

	absIPDir, err := filepath.Abs(ipDir)
	if err != nil {
		fmt.Printf("ERROR: Failed to resolve IP directory path: %v\n", err)
		os.Exit(config.ExitErrorIPListRead)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()

		// Check for EOF or empty input
		if command == "" {
			continue
		}

		parts := strings.Split(command, "|")

		action := parts[0]
		var region string
		if len(parts) > 1 {
			region = parts[1]
		}

		result := executeActionWithResult(fw, action, region, absIPDir, waitTimeout)
		fmt.Println(result)
	}

	// If we get here, the parent has closed the pipe or we've reached EOF
	// Clean up before exiting
	fmt.Println("Parent process closed connection, cleaning up...")
	fw.UnblockAll()

	if err := scanner.Err(); err != nil {
		fmt.Printf("ERROR: Error reading input: %v\n", err)
		os.Exit(config.ExitErrorInvalidArgs)
	}

	os.Exit(config.ExitSuccess)
}

func executeActionWithResult(fw *firewall.Firewall, action, region, ipDir string, waitTimeout int) string {
	absIPDir, err := filepath.Abs(ipDir)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to resolve IP directory path: %v", err)
	}

	// For most actions, validate that Overwatch path is configured first
	if action != config.ActionSetPath &&
		action != config.ActionGetPath &&
		action != config.ActionUnblockAll &&
		action != config.ActionStatus {
		if !fw.HasOverwatchPath() {
			return "ERROR: Overwatch path not configured. Please detect Overwatch path first."
		}
	}

	switch action {
	case config.ActionBlock:
		isRunning, err := process.IsOverwatchRunning()
		if err != nil {
			return fmt.Sprintf("ERROR: Failed to check if Overwatch is running: %v", err)
		}

		if isRunning {
			result := "Overwatch is currently running. Waiting for Overwatch to close before applying firewall rules..."

			err = process.WaitForOverwatchToClose(waitTimeout)
			if err != nil {
				return fmt.Sprintf("%s\nERROR: %v", result, err)
			}

			// When Overwatch finally closes
			fmt.Println("Overwatch has closed, proceeding with IP blocking...")
		}

		result := fmt.Sprintf("Blocking IPs for region %s...\n", region)
		if err := fw.BlockIPs(region, absIPDir); err != nil {
			return fmt.Sprintf("%sERROR: Failed to block IPs: %v", result, err)
		}
		return result + "Successfully blocked IPs."

	case config.ActionUnblock:
		// We allow unblocking even if Overwatch is running
		result := fmt.Sprintf("Unblocking IPs for region %s...\n", region)
		if err := fw.UnblockIPs(region); err != nil {
			return fmt.Sprintf("%sERROR: Failed to unblock IPs: %v", result, err)
		}
		return result + "Successfully unblocked IPs."

	case config.ActionUnblockAll:
		// We allow unblocking all even if Overwatch is running or path not configured
		result := "Unblocking all IPs...\n"
		if err := fw.UnblockAll(); err != nil {
			return fmt.Sprintf("%sERROR: Failed to unblock all IPs: %v", result, err)
		}
		return result + "Successfully unblocked all IPs."

	case config.ActionSetPath:
		// Set custom Overwatch path
		if region == "" {
			return "ERROR: Path parameter is required for set-path action"
		}

		if err := fw.SetOverwatchPath(region); err != nil {
			return fmt.Sprintf("ERROR: Failed to set Overwatch path: %v", err)
		}
		return fmt.Sprintf("Overwatch path set to: %s", region)

	case config.ActionGetPath:
		path := fw.GetOverwatchPath()
		if path == "" {
			return "Overwatch path not configured"
		}
		return fmt.Sprintf("Current Overwatch path: %s", path)

	case config.ActionStatus:
		isRunning, err := process.IsOverwatchRunning()
		if err != nil {
			return fmt.Sprintf("ERROR: Failed to check if Overwatch is running: %v", err)
		}

		pathStatus := ""
		if !fw.HasOverwatchPath() {
			pathStatus = " - Overwatch path not configured"
		}

		if isRunning {
			return "Status: Overwatch is currently running" + pathStatus
		} else {
			return "Status: Overwatch is not running" + pathStatus
		}

	default:
		return fmt.Sprintf("ERROR: Unknown action '%s'", action)
	}
}

func executeAction(fw *firewall.Firewall, action, region, ipDir string, waitTimeout int) {
	result := executeActionWithResult(fw, action, region, ipDir, waitTimeout)
	fmt.Println(result)

	if strings.Contains(result, "ERROR:") {
		os.Exit(config.ExitErrorFirewall)
	}
	os.Exit(config.ExitSuccess)
}
