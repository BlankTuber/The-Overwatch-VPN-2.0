package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quidque.no/ow2-ip-puller/internal/regions"
)

// Default output directory
const DefaultOutputDir = "ips"

// CreateOutputDirectory creates the output directory
func CreateOutputDirectory(dirName string) error {
	if dirName == "" {
		dirName = DefaultOutputDir
	}
	return os.MkdirAll(dirName, 0755)
}

// WriteIPsToFiles writes IPs to files by region using the default output directory
func WriteIPsToFiles(ipsByRegion map[regions.Region][]string) {
	WriteIPsToFilesWithDir(ipsByRegion, DefaultOutputDir)
}

// WriteIPsToFilesWithDir writes IPs to files by region using the specified output directory
func WriteIPsToFilesWithDir(ipsByRegion map[regions.Region][]string, dirName string) {
	if dirName == "" {
		dirName = DefaultOutputDir
	}

	for region, ips := range ipsByRegion {
		// Skip unknown region
		if region == regions.UNK {
			continue
		}

		// Skip if no IPs in this region
		if len(ips) == 0 {
			continue
		}

		// Create the file content
		content := createFileContent(ips)

		// Write to file
		writeRegionFile(region, content, dirName)
	}
}

// createFileContent creates file content from IP list
func createFileContent(ips []string) string {
	// Remove duplicates
	uniqueIPs := make(map[string]bool)
	for _, ip := range ips {
		uniqueIPs[ip] = true
	}

	// Convert back to slice
	var uniqueIPsList []string
	for ip := range uniqueIPs {
		uniqueIPsList = append(uniqueIPsList, ip)
	}

	// Create content
	content := strings.Join(uniqueIPsList, "\n")
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

// writeRegionFile writes the region file
func writeRegionFile(region regions.Region, content string, dirName string) {
	filename := filepath.Join(dirName, fmt.Sprintf("%s.txt", region))
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Printf("Error writing to file %s: %v\n", filename, err)
	} else {
		fmt.Printf("Successfully wrote %d IPs to %s\n", strings.Count(content, "\n"), filename)
	}
}
