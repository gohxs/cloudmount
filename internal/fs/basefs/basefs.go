//Package basefs implements a base filesystem caching entries
package basefs

import (
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"syscall"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/api/googleapi"

	"github.com/gohxs/cloudmount/internal/core"
	"github.com/gohxs/prettylog"
	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

const maxInodes = math.MaxUint64

var (
	pname  = "basefs"
	log    = prettylog.Dummy()
	errlog = prettylog.New(pname + "-err")
	// ErrNotImplemented basic Not implemented error
	ErrNotImplemented = errors.New("Not implemented")
	// ErrPermission permission denied error
	ErrPermission = errors.New("Permission denied")
)

type handle struct {
	ID           fuseops.HandleID
	entry        *FileEntry
	uploadOnDone bool
	// Handling for dir
	entries []fuseutil.Dirent
}

// BaseFS data
type BaseFS struct {
	fuseutil.NotImplementedFileSystem // Defaults

	Config      *core.Config //core   *core.Core // Core Config instead?
	Root        *FileContainer
	fileHandles sync.Map
	handleMU    *sync.Mutex
	Service     Service
}

// New Creates a new BaseFS with config based on core
func New(core *core.Core) *BaseFS {
	if core.Config.VerboseLog {
		log = prettylog.New(pname)
	}

	fs := &BaseFS{
		Config:      &core.Config,
		fileHandles: sync.Map{},
		handleMU:    &sync.Mutex{},
	}

	fs.Root = NewFileContainer(fs)
	fs.Root.uid = fs.Config.Options.UID
	fs.Root.gid = fs.Config.Options.GID

	loadingFile := File{Name: "Loading...", ID: "0"}
	entry := fs.Root.FileEntry(&loadingFile, maxInodes) // Last inode
	entry.Attr.Mode = os.FileMode(0)

	return fs
}

// Start BaseFS service with loop for changes
func (fs *BaseFS) Start() {
	// Fill root container and do changes
	go func() {
		fs.Refresh()
		log.Println("Files loaded:", len(fs.Root.fileEntries))
		for {
			fs.CheckForChanges()
			time.Sleep(fs.Config.RefreshTime)
		}
	}()
}

// Refresh should be renamed to Load or something
func (fs *BaseFS) Refresh() {
	// Try
	files, err := fs.Service.ListAll()
	if err != nil { // Repeat refresh maybe?
	}
	root := NewFileContainer(fs)
	// Two passes first the ones with existing entries next the non existent
	for i := 0; i < len(files); i++ {
		file := files[i]
		oldEntry := fs.Root.FindByID(file.ID)
		if oldEntry == nil {
			continue // not found skip 'i' will increase here
		}
		root.FileEntry(file, oldEntry.Inode)      // Try to find in previous root
		files = append(files[:i], files[i+1:]...) // Remove item (is this range safe?)
		i--                                       // rollback one

	}
	// Rest of the files (new files)
	for _, file := range files {
		root.FileEntry(file) // Try to find in previous root
	}
	fs.Root = root // Swap root
}

// CheckForChanges polling
func (fs *BaseFS) CheckForChanges() {
	changes, err := fs.Service.Changes()
	if err != nil {
		return
	}
	for _, c := range changes {
		entry := fs.Root.FindByID(c.ID)
		if c.Remove {
			if entry != nil {
				fs.Root.RemoveEntry(entry)
			}
			continue
		}
		if entry != nil {
			//Remove old entry?
			fs.Root.RemoveEntry(entry)
			fs.Root.FileEntry(c.File, entry.Inode) // Add Entry with same inode and new File?
			//entry.SetFile(c.File, fs.Config.Options.UID, fs.Config.Options.GID)
			//entry.SetFile(c.File)
		} else {
			//Create new one
			fs.Root.FileEntry(c.File) // Creating new one
		}
	}
}

////////////////////////////////////////////////////////
// TOOLS & HELPERS
////////////////////////////////////////////////////////

// COMMON
func (fs *BaseFS) createHandle() *handle {

	// Assure we get a unique ID
	fs.handleMU.Lock()
	defer fs.handleMU.Unlock()

	var handleID fuseops.HandleID
	for handleID = 1; handleID < math.MaxUint64; handleID++ {
		_, ok := fs.fileHandles.Load(handleID)
		if !ok {
			break
		}
	}

	h := &handle{ID: handleID}
	fs.fileHandles.Store(handleID, h)

	return h
}

// Client is still here
const fileFields = googleapi.Field("id, name, size,mimeType, parents,createdTime,modifiedTime")
const gdFields = googleapi.Field("files(" + fileFields + ")")

///////////////////////////////
// Fuse operations
////////////

// StatFS this is used by DF  -- TESTING
func (fs *BaseFS) StatFS(ctx context.Context, op *fuseops.StatFSOp) (err error) {
	err = fs.Service.StatFS(op)
	if err != nil {
		return err
	}
	op.Inodes = uint64(len(fs.Root.fileEntries))
	op.InodesFree = math.MaxUint64 - op.Inodes
	log.Println("Free inodes:", op.InodesFree)
	//op.BlockSize = 48
	//op.BlocksAvailable = 2
	//op.Blocks = 2
	//op.Inodes = uint64(len(fs.Root.fileEntries))
	//op.InodesFree = 2
	//op.IoSize = 1024
	//
	return nil
}

// OpenDir return nil error allows open dir
// COMMON for drivers
func (fs *BaseFS) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) (err error) {

	entry := fs.Root.FindByInode(op.Inode)
	if entry == nil {
		return fuse.ENOENT
	}

	fh := fs.createHandle()
	fh.entry = entry
	op.Handle = fh.ID

	return // No error allow, dir open
}

// ReadDir lists files into readdirop
// Common for drivers
func (fs *BaseFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) (err error) {

	fhi, ok := fs.fileHandles.Load(op.Handle)
	if !ok {
		return fuse.EIO
	}
	fh := fhi.(*handle)

	if op.Offset == 0 { // Rebuild/rewind dir list

		fh.entries = []fuseutil.Dirent{}
		children := fs.Root.ListByParent(fh.entry)
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

// SetInodeAttributes Not sure what attributes gdrive support we just leave this blank for now
// SPECIFIC code
func (fs *BaseFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) (err error) {

	// Hack to truncate file?

	if op.Size != nil {
		entry := fs.Root.FindByInode(op.Inode)

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
	fs.fileHandles.Delete(op.Handle)
	return
}

// LookUpInode based on Parent and Name we return a self cached inode
// Cloud be COMMON but has specific ID
func (fs *BaseFS) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) (err error) {

	parentFile := fs.Root.FindByInode(op.Parent) // true means transverse all
	if parentFile == nil {
		return fuse.ENOENT
	}

	entry := fs.Root.Lookup(parentFile, op.Name)

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
// XXX: Check what to do if the tempfile exists locally
func (fs *BaseFS) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) (err error) {
	f := fs.Root.FindByInode(op.Inode) // might not exists

	// Generate new handle
	fh := fs.createHandle()
	fh.entry = f

	op.Handle = fh.ID
	op.UseDirectIO = true

	return
}

// ReadFile  if the first time we download the google drive file into a local temporary file
// COMMON but specific in cache
func (fs *BaseFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {
	fhi, ok := fs.fileHandles.Load(op.Handle)
	if !ok {
		return fuse.EIO
	}
	fh := fhi.(*handle)

	localFile := fh.entry.Cache(fs.Root)
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

	existsFile := fs.Root.Lookup(parentFile, op.Name)
	if existsFile != nil {
		return fuse.EEXIST
	}

	// Parent entry/Name
	entry, err := fs.Root.CreateFile(parentFile, op.Name, false)
	if err != nil {
		return fuseErr(err)
	}
	// Associate a temp file to a new handle
	// Local copy
	// Lock
	fh := fs.createHandle()
	fh.entry = entry
	fh.uploadOnDone = true
	//
	op.Handle = fh.ID
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

	fhi, ok := fs.fileHandles.Load(op.Handle)
	if !ok {
		return fuse.EIO
	}
	fh := fhi.(*handle)

	localFile := fh.entry.Cache(fs.Root)
	if localFile == nil {
		return fuse.EINVAL
	}
	_, err = localFile.WriteAt(op.Data, op.Offset)
	if err != nil {
		err = fuse.EIO
		return
	}
	fh.uploadOnDone = true

	return
}

// FlushFile just returns no error, maybe upload should be handled here
// COMMON
func (fs *BaseFS) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) (err error) {
	fhi, ok := fs.fileHandles.Load(op.Handle)
	if !ok {
		return fuse.EIO
	}
	fh := fhi.(*handle)

	if fh.entry.tempFile == nil {
		return
	}
	if fh.uploadOnDone { // or if content changed basically
		err = fh.entry.Sync(fs.Root)
		if err != nil {
			return fuseErr(err)
		}
	}
	return
}

// ReleaseFileHandle closes and deletes any temporary files, upload in case if changed locally
// COMMON
func (fs *BaseFS) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) (err error) {
	fhi, ok := fs.fileHandles.Load(op.Handle)
	if !ok {
		return nil
	}
	fh := fhi.(*handle)

	fh.entry.ClearCache()

	fs.fileHandles.Delete(op.Handle)

	return
}

// Unlink remove file and remove from local cache entry
// SPECIFIC
func (fs *BaseFS) Unlink(ctx context.Context, op *fuseops.UnlinkOp) (err error) {

	parentEntry := fs.Root.FindByInode(op.Parent)
	if parentEntry == nil {
		return fuse.ENOENT
	}

	fileEntry := fs.Root.Lookup(parentEntry, op.Name)
	if fileEntry == nil {
		return fuse.ENOATTR
	}
	err = fs.Root.DeleteFile(fileEntry)

	return fuseErr(err)
}

// MkDir creates a directory on a parent dir
func (fs *BaseFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) (err error) {

	parentFile := fs.Root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}

	entry, err := fs.Root.CreateFile(parentFile, op.Name, true)
	if err != nil {
		return fuseErr(err)
	}

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

	parentFile := fs.Root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}

	theFile := fs.Root.Lookup(parentFile, op.Name)

	err = fs.Root.DeleteFile(theFile)
	if err != nil {
		return fuseErr(err)
	}

	return
}

// Rename fuse implementation
func (fs *BaseFS) Rename(ctx context.Context, op *fuseops.RenameOp) (err error) {
	oldParentEntry := fs.Root.FindByInode(op.OldParent)
	if oldParentEntry == nil {
		return fuse.ENOENT
	}
	newParentEntry := fs.Root.FindByInode(op.NewParent)
	if newParentEntry == nil {
		return fuse.ENOENT
	}

	oldEntry := fs.Root.Lookup(oldParentEntry, op.OldName)
	if oldEntry == nil {
		return fuse.ENOENT
	}

	// Although GDrive allows duplicate names, there is some issue with inode caching
	// So we prevent a rename to a file with same name
	//existsFile := newParentFile.FindByName(op.NewName, false)
	existsEntry := fs.Root.Lookup(newParentEntry, op.NewName)
	if existsEntry != nil {
		return fuse.EEXIST
	}

	nFile, err := fs.Service.Move(oldEntry.File, newParentEntry.File, op.NewName)
	if err != nil {
		return fuseErr(err)
	}

	// Why remove and add instead of setting file, is just in case we have an
	// existing name FileEntry solves the name adding duplicates helpers
	fs.Root.RemoveEntry(oldEntry)
	fs.Root.FileEntry(nFile, oldEntry.Inode) // Use this same inode

	return

}

func fuseErr(err error) error {
	switch err {
	case ErrPermission:
		return syscall.EPERM
	case ErrNotImplemented:
		return fuse.ENOSYS
	case nil:
		return nil
	default:
		return fuse.EINVAL
	}
}
