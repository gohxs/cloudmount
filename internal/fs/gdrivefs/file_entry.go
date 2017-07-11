package gdrivefs

import (
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	drive "google.golang.org/api/drive/v3"
)

//FileEntry entry to handle files
type FileEntry struct {
	//parent *FileEntry
	container *FileContainer
	//fs    *GDriveFS
	GID   string      // google driveID
	GFile *drive.File // GDrive file
	Name  string      // local name
	// fuseops
	Inode fuseops.InodeID
	Attr  fuseops.InodeAttributes // Cached attributes

	// cache file
	tempFile *os.File // Cached file
	// childs
	//children []*FileEntry // children
}

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

/*func (fe *FileEntry) AddChild(child *FileEntry) {
	//child.parent = fe // is this needed at all?
	// Solve name here?

	fe.children = append(fe.children, child)
}*/

/*func (fe *FileEntry) RemoveChild(child *FileEntry) {
	toremove := -1
	for i, v := range fe.children {
		if v == child {
			toremove = i
			break
		}
	}
	if toremove == -1 {
		return
	}
	fe.children = append(fe.children[:toremove], fe.children[toremove+1:]...)
}*/

// useful for debug to count children
/*func (fe *FileEntry) Count() int {
	count := 0

	for _, c := range fe.children {
		count += c.Count()
	}
	return count + len(fe.children)
}*/

// SetGFile update attributes and set drive.File
func (fe *FileEntry) SetGFile(gfile *drive.File) {
	if gfile == nil {
		fe.GFile = nil
	} else {
		fe.GFile = gfile
	}

	// Create Attribute
	attr := fuseops.InodeAttributes{}
	attr.Nlink = 1
	attr.Uid = fe.container.uid
	attr.Gid = fe.container.gid

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
	up := fe.container.fs.client.Files.Update(fe.GFile.Id, ngFile)
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
	switch fe.GFile.MimeType { // Make this somewhat optional
	case "application/vnd.google-apps.document":
		log.Println("Exporting as: text/markdown")
		res, err = fe.container.fs.client.Files.Export(fe.GFile.Id, "text/plain").Download()
	case "application/vnd.google-apps.spreadsheet":
		log.Println("Exporting as: text/csv")
		res, err = fe.container.fs.client.Files.Export(fe.GFile.Id, "text/csv").Download()
	default:
		res, err = fe.container.fs.client.Files.Get(fe.GFile.Id).Download()
	}

	if err != nil {
		log.Println("MimeType:", fe.GFile.MimeType)
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

func (fe *FileEntry) IsDir() bool {
	return fe.Attr.Mode&os.ModeDir == os.ModeDir
}
