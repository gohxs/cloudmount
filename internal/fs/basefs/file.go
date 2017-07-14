package basefs

import "github.com/jacobsa/fuse/fuseops"

type File interface {
	ID() string
	Name() string
	Attr() fuseops.InodeAttributes
	Parents() []string
}
