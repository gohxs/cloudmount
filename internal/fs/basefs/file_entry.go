package basefs

import (
	"os"

	"github.com/jacobsa/fuse/fuseops"
)

//FileEntry entry to handle files
type FileEntry struct {
	//Inode fuseops.InodeID
	GID      string // google driveID
	File     File
	Name     string                  // local name
	Attr     fuseops.InodeAttributes // Cached attributes
	tempFile *os.File                // Cached file
}

// Why?
func (fe *FileEntry) HasParent(parent *FileEntry) bool {

	// Exceptional case
	/*if fe.Inode == fuseops.RootInodeID {
		return false
	}*/
	if parent.GID == "" && fe.File == nil && len(fe.File.Parents()) == 0 { // We are looking in root
		return true
	}

	if fe.File == nil { // Case gid is not empty and GFile is nil
		return false
	}
	for _, pgid := range fe.File.Parents() {
		if pgid == parent.GID {
			return true
		}
	}
	return false
}
func (fe *FileEntry) HasParentGID(parentGID string) bool {

	// Exceptional case
	/*if fe.Inode == fuseops.RootInodeID {
		return false
	}*/
	if parentGID == "" && fe.File == nil && len(fe.File.Parents()) == 0 { // We are looking in root
		return true
	}
	if fe.File == nil { // Case gid is not empty and GFile is null
		return false
	}
	for _, pgid := range fe.File.Parents() {
		if pgid == parentGID {
			return true
		}
	}
	return false
}

// SetGFile update attributes and set drive.File
func (fe *FileEntry) SetFile(file File, uid, gid uint32) { // Should remove from here maybe?
	fe.File = file

	// GetAttribute from GFile somehow
	// Create Attribute
	fe.Attr = file.Attr()
	//fe.Attr.Uid = fe.container.uid
	//fe.Attr.Gid = fe.container.gid
}

// Sync cached , upload to gdrive
// IsDir returns true if entry is a directory:w
func (fe *FileEntry) IsDir() bool {
	return fe.Attr.Mode&os.ModeDir == os.ModeDir
}
