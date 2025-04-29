package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

var regions = []string{"EU", "NA", "AS", "AFR", "ME", "OCE", "SA"}

type OwVpnGui struct {
	window        fyne.Window
	logText       *widget.Label
	statusLabel   *widget.Label
	statusIcon    *canvas.Image
	progressBar   *widget.ProgressBarInfinite
	regionButtons map[string]*widget.Button
	firewallCmd   *exec.Cmd
	cmdStdin      io.WriteCloser
	blocked       map[string]bool
	waiting       bool
	waitingMutex  sync.Mutex
}

func main() {
	a := app.New()
	w := a.NewWindow("Overwatch VPN")
	w.Resize(fyne.NewSize(600, 400))

	gui := &OwVpnGui{
		window:        w,
		logText:       widget.NewLabel("Starting application..."),
		statusLabel:   widget.NewLabel("Initializing..."),
		statusIcon:    canvas.NewImageFromResource(theme.InfoIcon()),
		progressBar:   widget.NewProgressBarInfinite(),
		regionButtons: make(map[string]*widget.Button),
		blocked:       make(map[string]bool),
		waiting:       false,
	}

	gui.statusIcon.SetMinSize(fyne.NewSize(20, 20))
	gui.progressBar.Hide()

	gui.logText.Wrapping = fyne.TextWrapWord
	scrollLog := container.NewScroll(gui.logText)
	scrollLog.SetMinSize(fyne.NewSize(580, 150))

	statusBox := container.NewHBox(
		gui.statusIcon,
		widget.NewLabel("Status:"),
		gui.statusLabel,
		gui.progressBar,
	)

	regionButtons := container.NewGridWithColumns(4)
	for _, region := range regions {
		btn := widget.NewButton(fmt.Sprintf("Block %s", region), nil)
		regionName := region
		btn.OnTapped = func() {
			gui.toggleRegion(regionName)
		}
		gui.regionButtons[region] = btn
		regionButtons.Add(btn)
	}

	unblockAllBtn := widget.NewButton("Unblock All Regions", func() {
		gui.unblockAll()
	})

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

	w.SetOnClosed(func() {
		gui.cleanup()
	})

	go gui.initialize()

	w.ShowAndRun()
}

func (g *OwVpnGui) initialize() {
	g.log("Initializing application...")

	g.log("Fetching IP addresses...")
	if err := g.runIpPuller(); err != nil {
		g.log(fmt.Sprintf("Error fetching IPs: %v", err))
		g.setStatus("Error: IP Puller failed", theme.ErrorIcon())
		dialog.ShowError(fmt.Errorf("failed to run IP Puller: %v", err), g.window)
		return
	}
	g.log("Successfully fetched IP addresses")

	g.log("Starting firewall daemon...")
	if err := g.startFirewallDaemon(); err != nil {
		g.log(fmt.Sprintf("Error starting firewall daemon: %v", err))
		g.setStatus("Error: Firewall daemon failed", theme.ErrorIcon())
		dialog.ShowError(fmt.Errorf("failed to start firewall daemon: %v", err), g.window)
		return
	}
	g.log("Firewall daemon started successfully")

	g.setStatus("Ready", theme.ConfirmIcon())

	go func() {
		for {
			g.checkStatus()
			time.Sleep(5 * time.Second)
		}
	}()
}

func (g *OwVpnGui) runIpPuller() error {
	exePath, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), "ip-puller.exe"))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	g.log("Running IP Puller...")
	cmd := exec.Command(exePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute IP Puller: %v - output: %s", err, string(output))
	}
	return nil
}

func (g *OwVpnGui) startFirewallDaemon() error {
	exePath, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), "firewall-sidecar.exe"))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	g.log("Starting firewall daemon...")
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

	stderr, err := g.firewallCmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %v", err)
	}

	if err := g.firewallCmd.Start(); err != nil {
		return fmt.Errorf("failed to start firewall daemon: %v", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			text := scanner.Text()
			g.processFirewallOutput(text)
		}
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			text := scanner.Text()
			g.log(fmt.Sprintf("Error: %s", text))
		}
	}()

	time.Sleep(500 * time.Millisecond)

	return nil
}

func (g *OwVpnGui) processFirewallOutput(text string) {
	// Log errors and success messages
	if strings.Contains(text, "ERROR:") || strings.Contains(text, "Successfully") {
		g.log(text)
	}

	// Detect waiting state
	if strings.Contains(text, "Overwatch is currently running") ||
		strings.Contains(text, "Waiting for Overwatch to close") {
		g.setWaiting(true)
		g.log("Waiting for Overwatch to close before applying changes...")
		g.setStatus("Waiting for Overwatch to close", theme.WarningIcon())
	}

	// Detect operations in progress
	if strings.Contains(text, "Blocking IPs") || strings.Contains(text, "Unblocking IPs") {
		g.setWaiting(true)
		g.setStatus("Working...", theme.InfoIcon())
	}

	// Detect completion
	if strings.Contains(text, "Successfully blocked") ||
		strings.Contains(text, "Successfully unblocked") ||
		strings.Contains(text, "Successfully removed") {
		g.setWaiting(false)
		g.setStatus("Ready", theme.ConfirmIcon())
	}

	// Detect errors
	if strings.Contains(text, "ERROR:") {
		g.setWaiting(false)
		g.setStatus("Error", theme.ErrorIcon())
	}

	// Status check response
	if strings.Contains(text, "Status: Overwatch is currently running") {
		g.setStatus("Overwatch is running", theme.InfoIcon())
	} else if strings.Contains(text, "Status: Overwatch is not running") {
		g.setStatus("Ready", theme.ConfirmIcon())
	}
}

func (g *OwVpnGui) setWaiting(waiting bool) {
	g.waitingMutex.Lock()
	defer g.waitingMutex.Unlock()

	if g.waiting == waiting {
		return
	}

	g.waiting = waiting

	if waiting {
		g.progressBar.Show()
		for _, btn := range g.regionButtons {
			btn.Disable()
		}
	} else {
		g.progressBar.Hide()
		for _, btn := range g.regionButtons {
			btn.Enable()
		}
	}

	g.window.Canvas().Refresh(g.progressBar)
}

func (g *OwVpnGui) toggleRegion(region string) {
	isBlocked := g.blocked[region]

	// Don't allow toggling while in waiting state
	if g.waiting {
		g.log("Please wait for current operation to complete")
		return
	}

	if isBlocked {
		g.log(fmt.Sprintf("Unblocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("unblock|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error unblocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = false
		g.window.Canvas().Refresh(g.regionButtons[region])
		g.regionButtons[region].SetText(fmt.Sprintf("Block %s", region))
		g.setWaiting(true)
	} else {
		g.log(fmt.Sprintf("Blocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("block|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error blocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = true
		g.window.Canvas().Refresh(g.regionButtons[region])
		g.regionButtons[region].SetText(fmt.Sprintf("Unblock %s", region))
		g.setWaiting(true)
	}
}

func (g *OwVpnGui) unblockAll() {
	// Don't allow unblocking all while in waiting state
	if g.waiting {
		g.log("Please wait for current operation to complete")
		return
	}

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
	g.setWaiting(true)
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

func (g *OwVpnGui) setStatus(status string, icon fyne.Resource) {
	g.statusLabel.SetText(status)
	g.statusIcon.Resource = icon
	g.window.Canvas().Refresh(g.statusLabel)
	g.window.Canvas().Refresh(g.statusIcon)
}

func (g *OwVpnGui) log(message string) {
	fmt.Println(message)
	currentText := g.logText.Text
	g.logText.SetText(message + "\n" + currentText)
	g.window.Canvas().Refresh(g.logText)
}

func (g *OwVpnGui) cleanup() {
	g.log("Cleaning up...")

	// Send unblock-all command
	if g.cmdStdin != nil {
		_ = g.sendCommand("unblock-all")
		// Give more time for the unblock operation to complete
		time.Sleep(1 * time.Second)
	}

	// Close stdin pipe to signal EOF to the sidecar
	if g.cmdStdin != nil {
		_ = g.cmdStdin.Close()
	}

	// Wait for firewall process to exit
	if g.firewallCmd != nil && g.firewallCmd.Process != nil {
		// Give the process a chance to exit gracefully
		waitCh := make(chan error, 1)
		go func() {
			waitCh <- g.firewallCmd.Wait()
		}()

		// Wait for process to exit or timeout
		select {
		case <-waitCh:
			// Process exited
		case <-time.After(2 * time.Second):
			// Timeout, force kill
			_ = g.firewallCmd.Process.Kill()
			_ = g.firewallCmd.Wait()
		}
	}

	g.log("Cleanup complete")
}
