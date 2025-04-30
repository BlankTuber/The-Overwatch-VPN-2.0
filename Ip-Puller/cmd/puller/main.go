package main

import (
	"flag"
	"fmt"
	"os"

	"quidque.no/ow2-ip-puller/internal/api"
	"quidque.no/ow2-ip-puller/internal/github"
	"quidque.no/ow2-ip-puller/internal/output"
	"quidque.no/ow2-ip-puller/internal/regions"
)

func main() {
	// Define command-line flags
	useGithub := flag.Bool("github", false, "Use alternative GitHub source (Overwatch-Server-Selector)")
	flag.Parse()

	// Initialize region map
	regions.InitRegionMap()

	var ipsByRegion map[regions.Region][]string
	var outputDir string

	if *useGithub {
		// Use GitHub source
		fmt.Println("Using GitHub source from Overwatch-Server-Selector...")
		outputDir = "ips_mina"

		// Fetch and categorize IPs from GitHub
		var err error
		ipsByRegion, err = github.FetchAndCategorizeIPs()
		if err != nil {
			exitWithError(err)
		}
	} else {
		// Use original API source
		fmt.Println("Using default API source (BGPView)...")
		outputDir = "ips"

		// Fetch IP prefixes
		data, err := api.FetchIPPrefixes()
		if err != nil {
			exitWithError(err)
		}

		// Parse IP prefixes
		response, err := api.ParseIPPrefixes(data)
		if err != nil {
			exitWithError(err)
		}

		// Categorize IPs by region
		ipsByRegion = regions.CategorizeIPsByRegion(response)
	}

	// Create output directory
	if err := output.CreateOutputDirectory(outputDir); err != nil {
		exitWithError(fmt.Errorf("error creating output directory: %w", err))
	}

	// Write IPs to files
	output.WriteIPsToFilesWithDir(ipsByRegion, outputDir)

	fmt.Printf("Successfully processed IP ranges and saved to %s/ directory\n", outputDir)
}

// Print error and exit
func exitWithError(err error) {
	fmt.Println(err)
	os.Exit(1)
}
