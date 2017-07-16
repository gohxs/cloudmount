package dropboxfs

import (
	"io/ioutil"
	glog "log"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	log = glog.New(ioutil.Discard, "", 0)
)

// New Create basefs with Dropbox service
func New(core *core.Core) core.DriverFS {
	if core.Config.VerboseLog {
		log = prettylog.New("dropboxfs")
	}
	fs := basefs.New(core)
	fs.Service = NewService(&core.Config) // DropBoxService

	return fs
}
