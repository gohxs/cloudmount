package megafs

import (
	"github.com/gohxs/cloudmount/internal/core"
	"github.com/gohxs/cloudmount/internal/fs/basefs"
	"github.com/gohxs/prettylog"
)

var (
	pname  = "mega"
	log    = prettylog.Dummy()
	errlog = prettylog.New(pname + "-err")
)

// New new Filesystem implementation based on gdrive Service
func New(core *core.Core) core.DriverFS {

	if core.Config.VerboseLog {
		log = prettylog.New(pname)
	}

	fs := basefs.New(core)
	fs.Service = NewService(&core.Config, fs)

	return fs
}
