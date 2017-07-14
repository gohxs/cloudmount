// Tempoarry in basefs since we dont have the service yet
package basefs

import (
	"os"
	"time"

	"github.com/jacobsa/fuse/fuseops"
	drive "google.golang.org/api/drive/v3"
)

type GFile struct {
	*drive.File
}

func (gf *GFile) ID() string {
	return gf.Id
}

func (gf *GFile) Name() string {
	return gf.File.Name
}

func (gf *GFile) Parents() []string {

	return gf.File.Parents
}

func (gf *GFile) Attr() fuseops.InodeAttributes {

	attr := fuseops.InodeAttributes{}
	attr.Nlink = 1
	//attr.Uid = fe.container.uid
	//attr.Gid = fe.container.gid

	attr.Mode = os.FileMode(0644) // default
	//if gfile != nil {
	attr.Size = uint64(gf.File.Size)
	attr.Crtime, _ = time.Parse(time.RFC3339, gf.File.CreatedTime)
	attr.Ctime = attr.Crtime // Set CTime to created, although it is change inode metadata
	attr.Mtime, _ = time.Parse(time.RFC3339, gf.File.ModifiedTime)
	attr.Atime = attr.Mtime // Set access time to modified, not sure if gdrive has access time
	if gf.MimeType == "application/vnd.google-apps.folder" {
		attr.Mode = os.FileMode(0755) | os.ModeDir
	}
	return attr
}
