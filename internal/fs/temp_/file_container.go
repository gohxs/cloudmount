package basefs

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/jacobsa/fuse/fuseops"
)

type FileContainer struct {
	fileEntries map[fuseops.InodeID]*FileEntry
	//fs          *GDriveFS
	//client *drive.Service // Wrong should be common
	uid uint32
	gid uint32

	inodeMU *sync.Mutex
}

// Pass config core somehow?
func NewFileContainer(config *Config) *FileContainer {
	fc := &FileContainer{
		fileEntries: map[fuseops.InodeID]*FileEntry{},
		//fs:          fs,
		//client:  client,
		inodeMU: &sync.Mutex{},
		uid:     config.UID,
		gid:     config.GID,
	}
	rootEntry := fc.FileEntry("", nil, fuseops.RootInodeID)
	rootEntry.Attr.Mode = os.FileMode(0755) | os.ModeDir

	return fc
}

func (fc *FileContainer) FileEntry(Name string, gfile interface{}, inodeOps ...fuseops.InodeID) *FileEntry {

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
		name = Name //Add Name in param?
		count := 1
		nameParts := strings.Split(name, ".")
		for {
			// We find if we have a GFile in same parent with same name
			var entry *FileEntry
			// Check parent somehow maybe with inode
			//for _, p := range gfile.Parents {
			//	entry = fc.LookupByGID(p, name)
			//	if entry != nil {
			//		break
			//	}
			//}

			if entry == nil { // Not found return
				break
			}
			count++
			if len(nameParts) > 1 {
				name = fmt.Sprintf("%s(%d).%s", nameParts[0], count, strings.Join(nameParts[1:], "."))
			} else {
				name = fmt.Sprintf("%s(%d)", nameParts[0], count)
			}
			log.Printf("Conflicting name generated new '%s' as '%s'", Name, name)
		}
	}

	fe := &FileEntry{
		//GFile:     gfile,
		Inode:     inode,
		container: fc,
		Name:      name,
		//children:  []*FileEntry{},
		Attr: fuseops.InodeAttributes{
			Uid: fc.uid,
			Gid: fc.gid,
		},
	}
	// fe.SetGFile(gfile) // Somehow get necessary information from here
	fc.fileEntries[inode] = fe

	return fe
}

func (fc *FileContainer) FindByInode(inode fuseops.InodeID) *FileEntry {

	return nil // Not implemented
}

func (fc *FileContainer) ListByParent(parent *FileEntry) []*FileEntry {

	return nil
}

func (fc *FileContainer) LookupByParent(parent *FileEntry, name string) *FileEntry {

	return nil
}
