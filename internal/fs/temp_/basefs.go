// gdrivemount implements a google drive fuse driver
package basefs

import (
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/prettylog"

	"golang.org/x/net/context"

	drive "google.golang.org/api/drive/v2"
	"google.golang.org/api/googleapi"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

var (
	log = prettylog.New("commonfs")
)

// GDriveFS
type BaseFS struct {
	fuseutil.NotImplementedFileSystem // Defaults

	config *core.Config //core   *core.Core // Core Config instead?
	//serviceConfig *Config
	//client        *drive.Service
	//root   *FileEntry // hiearchy reference
	root *FileContainer

	fileHandles map[fuseops.HandleID]*Handle

	nextRefresh time.Time

	handleMU *sync.Mutex
	//fileMap map[string]
	// Map IDS with FileEntries
}

func New(core *core.Core, driver Driver) core.DriverFS {

	fs := &BaseFS{
		config: &core.Config,
		//serviceConfig: &Config{}, // This is on service Driver
		fileHandles: map[fuseops.HandleID]*Handle{},
		handleMU:    &sync.Mutex{},
	}
	//client := fs.initClient() // Init Oauth2 client on service Driver
	fs.root = NewFileContainer(fs, driver)
	fs.root.uid = core.Config.UID
	fs.root.gid = core.Config.GID

	//fs.root = rootEntry

	// Temporary entry
	entry := fs.root.FileEntry("Loading...", nil, 9999)
	entry.Attr.Mode = os.FileMode(0)

	return fs
}

// Async
func (fs *BaseFS) Start() {
	go func() {
		//fs.Refresh() // First load

		// Change reader loop
		/*startPageTokenRes, err := fs.root.client.Changes.GetStartPageToken().Do()
		if err != nil {
			log.Println("GDrive err", err)
		}
		savedStartPageToken := startPageTokenRes.StartPageToken
		for {
			pageToken := savedStartPageToken
			for pageToken != "" {
				changesRes, err := fs.root.client.Changes.List(pageToken).Fields(googleapi.Field("newStartPageToken,nextPageToken,changes(removed,fileId,file(" + fileFields + "))")).Do()
				if err != nil {
					log.Println("Err fetching changes", err)
					break
				}
				//log.Println("Changes:", len(changesRes.Changes))
				for _, c := range changesRes.Changes {
					entry := fs.root.FindByGID(c.FileId)
					if c.Removed {
						if entry == nil {
							continue
						} else {
							fs.root.RemoveEntry(entry)
						}
						continue
					}

					if entry != nil {
						entry.SetGFile(c.File)
					} else {
						//Create new one
						fs.root.FileEntry(c.File) // Creating new one
					}
				}
				if changesRes.NewStartPageToken != "" {
					savedStartPageToken = changesRes.NewStartPageToken
				}
				pageToken = changesRes.NextPageToken
			}

			time.Sleep(fs.config.RefreshTime)
		}*/
	}()
}

////////////////////////////////////////////////////////
// TOOLS & HELPERS
////////////////////////////////////////////////////////

// COMMON
func (fs *BaseFS) createHandle() *Handle {
	// Lock here instead
	fs.handleMU.Lock()
	defer fs.handleMU.Unlock()

	var handleID fuseops.HandleID

	for handleID = 1; handleID < 99999; handleID++ {
		_, ok := fs.fileHandles[handleID]
		if !ok {
			break
		}
	}

	handle := &Handle{ID: handleID}
	fs.fileHandles[handleID] = handle

	return handle
}

const fileFields = googleapi.Field("id, name, size,mimeType, parents,createdTime,modifiedTime")
const gdFields = googleapi.Field("files(" + fileFields + ")")

// FULL Refresh service files

///////////////////////////////
// Fuse operations
////////////

// OpenDir return nil error allows open dir
// COMMON for drivers
func (fs *BaseFS) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) (err error) {

	entry := fs.root.FindByInode(op.Inode)
	if entry == nil {
		return fuse.ENOENT
	}

	handle := fs.createHandle()
	handle.entry = entry
	op.Handle = handle.ID

	return // No error allow, dir open
}

// ReadDir lists files into readdirop
// Common for drivers
func (fs *BaseFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) (err error) {
	fh, ok := fs.fileHandles[op.Handle]
	if !ok {
		log.Fatal("Handle does not exists")
	}

	if op.Offset == 0 { // Rebuild/rewind dir list
		fh.entries = []fuseutil.Dirent{}

		children := fs.root.ListByParent(fh.entry)
		//children := fs.root.ListByParentGID(fh.entry.GID)

		for i, v := range children {
			fusetype := fuseutil.DT_File
			if v.IsDir() {
				fusetype = fuseutil.DT_Directory
			}
			dirEnt := fuseutil.Dirent{
				Inode:  v.Inode,
				Name:   v.Name,
				Type:   fusetype,
				Offset: fuseops.DirOffset(i) + 1,
			}
			//	written += fuseutil.WriteDirent(fh.buf[written:], dirEnt)
			fh.entries = append(fh.entries, dirEnt)
		}
	}

	index := int(op.Offset)
	if index > len(fh.entries) {
		return fuse.EINVAL
	}
	if index > 0 {
		index++
	}
	for i := index; i < len(fh.entries); i++ {
		n := fuseutil.WriteDirent(op.Dst[op.BytesRead:], fh.entries[i])
		if n == 0 {
			break
		}
		op.BytesRead += n
	}
	return
}

// SPECIFIC code
func (fs *BaseFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) (err error) {

	// Hack to truncate file?

	if op.Size != nil {
		entry := fs.root.FindByInode(op.Inode)

		if *op.Size != 0 { // We only allow truncate to 0
			return fuse.ENOSYS
		}
		err = entry.Truncate()
	}

	return
}

//GetInodeAttributes return attributes
// COMMON
func (fs *BaseFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) (err error) {

	f := fs.root.FindByInode(op.Inode)
	if f == nil {
		return fuse.ENOENT
	}
	op.Attributes = f.Attr
	op.AttributesExpiration = time.Now().Add(time.Minute)

	return
}

// ReleaseDirHandle deletes file handle entry
// COMMON
func (fs *BaseFS) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) (err error) {
	delete(fs.fileHandles, op.Handle)
	return
}

// LookUpInode based on Parent and Name we return a self cached inode
// Cloud be COMMON but has specific ID
func (fs *BaseFS) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent) // true means transverse all
	if parentFile == nil {
		return fuse.ENOENT
	}

	entry := fs.root.LookupByParent(parentFile, op.Name)

	if entry == nil {
		return fuse.ENOENT
	}

	// Transverse only local

	now := time.Now()
	op.Entry = fuseops.ChildInodeEntry{
		Attributes:           entry.Attr,
		Child:                entry.Inode,
		AttributesExpiration: now.Add(time.Second),
		EntryExpiration:      now.Add(time.Second),
	}
	return
}

// ForgetInode allows to forgetInode
// COMMON
func (fs *BaseFS) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) (err error) {
	return
}

// GetXAttr special attributes
// COMMON
func (fs *BaseFS) GetXAttr(ctx context.Context, op *fuseops.GetXattrOp) (err error) {
	return
}

//////////////////////////////////////////////////////////////////////////
// File OPS
//////////////////////////////////////////////////////////////////////////

// OpenFile creates a temporary handle to be handled on read or write
// COMMON
func (fs *BaseFS) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) (err error) {
	f := fs.root.FindByInode(op.Inode) // might not exists

	// Generate new handle
	handle := fs.createHandle()
	handle.entry = f

	op.Handle = handle.ID
	op.UseDirectIO = true

	return
}

// COMMON but specific in cache
func (fs *BaseFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {
	handle := fs.fileHandles[op.Handle]

	localFile := handle.entry.Cache()
	op.BytesRead, err = localFile.ReadAt(op.Dst, op.Offset)
	if err == io.EOF { // fuse does not expect a EOF
		err = nil
	}

	return
}

// CreateFile creates empty file in google Drive and returns its ID and attributes, only allows file creation on 'My Drive'
// Cloud SPECIFIC
func (fs *BaseFS) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	// Only write on child folders
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	existsFile := fs.root.LookupByParent(parentFile, op.Name)
	//existsFile := parentFile.FindByName(op.Name, false)
	if existsFile != nil {
		return fuse.EEXIST
	}

	// Parent entry/Name
	entry, err := fs.root.CreateFile(parentFile, op.Name, false)
	if err != nil {
		return err
	}
	// Associate a temp file to a new handle
	// Local copy
	// Lock
	handle := fs.createHandle()
	handle.entry = entry
	handle.uploadOnDone = true
	//
	op.Handle = handle.ID
	op.Entry = fuseops.ChildInodeEntry{
		Attributes:           entry.Attr,
		Child:                entry.Inode,
		AttributesExpiration: time.Now().Add(time.Minute),
		EntryExpiration:      time.Now().Add(time.Minute),
	}
	op.Mode = entry.Attr.Mode

	return
}

// WriteFile as ReadFile it creates a temporary file on first read
// Maybe the ReadFile should be called here aswell to cache current contents since we are using writeAt
// CLOUD SPECIFIC
func (fs *BaseFS) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) (err error) {
	handle, ok := fs.fileHandles[op.Handle]
	if !ok {
		return fuse.EIO
	}

	localFile := handle.entry.Cache()
	if localFile == nil {
		return fuse.EINVAL
	}

	_, err = localFile.WriteAt(op.Data, op.Offset)
	if err != nil {
		err = fuse.EIO
		return
	}
	handle.uploadOnDone = true

	return
}

// FlushFile just returns no error, maybe upload should be handled here
// COMMON
func (fs *BaseFS) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) (err error) {
	handle, ok := fs.fileHandles[op.Handle]
	if !ok {
		return fuse.EIO
	}
	if handle.entry.tempFile == nil {
		return
	}
	if handle.uploadOnDone { // or if content changed basically
		err = handle.entry.Sync()
		if err != nil {
			return fuse.EINVAL
		}
	}
	return
}

// ReleaseFileHandle closes and deletes any temporary files, upload in case if changed locally
// COMMON
func (fs *BaseFS) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) (err error) {
	handle := fs.fileHandles[op.Handle]

	handle.entry.ClearCache()

	delete(fs.fileHandles, op.Handle)

	return
}

// Unlink remove file and remove from local cache entry
// SPECIFIC
func (fs *BaseFS) Unlink(ctx context.Context, op *fuseops.UnlinkOp) (err error) {
	parentEntry := fs.root.FindByInode(op.Parent)
	if parentEntry == nil {
		return fuse.ENOENT
	}
	if parentEntry.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	fileEntry := fs.root.LookupByGID(parentEntry.GID, op.Name)
	//fileEntry := parentEntry.FindByName(op.Name, false)
	if fileEntry == nil {
		return fuse.ENOATTR
	}
	return fs.root.DeleteFile(fileEntry)
}

// MkDir creates a directory on a parent dir
func (fs *BaseFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	entry, err := fs.root.CreateFile(parentFile, op.Name, true)
	if err != nil {
		return err
	}
	//entry = parentFile.AppendGFile(fi, entry.Inode)
	//if entry == nil {
	//		return fuse.EINVAL
	//	}

	op.Entry = fuseops.ChildInodeEntry{
		Attributes:           entry.Attr,
		Child:                entry.Inode,
		AttributesExpiration: time.Now().Add(time.Minute),
		EntryExpiration:      time.Now().Add(time.Microsecond),
	}

	return
}

// RmDir fuse implementation
func (fs *BaseFS) RmDir(ctx context.Context, op *fuseops.RmDirOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	theFile := fs.root.LookupByGID(parentFile.GID, op.Name)
	//theFile := parentFile.FindByName(op.Name, false)

	err = fs.root.DeleteFile(theFile)
	//err = fs.client.Files.Delete(theFile.GFile.Id).Do()
	if err != nil {
		return fuse.ENOTEMPTY
	}

	//parentFile.RemoveChild(theFile)

	// Remove from entry somehow

	return
}

// Rename fuse implementation
func (fs *GDriveFS) Rename(ctx context.Context, op *fuseops.RenameOp) (err error) {
	oldParentFile := fs.root.FindByInode(op.OldParent)
	if oldParentFile == nil {
		return fuse.ENOENT
	}
	newParentFile := fs.root.FindByInode(op.NewParent)
	if newParentFile == nil {
		return fuse.ENOENT
	}

	if oldParentFile.Inode == fuseops.RootInodeID || newParentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	//oldFile := oldParentFile.FindByName(op.OldName, false)
	oldEntry := fs.root.LookupByGID(oldParentFile.GID, op.OldName)

	// Although GDrive allows duplicate names, there is some issue with inode caching
	// So we prevent a rename to a file with same name
	//existsFile := newParentFile.FindByName(op.NewName, false)
	existsEntry := fs.root.LookupByGID(newParentFile.GID, op.NewName)
	if existsEntry != nil {
		return fuse.EEXIST
	}

	// Rename somehow
	ngFile := &drive.File{
		Name: op.NewName,
	}

	updateCall := fs.root.client.Files.Update(oldEntry.GID, ngFile).Fields(fileFields)
	if oldParentFile != newParentFile {
		updateCall.RemoveParents(oldParentFile.GID)
		updateCall.AddParents(newParentFile.GID)
	}
	updatedFile, err := updateCall.Do()

	oldEntry.SetGFile(updatedFile)

	//oldParentFile.RemoveChild(oldFile)
	//newParentFile.AppendGFile(updatedFile, oldFile.Inode)

	return

}
