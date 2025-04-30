package firewall

import (
	"bufio"
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
}

func New() *Firewall {
	fw := &Firewall{
		rulePrefix: config.FirewallRulePrefix,
		exePath:    "",
		pathFile:   "overwatch_path.txt",
	}

	// Try to load the path from file
	fw.loadPathFromFile()
	return fw
}

// SavePathToFile saves the Overwatch path to a file
func (f *Firewall) SavePathToFile() error {
	f.exePathMutex.RLock()
	path := f.exePath
	f.exePathMutex.RUnlock()

	if path == "" {
		return fmt.Errorf("no Overwatch path configured")
	}

	return os.WriteFile(f.pathFile, []byte(path), 0644)
}

// LoadPathFromFile loads the Overwatch path from a file
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

// SetOverwatchPath sets the Overwatch executable path
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

	// Save path to file
	if err := f.SavePathToFile(); err != nil {
		fmt.Printf("Warning: Failed to save path to file: %v\n", err)
	}

	return nil
}

// GetOverwatchPath returns the current Overwatch path
func (f *Firewall) GetOverwatchPath() string {
	f.exePathMutex.RLock()
	defer f.exePathMutex.RUnlock()
	return f.exePath
}

// HasOverwatchPath returns true if Overwatch path is configured
func (f *Firewall) HasOverwatchPath() bool {
	f.exePathMutex.RLock()
	defer f.exePathMutex.RUnlock()
	return f.exePath != ""
}

func (f *Firewall) BlockIPs(region string, ipListDir string) error {
	// Check if Overwatch path is configured
	if !f.HasOverwatchPath() {
		return fmt.Errorf("Overwatch path not configured")
	}

	filePath := filepath.Join(ipListDir, fmt.Sprintf("%s.txt", region))

	ips, err := readIPsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read IP list: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IPs found for region %s", region)
	}

	// Get thread-safe copy of exePath
	f.exePathMutex.RLock()
	exePath := f.exePath
	f.exePathMutex.RUnlock()

	// Bulk block IPs in batches
	batchSize := 50
	totalBatches := (len(ips) + batchSize - 1) / batchSize

	fmt.Printf("Starting to block %d IPs for region %s in %d batches\n", len(ips), region, totalBatches)
	fmt.Printf("Using Overwatch executable path: %s\n", exePath)

	for i := 0; i < len(ips); i += batchSize {
		end := i + batchSize
		if end > len(ips) {
			end = len(ips)
		}

		batchNum := i/batchSize + 1
		fmt.Printf("Processing batch %d of %d for region %s...\n", batchNum, totalBatches, region)

		batch := ips[i:end]

		// Create outbound rule for batch
		ruleName := fmt.Sprintf("%s%s-Batch%d", f.rulePrefix, region, batchNum)
		ipList := strings.Join(batch, ",")

		err := f.execFirewallCmd("add", "rule",
			"name="+ruleName,
			"dir=out",
			"action=block",
			"program="+exePath,
			"remoteip="+ipList)

		if err != nil {
			return err
		}

		// Create inbound rule for batch
		err = f.execFirewallCmd("add", "rule",
			"name="+ruleName+"-In",
			"dir=in",
			"action=block",
			"program="+exePath,
			"remoteip="+ipList)

		if err != nil {
			return err
		}
	}

	fmt.Printf("Successfully blocked %d IPs for region %s in %d batches\n", len(ips), region, totalBatches)
	return nil
}

func (f *Firewall) UnblockIPs(region string) error {
	return f.removeRules(region)
}

func (f *Firewall) UnblockAll() error {
	fmt.Println("Cleaning up all firewall rules...")
	rules, err := f.listRules()
	if err != nil {
		return fmt.Errorf("failed to list firewall rules: %w", err)
	}

	count := 0
	for _, rule := range rules {
		if strings.HasPrefix(rule, f.rulePrefix) {
			err := f.execFirewallCmd("delete", "rule", "name="+rule)
			if err != nil {
				return fmt.Errorf("failed to delete rule %s: %w", rule, err)
			}
			count++
		}
	}

	if count > 0 {
		fmt.Printf("Successfully removed %d firewall rules\n", count)
	} else {
		fmt.Println("No firewall rules needed to be removed")
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
		ip := strings.TrimSpace(scanner.Text())
		if ip != "" {
			ips = append(ips, ip)
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
		return err
	}

	for _, rule := range rules {
		if strings.HasPrefix(rule, prefix) {
			err := f.execFirewallCmd("delete", "rule", "name="+rule)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (f *Firewall) listRules() ([]string, error) {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name=all")
	// Hide the console window
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list firewall rules: %w", err)
	}

	var rules []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := scanner.Text()
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

func (f *Firewall) execFirewallCmd(args ...string) error {
	allArgs := append([]string{"advfirewall", "firewall"}, args...)
	cmd := exec.Command("netsh", allArgs...)

	// Hide the console window
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("firewall command failed: %s, error: %w", string(output), err)
	}
	return nil
}

func IsAdminPrivilegesAvailable() bool {
	cmd := exec.Command("net", "session")
	// Hide the console window
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	err := cmd.Run()
	return err == nil
}
