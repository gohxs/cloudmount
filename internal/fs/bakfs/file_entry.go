package basefs

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	drive "google.golang.org/api/drive/v3"
)

//FileEntry entry to handle files
type FileEntry struct {
	//parent *FileEntry
	fs    *BaseFS
	GID   string      // google driveID
	GFile *drive.File // GDrive file // Interface maybe?
	Name  string      // local name
	// fuseops
	Inode fuseops.InodeID
	Attr  fuseops.InodeAttributes // Cached attributes

	// cache file
	tempFile *os.File // Cached file
	// childs
	//children []*FileEntry // children
}

// Why?
func (fe *FileEntry) HasParentGID(gid string) bool {

	// Exceptional case
	if fe.Inode == fuseops.RootInodeID {
		return false
	}
	if gid == "" { // We are looking in root
		if fe.GFile == nil {
			return true
		}
		if len(fe.GFile.Parents) == 0 {
			return true
		}
	}

	if fe.GFile == nil { // Case gid is not empty and GFile is null
		return false
	}
	for _, pgid := range fe.GFile.Parents {
		if pgid == gid {
			return true
		}
	}
	return false
}

// SetGFile update attributes and set drive.File
func (fe *FileEntry) SetGFile(gfile *drive.File) { // Should remove from here maybe?
	if gfile == nil {
		fe.GFile = nil
	} else {
		fe.GFile = gfile
	}

	// GetAttribute from GFile somehow
	// Create Attribute
	attr := fuseops.InodeAttributes{}
	attr.Nlink = 1
	attr.Uid = fe.fs.Config.UID
	attr.Gid = fe.fs.Config.GID

	attr.Mode = os.FileMode(0644) // default
	if gfile != nil {
		attr.Size = uint64(gfile.Size)
		attr.Crtime, _ = time.Parse(time.RFC3339, gfile.CreatedTime)
		attr.Ctime = attr.Crtime // Set CTime to created, although it is change inode metadata
		attr.Mtime, _ = time.Parse(time.RFC3339, gfile.ModifiedTime)
		attr.Atime = attr.Mtime // Set access time to modified, not sure if gdrive has access time
		if gfile.MimeType == "application/vnd.google-apps.folder" {
			attr.Mode = os.FileMode(0755) | os.ModeDir
		}
		fe.GID = gfile.Id
	}
	fe.Attr = attr
}

// Sync cached , upload to gdrive

func (fe *FileEntry) Sync() (err error) {
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.Sync()
	fe.tempFile.Seek(0, io.SeekStart)

	ngFile := &drive.File{}
	up := fe.fs.Client.Files.Update(fe.GFile.Id, ngFile)
	upFile, err := up.Media(fe.tempFile).Fields(fileFields).Do()

	fe.SetGFile(upFile) // update local GFile entry
	return

}

//ClearCache remove local file
func (fe *FileEntry) ClearCache() (err error) {
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.Close()
	os.Remove(fe.tempFile.Name())
	fe.tempFile = nil
	return
}

// Cache download GDrive file to a temporary local file or return already created file
func (fe *FileEntry) Cache() *os.File {
	if fe.tempFile != nil {
		return fe.tempFile
	}
	var res *http.Response
	var err error
	// Export GDocs (Special google doc documents needs to be exported make a config somewhere for this)
	switch fe.GFile.MimeType { // Make this somewhat optional special case
	case "application/vnd.google-apps.document":
		log.Println("Exporting as: text/markdown")
		res, err = fe.fs.Client.Files.Export(fe.GFile.Id, "text/plain").Download()
	case "application/vnd.google-apps.spreadsheet":
		log.Println("Exporting as: text/csv")
		res, err = fe.fs.Client.Files.Export(fe.GFile.Id, "text/csv").Download()
	default:
		res, err = fe.fs.Client.Files.Get(fe.GFile.Id).Download()
	}

	if err != nil {
		log.Println("Error from GDrive API", err)
		return nil
	}
	defer res.Body.Close()

	// Local copy
	fe.tempFile, err = ioutil.TempFile(os.TempDir(), "gdfs") // TODO: const this elsewhere
	if err != nil {
		log.Println("Error creating temp file")
		return nil
	}
	io.Copy(fe.tempFile, res.Body)

	fe.tempFile.Seek(0, io.SeekStart)
	return fe.tempFile

}

func (fe *FileEntry) Truncate() (err error) { // DriverTruncate
	// Delete and create another on truncate 0
	err = fe.fs.Client.Files.Delete(fe.GFile.Id).Do() // XXX: Careful on this
	createdFile, err := fe.fs.Client.Files.Create(&drive.File{Parents: fe.GFile.Parents, Name: fe.GFile.Name}).Fields(fileFields).Do()
	if err != nil {
		return fuse.EINVAL // ??
	}
	fe.SetGFile(createdFile) // Set new file

	return
}

// IsDir returns true if entry is a directory:w
func (fe *FileEntry) IsDir() bool {
	return fe.Attr.Mode&os.ModeDir == os.ModeDir
}
