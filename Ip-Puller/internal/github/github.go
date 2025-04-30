package github

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"quidque.no/ow2-ip-puller/internal/regions"
)

const (
	TimeoutSeconds     = 30
	UrlsContainerURL   = "https://raw.githubusercontent.com/foryVERX/Overwatch-Server-Selector/main/ip_lists/urlsContainer.txt"
	VersionFileURL     = "https://raw.githubusercontent.com/foryVERX/Overwatch-Server-Selector/main/ip_lists/IP_version.txt"
	MaxConcurrentFetch = 10
)

func FetchVersionNumber() (string, error) {
	client := &http.Client{
		Timeout: time.Duration(TimeoutSeconds) * time.Second,
	}

	content, err := fetchContent(client, VersionFileURL)
	if err != nil {
		return "", fmt.Errorf("error fetching version file: %w", err)
	}

	version := strings.TrimSpace(content)
	if version == "" {
		return "", fmt.Errorf("empty version number received")
	}

	return version, nil
}

func FetchAndCategorizeIPs() (map[regions.Region][]string, error) {
	client := &http.Client{
		Timeout: time.Duration(TimeoutSeconds) * time.Second,
	}

	urlsContent, err := fetchContent(client, UrlsContainerURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching URLs container: %w", err)
	}

	urls := parseURLs(urlsContent)
	ipsByRegion := make(map[regions.Region][]string)

	type fetchResult struct {
		region regions.Region
		ips    []string
		err    error
	}

	urls = filterIPFilesURLs(urls)

	resultChan := make(chan fetchResult, len(urls))
	semaphore := make(chan struct{}, MaxConcurrentFetch)
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			region := getRegionFromURL(url)
			if region == regions.UNK {
				resultChan <- fetchResult{regions.UNK, nil, nil}
				return
			}

			content, err := fetchContent(client, url)
			if err != nil {
				resultChan <- fetchResult{region, nil, fmt.Errorf("error fetching from %s: %v", url, err)}
				return
			}

			ips := parseIPs(content)
			resultChan <- fetchResult{region, ips, nil}
		}(url)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for result := range resultChan {
		if result.err != nil {
			fmt.Printf("Warning: %v\n", result.err)
			continue
		}

		if len(result.ips) > 0 {
			ipsByRegion[result.region] = append(ipsByRegion[result.region], result.ips...)
		}
	}

	// Also save the version file
	versionContent, err := fetchContent(client, VersionFileURL)
	if err != nil {
		fmt.Printf("Warning: Could not fetch version file: %v\n", err)
	} else {
		// Write to output directory - this will be done by the main function
		if err := os.WriteFile("ips_mina/IP_version.txt", []byte(versionContent), 0644); err != nil {
			fmt.Printf("Warning: Could not save version file: %v\n", err)
		}
	}

	return ipsByRegion, nil
}

func filterIPFilesURLs(urls []string) []string {
	filteredURLs := make([]string, 0, len(urls))
	for _, url := range urls {
		if isIPFilesURL(url) {
			filteredURLs = append(filteredURLs, url)
		}
	}
	return filteredURLs
}

func fetchContent(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response body: %w", err)
	}

	return string(body), nil
}

func parseURLs(content string) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			result = append(result, line)
		}
	}

	return result
}

func getFilenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]
	return strings.ReplaceAll(filename, "%20", " ")
}

func isIPFilesURL(url string) bool {
	filename := getFilenameFromURL(url)

	return strings.HasSuffix(filename, ".txt") &&
		!strings.Contains(filename, "BlockingConfig") &&
		!strings.Contains(filename, "IP_version") &&
		!strings.Contains(filename, "pinglist") &&
		!strings.Contains(filename, "urlsContainer")
}

func getRegionFromURL(url string) regions.Region {
	filename := getFilenameFromURL(url)

	if strings.Contains(filename, "Ip_ranges_EU") {
		return regions.EU
	} else if strings.Contains(filename, "Ip_ranges_NA_") {
		return regions.NA
	} else if strings.Contains(filename, "Ip_ranges_Brazil") ||
		strings.Contains(filename, "Ip_ranges_SA") {
		return regions.SA
	} else if strings.Contains(filename, "Ip_ranges_AS_") ||
		strings.Contains(filename, "Ip_ranges_AS") {
		return regions.AS
	} else if strings.Contains(filename, "Ip_ranges_ME") {
		return regions.ME
	} else if strings.Contains(filename, "Ip_ranges_Australia") ||
		strings.Contains(filename, "Ip_ranges_OCE") ||
		strings.Contains(filename, "Ip_ranges_Oce") {
		return regions.OCE
	} else if strings.Contains(filename, "Ip_ranges_AFR") ||
		strings.Contains(filename, "Ip_ranges_Afr") {
		return regions.AFR
	}

	if strings.Contains(filename, "cfg - EU") {
		return regions.EU
	} else if strings.Contains(filename, "cfg - NA") {
		return regions.NA
	} else if strings.Contains(filename, "cfg - Other - Brazil") {
		return regions.SA
	} else if strings.Contains(filename, "cfg - Asia") {
		return regions.AS
	} else if strings.Contains(filename, "cfg - Other - Bahrain") ||
		strings.Contains(filename, "cfg - Other - KSA") ||
		strings.Contains(filename, "cfg - Other - Qatar") {
		return regions.ME
	} else if strings.Contains(filename, "cfg - Other - Australia") {
		return regions.OCE
	}

	return regions.UNK
}

func parseIPs(content string) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if looksLikeIPRange(line) {
			result = append(result, normalizeIPRange(line))
		}
	}

	return result
}

func looksLikeIPRange(line string) bool {
	return strings.Contains(line, ".") &&
		(strings.Contains(line, "/") ||
			strings.Contains(line, "-") ||
			!strings.Contains(line, " ")) &&
		!strings.Contains(line, "ipRangeName")
}

func normalizeIPRange(ipRange string) string {
	// Already has subnet mask
	if strings.Contains(ipRange, "/") {
		return ipRange
	}

	// IP range with dash
	if strings.Contains(ipRange, "-") {
		return ipRange
	}

	// Single IP - add /32 subnet mask
	if !strings.Contains(ipRange, "/") && strings.Count(ipRange, ".") == 3 {
		return ipRange + "/32"
	}

	return ipRange
}
