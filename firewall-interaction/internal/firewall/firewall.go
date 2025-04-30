package firewall

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"quidque.no/ow-firewall-sidecar/internal/config"
)

type Firewall struct {
	rulePrefix   string
	exePath      string
	exePathMutex sync.RWMutex
	pathFile     string
	configFile   string
}

func New() *Firewall {
	fw := &Firewall{
		rulePrefix: config.FirewallRulePrefix,
		exePath:    "",
		pathFile:   "overwatch_path.txt",
		configFile: "config.json",
	}

	// Try to load path from config first, then fallback to legacy path file
	if !fw.loadPathFromConfig() {
		fw.loadPathFromFile()
	}

	return fw
}

func (f *Firewall) loadPathFromConfig() bool {
	// Try to read from config.json
	data, err := os.ReadFile(f.configFile)
	if err != nil {
		return false
	}

	var cfg config.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false
	}

	if cfg.OverwatchPath != "" && fileExists(cfg.OverwatchPath) {
		f.exePathMutex.Lock()
		f.exePath = cfg.OverwatchPath
		f.exePathMutex.Unlock()
		fmt.Printf("Loaded Overwatch path from config: %s\n", cfg.OverwatchPath)
		return true
	}

	return false
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func (f *Firewall) loadPathFromFile() bool {
	data, err := os.ReadFile(f.pathFile)
	if err != nil {
		return false
	}

	path := strings.TrimSpace(string(data))
	if path != "" && fileExists(path) {
		f.exePathMutex.Lock()
		f.exePath = path
		f.exePathMutex.Unlock()
		fmt.Printf("Loaded Overwatch path from file: %s\n", path)

		// Update config file with the path
		f.updateConfigFile(path)
		return true
	}

	return false
}

func (f *Firewall) updateConfigFile(path string) {
	// Try to read existing config
	data, err := os.ReadFile(f.configFile)
	if err == nil {
		var cfg config.Config
		if err := json.Unmarshal(data, &cfg); err == nil {
			// Update path and save back
			cfg.OverwatchPath = path
			if newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
				os.WriteFile(f.configFile, newData, 0644)
			}
		}
	}
}

func (f *Firewall) SetOverwatchPath(path string) error {
	if path == "" {
		return fmt.Errorf("path cannot be empty")
	}

	if !fileExists(path) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	f.exePathMutex.Lock()
	defer f.exePathMutex.Unlock()

	fmt.Printf("Setting Overwatch path to: %s\n", path)
	f.exePath = path

	// Save to both legacy path file and config file
	_ = os.WriteFile(f.pathFile, []byte(path), 0644)
	f.updateConfigFile(path)

	return nil
}

func (f *Firewall) GetOverwatchPath() string {
	f.exePathMutex.RLock()
	defer f.exePathMutex.RUnlock()
	return f.exePath
}

func (f *Firewall) HasOverwatchPath() bool {
	f.exePathMutex.RLock()
	defer f.exePathMutex.RUnlock()
	return f.exePath != "" && fileExists(f.exePath)
}

func (f *Firewall) BlockIPs(region string, ipListDir string) error {
	// Verify we have a valid Overwatch path
	if !f.HasOverwatchPath() {
		return fmt.Errorf("Overwatch path not configured")
	}

	// Get the path to the region's IP list file
	filePath := filepath.Join(ipListDir, fmt.Sprintf("%s.txt", region))

	// Check if the file exists and has content
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("IP list file not found: %s", filePath)
	}
	if fileInfo.Size() == 0 {
		return fmt.Errorf("IP list file is empty: %s", filePath)
	}

	// Read IPs from the file
	ips, err := readIPsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read IP list: %w", err)
	}

	// Verify we have IPs to block
	if len(ips) == 0 {
		return fmt.Errorf("no IPs found in file: %s", filePath)
	}

	fmt.Printf("Found %d IPs to block for region %s\n", len(ips), region)

	// Get thread-safe copy of exePath
	f.exePathMutex.RLock()
	exePath := f.exePath
	f.exePathMutex.RUnlock()

	// Verify the executable still exists
	if !fileExists(exePath) {
		return fmt.Errorf("Overwatch executable no longer exists: %s", exePath)
	}

	// First, remove any existing rules for this region to avoid duplicates
	if err := f.removeRules(region); err != nil {
		fmt.Printf("Warning: Failed to clean up existing rules: %v\n", err)
		// Continue anyway - this isn't fatal
	}

	// Process IPs in batches to avoid command line length limitations
	batchSize := 50
	totalBatches := (len(ips) + batchSize - 1) / batchSize

	fmt.Printf("Processing %d IPs in %d batches\n", len(ips), totalBatches)

	rulesCreated := 0
	for i := 0; i < len(ips); i += batchSize {
		end := i + batchSize
		if end > len(ips) {
			end = len(ips)
		}

		batch := ips[i:end]
		batchNum := i/batchSize + 1

		// Create rule name with region and batch number
		ruleName := fmt.Sprintf("%s%s-Batch%d", f.rulePrefix, region, batchNum)
		ipList := strings.Join(batch, ",")

		// Create outbound rule
		output, err := f.executeFirewallCmd("add", "rule",
			"name="+ruleName,
			"dir=out",
			"action=block",
			"program="+exePath,
			"remoteip="+ipList)

		if err != nil {
			return fmt.Errorf("failed to create outbound rule (batch %d): %v\n%s", batchNum, err, output)
		}
		rulesCreated++

		// Create inbound rule
		output, err = f.executeFirewallCmd("add", "rule",
			"name="+ruleName+"-In",
			"dir=in",
			"action=block",
			"program="+exePath,
			"remoteip="+ipList)

		if err != nil {
			return fmt.Errorf("failed to create inbound rule (batch %d): %v\n%s", batchNum, err, output)
		}
		rulesCreated++

		fmt.Printf("Created rules for batch %d of %d (%d IPs)\n", batchNum, totalBatches, len(batch))
	}

	// Verify that rules were created
	rules, err := f.listRules()
	if err != nil {
		return fmt.Errorf("failed to verify firewall rules: %w", err)
	}

	rulePattern := f.rulePrefix + region
	verifiedRules := 0
	for _, rule := range rules {
		if strings.Contains(rule, rulePattern) {
			verifiedRules++
		}
	}

	if verifiedRules == 0 {
		return fmt.Errorf("no firewall rules were created (verification failed)")
	}

	fmt.Printf("Successfully blocked %d IPs for region %s (%d rules verified)\n", len(ips), region, verifiedRules)
	return nil
}

func (f *Firewall) UnblockIPs(region string) error {
	fmt.Printf("Unblocking region: %s\n", region)
	return f.removeRules(region)
}

func (f *Firewall) UnblockAll() error {
	fmt.Println("Unblocking all regions...")

	// List all rules matching our prefix
	rules, err := f.listRules()
	if err != nil {
		return fmt.Errorf("failed to list firewall rules: %w", err)
	}

	if len(rules) == 0 {
		fmt.Println("No firewall rules found to remove")
		return nil
	}

	// Delete each matching rule
	removed := 0
	for _, rule := range rules {
		if strings.HasPrefix(rule, f.rulePrefix) {
			output, err := f.executeFirewallCmd("delete", "rule", "name="+rule)
			if err != nil {
				fmt.Printf("Warning: Failed to delete rule %s: %v\nOutput: %s\n", rule, err, output)
				continue
			}
			removed++
		}
	}

	// Report results
	if removed > 0 {
		fmt.Printf("Successfully removed %d firewall rules\n", removed)
	} else {
		fmt.Println("No firewall rules were removed")
	}

	// Verify all rules are gone
	remainingRules, _ := f.listRules()
	remaining := 0
	for _, rule := range remainingRules {
		if strings.HasPrefix(rule, f.rulePrefix) {
			remaining++
		}
	}

	if remaining > 0 {
		return fmt.Errorf("%d rules still remain after cleanup", remaining)
	}

	return nil
}

func readIPsFromFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var ips []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			ips = append(ips, line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return ips, nil
}

func (f *Firewall) removeRules(region string) error {
	prefix := f.rulePrefix
	if region != "" {
		prefix = prefix + region
	}

	// List all existing rules
	rules, err := f.listRules()
	if err != nil {
		return fmt.Errorf("failed to list firewall rules: %w", err)
	}

	// Count how many rules match our prefix
	matchingRules := 0
	for _, rule := range rules {
		if strings.HasPrefix(rule, prefix) {
			matchingRules++
		}
	}

	if matchingRules == 0 {
		fmt.Printf("No rules found matching prefix: %s\n", prefix)
		return nil
	}

	fmt.Printf("Found %d rules to remove matching prefix: %s\n", matchingRules, prefix)

	// Delete each matching rule
	removed := 0
	for _, rule := range rules {
		if strings.HasPrefix(rule, prefix) {
			output, err := f.executeFirewallCmd("delete", "rule", "name="+rule)
			if err != nil {
				fmt.Printf("Warning: Failed to delete rule %s: %v\nOutput: %s\n", rule, err, output)
				continue
			}
			removed++
		}
	}

	// Report results
	if removed == matchingRules {
		fmt.Printf("Successfully removed all %d rules\n", removed)
	} else {
		return fmt.Errorf("removed %d out of %d rules", removed, matchingRules)
	}

	return nil
}

func (f *Firewall) listRules() ([]string, error) {
	output, err := f.executeFirewallCmd("show", "rule", "name=all")
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall rules: %w", err)
	}

	var rules []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		if strings.Contains(line, "Rule Name:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				ruleName := strings.TrimSpace(parts[1])
				if strings.HasPrefix(ruleName, f.rulePrefix) {
					rules = append(rules, ruleName)
				}
			}
		}
	}

	return rules, nil
}

func (f *Firewall) executeFirewallCmd(args ...string) (string, error) {
	cmdArgs := append([]string{"advfirewall", "firewall"}, args...)
	cmd := exec.Command("netsh", cmdArgs...)

	// Hide console window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	// Capture both stdout and stderr
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}

	return string(output), nil
}

func IsAdminPrivilegesAvailable() bool {
	cmd := exec.Command("net", "session")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd.Run() == nil
}
