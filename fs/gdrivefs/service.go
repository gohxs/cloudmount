package gdrivefs

import "github.com/jacobsa/fuse/fuseutil"

// Driver for gdrive
type GDriveDriver interface {
	Fuse() fuseutil.FileSystem // Fetch the file system
}

type gdriveService struct {
}
