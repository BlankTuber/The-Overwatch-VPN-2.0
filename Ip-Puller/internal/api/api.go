package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// API URL and timeout
const (
	ApiURL         = "https://api.bgpview.io/asn/57976/prefixes"
	TimeoutSeconds = 30
)

// Response represents the API response structure
type Response struct {
	Status        string `json:"status"`
	StatusMessage string `json:"status_message"`
	Data          Data   `json:"data"`
}

// Data represents the data field in the API response
type Data struct {
	IPv4Prefixes []Prefix `json:"ipv4_prefixes"`
	IPv6Prefixes []Prefix `json:"ipv6_prefixes"`
}

// Prefix represents an IP prefix in the API response
type Prefix struct {
	Prefix      string `json:"prefix"`
	CountryCode string `json:"country_code"`
}

// FetchIPPrefixes fetches IP prefixes from the API
func FetchIPPrefixes() ([]byte, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: time.Duration(TimeoutSeconds) * time.Second,
	}

	// Make HTTP request
	resp, err := client.Get(ApiURL)
	if err != nil {
		return nil, fmt.Errorf("error making HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP request failed with status code: %d", resp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response body: %w", err)
	}

	return body, nil
}

// ParseIPPrefixes parses JSON response
func ParseIPPrefixes(data []byte) (*Response, error) {
	var response Response
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("error parsing JSON: %w", err)
	}

	// Check if the API returned a successful response
	if response.Status != "ok" {
		return nil, fmt.Errorf("API returned error: %s", response.StatusMessage)
	}

	return &response, nil
}
