package core

import "github.com/jacobsa/fuse/fuseutil"

// Base Driver
type Driver interface {
	fuseutil.FileSystem
	Refresh()
}

type DriverFactory func() Driver
