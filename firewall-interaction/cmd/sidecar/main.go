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
	// Check for admin privileges
	if !firewall.IsAdminPrivilegesAvailable() {
		fmt.Println("ERROR: This application requires administrator privileges.")
		fmt.Println("Please right-click and select 'Run as administrator'.")
		os.Exit(config.ExitErrorAdminRights)
	}

	// Parse command line flags
	action := flag.String("action", "", "Action to perform: block, unblock, unblock-all, status")
	region := flag.String("region", "", "Region to block/unblock (EU, NA, AS, etc.)")
	ipDir := flag.String("ip-dir", config.DefaultIPListDir, "Directory containing IP list files")
	waitTimeout := flag.Int("wait-timeout", 0, "Timeout in seconds to wait for Overwatch to close (0 = no timeout)")
	flag.Parse()

	// Initialize firewall
	fw := firewall.New()

	// Check if we're running in daemon mode
	if flag.Arg(0) == "daemon" {
		runDaemonMode(fw, *ipDir, *waitTimeout)
		return
	}

	// Validate arguments for one-shot mode
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

	// Execute the requested action
	executeAction(fw, *action, *region, *ipDir, *waitTimeout)
}

// Run in daemon mode, listening for commands
func runDaemonMode(fw *firewall.Firewall, ipDir string, waitTimeout int) {
	fmt.Println("Starting firewall sidecar in daemon mode")
	fmt.Println("Elevated privileges obtained")

	// Make IP directory path absolute if it's relative
	absIPDir, err := filepath.Abs(ipDir)
	if err != nil {
		fmt.Printf("ERROR: Failed to resolve IP directory path: %v\n", err)
		os.Exit(config.ExitErrorIPListRead)
	}

	// Set up channel to receive signals
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	// Start a goroutine to handle signals
	go func() {
		// Block until signal is received
		<-c
		fmt.Println("Shutting down, cleaning up firewall rules...")
		fw.UnblockAll()
		os.Exit(config.ExitSuccess)
	}()

	// Process standard input for commands
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := scanner.Text()
		parts := strings.Split(command, "|")
		
		action := parts[0]
		var region string
		if len(parts) > 1 {
			region = parts[1]
		}
		
		result := executeActionWithResult(fw, action, region, absIPDir, waitTimeout)
		fmt.Println(result) // Send result back to the parent process
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("ERROR: Error reading input: %v\n", err)
		os.Exit(config.ExitErrorInvalidArgs)
	}
}

// Execute an action and return the result
func executeActionWithResult(fw *firewall.Firewall, action, region, ipDir string, waitTimeout int) string {
	// Make IP directory path absolute if it's relative
	absIPDir, err := filepath.Abs(ipDir)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to resolve IP directory path: %v", err)
	}

	// Handle different actions
	switch action {
	case config.ActionBlock:
		// Check if Overwatch is running
		isRunning, err := process.IsOverwatchRunning()
		if err != nil {
			return fmt.Sprintf("ERROR: Failed to check if Overwatch is running: %v", err)
		}

		if isRunning {
			result := "Overwatch is currently running.\n"
			result += "Waiting for Overwatch to close before applying firewall rules..."
			
			// Wait for Overwatch to close
			err = process.WaitForOverwatchToClose(waitTimeout)
			if err != nil {
				return fmt.Sprintf("%s\nERROR: %v", result, err)
			}
		}

		// Apply block
		result := fmt.Sprintf("Blocking IPs for region %s...\n", region)
		if err := fw.BlockIPs(region, absIPDir); err != nil {
			return fmt.Sprintf("%sERROR: Failed to block IPs: %v", result, err)
		}
		return result + "Successfully blocked IPs."

	case config.ActionUnblock:
		result := fmt.Sprintf("Unblocking IPs for region %s...\n", region)
		if err := fw.UnblockIPs(region); err != nil {
			return fmt.Sprintf("%sERROR: Failed to unblock IPs: %v", result, err)
		}
		return result + "Successfully unblocked IPs."

	case config.ActionUnblockAll:
		result := "Unblocking all IPs...\n"
		if err := fw.UnblockAll(); err != nil {
			return fmt.Sprintf("%sERROR: Failed to unblock all IPs: %v", result, err)
		}
		return result + "Successfully unblocked all IPs."

	case config.ActionStatus:
		isRunning, err := process.IsOverwatchRunning()
		if err != nil {
			return fmt.Sprintf("ERROR: Failed to check if Overwatch is running: %v", err)
		}

		if isRunning {
			return "Status: Overwatch is currently running"
		} else {
			return "Status: Overwatch is not running"
		}

	default:
		return fmt.Sprintf("ERROR: Unknown action '%s'", action)
	}
}

// Execute an action (one-shot mode)
func executeAction(fw *firewall.Firewall, action, region, ipDir string, waitTimeout int) {
	result := executeActionWithResult(fw, action, region, ipDir, waitTimeout)
	fmt.Println(result)
	
	// Exit with appropriate code
	if strings.Contains(result, "ERROR:") {
		os.Exit(config.ExitErrorFirewall)
	}
	os.Exit(config.ExitSuccess)
}