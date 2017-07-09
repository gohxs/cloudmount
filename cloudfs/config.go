package cloudfs

// Config struct
type Config struct {
	Daemonize     bool
	CloudFSDriver string
	VerboseLog    bool

	HomeDir string
	UID     uint32 // Mount UID
	GID     uint32 // Mount GID
}
