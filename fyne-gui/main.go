package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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

var (
	colorBlocked   = color.NRGBA{R: 217, G: 83, B: 79, A: 255}
	colorUnblocked = color.NRGBA{R: 0, G: 177, B: 87, A: 255}
	colorTitle     = color.NRGBA{R: 66, G: 139, B: 202, A: 255}
)

type Config struct {
	OverwatchPath   string `json:"overwatchPath"`
	UseGithubSource bool   `json:"useGithubSource"`
}

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
	useGithubSource    bool
	sourceToggle       *widget.Check
	config             Config
	configPath         string
	isOverwatchRunning bool
	processMutex       sync.Mutex
	isInitialized      bool
	isChangingSource   bool
}

func checkAdminPermissions() bool {
	cmd := exec.Command("net", "session")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	return cmd.Run() == nil
}

func main() {
	a := app.New()
	w := a.NewWindow("Overwatch VPN 2.0")
	w.Resize(fyne.NewSize(800, 600))

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
		configPath:         "config.json",
		isInitialized:      false,
		isChangingSource:   false,
	}

	gui.loadConfig()
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

func (g *OwVpnGui) loadConfig() {
	data, err := os.ReadFile(g.configPath)
	if err == nil {
		if err := json.Unmarshal(data, &g.config); err == nil {
			g.log(fmt.Sprintf("Loaded configuration from %s", g.configPath))
			g.overwatchPath = g.config.OverwatchPath
			g.useGithubSource = g.config.UseGithubSource

			if g.overwatchPath != "" && fileExists(g.overwatchPath) {
				g.pathConfigured = true
				g.log(fmt.Sprintf("Using configured Overwatch path: %s", g.overwatchPath))
			}
		}
	}
}

func (g *OwVpnGui) saveConfig() {
	g.config.OverwatchPath = g.overwatchPath
	g.config.UseGithubSource = g.useGithubSource

	data, err := json.MarshalIndent(g.config, "", "  ")
	if err != nil {
		g.log(fmt.Sprintf("Error creating config JSON: %v", err))
		return
	}

	if err := os.WriteFile(g.configPath, data, 0644); err != nil {
		g.log(fmt.Sprintf("Error writing config file: %v", err))
	} else {
		g.log("Configuration saved successfully")
	}
}

func (g *OwVpnGui) getIPDirectory() string {
	if g.useGithubSource {
		return "ips_mina"
	}
	return "ips"
}

func (g *OwVpnGui) updateAvailableRegions() {
	g.log("Checking available region IP lists...")
	ipDir := g.getIPDirectory()

	if _, err := os.Stat(ipDir); os.IsNotExist(err) {
		g.log(fmt.Sprintf("IP directory %s not found, will be created after IP Puller runs", ipDir))
		return
	}

	g.availableRegions = []string{}

	for _, region := range regions {
		filename := filepath.Join(ipDir, fmt.Sprintf("%s.txt", region))
		if info, err := os.Stat(filename); err == nil && !info.IsDir() {
			g.availableRegions = append(g.availableRegions, region)
			g.log(fmt.Sprintf("Found IP list for region: %s in %s", region, ipDir))
		}
	}

	g.updateRegionButtons()
}

func (g *OwVpnGui) promptForOverwatchPath() {
	content := container.NewVBox(
		widget.NewLabel("You must locate Overwatch before using this application."),
		widget.NewLabel("Please launch Overwatch, then click 'Detect Overwatch'."),
	)

	detectBtn := widget.NewButton("Detect Overwatch", func() {
		if g.modalRef != nil {
			g.modalRef.(fyne.CanvasObject).Hide()
		}

		g.detectOverwatchPath()
	})

	buttonBox := container.NewCenter(detectBtn)
	finalContent := container.NewVBox(content, buttonBox)

	modal := widget.NewModalPopUp(finalContent, g.window.Canvas())
	modal.Show()

	g.modalRef = modal

	go func() {
		for !g.pathConfigured {
			time.Sleep(1 * time.Second)

			if err := g.sendCommand("get-path"); err == nil {
				time.Sleep(500 * time.Millisecond)
				if g.pathConfigured {
					modal.Hide()
					return
				}
			}
		}

		modal.Hide()
	}()
}

func (g *OwVpnGui) detectOverwatchPath() {
	g.log("Attempting to detect Overwatch path...")

	if path, success := g.findOverwatchProcess(); success {
		g.overwatchPath = path
		g.pathConfigured = true
		g.log(fmt.Sprintf("Detected Overwatch at: %s", path))

		if err := g.sendCommand(fmt.Sprintf("set-path|%s", path)); err != nil {
			g.log(fmt.Sprintf("Error setting Overwatch path: %v", err))
			g.pathConfigured = false
		} else {
			g.enableRegionButtons()
			g.setStatus("Overwatch detected, ready to use", theme.ConfirmIcon())
			g.saveConfig()
		}

		if g.modalRef != nil {
			g.modalRef.(fyne.CanvasObject).Hide()
			g.modalRef = nil
		}
	} else {
		g.log("Could not detect Overwatch. Please make sure Overwatch is running.")

		if g.isInitialized {
			g.showOverwatchNotRunningDialog()
		} else {
			if g.modalRef != nil {
				g.modalRef.(fyne.CanvasObject).Show()
			}
		}
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
	cmd := exec.Command("powershell", "-Command",
		"Get-Process -Name 'Overwatch' -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Path")

	// Hide the console window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

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

func (g *OwVpnGui) isOverwatchProcessRunning() bool {
	cmd := exec.Command("powershell", "-Command",
		"Get-Process -Name 'Overwatch' -ErrorAction SilentlyContinue | Measure-Object | Select-Object -ExpandProperty Count")

	// Hide the console window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.Output()
	if err != nil {
		return false
	}

	count := strings.TrimSpace(string(output))
	return count != "0"
}

func (g *OwVpnGui) checkOverwatchProcessStatus() {
	g.processMutex.Lock()
	wasRunning := g.isOverwatchRunning
	isRunning := g.isOverwatchProcessRunning()
	g.isOverwatchRunning = isRunning
	g.processMutex.Unlock()

	if wasRunning && !isRunning {
		g.log("Detected Overwatch has closed")
		if g.pathConfigured {
			g.setStatus("Ready", theme.ConfirmIcon())
			g.enableRegionButtons()
		}
	} else if !wasRunning && isRunning {
		g.log("Detected Overwatch is now running")
		g.setStatus("Overwatch is running", theme.WarningIcon())
		g.updateButtonStatesForOverwatchRunning()

		if !g.pathConfigured && g.isInitialized {
			g.detectOverwatchPath()
		}
	}
}

func (g *OwVpnGui) startProcessMonitoring() {
	go func() {
		time.Sleep(2 * time.Second)

		for {
			g.checkOverwatchProcessStatus()
			time.Sleep(5 * time.Second)
		}
	}()
}

func (g *OwVpnGui) enableRegionButtons() {
	for region, btn := range g.regionButtons {
		if !g.isOverwatchRunning || g.blocked[region] {
			btn.Enable()
		}
	}
	g.window.Canvas().Refresh(g.window.Content())
}

func (g *OwVpnGui) disableRegionButtons() {
	for _, btn := range g.regionButtons {
		btn.Disable()
	}
	g.window.Canvas().Refresh(g.window.Content())
}

func (g *OwVpnGui) updateButtonStatesForOverwatchRunning() {
	for region, btn := range g.regionButtons {
		if g.isOverwatchRunning && !g.blocked[region] {
			btn.Disable()
		} else {
			btn.Enable()
		}
	}
	g.window.Canvas().Refresh(g.window.Content())
}

func (g *OwVpnGui) updateRegionButtons() {
	regionButtons := container.NewGridWithColumns(3)
	g.regionButtons = make(map[string]*widget.Button)

	if len(g.availableRegions) == 0 {
		noRegionsLabel := widget.NewLabel("No region IP lists available. Please run IP Puller first.")
		regionButtons.Add(noRegionsLabel)
	} else {
		for _, region := range g.availableRegions {
			btn := widget.NewButton(region, nil)
			btn.Importance = widget.SuccessImportance
			btn.SetIcon(theme.ContentRemoveIcon())

			btn.Disable()

			regionName := region

			btn.OnTapped = func() {
				g.toggleRegion(regionName)
			}

			buttonContainer := container.NewPadded(btn)

			g.regionButtons[region] = btn
			regionButtons.Add(buttonContainer)
		}
	}

	titleLabel := canvas.NewText("OVERWATCH VPN", colorTitle)
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

	regionLabel := canvas.NewText("SELECT REGIONS TO BLOCK", colorTitle)
	regionLabel.TextSize = 18
	regionLabel.TextStyle = fyne.TextStyle{Bold: true}
	regionLabel.Alignment = fyne.TextAlignCenter

	unblockAllBtn := widget.NewButton("UNBLOCK ALL REGIONS", func() {
		g.unblockAll()
	})
	unblockAllBtn.Importance = widget.HighImportance
	unblockAllBtnContainer := container.NewPadded(unblockAllBtn)

	sourceToggleNew := widget.NewCheck("Use GitHub Source", nil)
	sourceToggleNew.SetChecked(g.useGithubSource)

	sourceToggleNew.OnChanged = func(value bool) {
		if !g.isChangingSource && g.isInitialized {
			g.isChangingSource = true
			g.useGithubSource = value
			g.saveConfig()
			g.switchIPSource()
			g.isChangingSource = false
		}
	}

	g.sourceToggle = sourceToggleNew
	sourceToggleContainer := container.NewPadded(g.sourceToggle)

	logLabel := canvas.NewText("CONNECTION LOG", colorTitle)
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
		container.NewCenter(sourceToggleContainer),
		widget.NewSeparator(),
		container.NewPadded(logLabel),
		container.NewPadded(scrollLog),
	)

	g.window.SetContent(container.NewPadded(content))
}

func (g *OwVpnGui) switchIPSource() {
	g.log(fmt.Sprintf("Switching to %s source",
		map[bool]string{true: "GitHub", false: "BGPView API"}[g.useGithubSource]))
	g.updateAvailableRegions()
}

func (g *OwVpnGui) initialize() {
	g.log("Initializing application...")

	os.MkdirAll("ips", 0755)
	os.MkdirAll("ips_mina", 0755)

	g.log("Fetching IP addresses from BGPView API source...")
	if err := g.runIpPuller(false); err != nil {
		g.log(fmt.Sprintf("Error fetching IPs from BGPView API: %v", err))
		g.setStatus("Error: IP Puller failed", theme.ErrorIcon())
		dialog.ShowError(fmt.Errorf("failed to run IP Puller: %v", err), g.window)
		return
	}
	g.log("Successfully fetched IPs from BGPView API")

	g.log("Fetching IP addresses from GitHub source...")
	if err := g.runIpPuller(true); err != nil {
		g.log(fmt.Sprintf("Error fetching IPs from GitHub: %v", err))
	} else {
		g.log("Successfully fetched IPs from GitHub")
	}

	g.updateAvailableRegions()

	g.log("Initializing firewall sidecar...")
	if err := g.startFirewallDaemon(); err != nil {
		g.log(fmt.Sprintf("Error starting firewall daemon: %v", err))
		g.setStatus("Error: Firewall daemon failed", theme.ErrorIcon())
		dialog.ShowError(fmt.Errorf("failed to start firewall daemon: %v", err), g.window)
		return
	}
	g.log("Firewall daemon started successfully")

	if err := g.sendCommand("get-path"); err != nil {
		g.log(fmt.Sprintf("Error checking Overwatch path: %v", err))
	}

	time.Sleep(500 * time.Millisecond)

	g.startProcessMonitoring()

	if !g.pathConfigured {
		g.detectOverwatchPath()
	}

	if !g.pathConfigured {
		g.setStatus("Overwatch path not configured", theme.WarningIcon())
		g.promptForOverwatchPath()
	} else {
		g.setStatus("Ready", theme.ConfirmIcon())
		g.enableRegionButtons()
	}

	g.isInitialized = true

	go func() {
		for {
			g.checkStatus()
			time.Sleep(10 * time.Second)
		}
	}()
}

func (g *OwVpnGui) runIpPuller(useGithub bool) error {
	exePath, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), "ip-puller.exe"))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	var cmd *exec.Cmd
	if useGithub {
		cmd = exec.Command(exePath, "-github")
	} else {
		cmd = exec.Command(exePath)
	}

	// Hide the console window
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to execute IP Puller: %v - output: %s", err, string(output))
	}

	outputStr := string(output)
	if len(outputStr) > 500 {
		outputStr = outputStr[:500] + "... [output truncated]"
	}
	g.log(fmt.Sprintf("IP Puller output: %s", outputStr))
	return nil
}

func (g *OwVpnGui) startFirewallDaemon() error {
	exePath, err := filepath.Abs(filepath.Join(filepath.Dir(os.Args[0]), "firewall-sidecar.exe"))
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	g.log("Starting firewall daemon process...")
	g.firewallCmd = exec.Command(exePath, "daemon")

	// Hide the console window
	g.firewallCmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

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
		g.saveConfig()
	}

	if strings.Contains(text, "Overwatch path set to:") {
		g.pathConfigured = true
		g.overwatchPath = strings.TrimPrefix(text, "Overwatch path set to: ")
		g.log(fmt.Sprintf("Overwatch path set to: %s", g.overwatchPath))
		g.enableRegionButtons()
		g.saveConfig()
	}

	if strings.Contains(text, "Successfully blocked") {
		g.setStatus("Ready", theme.ConfirmIcon())
	}

	if strings.Contains(text, "Unblocking IPs") || strings.Contains(text, "Unblocking all IPs") {
		g.setStatus("Unblocking...", theme.InfoIcon())
	}

	if strings.Contains(text, "Successfully unblocked") {
		g.setStatus("Ready", theme.ConfirmIcon())
	}

	if strings.Contains(text, "ERROR:") {
		g.setStatus("Error", theme.ErrorIcon())
	}
}

func (g *OwVpnGui) toggleRegion(region string) {
	if !g.pathConfigured {
		g.log("Overwatch path not configured. Please detect Overwatch path first.")
		g.promptForOverwatchPath()
		return
	}

	isBlocked := g.blocked[region]

	if isBlocked {
		g.log(fmt.Sprintf("Unblocking region %s...", region))
		if err := g.sendCommand(fmt.Sprintf("unblock|%s", region)); err != nil {
			g.log(fmt.Sprintf("Error unblocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = false

		g.regionButtons[region].Importance = widget.SuccessImportance
		g.regionButtons[region].SetText(region)
		g.regionButtons[region].SetIcon(theme.ContentRemoveIcon())

		if g.isOverwatchRunning {
			g.regionButtons[region].Disable()
		}

		g.window.Canvas().Refresh(g.regionButtons[region])
	} else {
		g.processMutex.Lock()
		isRunning := g.isOverwatchRunning
		g.processMutex.Unlock()

		if isRunning {
			g.log("Cannot block region while Overwatch is running. Please close Overwatch first.")
			content := container.NewVBox(
				widget.NewLabel("Overwatch is currently running."),
				widget.NewLabel("Please close Overwatch before blocking regions."),
			)
			dialog.ShowCustom("Overwatch Running", "OK", content, g.window)
			return
		}

		g.log(fmt.Sprintf("Blocking region %s...", region))
		ipDir := g.getIPDirectory()
		if err := g.sendCommand(fmt.Sprintf("block|%s|%s", region, ipDir)); err != nil {
			g.log(fmt.Sprintf("Error blocking region %s: %v", region, err))
			return
		}
		g.blocked[region] = true

		g.regionButtons[region].Importance = widget.DangerImportance
		g.regionButtons[region].SetText(region)
		g.regionButtons[region].SetIcon(theme.ContentAddIcon())

		g.window.Canvas().Refresh(g.regionButtons[region])
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
		g.regionButtons[region].Importance = widget.SuccessImportance
		g.regionButtons[region].SetText(region)
		g.regionButtons[region].SetIcon(theme.ContentRemoveIcon())

		if g.isOverwatchRunning {
			g.regionButtons[region].Disable()
		} else if g.pathConfigured {
			g.regionButtons[region].Enable()
		}
	}
	g.window.Canvas().Refresh(g.window.Content())
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
	g.window.Hide()

	if g.cmdStdin != nil {
		g.log("Sending cleanup command to firewall daemon...")

		if err := g.sendCommand("unblock-all"); err != nil {
			g.log(fmt.Sprintf("Warning: Error sending unblock-all command: %v", err))
		} else {
			g.log("Waiting for cleanup to complete...")
			time.Sleep(1 * time.Second)
		}

		if err := g.sendCommand("exit"); err != nil {
			g.log(fmt.Sprintf("Warning: Error sending exit command: %v", err))
		}

		g.log("Closing connection to firewall daemon...")
		_ = g.cmdStdin.Close()

		g.log("Waiting for firewall daemon to exit...")
		go func() {
			_ = g.firewallCmd.Wait()
		}()

		time.Sleep(500 * time.Millisecond)
	}

	g.log("Cleanup completed, exiting...")
	os.Exit(0)
}

func fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}
