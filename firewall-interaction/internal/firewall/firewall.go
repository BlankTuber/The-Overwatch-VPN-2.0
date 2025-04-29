package firewall

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"quidque.no/ow-firewall-sidecar/internal/config"
)

// Firewall represents a Windows firewall manager
type Firewall struct {
	rulePrefix string
	exePath    string
}

// New creates a new Firewall instance
func New() *Firewall {
	return &Firewall{
		rulePrefix: config.FirewallRulePrefix,
		exePath:    getOverwatchExePath(),
	}
}

func getOverwatchExePath() string {
	// Check common installation paths
	commonPaths := []string{
		"C:\\Program Files\\Overwatch\\" + config.OverwatchProcessName,
		"C:\\Program Files (x86)\\Overwatch\\" + config.OverwatchProcessName,
		"C:\\Program Files\\Battle.net\\Games\\Overwatch\\" + config.OverwatchProcessName,
		"C:\\Program Files (x86)\\Battle.net\\Games\\Overwatch\\" + config.OverwatchProcessName,
	}
	
	// Try to find from Battle.net registry entries
	battleNetPaths := getBattleNetGamePaths()
	if len(battleNetPaths) > 0 {
		commonPaths = append(commonPaths, battleNetPaths...)
	}
	
	// Check if any of these paths exist
	for _, path := range commonPaths {
		if fileExists(path) {
			return path
		}
	}
	
	// Fall back to the default path if nothing else is found
	return "C:\\Program Files (x86)\\Overwatch\\" + config.OverwatchProcessName
}

func getBattleNetGamePaths() []string {
	// This would normally use the golang.org/x/sys/windows/registry package
	// to look for Battle.net installations in the Windows registry
	// For example locations like:
	// HKEY_LOCAL_MACHINE\SOFTWARE\WOW6432Node\Blizzard Entertainment\Battle.net\InstallPath
	// and then look for Overwatch in product-specific subkeys
	
	// For simplicity in this example, we're just checking some additional common paths
	return []string{
		"D:\\Games\\Overwatch\\" + config.OverwatchProcessName,
		"D:\\Blizzard\\Overwatch\\" + config.OverwatchProcessName,
		"D:\\Battle.net\\Games\\Overwatch\\" + config.OverwatchProcessName,
		"E:\\Games\\Overwatch\\" + config.OverwatchProcessName,
	}
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// BlockIPs blocks the IPs in the specified region for Overwatch
func (f *Firewall) BlockIPs(region string, ipListDir string) error {
	// Construct the file path for the region's IP list
	filePath := filepath.Join(ipListDir, fmt.Sprintf("%s.txt", region))
	
	// Read IPs from file
	ips, err := readIPsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read IP list: %w", err)
	}
	
	// Create firewall rules for each IP
	for _, ip := range ips {
		err := f.createBlockRule(ip, region)
		if err != nil {
			return fmt.Errorf("failed to create block rule for %s: %w", ip, err)
		}
	}
	
	return nil
}

// UnblockIPs unblocks the IPs in the specified region
func (f *Firewall) UnblockIPs(region string) error {
	return f.removeRules(region)
}

// UnblockAll removes all firewall rules created by this application
func (f *Firewall) UnblockAll() error {
	return f.removeRules("")
}

// readIPsFromFile reads IPs from a text file
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

// createBlockRule creates a Windows Firewall rule to block a specific IP for Overwatch
func (f *Firewall) createBlockRule(ip, region string) error {
	// Create unique rule name
	ruleName := fmt.Sprintf("%s%s-%s", f.rulePrefix, region, sanitizeIP(ip))
	
	// Create outbound block rule
	err := f.execFirewallCmd("add", "rule", 
		"name="+ruleName,
		"dir=out",
		"action=block",
		"program="+f.exePath,
		"remoteip="+ip)
	
	if err != nil {
		return err
	}
	
	// Create inbound block rule
	return f.execFirewallCmd("add", "rule", 
		"name="+ruleName+"-In",
		"dir=in",
		"action=block",
		"program="+f.exePath,
		"remoteip="+ip)
}

// removeRules removes firewall rules for a specific region or all regions if region is empty
func (f *Firewall) removeRules(region string) error {
	// Construct the rule prefix to look for
	prefix := f.rulePrefix
	if region != "" {
		prefix = prefix + region
	}
	
	// Get all existing rules
	rules, err := f.listRules()
	if err != nil {
		return err
	}
	
	// Delete matching rules
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

// listRules lists all firewall rules created by this application
func (f *Firewall) listRules() ([]string, error) {
	cmd := exec.Command("netsh", "advfirewall", "firewall", "show", "rule", "name=all")
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

// execFirewallCmd executes a Windows firewall command
func (f *Firewall) execFirewallCmd(args ...string) error {
	allArgs := append([]string{"advfirewall", "firewall"}, args...)
	cmd := exec.Command("netsh", allArgs...)
	output, err := cmd.CombinedOutput()
	
	if err != nil {
		return fmt.Errorf("firewall command failed: %s, error: %w", string(output), err)
	}
	
	return nil
}

// sanitizeIP sanitizes an IP address for use in a rule name
func sanitizeIP(ip string) string {
	return strings.NewReplacer(".", "-", "/", "_", ":", "--").Replace(ip)
}

// IsAdminPrivilegesAvailable checks if the application has admin privileges
func IsAdminPrivilegesAvailable() bool {
	cmd := exec.Command("net", "session")
	err := cmd.Run()
	return err == nil
}