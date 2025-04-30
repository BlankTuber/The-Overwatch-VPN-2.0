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
	"time"

	"quidque.no/ow-firewall-sidecar/internal/config"
	"quidque.no/ow-firewall-sidecar/internal/firewall"
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
	flag.Parse()

	fw := firewall.New()

	setupCleanupHandler(fw)

	if flag.Arg(0) == "daemon" {
		runDaemonMode(fw, *ipDir)
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

	executeAction(fw, *action, *region, *ipDir)
}

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

func runDaemonMode(fw *firewall.Firewall, ipDir string) {
	fmt.Println("Starting firewall sidecar daemon")

	absIPDir, err := filepath.Abs(ipDir)
	if err != nil {
		fmt.Printf("ERROR: Failed to resolve IP directory path: %v\n", err)
		os.Exit(config.ExitErrorIPListRead)
	}

	// Setup a heartbeat to detect if parent process died
	go func() {
		for {
			// If stdin is closed, parent process has terminated
			if _, err := os.Stdin.Stat(); err != nil {
				fmt.Println("Parent process closed connection, cleaning up...")
				fw.UnblockAll()
				fmt.Println("Cleanup completed, exiting...")
				os.Exit(config.ExitSuccess)
			}
			// Sleep to avoid excessive CPU usage - Windows compatible
			time.Sleep(5 * time.Second)
		}
	}()

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		command := strings.TrimSpace(scanner.Text())

		if command == "" {
			continue
		}

		parts := strings.Split(command, "|")

		action := parts[0]
		var region string
		var customIPDir string

		if len(parts) > 1 {
			region = parts[1]
		}
		if len(parts) > 2 {
			customIPDir = parts[2]
		}

		// Handle exit command
		if action == "exit" {
			fmt.Println("Received exit command, cleaning up...")
			fw.UnblockAll()
			fmt.Println("Cleanup completed, exiting...")
			os.Exit(config.ExitSuccess)
		}

		if customIPDir != "" {
			result := executeActionWithResult(fw, action, region, customIPDir)
			fmt.Println(result)
		} else {
			result := executeActionWithResult(fw, action, region, absIPDir)
			fmt.Println(result)
		}
	}

	// If we get here, stdin was closed
	fmt.Println("Parent process closed connection, cleaning up...")
	fw.UnblockAll()
	fmt.Println("Cleanup completed, exiting...")

	if err := scanner.Err(); err != nil {
		fmt.Printf("ERROR: Error reading input: %v\n", err)
		os.Exit(config.ExitErrorInvalidArgs)
	}

	os.Exit(config.ExitSuccess)
}

func executeActionWithResult(fw *firewall.Firewall, action, region, ipDir string) string {
	absIPDir, err := filepath.Abs(ipDir)
	if err != nil {
		return fmt.Sprintf("ERROR: Failed to resolve IP directory path: %v", err)
	}

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
		result := fmt.Sprintf("Blocking IPs for region %s from directory %s...\n", region, absIPDir)
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

	case config.ActionSetPath:
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
		pathStatus := ""
		if !fw.HasOverwatchPath() {
			pathStatus = " - Overwatch path not configured"
		}
		return "Status: Ready" + pathStatus

	default:
		return fmt.Sprintf("ERROR: Unknown action '%s'", action)
	}
}

func executeAction(fw *firewall.Firewall, action, region, ipDir string) {
	result := executeActionWithResult(fw, action, region, ipDir)
	fmt.Println(result)

	if strings.Contains(result, "ERROR:") {
		os.Exit(config.ExitErrorFirewall)
	}
	os.Exit(config.ExitSuccess)
}
