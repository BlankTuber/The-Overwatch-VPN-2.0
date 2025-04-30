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

var (
	colorBlocked   = color.NRGBA{R: 217, G: 83, B: 79, A: 255}  // Red
	colorUnblocked = color.NRGBA{R: 0, G: 177, B: 87, A: 255}   // Green
	colorTitle     = color.NRGBA{R: 66, G: 139, B: 202, A: 255} // Blue
)

type OwVpnGui struct {
	window           fyne.Window
	logText          *widget.Label
	statusLabel      *widget.Label
	statusIcon       *canvas.Image
	progressBar      *widget.ProgressBarInfinite
	regionButtons    map[string]*widget.Button
	firewallCmd      *exec.Cmd
	cmdStdin         io.WriteCloser
	blocked          map[string]bool
	blockingInProgress bool
	blockingMutex    sync.Mutex
	availableRegions []string
	overwatchPath    string
	pathConfigured   bool
}

func checkAdminPermissions() bool {
	cmd := exec.Command("net", "session")
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
		window:           w,
		logText:          widget.NewLabel("Starting application..."),
		statusLabel:      widget.NewLabel("Initializing..."),
		statusIcon:       canvas.NewImageFromResource(theme.InfoIcon()),
		progressBar:      widget.NewProgressBarInfinite(),
		regionButtons:    make(map[string]*widget.Button),
		blocked:          make(map[string]bool),
		blockingInProgress: false,
		availableRegions: []string{},
		pathConfigured:   false,
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

	if _, err := os.Stat(ipDir); os.IsNotExist(err) {
		g.log("IP directory not found, will be created after IP Puller runs")
		return
	}

	g.availableRegions = []string{}

	for _, region := range regions {
		filename := filepath.Join(ipDir, fmt.Sprintf("%s.txt", region))
		if info, err := os.Stat(filename); err == nil && !info.IsDir() {
			g.availableRegions = append(g.availableRegions, region)
			g.log(fmt.Sprintf("Found IP list for region: %s", region))
		}
	}

	g.updateRegionButtons()
}

func (g *OwVpnGui) promptForOverwatchPath() {
	content := container.NewVBox(
		widget.NewLabel("Overwatch path not found. Please start Overwatch so we can detect it,"),
		widget.NewLabel("or click 'Skip' to use default paths."),
	)

	detectBtn := widget.NewButton("Detect Overwatch", func() {
		g.detectOverwatchPath()
	})
	
	skipBtn := widget.NewButton("Skip", func() {
		g.log("Using default Overwatch paths")
		g.pathConfigured = true
	})

	buttonBox := container.NewHBox(detectBtn, skipBtn)
	finalContent := container.NewVBox(content, buttonBox)

	dialog := dialog.NewCustom("Overwatch Path Setup", "", finalContent, g.window)
	dialog.Show()
}

func (g *OwVpnGui) detectOverwatchPath() {
	g.log("Attempting to detect Overwatch path...")
	if path, success := g.findOverwatchProcess(); success {
		g.overwatchPath = path
		g.pathConfigured = true
		g.log(fmt.Sprintf("Detected Overwatch at: %s", path))
		
		if err := g.sendCommand(fmt.Sprintf("set-path|%s", path)); err != nil {
			g.log(fmt.Sprintf("Error setting Overwatch path: %v", err))
		}
	} else {
		g.log("Could not detect Overwatch. Please make sure Overwatch is running.")
	}
}

func (g *OwVpnGui) findOverwatchProcess() (string, bool) {
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

			regionName := region

			btn.OnTapped = func() {
				g.toggleRegion(regionName)
			}

			buttonContainer := container.NewPadded(btn)

			g.regionButtons[region] = btn
			regionButtons.Add(buttonContainer)
		}
	}

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

	detectOverwatchBtn := widget.NewButton("Detect Overwatch Path", func() {
		g.detectOverwatchPath()
	})
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

	g.updateAvailableRegions()

	g.log("Starting firewall daemon...")
	if err := g.startFirewallDaemon(); err != nil {
		g.log(fmt.Sprintf("Error starting firewall daemon: %v", err))
		g.setStatus("Error: Firewall daemon failed", theme.ErrorIcon())
		dialog.ShowError(fmt.Errorf("failed to start firewall daemon: %v", err), g.window)
		return
	}
	g.log("Firewall daemon started successfully")

	g.promptForOverwatchPath()

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
	if strings.Contains(text, "ERROR:") || strings.Contains(text, "Successfully") {
		g.log(text)
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
		g.setStatus("Ready", theme.ConfirmIcon())
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
		for region, btn := range g.regionButtons {
			if !g.blocked[region] {
				btn.Disable()
			}
		}
	} else {
		g.progressBar.Hide()
		for _, btn := range g.regionButtons {
			btn.Enable()
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

		g.window.Canvas().Refresh(g.regionButtons[region])
	} else {
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
		g.regionButtons[region].Enable()
	}
	g.window.Canvas().Refresh(g.window.Content())
	
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

	g.window.Hide()

	if g.cmdStdin != nil {
		g.log("Sending cleanup command to firewall daemon...")
		_ = g.sendCommand("unblock-all")

		_ = g.cmdStdin.Close()
	}
	g.log("Cleanup initiated, exiting...")
	os.Exit(0)
}