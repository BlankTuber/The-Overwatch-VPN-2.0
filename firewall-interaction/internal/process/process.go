package process

import (
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"quidque.no/ow-firewall-sidecar/internal/config"
)

// IsOverwatchRunning checks if Overwatch is currently running
func IsOverwatchRunning() (bool, error) {
	processes, err := process.Processes()
	if err != nil {
		return false, fmt.Errorf("failed to get process list: %w", err)
	}

	for _, p := range processes {
		name, err := p.Name()
		if err != nil {
			continue // Skip processes we can't get names for
		}

		if name == config.OverwatchProcessName {
			return true, nil
		}
	}

	return false, nil
}

// WaitForOverwatchToClose waits until Overwatch is no longer running
// with a timeout in seconds (0 for no timeout)
func WaitForOverwatchToClose(timeoutSeconds int) error {
	start := time.Now()
	
	for {
		running, err := IsOverwatchRunning()
		if err != nil {
			return err
		}
		
		if !running {
			return nil
		}
		
		// Check if we've exceeded the timeout
		if timeoutSeconds > 0 && time.Since(start).Seconds() > float64(timeoutSeconds) {
			return fmt.Errorf("timeout waiting for Overwatch to close")
		}
		
		// Wait a bit before checking again
		time.Sleep(500 * time.Millisecond)
	}
}