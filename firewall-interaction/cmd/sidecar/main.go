package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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

	// Validate arguments
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

	// Initialize firewall
	fw := firewall.New()

	// Handle different actions
	switch *action {
	case config.ActionBlock:
		// Make IP directory path absolute if it's relative
		absIPDir, err := filepath.Abs(*ipDir)
		if err != nil {
			fmt.Printf("ERROR: Failed to resolve IP directory path: %v\n", err)
			os.Exit(config.ExitErrorIPListRead)
		}

		// Check if Overwatch is running
		isRunning, err := process.IsOverwatchRunning()
		if err != nil {
			fmt.Printf("ERROR: Failed to check if Overwatch is running: %v\n", err)
			os.Exit(config.ExitErrorProcessCheck)
		}

		if isRunning {
			fmt.Println("Overwatch is currently running.")
			fmt.Println("Waiting for Overwatch to close before applying firewall rules...")
			
			// Wait for Overwatch to close
			err = process.WaitForOverwatchToClose(*waitTimeout)
			if err != nil {
				fmt.Printf("ERROR: %v\n", err)
				os.Exit(config.ExitErrorProcessCheck)
			}
		}

		// Apply block
		fmt.Printf("Blocking IPs for region %s...\n", *region)
		if err := fw.BlockIPs(*region, absIPDir); err != nil {
			fmt.Printf("ERROR: Failed to block IPs: %v\n", err)
			os.Exit(config.ExitErrorFirewall)
		}
		fmt.Println("Successfully blocked IPs.")

	case config.ActionUnblock:
		fmt.Printf("Unblocking IPs for region %s...\n", *region)
		if err := fw.UnblockIPs(*region); err != nil {
			fmt.Printf("ERROR: Failed to unblock IPs: %v\n", err)
			os.Exit(config.ExitErrorFirewall)
		}
		fmt.Println("Successfully unblocked IPs.")

	case config.ActionUnblockAll:
		fmt.Println("Unblocking all IPs...")
		if err := fw.UnblockAll(); err != nil {
			fmt.Printf("ERROR: Failed to unblock all IPs: %v\n", err)
			os.Exit(config.ExitErrorFirewall)
		}
		fmt.Println("Successfully unblocked all IPs.")

	case config.ActionStatus:
		isRunning, err := process.IsOverwatchRunning()
		if err != nil {
			fmt.Printf("ERROR: Failed to check if Overwatch is running: %v\n", err)
			os.Exit(config.ExitErrorProcessCheck)
		}

		if isRunning {
			fmt.Println("Status: Overwatch is currently running")
		} else {
			fmt.Println("Status: Overwatch is not running")
		}

	default:
		fmt.Printf("ERROR: Unknown action '%s'\n", *action)
		flag.Usage()
		os.Exit(config.ExitErrorInvalidArgs)
	}

	// Setup cleanup on application shutdown if we're running as a daemon
	if flag.Arg(0) == "daemon" {
		fmt.Println("Running in daemon mode. Press Ctrl+C to exit.")
		
		// Set up channel to receive signals
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		
		// Block until signal is received
		<-c
		
		fmt.Println("Shutting down, cleaning up firewall rules...")
		fw.UnblockAll()
	}

	os.Exit(config.ExitSuccess)
}