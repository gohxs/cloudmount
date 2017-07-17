package dropboxfs

import (
	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	pname  = "dropboxfs"
	log    = prettylog.Dummy()
	errlog = prettylog.New(pname + "-err")
)

// New Create basefs with Dropbox service
func New(core *core.Core) core.DriverFS {
	if core.Config.VerboseLog {
		log = prettylog.New(pname)
	}
	fs := basefs.New(core)
	fs.Service = NewService(&core.Config) // DropBoxService

	return fs
}
