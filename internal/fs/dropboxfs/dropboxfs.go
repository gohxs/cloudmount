package dropboxfs

import (
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	log = prettylog.New("dropboxfs")
)

type DropboxFS struct {
	*basefs.BaseFS
	serviceConfig *Config
	nextRefresh   time.Time
}

func New(core *core.Core) core.DriverFS {

	fs := &DropboxFS{basefs.New(core), &Config{}, time.Now()}
	client := fs.initClient() // Init Oauth2 client
	//client.Verbose = true

	// Set necesary service
	fs.BaseFS.Service = &Service{client}

	return fs
}

func (fs *DropboxFS) Start() {
	Service := fs.Service.(*Service)
	// Fill root container and do changes
	go func() {
		fs.Refresh()
		for {
			Service.Changes()
			time.Sleep(fs.Config.RefreshTime)
		}
	}()
}
