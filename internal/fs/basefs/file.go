package basefs

import (
	"os"
	"time"
)

// This could be a struct
// And service would be creating these
type File struct {
	ID   string
	Name string
	// Build Attr from this
	Size         uint64
	CreatedTime  time.Time
	ModifiedTime time.Time
	AccessedTime time.Time
	Mode         os.FileMode
	Parents      []string
	Data         interface{} // Any thing
}

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
