package basefs

import (
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

type Handle struct {
	ID           fuseops.HandleID
	entry        *FileEntry
	uploadOnDone bool
	// Handling for dir
	entries []fuseutil.Dirent
}
