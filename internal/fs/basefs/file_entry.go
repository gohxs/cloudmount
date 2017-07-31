package basefs

import (
	"os"

	"github.com/jacobsa/fuse/fuseops"
)

//FileEntry entry to handle files
type FileEntry struct {
	Inode    fuseops.InodeID         // Inode
	File     *File                   // Remote file information
	Name     string                  // local name
	Attr     fuseops.InodeAttributes // Cached attributes
	tempFile *FileWrapper            // Cached file
}

// SetFile update attributes and set drive.File
func (fe *FileEntry) SetFile(file *File, uid, gid uint32) { // Should remove from here maybe?
	fe.File = file
	fe.Attr = fuseops.InodeAttributes{
		Size:   fe.File.Size,
		Crtime: file.CreatedTime,
		Ctime:  file.CreatedTime,
		Mtime:  file.ModifiedTime,
		Atime:  file.AccessedTime,
		Mode:   file.Mode,
		Uid:    uid,
		Gid:    gid,
	}
}

// IsDir returns true if entry is a directory:w
func (fe *FileEntry) IsDir() bool {
	return fe.Attr.Mode&os.ModeDir == os.ModeDir
}

// HasParentID check parent by cloud ID
func (fe *FileEntry) HasParentID(parentID string) bool {
	// Exceptional case
	if fe.Inode == fuseops.RootInodeID {
		return false
	}
	if parentID == "" {
		if fe.File == nil || len(fe.File.Parents) == 0 { // We are looking in root
			return true
		}
		return false
	}
	if fe.File == nil { // Case gid is not empty and GFile is null
		return false
	}
	for _, pgid := range fe.File.Parents {
		if pgid == parentID {
			return true
		}
	}
	return false
}

// HasParent check Parent by entry
func (fe *FileEntry) HasParent(parent *FileEntry) bool {
	// Exceptional case
	if fe.Inode == fuseops.RootInodeID {
		return false
	}
	if parent.File == nil {
		return fe.HasParentID("")
	}
	return fe.HasParentID(parent.File.ID)
}
