package core

import "time"

// Config struct
type Config struct {
	Daemonize     bool
	CloudFSDriver string
	VerboseLog    bool
	RefreshTime   time.Duration
	HomeDir       string
	UID           uint32 // Mount UID
	GID           uint32 // Mount GID
}
