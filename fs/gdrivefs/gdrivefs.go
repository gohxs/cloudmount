package gdrivefs

import "dev.hexasoftware.com/hxs/cloudmount/cloudfs"

// Driver for gdrive
type GDriveDriver struct {
	core *core.Core
}

func (d *GDriveDriver) Init(core *core.Core) {
	d.core = core

}

func (d *GDriveDriver) Start() {

}
