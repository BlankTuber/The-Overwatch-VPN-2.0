package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"quidque.no/ow2-ip-puller/internal/github"
	"quidque.no/ow2-ip-puller/internal/output"
	"quidque.no/ow2-ip-puller/internal/regions"
)

const (
	localVersionFile  = "ips_mina/IP_version.txt"
	outputDir         = "ips_mina"
	versionCheckOnly  = "check"
	versionForceFetch = "force"
)

func main() {
	versionAction := flag.String("version", "", "Version action: 'check' to only check for updates, 'force' to force update")
	flag.Parse()

	regions.InitRegionMap()

	needUpdate, remoteVersion, err := checkForUpdates()
	if err != nil {
		fmt.Printf("Warning: Could not check for IP list updates: %v\n", err)
	}

	if *versionAction == versionCheckOnly {
		if needUpdate {
			fmt.Printf("Update available: Version %s\n", remoteVersion)
			os.Exit(0)
		} else {
			fmt.Printf("No updates available. Current version: %s\n", remoteVersion)
			os.Exit(0)
		}
	}

	if !needUpdate && *versionAction != versionForceFetch {
		fmt.Printf("IP lists are up to date (version %s). Use -version=force to force update.\n", remoteVersion)
		os.Exit(0)
	}

	fmt.Println("Fetching IP addresses from GitHub source...")
	ipsByRegion, err := fetchFromGitHub()
	if err != nil {
		exitWithError(fmt.Errorf("failed to fetch IP data from GitHub: %w", err))
	}

	// Validate IPs before writing
	ipsByRegion = validateIPs(ipsByRegion)

	if err := output.CreateOutputDirectory(outputDir); err != nil {
		exitWithError(fmt.Errorf("error creating output directory: %w", err))
	}

	output.WriteIPsToFilesWithDir(ipsByRegion, outputDir)
	fmt.Printf("Successfully processed IP ranges and saved to %s/ directory\n", outputDir)
}

func checkForUpdates() (bool, string, error) {
	// Check if we have a local version file
	localVersion := ""
	localData, err := os.ReadFile(localVersionFile)
	if err == nil {
		localVersion = strings.TrimSpace(string(localData))
	}

	// Get remote version
	remoteVersion, err := github.FetchVersionNumber()
	if err != nil {
		return true, "", err
	}

	// If we don't have a local version, we need an update
	if localVersion == "" {
		return true, remoteVersion, nil
	}

	// Compare versions - assuming semantic versioning format (x.y.z)
	needsUpdate, err := isNewerVersion(remoteVersion, localVersion)
	if err != nil {
		return true, remoteVersion, err
	}

	return needsUpdate, remoteVersion, nil
}

func isNewerVersion(remote, local string) (bool, error) {
	// Simple version comparison - assumes format x.y.z
	// Convert to numbers and compare
	remoteNums := strings.Split(remote, ".")
	localNums := strings.Split(local, ".")

	// Handle different format lengths
	maxLen := len(remoteNums)
	if len(localNums) > maxLen {
		maxLen = len(localNums)
	}

	for i := 0; i < maxLen; i++ {
		var remoteNum, localNum int
		var err error

		if i < len(remoteNums) {
			remoteNum, err = strconv.Atoi(remoteNums[i])
			if err != nil {
				return true, fmt.Errorf("invalid remote version format: %s", remote)
			}
		}

		if i < len(localNums) {
			localNum, err = strconv.Atoi(localNums[i])
			if err != nil {
				return true, fmt.Errorf("invalid local version format: %s", local)
			}
		}

		if remoteNum > localNum {
			return true, nil
		} else if localNum > remoteNum {
			return false, nil
		}
	}

	// Versions are identical
	return false, nil
}

func fetchFromGitHub() (map[regions.Region][]string, error) {
	return github.FetchAndCategorizeIPs()
}

func validateIPs(ipsByRegion map[regions.Region][]string) map[regions.Region][]string {
	validatedIPs := make(map[regions.Region][]string)

	for region, ips := range ipsByRegion {
		validIPs := make([]string, 0, len(ips))
		for _, ip := range ips {
			if isValidIPRange(ip) {
				validIPs = append(validIPs, ip)
			}
		}
		validatedIPs[region] = validIPs
	}

	return validatedIPs
}

func isValidIPRange(ipRange string) bool {
	// Check CIDR notation (e.g., 192.168.1.0/24)
	if strings.Contains(ipRange, "/") {
		parts := strings.Split(ipRange, "/")
		if len(parts) != 2 {
			return false
		}

		// Validate IP part
		if !isValidIPv4(parts[0]) {
			return false
		}

		// Validate subnet part
		subnet, err := strconv.Atoi(parts[1])
		if err != nil || subnet < 0 || subnet > 32 {
			return false
		}

		return true
	}

	// Check range notation (e.g., 192.168.1.1-192.168.1.10)
	if strings.Contains(ipRange, "-") {
		parts := strings.Split(ipRange, "-")
		if len(parts) != 2 {
			return false
		}

		// Validate start and end IPs
		return isValidIPv4(parts[0]) && isValidIPv4(parts[1])
	}

	// Single IP address
	return isValidIPv4(ipRange)
}

func isValidIPv4(ip string) bool {
	ip = strings.TrimSpace(ip)
	parts := strings.Split(ip, ".")

	if len(parts) != 4 {
		return false
	}

	for _, part := range parts {
		num, err := strconv.Atoi(part)
		if err != nil || num < 0 || num > 255 {
			return false
		}
	}

	return true
}

func exitWithError(err error) {
	fmt.Println(err)
	os.Exit(1)
}
