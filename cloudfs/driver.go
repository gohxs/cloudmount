package core

import "github.com/jacobsa/fuse/fuseutil"

// Base Driver
type Driver interface {
	fuseutil.FileSystem
	Init()
	Refresh()
}

type DriverFactory func() Driver
