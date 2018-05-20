package basefs

import (
	"io"
	"io/ioutil"
	"os"
	"sync"

	"github.com/jacobsa/fuse/fuseops"
)

//FileEntry entry to handle files
type FileEntry struct {
	sync.Mutex
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

//ClearCache remove local file
// XXX: move this to FileEntry
func (fe *FileEntry) ClearCache() (err error) {
	fe.Lock()
	defer fe.Unlock()
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.RealClose()
	os.Remove(fe.tempFile.Name())
	fe.tempFile = nil
	return
}

// Truncate truncates localFile to 0 bytes
func (fe *FileEntry) Truncate() (err error) {
	fe.Lock()
	defer fe.Unlock()
	// Delete and create another on truncate 0
	localFile, err := ioutil.TempFile(os.TempDir(), "gdfs") // TODO: const this elsewhere
	if err != nil {
		return err
	}
	fe.tempFile = &FileWrapper{localFile}

	return
}

//Sync will flush, upload file and update local entry
func (fe *FileEntry) Sync(fc *FileContainer) (err error) {
	fe.Lock()
	defer fe.Unlock()

	if fe.tempFile == nil {
		return
	}
	fe.tempFile.Sync()
	fe.tempFile.Seek(0, io.SeekStart) // Depends??, for reading?

	upFile, err := fc.fs.Service.Upload(fe.tempFile, fe.File)
	if err != nil {
		return err
	}
	fe.SetFile(upFile, fc.uid, fc.gid) // update local GFile entry
	return

}

//Cache download cloud file to a temporary local file or return already created file
func (fe *FileEntry) Cache(fc *FileContainer) *FileWrapper {
	fe.Lock()
	defer fe.Unlock()

	if fe.tempFile != nil {
		return fe.tempFile
	}
	var err error

	// Local copy
	localFile, err := ioutil.TempFile(os.TempDir(), "gdfs") // TODO: const this elsewhere
	if err != nil {
		return nil
	}
	fe.tempFile = &FileWrapper{localFile}

	err = fc.fs.Service.DownloadTo(fe.tempFile, fe.File)
	// ignore download since can be a bogus file, for certain file systems
	//if err != nil { // Ignore this error
	//    return nil
	//}

	// tempFile could change to null in the meantime (download might take long?)
	fe.tempFile.Seek(0, io.SeekStart)

	return fe.tempFile

}
