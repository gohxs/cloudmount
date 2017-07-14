package core

import "github.com/jacobsa/fuse/fuseutil"

// Base Driver
type DriverFS interface {
	fuseutil.FileSystem
	//Init()
	Start()
	//Refresh()
}

type DriverFactory func(*Core) DriverFS
