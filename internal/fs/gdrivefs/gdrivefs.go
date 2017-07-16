package gdrivefs

import (
	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	log = prettylog.New("gdrivefs")
)

// New new Filesystem implementation based on gdrive Service
func New(core *core.Core) core.DriverFS {

	fs := basefs.New(core)
	fs.Service = NewService(&core.Config)

	return fs
}
