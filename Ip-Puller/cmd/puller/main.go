package main

import (
	"flag"
	"fmt"
	"os"
	"sync"

	"quidque.no/ow2-ip-puller/internal/api"
	"quidque.no/ow2-ip-puller/internal/github"
	"quidque.no/ow2-ip-puller/internal/output"
	"quidque.no/ow2-ip-puller/internal/regions"
)

func main() {
	useGithub := flag.Bool("github", false, "Use alternative GitHub source (Overwatch-Server-Selector)")
	useBoth := flag.Bool("both", false, "Use both sources (BGPView and GitHub)")
	flag.Parse()

	regions.InitRegionMap()

	var wg sync.WaitGroup
	var apiIPsByRegion, githubIPsByRegion map[regions.Region][]string
	var apiErr, githubErr error

	if !*useGithub || *useBoth {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Println("Using default API source (BGPView)...")
			apiIPsByRegion, apiErr = fetchFromAPI()
			if apiErr != nil {
				fmt.Printf("Error fetching from API: %v\n", apiErr)
			} else {
				outputDir := "ips"
				if err := output.CreateOutputDirectory(outputDir); err != nil {
					apiErr = fmt.Errorf("error creating output directory: %w", err)
					return
				}
				output.WriteIPsToFilesWithDir(apiIPsByRegion, outputDir)
				fmt.Printf("Successfully processed IP ranges and saved to %s/ directory\n", outputDir)
			}
		}()
	}

	if *useGithub || *useBoth {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Println("Using GitHub source from Overwatch-Server-Selector...")
			githubIPsByRegion, githubErr = fetchFromGitHub()
			if githubErr != nil {
				fmt.Printf("Error fetching from GitHub: %v\n", githubErr)
			} else {
				outputDir := "ips_mina"
				if err := output.CreateOutputDirectory(outputDir); err != nil {
					githubErr = fmt.Errorf("error creating output directory: %w", err)
					return
				}
				output.WriteIPsToFilesWithDir(githubIPsByRegion, outputDir)
				fmt.Printf("Successfully processed IP ranges and saved to %s/ directory\n", outputDir)
			}
		}()
	}

	wg.Wait()

	if (*useGithub && githubErr != nil) || (!*useGithub && apiErr != nil) ||
		(*useBoth && apiErr != nil && githubErr != nil) {
		exitWithError(fmt.Errorf("failed to fetch IP data from requested sources"))
	}
}

func fetchFromAPI() (map[regions.Region][]string, error) {
	data, err := api.FetchIPPrefixes()
	if err != nil {
		return nil, err
	}

	response, err := api.ParseIPPrefixes(data)
	if err != nil {
		return nil, err
	}

	return regions.CategorizeIPsByRegion(response), nil
}

func fetchFromGitHub() (map[regions.Region][]string, error) {
	return github.FetchAndCategorizeIPs()
}

func exitWithError(err error) {
	fmt.Println(err)
	os.Exit(1)
}
