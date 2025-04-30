package process

// This file is simplified since the GUI now handles Overwatch process detection
// The package is kept for API compatibility

// IsOverwatchRunning always returns false since the GUI handles process detection
func IsOverwatchRunning() (bool, error) {
	// GUI handles this functionality now
	return false, nil
}
