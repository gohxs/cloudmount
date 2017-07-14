package basefs

import (
	"errors"
	"os"

	"github.com/jacobsa/fuse/fuseops"
)

var (
	ErrNotImplemented = errors.New("Not implemented")
)

type FileEntry struct {
	container *FileContainer // Container reference
	Name      string
	Inode     fuseops.InodeID
	Attr      fuseops.InodeAttributes
	tempFile  *os.File
}

func (fe *FileEntry) IsDir() bool {
	return false
}

func (fe *FileEntry) Truncate() error {
	return ErrNotImplemented
}

func (fe *FileEntry) Cache() *os.File {

	return nil
}
