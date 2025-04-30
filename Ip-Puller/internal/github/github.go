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
	TimeoutSeconds   = 30
	UrlsContainerURL = "https://raw.githubusercontent.com/foryVERX/Overwatch-Server-Selector/main/ip_lists/urlsContainer.txt"
)

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

	urlChan := make(chan string)
	resultChan := make(chan struct {
		region regions.Region
		ips    []string
		err    error
	})

	workerCount := 5
	for i := 0; i < workerCount; i++ {
		go func() {
			for url := range urlChan {
				result := struct {
					region regions.Region
					ips    []string
					err    error
				}{regions.UNK, nil, nil}

				if !isIPFilesURL(url) {
					resultChan <- result
					continue
				}

				content, err := fetchContent(client, url)
				if err != nil {
					result.err = fmt.Errorf("error fetching content from %s: %v", url, err)
					resultChan <- result
					continue
				}

				region := getRegionFromURL(url)
				if region == regions.UNK {
					resultChan <- result
					continue
				}

				ips := parseIPs(content)
				result.region = region
				result.ips = ips

				resultChan <- result
			}
		}()
	}

	go func() {
		for _, url := range urls {
			urlChan <- url
		}
		close(urlChan)
	}()

	for i := 0; i < len(urls); i++ {
		result := <-resultChan
		if result.err != nil {
			fmt.Printf("Warning: %v\n", result.err)
			continue
		}
		if len(result.ips) > 0 {
			ipsByRegion[result.region] = append(ipsByRegion[result.region], result.ips...)
			fmt.Printf("Found %d IPs for region %s\n", len(result.ips), result.region)
		}
	}

	return ipsByRegion, nil
}

func fetchContent(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("error making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP request failed with status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("error reading response body: %w", err)
	}

	return string(body), nil
}

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
	var ips []string
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !looksLikeIPRange(line) {
			continue
		}

		ips = append(ips, normalizeIPRange(line))
	}

	return ips
}

func looksLikeIPRange(line string) bool {
	return strings.Contains(line, ".") &&
		(strings.Contains(line, "/") ||
			strings.Contains(line, "-") ||
			!strings.Contains(line, " ")) &&
		!strings.Contains(line, "ipRangeName")
}

func normalizeIPRange(ipRange string) string {
	if strings.Contains(ipRange, "/") {
		return ipRange
	}

	if strings.Contains(ipRange, "-") {
		return ipRange
	}

	if !strings.Contains(ipRange, "/") && strings.Count(ipRange, ".") == 3 {
		return ipRange + "/32"
	}

	return ipRange
}
