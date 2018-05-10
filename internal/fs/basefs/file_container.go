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

//FileContainer will hold file entries
type FileContainer struct {
	fileEntries map[fuseops.InodeID]*FileEntry
	///	tree        *FileEntry
	fs *BaseFS
	//client *drive.Service // Wrong should be common
	uid uint32
	gid uint32

	inodeMU *sync.Mutex
}

//NewFileContainer creates and initialize a FileContainer
func NewFileContainer(fs *BaseFS) *FileContainer {

	fc := &FileContainer{
		fileEntries: map[fuseops.InodeID]*FileEntry{},
		fs:          fs,
		//client:  fs.Client,
		inodeMU: &sync.Mutex{},
		uid:     fs.Config.Options.UID,
		gid:     fs.Config.Options.GID,
	}
	rootEntry := fc.FileEntry(nil, fuseops.RootInodeID)
	rootEntry.Attr.Mode = os.FileMode(0755) | os.ModeDir
	rootEntry.Attr.Uid = fc.uid
	rootEntry.Attr.Gid = fc.gid

	return fc
}

//Count the total number of fileentries
func (fc *FileContainer) Count() int {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	return len(fc.fileEntries)
}

//FindByInode retrieves a file entry by inode
func (fc *FileContainer) FindByInode(inode fuseops.InodeID) *FileEntry {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	return fc.fileEntries[inode]
}

//FindByID retrives by ID
func (fc *FileContainer) FindByID(id string) *FileEntry {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	for _, v := range fc.fileEntries {
		if v.File == nil && id == "" {
			log.Println("Found cause file is nil and id '' inode:", v.Inode)
			return v
		}
		if v.File != nil && v.File.ID == id {
			return v
		}
	}
	return nil
}

//Lookup retrives a FileEntry from a parent(folder) with name
func (fc *FileContainer) Lookup(parent *FileEntry, name string) *FileEntry {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	for _, entry := range fc.fileEntries {
		if entry.HasParent(parent) && entry.Name == name {
			return entry
		}
	}
	return nil
}

//ListByParent entries from parent
func (fc *FileContainer) ListByParent(parent *FileEntry) []*FileEntry {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	ret := []*FileEntry{}
	for _, entry := range fc.fileEntries {
		if entry.HasParent(parent) {
			ret = append(ret, entry)
		}
	}
	return ret

}

//CreateFile tell service to create a file
func (fc *FileContainer) CreateFile(parentFile *FileEntry, name string, isDir bool) (*FileEntry, error) {

	createdFile, err := fc.fs.Service.Create(parentFile.File, name, isDir)
	if err != nil {
		return nil, err
	}
	entry := fc.FileEntry(createdFile) // New Entry added // locks

	return entry, nil
}

//DeleteFile tell service to delete a file
func (fc *FileContainer) DeleteFile(entry *FileEntry) error {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	err := fc.fs.Service.Delete(entry.File)
	if err != nil {
		return err
	}
	fc.removeEntry(entry)
	return nil
}

//////////////

//FileEntry Create or Update a FileEntry by inode, inode is an optional argument
func (fc *FileContainer) FileEntry(file *File, inodeOps ...fuseops.InodeID) *FileEntry {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	var inode fuseops.InodeID
	if len(inodeOps) > 0 {
		inode = inodeOps[0]
		if fe, ok := fc.fileEntries[inode]; ok {
			return fe
		}
	} else { // generate new inode
		// Max Inode Number
		for inode = 2; inode < math.MaxUint64; inode++ {
			_, ok := fc.fileEntries[inode]
			if !ok {
				break
			}
		}
	}
	//////////////////////////////////////////////////////////////////////////////////////////
	// Some cloud services supports duplicated names, we add an index if name is duplicated
	////////////////////////////////////
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
				entry = fc.lookupByID(p, name)
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
	/////////////////////////////////////////////////////////////
	// Important some cloud services might support insane chars
	////////////////////////////////////
	if strings.Contains(name, "/") { // Something to inform user
		newName := strings.Replace(name, "/", "_", -1)
		log.Printf("Filename contains invalid chars, sanitizing: '%s'-'%s'", name, newName)
		name = newName
	}
	fe := &FileEntry{
		Inode: inode,
		Name:  name,
	}
	// Temp gfile?
	if file != nil {
		fe.SetFile(file, fc.uid, fc.gid)
		//fe.SetFile(file)
	}
	fc.fileEntries[inode] = fe

	return fe
}

//SetEntry Adds an entry to file container based on inode
func (fc *FileContainer) SetEntry(inode fuseops.InodeID, entry *FileEntry) {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()

	fc.fileEntries[inode] = entry
}

// RemoveEntry remove file entry
func (fc *FileContainer) RemoveEntry(entry *FileEntry) {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()
	fc.removeEntry(entry)
}

//Sync will flush, upload file and update local entry
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

//Cache download GDrive file to a temporary local file or return already created file
func (fc *FileContainer) Cache(fe *FileEntry) *FileWrapper {
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
	fe.tempFile = &FileWrapper{localFile}

	return
}

// LookupByID lookup by remote ID
func (fc *FileContainer) LookupByID(parentID string, name string) *FileEntry {
	fc.inodeMU.Lock()
	defer fc.inodeMU.Unlock()
	return fc.lookupByID(parentID, name)
}

// non lock lookupByID
func (fc *FileContainer) lookupByID(parentID string, name string) *FileEntry {
	for _, entry := range fc.fileEntries {
		if entry.HasParentID(parentID) && entry.Name == name {
			return entry
		}
	}
	return nil

}

func (fc *FileContainer) removeEntry(entry *FileEntry) {

	var inode fuseops.InodeID
	for k, e := range fc.fileEntries {
		if e == entry {
			inode = k
		}
	}
	delete(fc.fileEntries, inode)
}
