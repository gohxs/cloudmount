package basefs

import (
	"fmt"
	"io"
	"os"
	"strings"
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
	log = prettylog.New("commonfs")
)

// BaseFS struct
type BaseFS struct {
	fuseutil.NotImplementedFileSystem // Defaults

	Config *core.Config //core   *core.Core // Core Config instead?
	//serviceConfig                     *Config
	Client *drive.Service

	fileHandles map[fuseops.HandleID]*Handle
	fileEntries map[fuseops.InodeID]*FileEntry

	//nextRefresh time.Time
	handleMU *sync.Mutex
	inodeMU  *sync.Mutex
	//fileMap map[string]
	// Map IDS with FileEntries
}

func New(core *core.Core) *BaseFS {

	fs := &BaseFS{
		Config: &core.Config,
		//serviceConfig: &Config{}, // This is on service Driver
		fileHandles: map[fuseops.HandleID]*Handle{},
		handleMU:    &sync.Mutex{},
	}
	// Temporary entry
	entry := fs.fileEntry("Loading...", nil, 9999)
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

	entry := fs.findByInode(op.Inode)
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

		children := fs.listByParent(fh.entry)

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

	f := fs.findByInode(op.Inode)
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

	parentFile := fs.findByInode(op.Parent) // true means transverse all
	if parentFile == nil {
		return fuse.ENOENT
	}

	entry := fs.lookupByParent(parentFile, op.Name)

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
	f := fs.findByInode(op.Inode) // might not exists

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

	parentFile := fs.findByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	// Only write on child folders
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	existsFile := fs.lookupByParent(parentFile, op.Name)
	//existsFile := parentFile.FindByName(op.Name, false)
	if existsFile != nil {
		return fuse.EEXIST
	}

	// Parent entry/Name
	entry, err := fs.createFile(parentFile, op.Name, false)
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
	parentEntry := fs.findByInode(op.Parent)
	if parentEntry == nil {
		return fuse.ENOENT
	}
	if parentEntry.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	fileEntry := fs.lookupByParent(parentEntry, op.Name)
	//fileEntry := parentEntry.FindByName(op.Name, false)
	if fileEntry == nil {
		return fuse.ENOATTR
	}
	return fs.deleteFile(fileEntry)
}

// MkDir creates a directory on a parent dir
func (fs *BaseFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) (err error) {

	parentFile := fs.findByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	entry, err := fs.createFile(parentFile, op.Name, true)
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

	parentFile := fs.findByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	theFile := fs.lookupByParent(parentFile, op.Name)
	//theFile := parentFile.FindByName(op.Name, false)

	err = fs.deleteFile(theFile)
	//err = fs.Client.Files.Delete(theFile.GFile.Id).Do()
	if err != nil {
		return fuse.ENOTEMPTY
	}

	//parentFile.RemoveChild(theFile)

	// Remove from entry somehow

	return
}

// Rename fuse implementation
func (fs *BaseFS) Rename(ctx context.Context, op *fuseops.RenameOp) (err error) {
	oldParentFile := fs.findByInode(op.OldParent)
	if oldParentFile == nil {
		return fuse.ENOENT
	}
	newParentFile := fs.findByInode(op.NewParent)
	if newParentFile == nil {
		return fuse.ENOENT
	}

	if oldParentFile.Inode == fuseops.RootInodeID || newParentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	//oldFile := oldParentFile.FindByName(op.OldName, false)
	oldEntry := fs.lookupByParent(oldParentFile, op.OldName)

	// Although GDrive allows duplicate names, there is some issue with inode caching
	// So we prevent a rename to a file with same name
	//existsFile := newParentFile.FindByName(op.NewName, false)
	existsEntry := fs.lookupByGID(newParentFile.GID, op.NewName)
	if existsEntry != nil {
		return fuse.EEXIST
	}

	// Rename somehow
	ngFile := &drive.File{
		Name: op.NewName,
	}

	updateCall := fs.Client.Files.Update(oldEntry.GID, ngFile).Fields(fileFields)
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

func (fs *BaseFS) findByInode(inode fuseops.InodeID) *FileEntry {
	return fs.fileEntries[inode]
}

// GID specific functions
func (fs *BaseFS) findByGID(gid string) *FileEntry {
	for _, v := range fs.fileEntries {
		if v.GFile != nil && v.GFile.Id == gid {
			return v
		}
	}
	return nil
}
func (fs *BaseFS) lookupByGID(parentGID string, name string) *FileEntry {
	for _, entry := range fs.fileEntries {
		if entry.HasParentGID(parentGID) && entry.Name == name {
			return entry
		}
	}
	return nil
}

func (fs *BaseFS) listByParent(parent *FileEntry) []*FileEntry {
	ret := []*FileEntry{}
	for _, entry := range fs.fileEntries {
		if entry.HasParentGID(parent.GID) {
			ret = append(ret, entry)
		}
	}
	return ret
}
func (fs *BaseFS) lookupByParent(parent *FileEntry, name string) *FileEntry {
	for _, entry := range fs.fileEntries {
		if entry.HasParentGID(parent.GID) && entry.Name == name {
			return entry
		}
	}
	return nil

}

func (fs *BaseFS) createFile(parentFile *FileEntry, name string, isDir bool) (*FileEntry, error) {

	newGFile := &drive.File{
		Parents: []string{parentFile.GFile.Id},
		Name:    name,
	}
	if isDir {
		newGFile.MimeType = "application/vnd.google-apps.folder"
	}
	// Could be transformed to CreateFile in continer
	// InDriver
	createdGFile, err := fs.Client.Files.Create(newGFile).Fields(fileFields).Do()
	if err != nil {
		return nil, fuse.EINVAL
	}
	entry := fs.fileEntry(createdGFile.Name, createdGFile) // New Entry added // Or Return same?

	return entry, nil
}

func (fs *BaseFS) deleteFile(entry *FileEntry) error {
	err := fs.Client.Files.Delete(entry.GFile.Id).Do()
	if err != nil {
		return fuse.EIO
	}

	fs.removeEntry(entry)
	return nil
}

//////////////

//Return or create inode // Pass name maybe?
func (fs *BaseFS) fileEntry(aname string, gfile *drive.File, inodeOps ...fuseops.InodeID) *FileEntry {

	fs.inodeMU.Lock()
	defer fs.inodeMU.Unlock()

	var inode fuseops.InodeID
	if len(inodeOps) > 0 {
		inode = inodeOps[0]
		if fe, ok := fs.fileEntries[inode]; ok {
			return fe
		}
	} else { // generate new inode
		// Max Inode Number
		for inode = 2; inode < 99999; inode++ {
			_, ok := fs.fileEntries[inode]
			if !ok {
				break
			}
		}
	}

	name := ""
	if gfile != nil {
		name = aname
		count := 1
		nameParts := strings.Split(name, ".")
		for {
			// We find if we have a GFile in same parent with same name
			var entry *FileEntry
			for _, p := range gfile.Parents {
				entry = fs.lookupByGID(p, name)
				if entry != nil {
					break
				}
			}
			if entry == nil { // Not found return
				break
			}
			count++
			if len(nameParts) > 1 {
				name = fmt.Sprintf("%s(%d).%s", nameParts[0], count, strings.Join(nameParts[1:], "."))
			} else {
				name = fmt.Sprintf("%s(%d)", nameParts[0], count)
			}
			log.Printf("Conflicting name generated new '%s' as '%s'", gfile.Name, name)
		}
	}

	fe := &FileEntry{
		GFile: gfile,
		Inode: inode,
		fs:    fs,
		Name:  name,
		//children:  []*FileEntry{},
		Attr: fuseops.InodeAttributes{
			Uid: fs.Config.UID,
			Gid: fs.Config.GID,
		},
	}
	fe.SetGFile(gfile)

	fs.fileEntries[inode] = fe

	return fe
}

/*func (fs *BaseFS) addEntry(entry *FileEntry) {
	fc.fileEntries[entry.Inode] = entry
}*/

// RemoveEntry remove file entry
func (fs *BaseFS) removeEntry(entry *FileEntry) {
	var inode fuseops.InodeID
	for k, e := range fs.fileEntries {
		if e == entry {
			inode = k
		}
	}
	delete(fs.fileEntries, inode)
}
