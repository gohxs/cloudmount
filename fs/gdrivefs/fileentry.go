package gdrivefs

import (
	"fmt"
	"io"
	"io/ioutil"
	_ "log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	drive "google.golang.org/api/drive/v3"
)

//FileEntry entry to handle files
type FileEntry struct {
	//parent *FileEntry
	fs    *GDriveFS
	GFile *drive.File // GDrive file
	isDir bool        // Is dir
	Name  string      // local name
	// fuseops
	Inode fuseops.InodeID
	Attr  fuseops.InodeAttributes // Cached attributes

	// cache file
	tempFile *os.File // Cached file
	// childs
	children []*FileEntry // children
}

func (fe *FileEntry) AddChild(child *FileEntry) {
	//child.parent = fe // is this needed at all?
	// Solve name here?

	fe.children = append(fe.children, child)
}

func (fe *FileEntry) RemoveChild(child *FileEntry) {
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
}

// useful for debug to count children
func (fe *FileEntry) Count() int {
	count := 0

	for _, c := range fe.children {
		count += c.Count()
	}
	return count + len(fe.children)
}

// SetGFile update attributes and set drive.File
func (fe *FileEntry) SetGFile(f *drive.File) {
	// Create Attribute
	attr := fuseops.InodeAttributes{}
	attr.Nlink = 1
	attr.Size = uint64(f.Size)
	//attr.Size = uint64(f.QuotaBytesUsed)
	// Temp
	attr.Uid = fe.fs.core.Config.UID
	attr.Gid = fe.fs.core.Config.GID
	attr.Crtime, _ = time.Parse(time.RFC3339, f.CreatedTime)
	attr.Ctime = attr.Crtime // Set CTime to created, although it is change inode metadata
	attr.Mtime, _ = time.Parse(time.RFC3339, f.ModifiedTime)
	attr.Atime = attr.Mtime // Set access time to modified, not sure if gdrive has access time

	attr.Mode = os.FileMode(0644) // default

	if f.MimeType == "application/vnd.google-apps.folder" {
		attr.Mode = os.FileMode(0755) | os.ModeDir
	}

	fe.GFile = f
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
	up := fe.fs.client.Files.Update(fe.GFile.Id, ngFile)
	upFile, err := up.Media(fe.tempFile).Do()

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
		res, err = fe.fs.client.Files.Export(fe.GFile.Id, "text/plain").Download()
	case "application/vnd.google-apps.spreadsheet":
		log.Println("Exporting as: text/csv")
		res, err = fe.fs.client.Files.Export(fe.GFile.Id, "text/csv").Download()
	default:
		res, err = fe.fs.client.Files.Get(fe.GFile.Id).Download()
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

// Find the right parent?
// WRONG
func (fe *FileEntry) solveAppendGFile(f *drive.File, inode fuseops.InodeID) *FileEntry {

	fil := fe.FindByGID(f.Id, true)
	if fil != nil { // ignore existing ID
		return fil
	}

	if len(f.Parents) == 0 {
		return fe.AppendGFile(f, inode) // = append(fs.root.fileList, entry)
	}
	for _, parent := range f.Parents { // hierarchy add
		parentEntry := fe.FindByGID(parent, true)
		if parentEntry == nil {
			log.Fatalln("Non existent parent", parent)
		}
		// Here
		return parentEntry.AppendGFile(f, inode)
	}
	return nil
}

// Load append whatever?
// append file to this tree
func (fe *FileEntry) AppendGFile(f *drive.File, inode fuseops.InodeID) *FileEntry {

	name := f.Name
	count := 1
	nameParts := strings.Split(f.Name, ".")
	for {
		en := fe.FindByName(name, false) // locally only
		if en == nil {                   // ok we want no value
			break
		}
		count++
		if len(nameParts) > 1 {
			name = fmt.Sprintf("%s(%d).%s", nameParts[0], count, strings.Join(nameParts[1:], "."))
		} else {
			name = fmt.Sprintf("%s(%d)", nameParts[0], count)
		}
	}

	// Create an entry

	//log.Println("Creating new file entry for name:", name, "for GFile:", f.Name)
	// lock from find inode to fileList append
	entry := fe.fs.NewFileEntry()
	entry.Name = name
	entry.SetGFile(f)
	entry.Inode = inode

	fe.AddChild(entry)

	//fe.fileList = append(fe.fileList, entry)
	//fe.fileMap[f.Name] = entry

	return entry
}

//FindByInode find by Inode or return self
func (fe *FileEntry) FindByInode(inode fuseops.InodeID, recurse bool) *FileEntry {
	if inode == fe.Inode {
		return fe // return self
	}
	// Recurse??
	for _, e := range fe.children {
		if e.Inode == inode {
			return e
		}
		if recurse {
			re := e.FindByInode(inode, recurse)
			if re != nil {
				return re
			}
		}
	}
	// For each child we findByInode
	return nil
}

// FindByName return a child entry by name
func (fe *FileEntry) FindByName(name string, recurse bool) *FileEntry {
	// Recurse??
	for _, e := range fe.children {
		if e.Name == name {
			return e
		}
		if recurse {
			re := e.FindByName(name, recurse)
			if re != nil {
				return re
			}
		}
	}
	// For each child we findByInode
	return nil
}

// FindByGID find by google drive ID
func (fe *FileEntry) FindByGID(gdriveID string, recurse bool) *FileEntry {
	// Recurse??
	for _, e := range fe.children {
		if e.GFile.Id == gdriveID {
			return e
		}
		if recurse {
			re := e.FindByGID(gdriveID, recurse)
			if re != nil {
				return re
			}
		}
	}
	// For each child we findByInode
	return nil
}

func (fe *FileEntry) FindUnusedInode() fuseops.InodeID {
	var inode fuseops.InodeID
	for inode = 2; inode < 99999; inode++ {
		f := fe.FindByInode(inode, true)
		if f == nil {
			return inode
		}
	}
	log.Println("0 Inode ODD")
	return 0
}
