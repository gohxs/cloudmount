package basefs

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/jacobsa/fuse/fuseops"
)

type FileContainer struct {
	FileEntries map[fuseops.InodeID]*FileEntry
	///	tree        *FileEntry
	fs *BaseFS
	//client *drive.Service // Wrong should be common
	uid uint32
	gid uint32

	inodeMU *sync.Mutex
}

func NewFileContainer(fs *BaseFS) *FileContainer {
	fc := &FileContainer{
		FileEntries: map[fuseops.InodeID]*FileEntry{},
		fs:          fs,
		//client:  fs.Client,
		inodeMU: &sync.Mutex{},
		uid:     fs.Config.UID,
		gid:     fs.Config.GID,
	}
	rootEntry := fc.FileEntry(nil, fuseops.RootInodeID)
	rootEntry.Attr.Mode = os.FileMode(0755) | os.ModeDir
	rootEntry.Attr.Uid = fs.Config.UID
	rootEntry.Attr.Gid = fs.Config.GID

	return fc
}

func (fc *FileContainer) Count() int {
	return len(fc.FileEntries)
}

func (fc *FileContainer) FindByInode(inode fuseops.InodeID) *FileEntry {
	return fc.FileEntries[inode]
}
func (fc *FileContainer) FindByID(id string) *FileEntry {
	log.Println("Searching for :", id)
	for _, v := range fc.FileEntries {
		if v.File == nil && id == "" {
			log.Println("Found cause file is nil and id '' inode:", v.Inode)
			return v
		}
		if v.File != nil && v.File.ID == id {
			log.Println("Found by id")
			return v
		}
	}
	log.Println("Not found")
	return nil
}

// Try not to use this
func (fc *FileContainer) Lookup(parent *FileEntry, name string) *FileEntry {
	for _, entry := range fc.FileEntries {
		if entry.HasParent(parent) && entry.Name == name {
			return entry
		}
	}
	return nil
}

func (fc *FileContainer) ListByParent(parent *FileEntry) []*FileEntry {
	ret := []*FileEntry{}
	for _, entry := range fc.FileEntries {
		if entry.HasParent(parent) {
			ret = append(ret, entry)
		}
	}
	return ret

}

func (fc *FileContainer) CreateFile(parentFile *FileEntry, name string, isDir bool) (*FileEntry, error) {

	createdFile, err := fc.fs.Service.Create(parentFile.File, name, isDir)
	if err != nil {
		return nil, err
	}
	entry := fc.FileEntry(createdFile) // New Entry added // Or Return same?

	//fc.Truncate(entry) // Dropbox dont have a way to upload?

	return entry, nil
}

func (fc *FileContainer) DeleteFile(entry *FileEntry) error {
	err := fc.fs.Service.Delete(entry.File)
	if err != nil {
		return err
	}
	fc.RemoveEntry(entry)
	return nil
}

//////////////

//Return or create inode // Pass name maybe?
func (fc *FileContainer) FileEntry(file *File, inodeOps ...fuseops.InodeID) *FileEntry {

	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	var inode fuseops.InodeID
	if len(inodeOps) > 0 {
		inode = inodeOps[0]
		if fe, ok := fc.FileEntries[inode]; ok {
			return fe
		}
	} else { // generate new inode
		// Max Inode Number
		for inode = 2; inode < math.MaxUint64; inode++ {
			_, ok := fc.FileEntries[inode]
			if !ok {
				break
			}
		}
	}

	/////////////////////////////////////////////////////////////
	// Important some file systems might support insane chars
	////////////////////////////////////
	// Name solver
	name := ""
	if file != nil {
		name = file.Name
		count := 1
		nameParts := strings.Split(name, ".")
		for {
			// We find if we have a GFile in same parent with same name
			var entry *FileEntry
			// Only Place requireing a GID
			for _, p := range file.Parents {
				entry = fc.LookupByID(p, name)
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
			log.Printf("Conflicting name generated new '%s' as '%s'", file.Name, name)
		}
	}
	/////////////////////////////////
	// Filename sanitizer?, There might be other unsupported char
	////
	if strings.Contains(name, "/") { // Something to inform user
		newName := strings.Replace(name, "/", "_", -1)
		log.Println("Filename contains invalid chars, sanitizing: '%s'-'%s'", name, newName)
		name = newName
	}

	fe := &FileEntry{
		Inode: inode,
		Name:  name,
	}
	// Temp gfile?
	if file != nil {
		fe.SetFile(file, fc.uid, fc.gid)
	}
	fc.FileEntries[inode] = fe

	return fe
}

func (fc *FileContainer) SetEntry(inode fuseops.InodeID, entry *FileEntry) {
	fc.FileEntries[inode] = entry
}

// RemoveEntry remove file entry
func (fc *FileContainer) RemoveEntry(entry *FileEntry) {
	var inode fuseops.InodeID
	for k, e := range fc.FileEntries {
		if e == entry {
			inode = k
		}
	}
	delete(fc.FileEntries, inode)
}

func (fc *FileContainer) Sync(fe *FileEntry) (err error) {
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.Sync()
	fe.tempFile.Seek(0, io.SeekStart) // Depends??, for reading?

	upFile, err := fc.fs.Service.Upload(fe.tempFile, fe.File)
	if err != nil {
		return err
	}
	log.Println("Uploaded file size:", upFile.Size)
	fe.SetFile(upFile, fc.uid, fc.gid) // update local GFile entry
	return

}

//ClearCache remove local file
func (fc *FileContainer) ClearCache(fe *FileEntry) (err error) {
	if fe.tempFile == nil {
		return
	}
	fe.tempFile.RealClose()
	os.Remove(fe.tempFile.Name())
	fe.tempFile = nil
	return
}

// Cache download GDrive file to a temporary local file or return already created file
func (fc *FileContainer) Cache(fe *FileEntry) *fileWrapper {
	if fe.tempFile != nil {
		return fe.tempFile
	}
	var err error

	// Local copy
	localFile, err := ioutil.TempFile(os.TempDir(), "gdfs") // TODO: const this elsewhere
	if err != nil {
		return nil
	}
	fe.tempFile = &fileWrapper{localFile}

	err = fc.fs.Service.DownloadTo(fe.tempFile, fe.File)
	// ignore download since can be a bogus file, for certain file systems
	//if err != nil { // Ignore this error
	//    return nil
	//}
	fe.tempFile.Seek(0, io.SeekStart)
	return fe.tempFile

}

// Truncate truncates localFile to 0 bytes
func (fc *FileContainer) Truncate(fe *FileEntry) (err error) { // DriverTruncate
	// Delete and create another on truncate 0
	localFile, err := ioutil.TempFile(os.TempDir(), "gdfs") // TODO: const this elsewhere
	if err != nil {
		return err
	}
	fe.tempFile = &fileWrapper{localFile}
	//fc.Sync(fe) // Basically upload empty file??

	return
}

// LookupByID lookup by remote ID
func (fc *FileContainer) LookupByID(parentID string, name string) *FileEntry {
	for _, entry := range fc.FileEntries {
		if entry.HasParentID(parentID) && entry.Name == name {
			return entry
		}
	}
	return nil
}
