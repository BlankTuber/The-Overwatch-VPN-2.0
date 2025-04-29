package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

var regions = []string{"EU", "NA", "AS", "AFR", "ME", "OCE", "SA"}

type OwVpnGui struct {
	window        fyne.Window
	logText       *widget.Label
	statusLabel   *widget.Label
	regionButtons map[string]*widget.Button
	firewallCmd   *exec.Cmd
	cmdStdin      io.WriteCloser
	blocked       map[string]bool
}

func main() {
	a := app.New()
	w := a.NewWindow("Overwatch VPN")
	w.Resize(fyne.NewSize(600, 400))

	gui := &OwVpnGui{
		window:        w,
		logText:       widget.NewLabel("Starting application..."),
		statusLabel:   widget.NewLabel("Initializing..."),
		regionButtons: make(map[string]*widget.Button),
		blocked:       make(map[string]bool),
	}

	// Log display
	gui.logText.Wrapping = fyne.TextWrapWord
	scrollLog := container.NewScroll(gui.logText)
	scrollLog.SetMinSize(fyne.NewSize(580, 150))

	// Status bar
	statusBox := container.NewHBox(
		widget.NewLabel("Status:"),
		gui.statusLabel,
	)

	// Region buttons
	regionButtons := container.NewGridWithColumns(4)
	for _, region := range regions {
		btn := widget.NewButton(fmt.Sprintf("Block %s", region), nil)
		regionName := region // Capture for closure
		btn.OnTapped = func() {
			gui.toggleRegion(regionName)
		}
		gui.regionButtons[region] = btn
		regionButtons.Add(btn)
	}

	// Unblock all button
	unblockAllBtn := widget.NewButton("Unblock All Regions", func() {
		gui.unblockAll()
	})

	// Construct layout
	content := container.NewVBox(
		statusBox,
		widget.NewSeparator(),
		scrollLog,
		widget.NewSeparator(),
		widget.NewLabel("Region Control:"),
		regionButtons,
		widget.NewSeparator(),
		unblockAllBtn,
	)

	w.SetContent(content)

	// Handle cleanup on close
	w.SetOnClosed(func() {
		gui.cleanup()
	})

	// Initialize in a goroutine to keep UI responsive
	go gui.initialize()

	// Display and run
	w.ShowAndRun()
}

func (g *OwVpnGui) initialize() {
	g.log("Initializing application...")

	if !g.checkAdminPrivileges() {
		g.log("WARNING: Administrator privileges required.")
		g.setStatus("WARNING: Admin privileges required")
		dialog.ShowError(fmt.Errorf("this application requires administrator privileges"), g.window)
		return
	}

	g.log("Fetching IP addresses...")
	if err := g.runIpPuller(); err != nil {
		g.log(fmt.Sprintf("Error fetching IPs: %v", err))
		g.setStatus("Error: IP Puller failed")
		dialog.ShowError(fmt.Errorf("failed to run IP Puller: %v", err), g.window)
		return
	}
	g.log("Successfully fetched IP addresses")

	g.log("Starting firewall daemon...")
	if err := g.startFirewallDaemon(); err != nil {
		g.log(fmt.Sprintf("Error starting firewall daemon: %v", err))
		g.setStatus("Error: Firewall daemon failed")
		dialog.ShowError(fmt.Errorf("failed to start firewall daemon: %v", err), g.window)
		return
	}
	g.log("Firewall daemon started successfully")

	g.setStatus("Ready")

	// Periodically check Overwatch status
	go func() {
		for {
			g.checkStatus()
			time.Sleep(5 * time.Second)
		}
	}()
}

func (g *OwVpnGui) checkAdminPrivileges() bool {
	cmd := exec.Command("net", "session")
	return cmd.Run() == nil
}

func (g *OwVpnGui) runIpPuller() error {
	exePath := filepath.Join(".", "ip-puller.exe")
	cmd := exec.Command(exePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute IP Puller: %v - output: %s", err, string(output))
	}
	return nil
}

func (g *OwVpnGui) startFirewallDaemon() error {
	exePath := filepath.Join(".", "firewall-sidecar.exe")
	g.firewallCmd = exec.Command(exePath, "daemon")

	stdin, err := g.firewallCmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %v", err)
	}
	g.cmdStdin = stdin

	stdout, err := g.firewallCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %v", err)
	}

	if err := g.firewallCmd.Start(); err != nil {
		return fmt.Errorf("failed to start firewall daemon: %v", err)
	}

	// Read output in a separate goroutine
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			g.log(text)
		}
	}()

	// Wait a moment to ensure daemon is started
	time.Sleep(500 * time.Millisecond)

	return nil
}

func (g *OwVpnGui) toggleRegion(region string) {
	isBlocked := g.blocked[region]

	if isBlocked {
		g.log(fmt.Sprintf("Unblocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("unblock|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error unblocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = false
		g.window.Canvas().Refresh(g.regionButtons[region])
		g.regionButtons[region].SetText(fmt.Sprintf("Block %s", region))
	} else {
		g.log(fmt.Sprintf("Blocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("block|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error blocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = true
		g.window.Canvas().Refresh(g.regionButtons[region])
		g.regionButtons[region].SetText(fmt.Sprintf("Unblock %s", region))
	}
}

func (g *OwVpnGui) unblockAll() {
	g.log("Unblocking all regions...")
	if err := g.sendCommand("unblock-all"); err != nil {
		g.log(fmt.Sprintf("Error unblocking all regions: %v", err))
		return
	}

	for region := range g.blocked {
		g.blocked[region] = false
		g.regionButtons[region].SetText(fmt.Sprintf("Block %s", region))
	}
	g.window.Canvas().Refresh(g.window.Content())

	g.log("All regions unblocked")
}

func (g *OwVpnGui) checkStatus() {
	if err := g.sendCommand("status"); err != nil {
		g.log(fmt.Sprintf("Error checking status: %v", err))
	}
}

func (g *OwVpnGui) sendCommand(command string) error {
	if g.cmdStdin == nil {
		return fmt.Errorf("firewall daemon not running")
	}

	_, err := fmt.Fprintln(g.cmdStdin, command)
	return err
}

func (g *OwVpnGui) log(message string) {
	fmt.Println(message)
	currentText := g.logText.Text
	g.logText.SetText(message + "\n" + currentText)
	g.window.Canvas().Refresh(g.logText)
}

func (g *OwVpnGui) setStatus(status string) {
	g.statusLabel.SetText(status)
	g.window.Canvas().Refresh(g.statusLabel)
}

func (g *OwVpnGui) cleanup() {
	g.log("Cleaning up...")

	_ = g.sendCommand("unblock-all")

	time.Sleep(500 * time.Millisecond)

	if g.firewallCmd != nil && g.firewallCmd.Process != nil {
		_ = g.firewallCmd.Process.Kill()
		_ = g.firewallCmd.Wait()
	}

	g.log("Cleanup complete")
}
