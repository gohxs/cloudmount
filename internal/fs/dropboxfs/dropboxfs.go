package dropboxfs

import (
	"github.com/gohxs/cloudmount/internal/core"
	"github.com/gohxs/cloudmount/internal/fs/basefs"
	"github.com/gohxs/prettylog"
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
