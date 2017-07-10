package gdrivefs

import (
	"sync"

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
	}
	fc.tree = fc.FileEntry(fuseops.RootInodeID)
	return fc
}

func (fc *FileContainer) FindByInode(inode fuseops.InodeID) *FileEntry {
	return fc.fileEntries[inode]
}

//Return or create inode
func (fc *FileContainer) FileEntry(inodeOps ...fuseops.InodeID) *FileEntry {

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

	fe := &FileEntry{
		Inode:     inode,
		container: fc,
		//fs:        fc.fs,
		children: []*FileEntry{},
		Attr:     fuseops.InodeAttributes{},
	}
	fc.fileEntries[inode] = fe

	return fe
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
