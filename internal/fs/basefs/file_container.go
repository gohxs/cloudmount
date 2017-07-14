package basefs

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"sync"

	drive "google.golang.org/api/drive/v3"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
)

type FileContainer struct {
	fileEntries map[fuseops.InodeID]*FileEntry
	///	tree        *FileEntry
	fs *BaseFS
	//client *drive.Service // Wrong should be common
	uid uint32
	gid uint32

	inodeMU *sync.Mutex
}

func NewFileContainer(fs *BaseFS) *FileContainer {
	fc := &FileContainer{
		fileEntries: map[fuseops.InodeID]*FileEntry{},
		fs:          fs,
		//client:  fs.Client,
		inodeMU: &sync.Mutex{},
		uid:     fs.Config.UID,
		gid:     fs.Config.GID,
	}
	_, rootEntry := fc.FileEntry(nil, fuseops.RootInodeID)
	rootEntry.Attr.Mode = os.FileMode(0755) | os.ModeDir

	return fc
}

func (fc *FileContainer) Count() int {
	return len(fc.fileEntries)
}

func (fc *FileContainer) FindByInode(inode fuseops.InodeID) *FileEntry {
	return fc.fileEntries[inode]
}

// GID specific functions
func (fc *FileContainer) FindByGID(gid string) (fuseops.InodeID, *FileEntry) {
	for inode, v := range fc.fileEntries {
		if v.File != nil && v.File.ID() == gid {
			return inode, v
		}
	}
	return 0, nil
}

func (fc *FileContainer) LookupByGID(parentGID string, name string) (fuseops.InodeID, *FileEntry) {

	for inode, entry := range fc.fileEntries {
		if entry.HasParentGID(parentGID) && entry.Name == name {
			return inode, entry
		}
	}
	return 0, nil

}
func (fc *FileContainer) Lookup(parent *FileEntry, name string) (fuseops.InodeID, *FileEntry) {
	for inode, entry := range fc.fileEntries {
		if entry.HasParent(parent) && entry.Name == name {
			return inode, entry
		}
	}
	return 0, nil
}

func (fc *FileContainer) ListByParent(parent *FileEntry) map[fuseops.InodeID]*FileEntry {
	ret := map[fuseops.InodeID]*FileEntry{}
	for inode, entry := range fc.fileEntries {
		if entry.HasParent(parent) {
			ret[inode] = entry
		}
	}
	return ret

}

func (fc *FileContainer) CreateFile(parentFile *FileEntry, name string, isDir bool) (fuseops.InodeID, *FileEntry, error) {

	newGFile := &drive.File{
		Parents: []string{parentFile.File.ID()},
		Name:    name,
	}
	if isDir {
		newGFile.MimeType = "application/vnd.google-apps.folder"
	}
	// Could be transformed to CreateFile in continer
	createdGFile, err := fc.fs.Client.Files.Create(newGFile).Fields(fileFields).Do()
	if err != nil {
		return 0, nil, fuse.EINVAL
	}
	inode, entry := fc.FileEntry(createdGFile) // New Entry added // Or Return same?

	return inode, entry, nil
}

func (fc *FileContainer) DeleteFile(entry *FileEntry) error {
	err := fc.fs.Client.Files.Delete(entry.File.ID()).Do()
	if err != nil {
		return fuse.EIO
	}

	fc.RemoveEntry(entry)
	return nil
}

//////////////

//Return or create inode // Pass name maybe?
func (fc *FileContainer) FileEntry(gfile *drive.File, inodeOps ...fuseops.InodeID) (fuseops.InodeID, *FileEntry) {

	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	var inode fuseops.InodeID
	if len(inodeOps) > 0 {
		inode = inodeOps[0]
		if fe, ok := fc.fileEntries[inode]; ok {
			return inode, fe
		}
	} else { // generate new inode
		// Max Inode Number
		for inode = 2; inode < 99999; inode++ {
			_, ok := fc.fileEntries[inode]
			if !ok {
				break
			}
		}
	}

	name := ""
	if gfile != nil {
		name = gfile.Name
		count := 1
		nameParts := strings.Split(name, ".")
		for {
			// We find if we have a GFile in same parent with same name
			var entry *FileEntry
			// Only Place requireing a GID
			for _, p := range gfile.Parents {
				_, entry = fc.LookupByGID(p, name)
				if entry != nil {
					break
				}
			}
			if entry == nil { // Not found return
				break
			}
			count++
			if len(nameParts) > 1 {
				name = fmt.Sprintf("%s(%d).%s", nameParts[0], count, strings.Join(nameParts[1:], "."))
			} else {
				name = fmt.Sprintf("%s(%d)", nameParts[0], count)
			}
			log.Printf("Conflicting name generated new '%s' as '%s'", gfile.Name, name)
		}
	}

	fe := &FileEntry{
		//Inode: inode,
		//container: fc,
		Name: name,
		//children:  []*FileEntry{},
		Attr: fuseops.InodeAttributes{
			Uid: fc.uid,
			Gid: fc.gid,
		},
	}
	fe.SetFile(&GFile{gfile}, fc.uid, fc.gid)
	fc.fileEntries[inode] = fe

	return inode, fe
}

func (fc *FileContainer) SetEntry(inode fuseops.InodeID, entry *FileEntry) {
	fc.fileEntries[inode] = entry
}

// RemoveEntry remove file entry
func (fc *FileContainer) RemoveEntry(entry *FileEntry) {
	var inode fuseops.InodeID
	for k, e := range fc.fileEntries {
		if e == entry {
			inode = k
		}
	}
	delete(fc.fileEntries, inode)
}

func (fc *FileContainer) Sync(fe *FileEntry) (err error) {
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.Sync()
	fe.tempFile.Seek(0, io.SeekStart)

	upFile, err := fc.fs.Service.Upload(fe.tempFile, fe.File)
	if err != nil {
		return
	}
	fe.SetFile(upFile, fc.uid, fc.gid) // update local GFile entry
	return

}

//ClearCache remove local file
func (fc *FileContainer) ClearCache(fe *FileEntry) (err error) {
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.Close()
	os.Remove(fe.tempFile.Name())
	fe.tempFile = nil
	return
}

// Cache download GDrive file to a temporary local file or return already created file
func (fc *FileContainer) Cache(fe *FileEntry) *os.File {
	if fe.tempFile != nil {
		return fe.tempFile
	}
	var err error

	// Local copy
	fe.tempFile, err = ioutil.TempFile(os.TempDir(), "gdfs") // TODO: const this elsewhere
	if err != nil {
		log.Println("Error creating temp file")
		return nil
	}
	err = fc.fs.Service.DownloadTo(fe.tempFile, fe.File)
	if err != nil {
		return nil
	}
	fe.tempFile.Seek(0, io.SeekStart)
	return fe.tempFile

}

func (fc *FileContainer) Truncate(fe *FileEntry) (err error) { // DriverTruncate
	// Delete and create another on truncate 0
	newFile, err := fc.fs.Service.Truncate(fe.File)

	if err != nil {
		return fuse.EINVAL
	}
	fe.SetFile(newFile, fc.uid, fc.gid) // Set new file
	return
}
