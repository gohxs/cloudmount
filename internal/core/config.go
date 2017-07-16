package core

import "time"

// Config struct
type Config struct {
	Daemonize   bool
	Type        string
	VerboseLog  bool
	RefreshTime time.Duration
	HomeDir     string
	UID         uint32 // Mount UID
	GID         uint32 // Mount GID
	Target      string // should be a folder
	Source      string
	Safemode    bool

	// Driver specific params:
	Param map[string]interface{}
}
