package gdrivefs

import (
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	log = prettylog.New("gdrivefs")
)

type GDriveFS struct {
	*basefs.BaseFS
	serviceConfig *Config
	nextRefresh   time.Time
	//client        *drive.Service
}

func New(core *core.Core) core.DriverFS {

	fs := &GDriveFS{
		BaseFS:        basefs.New(core),
		serviceConfig: &Config{},
		nextRefresh:   time.Now(),
	}
	client := fs.initClient() // Init Oauth2 client

	//fs.BaseFS.Client = client // This will be removed
	fs.BaseFS.Service = &Service{client: client}

	return fs
}

// Start will loop to update File entries
func (fs *GDriveFS) Start() {
	go func() {
		fs.Refresh() // First load

		for {
			fs.CheckForChanges() // Loop
			time.Sleep(fs.Config.RefreshTime)
		}
		// Change reader loop
	}()
}

func (fs *GDriveFS) CheckForChanges() {

	Service := fs.Service.(*Service) // Our Service

	changes, err := Service.Changes()
	if err != nil {
		return
	}
	for _, c := range changes {
		entry := fs.Root.FindByID(c.FileId)
		if c.Removed {
			if entry == nil {
				continue
			} else {
				fs.Root.RemoveEntry(entry)
			}
			continue
		}
		if entry != nil {
			entry.SetFile(File(c.File), fs.Config.UID, fs.Config.GID)
		} else {
			//Create new one
			fs.Root.FileEntry(File(c.File)) // Creating new one
		}
	}
}
