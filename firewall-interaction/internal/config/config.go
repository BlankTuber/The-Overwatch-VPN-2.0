package config

const (
	OverwatchProcessName   = "Overwatch.exe"
	FirewallRulePrefix     = "OW-VPN-"
	DefaultIPListDir       = "ips"
	DefaultGitHubIPListDir = "ips_mina"
	IPCSocketName          = "ow-firewall-sidecar"
	ExitSuccess            = 0
	ExitErrorAdminRights   = 1
	ExitErrorIPListRead    = 2
	ExitErrorFirewall      = 3
	ExitErrorProcessCheck  = 4
	ExitErrorInvalidArgs   = 5
)

const (
	ActionBlock      = "block"
	ActionUnblock    = "unblock"
	ActionUnblockAll = "unblock-all"
	ActionStatus     = "status"
	ActionSetPath    = "set-path"
	ActionGetPath    = "get-path"
)

const (
	DirectionInbound  = "in"
	DirectionOutbound = "out"
)
