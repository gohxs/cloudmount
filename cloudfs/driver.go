package cloudfs

import "github.com/jacobsa/fuse/fuseutil"

// Base Driver
type Driver interface {
	fuseutil.FileSystem
	//Init()
	Start()
	Refresh()
}

type DriverFactory func(*Core) Driver
