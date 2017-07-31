package basefs

import (
	"os"
	"time"
)

//File entry structure all basefs based services must use these
type File struct {
	ID           string
	Name         string
	Size         uint64
	CreatedTime  time.Time
	ModifiedTime time.Time
	AccessedTime time.Time
	Mode         os.FileMode
	Parents      []string
	Data         interface{} // Any thing
}

// HasParent check file parenting
func (f *File) HasParent(parent *File) bool {
	parentID := ""
	if parent != nil {
		parentID = parent.ID
	}
	for _, p := range f.Parents {
		if p == parentID {
			return true
		}
	}
	return false
}
