package core

import "github.com/jacobsa/fuse/fuseutil"

// DriverFS default interface for fs driver
type DriverFS interface {
	fuseutil.FileSystem
	//Init()
	Start()
	//Refresh()
}

//DriverFactory function type for a FS factory
type DriverFactory func(*Core) DriverFS
