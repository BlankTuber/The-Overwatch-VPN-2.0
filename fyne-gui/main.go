package main

import (
	"bufio"
	"fmt"
	"image/color"
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

// Colors for UI elements
var (
	colorBlocked   = color.NRGBA{R: 217, G: 83, B: 79, A: 255}  // Red
	colorUnblocked = color.NRGBA{R: 0, G: 177, B: 87, A: 255}   // Green
	colorTitle     = color.NRGBA{R: 66, G: 139, B: 202, A: 255} // Blue
)

type OwVpnGui struct {
	window             fyne.Window
	logText            *widget.Label
	statusLabel        *widget.Label
	statusIcon         *canvas.Image
	progressBar        *widget.ProgressBarInfinite
	regionButtons      map[string]*widget.Button
	firewallCmd        *exec.Cmd
	cmdStdin           io.WriteCloser
	blocked            map[string]bool
	blockingInProgress bool
	blockingMutex      sync.Mutex
	availableRegions   []string
	overwatchPath      string
	pathConfigured     bool
	modalRef           fyne.CanvasObject
}

func checkAdminPermissions() bool {
	cmd := exec.Command("net", "session")
	return cmd.Run() == nil
}

func main() {
	a := app.New()
	w := a.NewWindow("Overwatch VPN 2.0")
	w.Resize(fyne.NewSize(800, 600))

	// Check for admin permissions first
	if !checkAdminPermissions() {
		showAdminPermissionsDialog(w)
		w.ShowAndRun()
		return
	}

	gui := &OwVpnGui{
		window:             w,
		logText:            widget.NewLabel("Starting application..."),
		statusLabel:        widget.NewLabel("Initializing..."),
		statusIcon:         canvas.NewImageFromResource(theme.InfoIcon()),
		progressBar:        widget.NewProgressBarInfinite(),
		regionButtons:      make(map[string]*widget.Button),
		blocked:            make(map[string]bool),
		blockingInProgress: false,
		availableRegions:   []string{},
		pathConfigured:     false,
	}

	gui.updateRegionButtons()

	w.SetOnClosed(func() {
		gui.cleanup()
	})

	go gui.initialize()

	w.ShowAndRun()
}

func showAdminPermissionsDialog(w fyne.Window) {
	content := container.NewVBox(
		widget.NewLabel("This application requires administrator privileges."),
		widget.NewLabel("Please right-click and select 'Run as administrator'."),
	)

	dialog := dialog.NewCustom("Administrator Privileges Required", "Exit", content, w)
	dialog.SetOnClosed(func() {
		os.Exit(1)
	})

	dialog.Show()
}

func (g *OwVpnGui) updateAvailableRegions() {
	g.log("Checking available region IP lists...")
	ipDir := "ips"

	// Create the directory if it doesn't exist
	if _, err := os.Stat(ipDir); os.IsNotExist(err) {
		g.log("IP directory not found, will be created after IP Puller runs")
		return
	}

	// Clear the available regions
	g.availableRegions = []string{}

	// Check each region
	for _, region := range regions {
		filename := filepath.Join(ipDir, fmt.Sprintf("%s.txt", region))
		if info, err := os.Stat(filename); err == nil && !info.IsDir() {
			// File exists and is not a directory
			g.availableRegions = append(g.availableRegions, region)
			g.log(fmt.Sprintf("Found IP list for region: %s", region))
		}
	}

	// Update the UI to show only available regions
	g.updateRegionButtons()
}

func (g *OwVpnGui) promptForOverwatchPath() {
	content := container.NewVBox(
		widget.NewLabel("You must locate Overwatch before using this application."),
		widget.NewLabel("Please launch Overwatch, then click 'Detect Overwatch'."),
	)

	detectBtn := widget.NewButton("Detect Overwatch", func() {
		g.detectOverwatchPath()
	})

	buttonBox := container.NewCenter(detectBtn)
	finalContent := container.NewVBox(content, buttonBox)

	// Create a custom modal instead of a dialog to avoid the dismiss button
	modal := widget.NewModalPopUp(finalContent, g.window.Canvas())
	modal.Show()

	// Store reference to the modal so we can dismiss it later
	g.modalRef = modal

	// Check periodically if we need to re-show the dialog
	go func() {
		for !g.pathConfigured {
			time.Sleep(1 * time.Second)

			// Check if the path was loaded from the firewall sidecar
			if err := g.sendCommand("get-path"); err == nil {
				// Give a moment for the response to be processed
				time.Sleep(500 * time.Millisecond)
				if g.pathConfigured {
					modal.Hide()
					return
				}
			}
		}

		// Hide modal when path is configured
		modal.Hide()
	}()
}

func (g *OwVpnGui) detectOverwatchPath() {
	g.log("Attempting to detect Overwatch path...")

	// Try to find the Overwatch process
	if path, success := g.findOverwatchProcess(); success {
		g.overwatchPath = path
		g.pathConfigured = true
		g.log(fmt.Sprintf("Detected Overwatch at: %s", path))

		// Send path to firewall sidecar
		if err := g.sendCommand(fmt.Sprintf("set-path|%s", path)); err != nil {
			g.log(fmt.Sprintf("Error setting Overwatch path: %v", err))
			g.pathConfigured = false
		} else {
			g.enableRegionButtons()
			g.setStatus("Overwatch detected, ready to use", theme.ConfirmIcon())
		}
	} else {
		g.log("Could not detect Overwatch. Please make sure Overwatch is running.")
		g.showOverwatchNotRunningDialog()
	}
}

func (g *OwVpnGui) showOverwatchNotRunningDialog() {
	content := container.NewVBox(
		widget.NewLabel("Overwatch process was not detected."),
		widget.NewLabel("Please make sure Overwatch is running, then try again."),
	)

	dialog.ShowCustom("Overwatch Not Detected", "OK", content, g.window)
}

func (g *OwVpnGui) findOverwatchProcess() (string, bool) {
	// PowerShell command to find Overwatch process and its path
	cmd := exec.Command("powershell", "-Command",
		"Get-Process -Name 'Overwatch' | Select-Object -ExpandProperty Path")

	output, err := cmd.Output()
	if err != nil {
		return "", false
	}

	path := strings.TrimSpace(string(output))
	if path != "" {
		return path, true
	}

	return "", false
}

func (g *OwVpnGui) enableRegionButtons() {
	for _, btn := range g.regionButtons {
		btn.Enable()
	}
	g.window.Canvas().Refresh(g.window.Content())
}

func (g *OwVpnGui) disableRegionButtons() {
	for _, btn := range g.regionButtons {
		btn.Disable()
	}
	g.window.Canvas().Refresh(g.window.Content())
}

func (g *OwVpnGui) updateRegionButtons() {
	// Create a new grid with 3 columns
	regionButtons := container.NewGridWithColumns(3)

	// Clear current buttons
	g.regionButtons = make(map[string]*widget.Button)

	if len(g.availableRegions) == 0 {
		noRegionsLabel := widget.NewLabel("No region IP lists available. Please run IP Puller first.")
		regionButtons.Add(noRegionsLabel)
	} else {
		for _, region := range g.availableRegions {
			// Create button with initial green/unblocked state
			btn := widget.NewButton(region, nil)
			btn.Importance = widget.SuccessImportance // Start as green
			btn.SetIcon(theme.ContentRemoveIcon())    // Using remove icon as "unblocked" (removing restrictions)

			// Initially disable buttons until Overwatch is configured
			btn.Disable()

			// Store region for callback
			regionName := region

			btn.OnTapped = func() {
				g.toggleRegion(regionName)
			}

			// Create a container with the button
			buttonContainer := container.NewPadded(btn)

			g.regionButtons[region] = btn
			regionButtons.Add(buttonContainer)
		}
	}

	// Create the updated content
	titleLabel := canvas.NewText("OVERWATCH VPN", color.NRGBA{R: 66, G: 139, B: 202, A: 255})
	titleLabel.TextSize = 28
	titleLabel.TextStyle = fyne.TextStyle{Bold: true}
	titleLabel.Alignment = fyne.TextAlignCenter

	statusLabel := canvas.NewText("Status:", color.White)
	statusLabel.TextStyle = fyne.TextStyle{Bold: true}

	statusBox := container.NewHBox(
		g.statusIcon,
		container.NewPadded(statusLabel),
		g.statusLabel,
		g.progressBar,
	)

	regionLabel := canvas.NewText("SELECT REGIONS TO BLOCK", color.NRGBA{R: 66, G: 139, B: 202, A: 255})
	regionLabel.TextSize = 18
	regionLabel.TextStyle = fyne.TextStyle{Bold: true}
	regionLabel.Alignment = fyne.TextAlignCenter

	unblockAllBtn := widget.NewButton("UNBLOCK ALL REGIONS", func() {
		g.unblockAll()
	})
	unblockAllBtn.Importance = widget.HighImportance
	unblockAllBtnContainer := container.NewPadded(unblockAllBtn)

	detectOverwatchBtn := widget.NewButton("DETECT OVERWATCH PATH", func() {
		g.detectOverwatchPath()
	})
	detectOverwatchBtn.Importance = widget.WarningImportance
	detectOverwatchBtnContainer := container.NewPadded(detectOverwatchBtn)

	logLabel := canvas.NewText("CONNECTION LOG", color.NRGBA{R: 66, G: 139, B: 202, A: 255})
	logLabel.TextSize = 16
	logLabel.TextStyle = fyne.TextStyle{Bold: true}
	logLabel.Alignment = fyne.TextAlignCenter

	scrollLog := container.NewScroll(g.logText)
	scrollLog.SetMinSize(fyne.NewSize(780, 150))

	content := container.NewVBox(
		container.NewPadded(titleLabel),
		container.NewPadded(statusBox),
		widget.NewSeparator(),
		container.NewPadded(regionLabel),
		container.NewPadded(regionButtons),
		container.NewCenter(unblockAllBtnContainer),
		container.NewCenter(detectOverwatchBtnContainer),
		widget.NewSeparator(),
		container.NewPadded(logLabel),
		container.NewPadded(scrollLog),
	)

	g.window.SetContent(container.NewPadded(content))
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

	// Check available regions and update UI
	g.updateAvailableRegions()

	g.log("Starting firewall daemon...")
	if err := g.startFirewallDaemon(); err != nil {
		g.log(fmt.Sprintf("Error starting firewall daemon: %v", err))
		g.setStatus("Error: Firewall daemon failed", theme.ErrorIcon())
		dialog.ShowError(fmt.Errorf("failed to start firewall daemon: %v", err), g.window)
		return
	}
	g.log("Firewall daemon started successfully")

	// Check if Overwatch path is already configured in firewall sidecar
	if err := g.sendCommand("get-path"); err != nil {
		g.log(fmt.Sprintf("Error checking Overwatch path: %v", err))
	}

	// Short delay to receive response
	time.Sleep(500 * time.Millisecond)

	// If path not configured, prompt user
	if !g.pathConfigured {
		g.setStatus("Overwatch path not configured", theme.WarningIcon())
		g.promptForOverwatchPath()
	} else {
		g.setStatus("Ready", theme.ConfirmIcon())
		g.enableRegionButtons()
	}

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
	if strings.Contains(text, "ERROR:") || strings.Contains(text, "Successfully") {
		g.log(text)
	}

	if strings.Contains(text, "Overwatch path not configured") {
		g.pathConfigured = false
		g.disableRegionButtons()
		g.setStatus("Overwatch path not configured", theme.WarningIcon())
	}

	if strings.Contains(text, "Current Overwatch path:") {
		g.pathConfigured = true
		g.overwatchPath = strings.TrimPrefix(text, "Current Overwatch path: ")
		g.log(fmt.Sprintf("Using Overwatch path: %s", g.overwatchPath))
		g.enableRegionButtons()
	}

	if strings.Contains(text, "Overwatch path set to:") {
		g.pathConfigured = true
		g.overwatchPath = strings.TrimPrefix(text, "Overwatch path set to: ")
		g.log(fmt.Sprintf("Overwatch path set to: %s", g.overwatchPath))
		g.enableRegionButtons()
	}

	if strings.Contains(text, "Overwatch is currently running") ||
		strings.Contains(text, "Waiting for Overwatch to close") {
		g.setBlockingInProgress(true)
		g.log("Waiting for Overwatch to close before applying block...")
		g.setStatus("Waiting for Overwatch to close", theme.WarningIcon())
	}

	if strings.Contains(text, "Overwatch has closed, proceeding with IP blocking") {
		g.log("Overwatch has closed, proceeding with IP blocking...")
	}

	if strings.Contains(text, "Blocking IPs") {
		g.setBlockingInProgress(true)
		g.setStatus("Blocking...", theme.InfoIcon())
	}

	if strings.Contains(text, "Successfully blocked") {
		g.setBlockingInProgress(false)
		g.setStatus("Ready", theme.ConfirmIcon())
	}

	if strings.Contains(text, "Unblocking IPs") || strings.Contains(text, "Unblocking all IPs") {
		g.setStatus("Unblocking...", theme.InfoIcon())
	}

	if strings.Contains(text, "Successfully unblocked") {
		g.setStatus("Ready", theme.ConfirmIcon())
	}

	if strings.Contains(text, "ERROR:") {
		g.setBlockingInProgress(false)
		g.setStatus("Error", theme.ErrorIcon())
	}

	if strings.Contains(text, "Status: Overwatch is currently running") {
		g.setStatus("Overwatch is running", theme.InfoIcon())
	} else if strings.Contains(text, "Status: Overwatch is not running") {
		if !g.pathConfigured {
			g.setStatus("Overwatch path not configured", theme.WarningIcon())
		} else {
			g.setStatus("Ready", theme.ConfirmIcon())
		}
	}
}

func (g *OwVpnGui) setBlockingInProgress(blocking bool) {
	g.blockingMutex.Lock()
	defer g.blockingMutex.Unlock()

	if g.blockingInProgress == blocking {
		return
	}

	g.blockingInProgress = blocking

	if blocking {
		g.progressBar.Show()
		// Only disable the block operations, not unblock
		for region, btn := range g.regionButtons {
			if !g.blocked[region] {
				btn.Disable()
			}
		}
	} else {
		g.progressBar.Hide()
		// Only enable buttons if path is configured
		if g.pathConfigured {
			for _, btn := range g.regionButtons {
				btn.Enable()
			}
		}
	}

	g.window.Canvas().Refresh(g.progressBar)
}

func (g *OwVpnGui) isBlockingInProgress() bool {
	g.blockingMutex.Lock()
	defer g.blockingMutex.Unlock()
	return g.blockingInProgress
}

func (g *OwVpnGui) toggleRegion(region string) {
	// Ensure Overwatch path is configured
	if !g.pathConfigured {
		g.log("Overwatch path not configured. Please detect Overwatch path first.")
		g.promptForOverwatchPath()
		return
	}

	isBlocked := g.blocked[region]

	if isBlocked {
		// Unblocking is always allowed
		g.log(fmt.Sprintf("Unblocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("unblock|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error unblocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = false

		// Use unblocked style (green with remove icon for "remove restrictions")
		g.regionButtons[region].Importance = widget.SuccessImportance
		g.regionButtons[region].SetText(region)
		g.regionButtons[region].SetIcon(theme.ContentRemoveIcon())

		g.window.Canvas().Refresh(g.regionButtons[region])
	} else {
		// Check if blocking operations are in progress
		if g.isBlockingInProgress() {
			g.log("Please wait for current blocking operation to complete")
			return
		}

		g.log(fmt.Sprintf("Blocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("block|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error blocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = true

		// Use blocked style (red with add icon for "add restrictions")
		g.regionButtons[region].Importance = widget.DangerImportance
		g.regionButtons[region].SetText(region)
		g.regionButtons[region].SetIcon(theme.ContentAddIcon())

		g.window.Canvas().Refresh(g.regionButtons[region])
	}
}

func (g *OwVpnGui) unblockAll() {
	// Unblock all is always allowed, even during blocking operations
	g.log("Unblocking all regions...")
	if err := g.sendCommand("unblock-all"); err != nil {
		g.log(fmt.Sprintf("Error unblocking all regions: %v", err))
		return
	}

	// Reset all buttons to unblocked state
	for region := range g.blocked {
		g.blocked[region] = false
		g.regionButtons[region].Importance = widget.SuccessImportance
		g.regionButtons[region].SetText(region)
		g.regionButtons[region].SetIcon(theme.ContentRemoveIcon())

		// Only enable if path is configured
		if g.pathConfigured {
			g.regionButtons[region].Enable()
		}
	}
	g.window.Canvas().Refresh(g.window.Content())

	// Clear any blocking in progress
	g.setBlockingInProgress(false)
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

	timestamp := time.Now().Format("15:04:05")
	formattedMsg := fmt.Sprintf("[%s] %s\n%s", timestamp, message, g.logText.Text)
	g.logText.SetText(formattedMsg)
	g.window.Canvas().Refresh(g.logText)
}

func (g *OwVpnGui) cleanup() {
	g.log("Cleaning up...")

	// Hide the window during cleanup
	g.window.Hide()

	// Send cleanup command to the firewall daemon
	if g.cmdStdin != nil {
		g.log("Sending cleanup command to firewall daemon...")
		_ = g.sendCommand("unblock-all")

		// Close stdin pipe to signal EOF to the sidecar
		_ = g.cmdStdin.Close()

		// Let the firewall daemon clean up in the background
		// The application will exit without waiting
	}

	// Exit the application - the firewall daemon will clean up in the background
	g.log("Cleanup initiated, exiting...")
	os.Exit(0)
}
