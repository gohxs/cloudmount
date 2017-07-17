package core

import (
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/internal/coreutil"
)

// Config struct
type Config struct {
	Daemonize   bool
	Type        string
	VerboseLog  bool
	Verbose2Log bool
	RefreshTime time.Duration
	HomeDir     string
	Target      string // should be a folder
	Source      string

	//Options map[string]string
	Options Options
}

// Options are specified in cloudmount -o option1=1, option2=2
type Options struct { // are Options for specific driver?
	// Sub options
	UID      uint32 `opt:"uid"`
	GID      uint32 `opt:"gid"` // Mount GID
	Readonly bool   `opt:"ro"`
}

func (o Options) String() string {
	return coreutil.OptionString(o)
}
func (o Options) ToMap() map[string]string {
	// Convert to map
	return map[string]string{}
}
