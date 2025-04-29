package main

import (
	"fmt"
	"os"

	"quidque.no/ow2-ip-puller/internal/api"
	"quidque.no/ow2-ip-puller/internal/output"
	"quidque.no/ow2-ip-puller/internal/regions"
)

func main() {
	// Initialize region map
	regions.InitRegionMap()
	
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
	ipsByRegion := regions.CategorizeIPsByRegion(response)
	
	// Create output directory
	if err := output.CreateOutputDirectory(); err != nil {
		exitWithError(fmt.Errorf("error creating output directory: %w", err))
	}
	
	// Write IPs to files
	output.WriteIPsToFiles(ipsByRegion)
}

// Print error and exit
func exitWithError(err error) {
	fmt.Println(err)
	os.Exit(1)
}