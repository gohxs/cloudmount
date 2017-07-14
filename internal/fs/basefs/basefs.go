// basefs implements a google drive fuse driver
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

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

var (
	log = prettylog.New("basefs")
)

type Handle struct {
	ID           fuseops.HandleID
	entry        *FileEntry
	uploadOnDone bool
	// Handling for dir
	entries []fuseutil.Dirent
}

// BaseFS data
type BaseFS struct {
	fuseutil.NotImplementedFileSystem // Defaults

	Config *core.Config //core   *core.Core // Core Config instead?
	Root   *FileContainer

	fileHandles map[fuseops.HandleID]*Handle
	handleMU    *sync.Mutex
	//serviceConfig *Config
	Client  *drive.Service
	Service Service
	//root   *FileEntry // hiearchy reference

	//fileMap map[string]
	// Map IDS with FileEntries
}

// New Creates a new BaseFS with config based on core
func New(core *core.Core) *BaseFS {

	fs := &BaseFS{
		Config:      &core.Config,
		fileHandles: map[fuseops.HandleID]*Handle{},
		handleMU:    &sync.Mutex{},
	}
	fs.Root = NewFileContainer(fs)
	fs.Root.uid = core.Config.UID
	fs.Root.gid = core.Config.GID

	_, entry := fs.Root.FileEntry(&drive.File{Id: "0", Name: "Loading..."}, 9999)
	entry.Attr.Mode = os.FileMode(0)

	return fs
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

// Client is still here
const fileFields = googleapi.Field("id, name, size,mimeType, parents,createdTime,modifiedTime")
const gdFields = googleapi.Field("files(" + fileFields + ")")

///////////////////////////////
// Fuse operations
////////////

// OpenDir return nil error allows open dir
// COMMON for drivers
func (fs *BaseFS) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) (err error) {

	entry := fs.Root.FindByInode(op.Inode)
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
		children := fs.Root.ListByParent(fh.entry)

		i := 0
		for inode, v := range children {
			fusetype := fuseutil.DT_File
			if v.IsDir() {
				fusetype = fuseutil.DT_Directory
			}
			dirEnt := fuseutil.Dirent{
				Inode:  inode,
				Name:   v.Name,
				Type:   fusetype,
				Offset: fuseops.DirOffset(i) + 1,
			}
			//	written += fuseutil.WriteDirent(fh.buf[written:], dirEnt)
			fh.entries = append(fh.entries, dirEnt)
			i++
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

// SetInodeAttributes Not sure what attributes gdrive support we just leave this blank for now
// SPECIFIC code
func (fs *BaseFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) (err error) {

	// Hack to truncate file?

	if op.Size != nil {
		entry := fs.Root.FindByInode(op.Inode)

		if *op.Size != 0 { // We only allow truncate to 0
			return fuse.ENOSYS
		}
		err = fs.Root.Truncate(entry)
	}

	return
}

//GetInodeAttributes return attributes
// COMMON
func (fs *BaseFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) (err error) {

	f := fs.Root.FindByInode(op.Inode)
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

	parentFile := fs.Root.FindByInode(op.Parent) // true means transverse all
	if parentFile == nil {
		return fuse.ENOENT
	}

	inode, entry := fs.Root.Lookup(parentFile, op.Name)

	if entry == nil {
		return fuse.ENOENT
	}

	// Transverse only local

	now := time.Now()
	op.Entry = fuseops.ChildInodeEntry{
		Attributes:           entry.Attr,
		Child:                inode,
		AttributesExpiration: now.Add(time.Second),
		EntryExpiration:      now.Add(time.Second),
	}
	return
}

// StatFS basically allows StatFS to run
/*func (fs *BaseFS) StatFS(ctx context.Context, op *fuseops.StatFSOp) (err error) {
	return
}*/

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
	f := fs.Root.FindByInode(op.Inode) // might not exists

	// Generate new handle
	handle := fs.createHandle()
	handle.entry = f

	op.Handle = handle.ID
	op.UseDirectIO = true

	return
}

// ReadFile  if the first time we download the google drive file into a local temporary file
// COMMON but specific in cache
func (fs *BaseFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {
	handle := fs.fileHandles[op.Handle]

	localFile := fs.Root.Cache(handle.entry)
	op.BytesRead, err = localFile.ReadAt(op.Dst, op.Offset)
	if err == io.EOF { // fuse does not expect a EOF
		err = nil
	}

	return
}

// CreateFile creates empty file in google Drive and returns its ID and attributes, only allows file creation on 'My Drive'
// Cloud SPECIFIC
func (fs *BaseFS) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) (err error) {

	parentFile := fs.Root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	// Only write on child folders
	if parentFile == fs.Root.fileEntries[fuseops.RootInodeID] {
		return syscall.EPERM
	}

	_, existsFile := fs.Root.Lookup(parentFile, op.Name)
	//existsFile := parentFile.FindByName(op.Name, false)
	if existsFile != nil {
		return fuse.EEXIST
	}

	// Parent entry/Name
	inode, entry, err := fs.Root.CreateFile(parentFile, op.Name, false)
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
		Child:                inode,
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

	localFile := fs.Root.Cache(handle.entry)
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
		err = fs.Root.Sync(handle.entry)
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

	fs.Root.ClearCache(handle.entry)

	delete(fs.fileHandles, op.Handle)

	return
}

// Unlink remove file and remove from local cache entry
// SPECIFIC
func (fs *BaseFS) Unlink(ctx context.Context, op *fuseops.UnlinkOp) (err error) {
	if op.Parent == fuseops.RootInodeID {
		return syscall.EPERM
	}
	parentEntry := fs.Root.FindByInode(op.Parent)
	if parentEntry == nil {
		return fuse.ENOENT
	}

	_, fileEntry := fs.Root.Lookup(parentEntry, op.Name)
	//fileEntry := parentEntry.FindByName(op.Name, false)
	if fileEntry == nil {
		return fuse.ENOATTR
	}
	return fs.Root.DeleteFile(fileEntry)
}

// MkDir creates a directory on a parent dir
func (fs *BaseFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) (err error) {
	if op.Parent == fuseops.RootInodeID {
		return syscall.EPERM
	}

	parentFile := fs.Root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}

	inode, entry, err := fs.Root.CreateFile(parentFile, op.Name, true)
	if err != nil {
		return err
	}

	op.Entry = fuseops.ChildInodeEntry{
		Attributes:           entry.Attr,
		Child:                inode,
		AttributesExpiration: time.Now().Add(time.Minute),
		EntryExpiration:      time.Now().Add(time.Microsecond),
	}

	return
}

// RmDir fuse implementation
func (fs *BaseFS) RmDir(ctx context.Context, op *fuseops.RmDirOp) (err error) {
	if op.Parent == fuseops.RootInodeID {
		return syscall.EPERM
	}

	parentFile := fs.Root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}

	_, theFile := fs.Root.Lookup(parentFile, op.Name)

	err = fs.Root.DeleteFile(theFile)
	if err != nil {
		return fuse.ENOTEMPTY
	}

	return
}

// Rename fuse implementation
func (fs *BaseFS) Rename(ctx context.Context, op *fuseops.RenameOp) (err error) {
	if op.OldParent == fuseops.RootInodeID || op.NewParent == fuseops.RootInodeID {
		return syscall.EPERM
	}
	oldParentFile := fs.Root.FindByInode(op.OldParent)
	if oldParentFile == nil {
		return fuse.ENOENT
	}
	newParentFile := fs.Root.FindByInode(op.NewParent)
	if newParentFile == nil {
		return fuse.ENOENT
	}

	//oldFile := oldParentFile.FindByName(op.OldName, false)
	_, oldEntry := fs.Root.Lookup(oldParentFile, op.OldName)

	// Although GDrive allows duplicate names, there is some issue with inode caching
	// So we prevent a rename to a file with same name
	//existsFile := newParentFile.FindByName(op.NewName, false)
	_, existsEntry := fs.Root.Lookup(newParentFile, op.NewName)
	if existsEntry != nil {
		return fuse.EEXIST
	}

	ngFile := &drive.File{
		Name: op.NewName,
	}

	updateCall := fs.Client.Files.Update(oldEntry.GID, ngFile).Fields(fileFields)
	if oldParentFile != newParentFile {
		updateCall.RemoveParents(oldParentFile.GID)
		updateCall.AddParents(newParentFile.GID)
	}
	updatedFile, err := updateCall.Do()

	oldEntry.SetFile(&GFile{updatedFile}, fs.Config.UID, fs.Config.GID)

	//oldParentFile.RemoveChild(oldFile)
	//newParentFile.AppendGFile(updatedFile, oldFile.Inode)

	return

}
