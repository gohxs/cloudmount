// gdrivemount implements a google drive fuse driver
package gdrivefs

import (
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/cloudfs"
	"dev.hexasoftware.com/hxs/prettylog"

	"golang.org/x/net/context"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseops"
	"github.com/jacobsa/fuse/fuseutil"
)

var (
	log = prettylog.New("gdrivemount")
)

type Handle struct {
	ID           fuseops.HandleID
	entry        *FileEntry
	uploadOnDone bool
	// Handling for dir
	entries []fuseutil.Dirent
}

// GDriveFS
type GDriveFS struct {
	fuseutil.NotImplementedFileSystem // Defaults

	core   *cloudfs.Core // Core Config instead?
	client *drive.Service
	//root   *FileEntry // hiearchy reference
	root *FileContainer

	fileHandles map[fuseops.HandleID]*Handle
	fileEntries map[fuseops.InodeID]*FileEntry

	nextRefresh time.Time

	handleMU *sync.Mutex
	//fileMap map[string]
	// Map IDS with FileEntries
}

func New(core *cloudfs.Core) cloudfs.Driver {

	fs := &GDriveFS{
		core:        core,
		fileHandles: map[fuseops.HandleID]*Handle{},
		handleMU:    &sync.Mutex{},
	}
	fs.initClient() // Init Oauth2 client
	fs.root = NewFileContainer(fs)
	fs.root.uid = core.Config.UID
	fs.root.gid = core.Config.GID

	rootEntry := fs.root.FileEntry(fuseops.RootInodeID)

	rootEntry.Attr = fuseops.InodeAttributes{
		Mode: os.FileMode(0755) | os.ModeDir,
		Uid:  core.Config.UID,
		Gid:  core.Config.GID,
	}
	rootEntry.isDir = true

	//fs.root = rootEntry

	// Temporary entry
	entry := fs.root.tree.AppendGFile(&drive.File{Id: "0", Name: "Loading..."}, 999999)
	entry.Attr.Mode = os.FileMode(0)

	return fs
}

func (fs *GDriveFS) Start() {
	fs.timedRefresh() // start synching
}

////////////////////////////////////////////////////////
// TOOLS & HELPERS
////////////////////////////////////////////////////////

func (fs *GDriveFS) createHandle() *Handle {
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

// Cache somewhere?
/*func (fs *GDriveFS) getUID() uint32 {
	uid, _ := strconv.Atoi(fs.osuser.Uid)
	return uint32(uid)
}
func (fs *GDriveFS) getGID() uint32 {
	gid, _ := strconv.Atoi(fs.osuser.Gid)
	return uint32(gid)
}*/

func (fs *GDriveFS) timedRefresh() {

	go func() {
		for {
			if time.Now().After(fs.nextRefresh) {
				fs.Refresh()
			}
			time.Sleep(2 * time.Minute) // 2 minutes
		}
	}()

}

// Refresh service files
func (fs *GDriveFS) Refresh() {
	fs.nextRefresh = time.Now().Add(1 * time.Minute)

	fileList := []*drive.File{}
	fileMap := map[string]*drive.File{} // Temporary map by google drive fileID

	gdFields := googleapi.Field("nextPageToken, files(id,name,size,quotaBytesUsed, mimeType,parents,createdTime,modifiedTime)")
	log.Println("Loading file entries from gdrive")
	r, err := fs.client.Files.List().
		OrderBy("createdTime").
		PageSize(1000).
		SupportsTeamDrives(true).
		IncludeTeamDriveItems(true).
		Fields(gdFields).
		Do()
	if err != nil {
		log.Println("GDrive ERR:", err)
		return
	}
	fileList = append(fileList, r.Files...)

	// Rest of the pages
	for r.NextPageToken != "" {
		r, err = fs.client.Files.List().
			OrderBy("createdTime").
			PageToken(r.NextPageToken).
			Fields(gdFields).
			Do()
		if err != nil {
			log.Println("GDrive ERR:", err)
			return
		}
		fileList = append(fileList, r.Files...)
	}
	log.Println("Total entries:", len(fileList))

	// TimeSort
	/*log.Println("Sort by time")
	sort.Slice(fileList, func(i, j int) bool {
		createdTimeI, _ := time.Parse(time.RFC3339, fileList[i].CreatedTime)
		createdTimeJ, _ := time.Parse(time.RFC3339, fileList[i].CreatedTime)
		if createdTimeI.Before(createdTimeJ) {
			return true
		}
		return false
	})*/

	// Cache ID for faster retrieval, might not be necessary
	for _, f := range fileList {
		fileMap[f.Id] = f
	}

	if err != nil || r == nil {
		log.Println("Unable to retrieve files", err)
		return
	}

	// Create clean fileList
	root := NewFileContainer(fs)

	// Helper func to recurse
	// Everything loaded we add to our entries
	// Add file and its parents priorizing it parent
	var appendFile func(df *drive.File)
	appendFile = func(df *drive.File) {
		for _, pID := range df.Parents {
			parentFile, ok := fileMap[pID]
			if !ok {
				parentFile, err = fs.client.Files.Get(pID).Do()
				if err != nil {
					panic(err)
				}
				fileMap[parentFile.Id] = parentFile
			}
			appendFile(parentFile) // Recurse
		}

		// Find existing entry
		var inode fuseops.InodeID
		entry := fs.root.tree.FindByGID(df.Id, true)
		if entry == nil {
			inode = root.FileEntry().Inode // This can be a problem if next time a existing inode comes? Allocate new file entry with new Inode
		} else {
			inode = entry.Inode //
		}

		newEntry := fs.root.tree.solveAppendGFile(df, inode) // Find right parent
		if entry != nil && entry.GFile.Name == df.Name {     // Copy name from old entry
			newEntry.Name = entry.Name
		}

		// add File
	}

	for _, f := range fileList { // Ordered
		appendFile(f) // Check parent first
	}

	log.Println("Refresh done, update root")
	//fs.root.children = root.children

	log.Println("File count:", fs.root.tree.Count())
}

///////////////////////////////
// Fuse operations
////////////

// OpenDir return nil error allows open dir
func (fs *GDriveFS) OpenDir(ctx context.Context, op *fuseops.OpenDirOp) (err error) {

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
func (fs *GDriveFS) ReadDir(ctx context.Context, op *fuseops.ReadDirOp) (err error) {
	fh, ok := fs.fileHandles[op.Handle]
	if !ok {
		log.Fatal("Handle does not exists")
	}

	if op.Offset == 0 { // Rebuild/rewind dir list
		fh.entries = []fuseutil.Dirent{}

		for i, v := range fh.entry.children {
			fusetype := fuseutil.DT_File
			if v.isDir {
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
		//log.Println("Written:", n)
		if n == 0 {
			break
		}
		op.BytesRead += n
	}
	return
}

// SetInodeAttributes Not sure what attributes gdrive support we just leave this blank for now
func (fs *GDriveFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) (err error) {

	// Hack to truncate file?

	if op.Size != nil {
		f := fs.root.FindByInode(op.Inode)

		if *op.Size != 0 { // We only allow truncate to 0
			return fuse.ENOSYS
		}
		// Delete and create another on truncate 0
		err = fs.client.Files.Delete(f.GFile.Id).Do() // XXX: Careful on this
		createdFile, err := fs.client.Files.Create(&drive.File{Parents: f.GFile.Parents, Name: f.GFile.Name}).Do()
		if err != nil {
			return fuse.EINVAL
		}
		f.SetGFile(createdFile) // Set new file
	}

	return
}

//GetInodeAttributes return attributes
func (fs *GDriveFS) GetInodeAttributes(ctx context.Context, op *fuseops.GetInodeAttributesOp) (err error) {

	f := fs.root.FindByInode(op.Inode)
	if f == nil {
		return fuse.ENOENT
	}
	op.Attributes = f.Attr
	op.AttributesExpiration = time.Now().Add(time.Minute)

	return
}

// ReleaseDirHandle deletes file handle entry
func (fs *GDriveFS) ReleaseDirHandle(ctx context.Context, op *fuseops.ReleaseDirHandleOp) (err error) {
	delete(fs.fileHandles, op.Handle)
	return
}

// LookUpInode based on Parent and Name we return a self cached inode
func (fs *GDriveFS) LookUpInode(ctx context.Context, op *fuseops.LookUpInodeOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent) // true means transverse all
	if parentFile == nil {
		return fuse.ENOENT
	}

	now := time.Now()
	// Transverse only local
	f := parentFile.FindByName(op.Name, false)
	if f == nil {
		return fuse.ENOENT
	}

	op.Entry = fuseops.ChildInodeEntry{
		Attributes:           f.Attr,
		Child:                f.Inode,
		AttributesExpiration: now.Add(time.Second),
		EntryExpiration:      now.Add(time.Second),
	}
	return
}

// StatFS basically allows StatFS to run
/*func (fs *GDriveFS) StatFS(ctx context.Context, op *fuseops.StatFSOp) (err error) {
	return
}*/

// ForgetInode allows to forgetInode
func (fs *GDriveFS) ForgetInode(ctx context.Context, op *fuseops.ForgetInodeOp) (err error) {
	return
}

// GetXAttr special attributes
func (fs *GDriveFS) GetXAttr(ctx context.Context, op *fuseops.GetXattrOp) (err error) {
	return
}

//////////////////////////////////////////////////////////////////////////
// File OPS
//////////////////////////////////////////////////////////////////////////

// OpenFile creates a temporary handle to be handled on read or write
func (fs *GDriveFS) OpenFile(ctx context.Context, op *fuseops.OpenFileOp) (err error) {
	f := fs.root.FindByInode(op.Inode) // might not exists

	// Generate new handle
	handle := fs.createHandle()
	handle.entry = f

	op.Handle = handle.ID
	op.UseDirectIO = true

	return
}

// ReadFile  if the first time we download the google drive file into a local temporary file
func (fs *GDriveFS) ReadFile(ctx context.Context, op *fuseops.ReadFileOp) (err error) {
	handle := fs.fileHandles[op.Handle]

	localFile := handle.entry.Cache()
	op.BytesRead, err = localFile.ReadAt(op.Dst, op.Offset)
	if err == io.EOF { // fuse does not expect a EOF
		err = nil
	}

	return
}

// CreateFile creates empty file in google Drive and returns its ID and attributes, only allows file creation on 'My Drive'
func (fs *GDriveFS) CreateFile(ctx context.Context, op *fuseops.CreateFileOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	// Only write on child folders
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	existsFile := parentFile.FindByName(op.Name, false)
	if existsFile != nil {
		return fuse.EEXIST
	}

	newFile := &drive.File{
		Parents: []string{parentFile.GFile.Id},
		Name:    op.Name,
	}

	createdFile, err := fs.client.Files.Create(newFile).Do()
	if err != nil {
		err = fuse.EINVAL
		return
	}

	// Temp
	entry := fs.root.FileEntry()

	entry = parentFile.AppendGFile(createdFile, entry.Inode) // Add new created file
	if entry == nil {
		err = fuse.EINVAL
		return
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
func (fs *GDriveFS) WriteFile(ctx context.Context, op *fuseops.WriteFileOp) (err error) {
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
func (fs *GDriveFS) FlushFile(ctx context.Context, op *fuseops.FlushFileOp) (err error) {
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
func (fs *GDriveFS) ReleaseFileHandle(ctx context.Context, op *fuseops.ReleaseFileHandleOp) (err error) {
	handle := fs.fileHandles[op.Handle]

	handle.entry.ClearCache()

	delete(fs.fileHandles, op.Handle)

	return
}

// Unlink remove file and remove from local cache entry
func (fs *GDriveFS) Unlink(ctx context.Context, op *fuseops.UnlinkOp) (err error) {
	parentEntry := fs.root.FindByInode(op.Parent)
	if parentEntry == nil {
		return fuse.ENOENT
	}
	if parentEntry.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	fileEntry := parentEntry.FindByName(op.Name, false)
	if fileEntry == nil {
		return fuse.ENOATTR
	}
	err = fs.client.Files.Delete(fileEntry.GFile.Id).Do()
	if err != nil {
		return fuse.EIO
	}

	fs.root.RemoveEntry(fileEntry)
	parentEntry.RemoveChild(fileEntry)

	return
}

// MkDir creates a directory on a parent dir
func (fs *GDriveFS) MkDir(ctx context.Context, op *fuseops.MkDirOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	// Should check existent first too
	fi, err := fs.client.Files.Create(&drive.File{
		Parents:  []string{parentFile.GFile.Id},
		MimeType: "application/vnd.google-apps.folder",
		Name:     op.Name,
	}).Do()
	if err != nil {
		return fuse.ENOATTR
	}
	entry := fs.root.FileEntry()
	entry = parentFile.AppendGFile(fi, entry.Inode)
	if entry == nil {
		return fuse.EINVAL
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
func (fs *GDriveFS) RmDir(ctx context.Context, op *fuseops.RmDirOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	theFile := parentFile.FindByName(op.Name, false)

	err = fs.client.Files.Delete(theFile.GFile.Id).Do()
	if err != nil {
		return fuse.ENOTEMPTY
	}

	parentFile.RemoveChild(theFile)

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

	oldFile := oldParentFile.FindByName(op.OldName, false)

	// Although GDrive allows duplicate names, there is some issue with inode caching
	// So we prevent a rename to a file with same name
	existsFile := newParentFile.FindByName(op.NewName, false)
	if existsFile != nil {
		return fuse.EEXIST
	}

	ngFile := &drive.File{
		Name: op.NewName,
	}

	updateCall := fs.client.Files.Update(oldFile.GFile.Id, ngFile)
	if oldParentFile != newParentFile {
		updateCall.RemoveParents(oldParentFile.GFile.Id)
		updateCall.AddParents(newParentFile.GFile.Id)
	}
	updatedFile, err := updateCall.Do()

	oldParentFile.RemoveChild(oldFile)
	newParentFile.AppendGFile(updatedFile, oldFile.Inode)

	return

}
