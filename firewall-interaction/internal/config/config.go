package config

// Constants related to Overwatch application
const (
	// Overwatch process name
	OverwatchProcessName = "Overwatch.exe"
	
	// Rule name prefix for firewall rules
	FirewallRulePrefix = "OW-VPN-"
	
	// Default directory where IP list files are stored
	DefaultIPListDir = "ips"
	
	// IPC socket name for communication with Tauri app
	IPCSocketName = "ow-firewall-sidecar"
	
	// Exit codes
	ExitSuccess           = 0
	ExitErrorAdminRights  = 1
	ExitErrorIPListRead   = 2
	ExitErrorFirewall     = 3
	ExitErrorProcessCheck = 4
	ExitErrorInvalidArgs  = 5
)

// Command line actions
const (
	ActionBlock      = "block"
	ActionUnblock    = "unblock"
	ActionUnblockAll = "unblock-all"
	ActionStatus     = "status"
	ActionSetPath    = "set-path"
)

// Firewall rule direction
const (
	DirectionInbound  = "in"
	DirectionOutbound = "out"
)