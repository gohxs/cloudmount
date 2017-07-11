package gdrivefs

import (
	"fmt"
	"os"
	"strings"
	"sync"

	drive "google.golang.org/api/drive/v3"

	"github.com/jacobsa/fuse/fuseops"
)

type FileContainer struct {
	fileEntries map[fuseops.InodeID]*FileEntry
	tree        *FileEntry
	fs          *GDriveFS
	uid         uint32
	gid         uint32

	inodeMU *sync.Mutex
}

func NewFileContainer(fs *GDriveFS) *FileContainer {
	fc := &FileContainer{
		fileEntries: map[fuseops.InodeID]*FileEntry{},
		fs:          fs,
		inodeMU:     &sync.Mutex{},
		uid:         fs.config.UID,
		gid:         fs.config.GID,
	}
	rootEntry := fc.FileEntry(nil, fuseops.RootInodeID)
	rootEntry.Attr.Mode = os.FileMode(0755) | os.ModeDir
	fc.tree = rootEntry

	return fc
}

func (fc *FileContainer) FindByInode(inode fuseops.InodeID) *FileEntry {
	return fc.fileEntries[inode]
}

func (fc *FileContainer) FindByGID(gid string) *FileEntry {
	for _, v := range fc.fileEntries {
		if v.GFile != nil && v.GFile.Id == gid {
			return v
		}
	}
	return nil
}

func (fc *FileContainer) LookupByGID(parentGID string, name string) *FileEntry {
	for _, entry := range fc.fileEntries {
		if entry.HasParentGID(parentGID) && entry.Name == name {
			return entry
		}
	}
	return nil
}

func (fc *FileContainer) ListByParentGID(parentGID string) []*FileEntry {
	ret := []*FileEntry{}
	for _, entry := range fc.fileEntries {
		if entry.HasParentGID(parentGID) {
			ret = append(ret, entry)
		}
	}
	return ret
}

//Return or create inode // Pass name maybe?
func (fc *FileContainer) FileEntry(gfile *drive.File, inodeOps ...fuseops.InodeID) *FileEntry {

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
			for _, p := range gfile.Parents {
				entry = fc.LookupByGID(p, name)
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
		GFile:     gfile,
		Inode:     inode,
		container: fc,
		Name:      name,
		//children:  []*FileEntry{},
		Attr: fuseops.InodeAttributes{
			Uid: fc.uid,
			Gid: fc.gid,
		},
	}
	fe.SetGFile(gfile)
	fc.fileEntries[inode] = fe

	return fe
}

func (fc *FileContainer) AddEntry(entry *FileEntry) {
	fc.fileEntries[entry.Inode] = entry
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

func (fc *FileContainer) AddGFile(gfile *drive.File) *FileEntry {
	entry := fc.FindByGID(gfile.Id)
	if entry != nil {
		return entry
	}
	// Create new Entry
	entry = fc.FileEntry(gfile)
	entry.SetGFile(gfile)

	return entry
}
