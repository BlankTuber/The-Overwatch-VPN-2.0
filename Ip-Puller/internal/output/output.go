package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quidque.no/ow2-ip-puller/internal/regions"
)

// Output directory
const OutputDir = "ips"

// CreateOutputDirectory creates the output directory
func CreateOutputDirectory() error {
	return os.MkdirAll(OutputDir, 0755)
}

// WriteIPsToFiles writes IPs to files by region
func WriteIPsToFiles(ipsByRegion map[regions.Region][]string) {
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
		writeRegionFile(region, content)
	}
}

// createFileContent creates file content from IP list
func createFileContent(ips []string) string {
	content := strings.Join(ips, "\n")
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

// writeRegionFile writes the region file
func writeRegionFile(region regions.Region, content string) {
	filename := filepath.Join(OutputDir, fmt.Sprintf("%s.txt", region))
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		fmt.Printf("Error writing to file %s: %v\n", filename, err)
	} else {
		fmt.Printf("Successfully wrote %d IPs to %s\n", strings.Count(content, "\n"), filename)
	}
}