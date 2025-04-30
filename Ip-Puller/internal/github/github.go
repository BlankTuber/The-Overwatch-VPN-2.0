package github

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"quidque.no/ow2-ip-puller/internal/regions"
)

const (
	TimeoutSeconds = 30

	// URL for the GitHub source
	UrlsContainerURL = "https://raw.githubusercontent.com/foryVERX/Overwatch-Server-Selector/main/ip_lists/urlsContainer.txt"
)

// FetchAndCategorizeIPs fetches IPs from GitHub and categorizes them by region
func FetchAndCategorizeIPs() (map[regions.Region][]string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(TimeoutSeconds) * time.Second,
	}

	// Fetch URLs container
	urlsContent, err := fetchContent(client, UrlsContainerURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching URLs container: %w", err)
	}

	// Parse URLs
	urls := parseURLs(urlsContent)

	// Initialize map for IPs by region
	ipsByRegion := make(map[regions.Region][]string)

	// Process each URL and categorize IPs
	for _, url := range urls {
		// Skip special files
		if !isIPFilesURL(url) {
			continue
		}

		// Fetch content
		content, err := fetchContent(client, url)
		if err != nil {
			fmt.Printf("Warning: Error fetching content from %s: %v\n", url, err)
			continue
		}

		// Determine region
		region := getRegionFromURL(url)

		// Skip unknown regions
		if region == regions.UNK {
			continue
		}

		// Parse IPs
		ips := parseIPs(content)

		// Add IPs to the region
		if len(ips) > 0 {
			fmt.Printf("Found %d IPs for region %s from %s\n", len(ips), region, getFilenameFromURL(url))
			ipsByRegion[region] = append(ipsByRegion[region], ips...)
		}
	}

	return ipsByRegion, nil
}

// fetchContent fetches content from a URL
func fetchContent(client *http.Client, url string) (string, error) {
	// Make HTTP request
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("error making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	return string(body), nil
}

// parseURLs parses URLs from the container content
func parseURLs(content string) []string {
	var urls []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			urls = append(urls, line)
		}
	}

	return urls
}

// getFilenameFromURL extracts the filename from a URL
func getFilenameFromURL(url string) string {
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]
	return strings.ReplaceAll(filename, "%20", " ") // Replace URL encoding
}

// isIPFilesURL checks if a URL is for an IP-related file
func isIPFilesURL(url string) bool {
	filename := getFilenameFromURL(url)

	return strings.HasSuffix(filename, ".txt") &&
		!strings.Contains(filename, "BlockingConfig") &&
		!strings.Contains(filename, "IP_version") &&
		!strings.Contains(filename, "pinglist") &&
		!strings.Contains(filename, "urlsContainer")
}

// getRegionFromURL extracts region from URL
func getRegionFromURL(url string) regions.Region {
	filename := getFilenameFromURL(url)

	// Check for Ip_ranges_* files
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

	// Check for cfg - * files
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

// parseIPs parses IPs from the content
func parseIPs(content string) []string {
	var ips []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip lines that don't look like IP ranges
		if !looksLikeIPRange(line) {
			continue
		}

		ips = append(ips, line)
	}

	return ips
}

// looksLikeIPRange checks if a line looks like an IP range
func looksLikeIPRange(line string) bool {
	// Check for IP range patterns
	return strings.Contains(line, ".") &&
		(strings.Contains(line, "/") || strings.Contains(line, "-") ||
			!strings.Contains(line, " ")) &&
		!strings.Contains(line, "ipRangeName") // Skip configuration headers
}
