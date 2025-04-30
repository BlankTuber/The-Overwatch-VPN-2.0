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
	configFile   string
}

func New() *Firewall {
	fw := &Firewall{
		rulePrefix: config.FirewallRulePrefix,
		exePath:    "",
		configFile: "config.json",
	}

	fw.loadPathFromConfig()

	return fw
}

func (f *Firewall) loadPathFromConfig() bool {
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

func (f *Firewall) updateConfigFile(path string) {
	data, err := os.ReadFile(f.configFile)
	if err == nil {
		var cfg config.Config
		if err := json.Unmarshal(data, &cfg); err == nil {
			cfg.OverwatchPath = path
			if newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
				os.WriteFile(f.configFile, newData, 0644)
			}
		}
	} else {
		cfg := config.Config{
			OverwatchPath:    path,
			UseGithubSource:  false,
			InitialSetupDone: true,
		}
		if newData, err := json.MarshalIndent(cfg, "", "  "); err == nil {
			os.WriteFile(f.configFile, newData, 0644)
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
	if !f.HasOverwatchPath() {
		return fmt.Errorf("overwatch path not configured")
	}

	filePath := filepath.Join(ipListDir, fmt.Sprintf("%s.txt", region))

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("ip list file not found: %s", filePath)
	}
	if fileInfo.Size() == 0 {
		return fmt.Errorf("ip list file is empty: %s", filePath)
	}

	ips, err := readIPsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read ip list: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no ips found in file: %s", filePath)
	}

	fmt.Printf("Found %d IPs to block for region %s\n", len(ips), region)

	f.exePathMutex.RLock()
	exePath := f.exePath
	f.exePathMutex.RUnlock()

	if !fileExists(exePath) {
		return fmt.Errorf("overwatch executable no longer exists: %s", exePath)
	}

	if err := f.removeRules(region); err != nil {
		fmt.Printf("Warning: Failed to clean up existing rules: %v\n", err)
	}

	batchSize := 25
	totalBatches := (len(ips) + batchSize - 1) / batchSize

	fmt.Printf("Processing %d IPs in %d batches\n", len(ips), totalBatches)

	var wg sync.WaitGroup
	errChan := make(chan error, totalBatches*2)
	successCount := make(chan int, totalBatches*2)

	maxConcurrent := 10
	sem := make(chan struct{}, maxConcurrent)

	for i := 0; i < len(ips); i += batchSize {
		end := i + batchSize
		if end > len(ips) {
			end = len(ips)
		}

		batch := ips[i:end]
		batchNum := i/batchSize + 1

		wg.Add(1)
		go func(batch []string, batchNum int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ruleName := fmt.Sprintf("%s%s-Batch%d", f.rulePrefix, region, batchNum)
			ipList := strings.Join(batch, ",")

			output, err := f.executeFirewallCmd("add", "rule",
				"name="+ruleName,
				"dir=out",
				"action=block",
				"program="+exePath,
				"remoteip="+ipList)

			if err != nil {
				errChan <- fmt.Errorf("failed to create outbound rule (batch %d): %v\n%s", batchNum, err, output)
				return
			}
			successCount <- 1

			output, err = f.executeFirewallCmd("add", "rule",
				"name="+ruleName+"-In",
				"dir=in",
				"action=block",
				"program="+exePath,
				"remoteip="+ipList)

			if err != nil {
				errChan <- fmt.Errorf("failed to create inbound rule (batch %d): %v\n%s", batchNum, err, output)
				return
			}
			successCount <- 1

		}(batch, batchNum)
	}

	wg.Wait()
	close(errChan)
	close(successCount)

	errors := make([]error, 0, cap(errChan))
	for err := range errChan {
		errors = append(errors, err)
	}

	totalSuccessRules := 0
	for count := range successCount {
		totalSuccessRules += count
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to create %d rules: %v", len(errors), errors[0])
	}

	// Removed the verification step here

	fmt.Printf("Attempted to block %d IPs for region %s (%d rules attempted)\n", len(ips), region, totalSuccessRules)
	return nil
}

func (f *Firewall) UnblockIPs(region string) error {
	fmt.Printf("Unblocking region: %s\n", region)
	return f.removeRules(region)
}

func (f *Firewall) UnblockAll() error {
	fmt.Println("Unblocking all regions...")

	rules, err := f.listRules()
	if err != nil {
		return fmt.Errorf("failed to list firewall rules: %w", err)
	}

	if len(rules) == 0 {
		fmt.Println("No firewall rules found to remove")
		return nil
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(rules))
	successCount := make(chan int, len(rules))

	maxConcurrent := 10
	sem := make(chan struct{}, maxConcurrent)

	for _, rule := range rules {
		if strings.HasPrefix(rule, f.rulePrefix) {
			wg.Add(1)
			go func(rule string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				output, err := f.executeFirewallCmd("delete", "rule", "name="+rule)
				if err != nil {
					errChan <- fmt.Errorf("failed to delete rule %s: %v\nOutput: %s", rule, err, output)
					return
				}
				successCount <- 1
			}(rule)
		}
	}

	wg.Wait()
	close(errChan)
	close(successCount)

	errors := make([]error, 0, cap(errChan))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove %d rules: %v", len(errors), errors[0])
	}

	removed := 0
	for count := range successCount {
		removed += count
	}

	if removed > 0 {
		fmt.Printf("Successfully removed %d firewall rules\n", removed)
	} else {
		fmt.Println("No firewall rules were removed")
	}

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

	rules, err := f.listRules()
	if err != nil {
		return fmt.Errorf("failed to list firewall rules: %w", err)
	}

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

	var wg sync.WaitGroup
	errChan := make(chan error, matchingRules)
	successCount := make(chan int, matchingRules)

	maxConcurrent := 10
	sem := make(chan struct{}, maxConcurrent)

	for _, rule := range rules {
		if strings.HasPrefix(rule, prefix) {
			wg.Add(1)
			go func(rule string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				output, err := f.executeFirewallCmd("delete", "rule", "name="+rule)
				if err != nil {
					errChan <- fmt.Errorf("failed to delete rule %s: %v\nOutput: %s", rule, err, output)
					return
				}
				successCount <- 1
			}(rule)
		}
	}

	wg.Wait()
	close(errChan)
	close(successCount)

	errors := make([]error, 0, cap(errChan))
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("failed to remove %d rules: %v", len(errors), errors[0])
	}

	removed := 0
	for count := range successCount {
		removed += count
	}

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

	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

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
