package firewall

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"quidque.no/ow-firewall-sidecar/internal/config"
)

type Firewall struct {
	rulePrefix string
	exePath    string
}

func New() *Firewall {
	exePath := getOverwatchExePath()
	return &Firewall{
		rulePrefix: config.FirewallRulePrefix,
		exePath:    exePath,
	}
}

func getOverwatchExePath() string {
	commonPaths := []string{
		"C:\\Program Files\\Overwatch\\" + config.OverwatchProcessName,
		"C:\\Program Files (x86)\\Overwatch\\" + config.OverwatchProcessName,
		"C:\\Program Files\\Battle.net\\Games\\Overwatch\\" + config.OverwatchProcessName,
		"C:\\Program Files (x86)\\Battle.net\\Games\\Overwatch\\" + config.OverwatchProcessName,
	}

	battleNetPaths := getBattleNetGamePaths()
	if len(battleNetPaths) > 0 {
		commonPaths = append(commonPaths, battleNetPaths...)
	}

	for _, path := range commonPaths {
		if fileExists(path) {
			return path
		}
	}

	return "C:\\Program Files (x86)\\Overwatch\\" + config.OverwatchProcessName
}

func getBattleNetGamePaths() []string {
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

func (f *Firewall) BlockIPs(region string, ipListDir string) error {
	filePath := filepath.Join(ipListDir, fmt.Sprintf("%s.txt", region))

	ips, err := readIPsFromFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read IP list: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("no IPs found for region %s", region)
	}

	// Bulk block IPs in batches
	batchSize := 50
	totalBatches := (len(ips) + batchSize - 1) / batchSize

	fmt.Printf("Starting to block %d IPs for region %s in %d batches\n", len(ips), region, totalBatches)

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
			"program="+f.exePath,
			"remoteip="+ipList)

		if err != nil {
			return err
		}

		// Create inbound rule for batch
		err = f.execFirewallCmd("add", "rule",
			"name="+ruleName+"-In",
			"dir=in",
			"action=block",
			"program="+f.exePath,
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
