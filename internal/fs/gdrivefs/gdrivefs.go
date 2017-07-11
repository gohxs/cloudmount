// gdrivemount implements a google drive fuse driver
package gdrivefs

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
	log = prettylog.New("gdrivefs")
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

	config        *core.Config //core   *core.Core // Core Config instead?
	serviceConfig *Config
	client        *drive.Service
	//root   *FileEntry // hiearchy reference
	root *FileContainer

	fileHandles map[fuseops.HandleID]*Handle
	fileEntries map[fuseops.InodeID]*FileEntry

	nextRefresh time.Time

	handleMU *sync.Mutex
	//fileMap map[string]
	// Map IDS with FileEntries
}

func New(core *core.Core) core.Driver {

	fs := &GDriveFS{
		config:        &core.Config,
		serviceConfig: &Config{},
		fileHandles:   map[fuseops.HandleID]*Handle{},
		handleMU:      &sync.Mutex{},
	}
	fs.initClient() // Init Oauth2 client
	fs.root = NewFileContainer(fs)
	fs.root.uid = core.Config.UID
	fs.root.gid = core.Config.GID

	//fs.root = rootEntry

	// Temporary entry
	entry := fs.root.FileEntry(&drive.File{Id: "0", Name: "Loading..."}, 9999)
	entry.Attr.Mode = os.FileMode(0)

	return fs
}

// Async
func (fs *GDriveFS) Start() {
	go func() {
		fs.Refresh() // First load

		// Change reader loop
		startPageTokenRes, err := fs.client.Changes.GetStartPageToken().Do()
		if err != nil {
			log.Println("GDrive err", err)
		}
		savedStartPageToken := startPageTokenRes.StartPageToken
		for {
			pageToken := savedStartPageToken
			for pageToken != "" {
				changesRes, err := fs.client.Changes.List(pageToken).Fields(googleapi.Field("newStartPageToken,nextPageToken,changes(removed,fileId,file(" + fileFields + "))")).Do()
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
		}
	}()
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

const fileFields = googleapi.Field("id, name, size,mimeType, parents,createdTime,modifiedTime")
const gdFields = googleapi.Field("files(" + fileFields + ")")

// FULL Refresh service files
func (fs *GDriveFS) Refresh() {
	fs.nextRefresh = time.Now().Add(1 * time.Minute)

	fileList := []*drive.File{}
	fileMap := map[string]*drive.File{} // Temporary map by google drive fileID

	r, err := fs.client.Files.List().
		OrderBy("createdTime").
		PageSize(1000).
		SupportsTeamDrives(true).
		IncludeTeamDriveItems(true).
		Fields(googleapi.Field("nextPageToken"), gdFields).
		Do()
	if err != nil {
		// Sometimes gdrive returns error 500 randomly
		log.Println("GDrive ERR:", err)
		fs.Refresh() // retry
		return
	}
	fileList = append(fileList, r.Files...)

	// Rest of the pages
	for r.NextPageToken != "" {
		r, err = fs.client.Files.List().
			OrderBy("createdTime").
			PageToken(r.NextPageToken).
			Fields(googleapi.Field("nextPageToken"), gdFields).
			Do()
		if err != nil {
			log.Println("GDrive ERR:", err)
			fs.Refresh() // retry // Same as above
			return
		}
		fileList = append(fileList, r.Files...)
	}
	log.Println("Total entries:", len(fileList))

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
	var appendFile func(gfile *drive.File)
	appendFile = func(gfile *drive.File) {
		for _, pID := range gfile.Parents {
			parentFile, ok := fileMap[pID]
			if !ok {
				parentFile, err = fs.client.Files.Get(pID).Do()
				if err != nil {
					log.Println("Error fetching single file:", err)
				}
				fileMap[parentFile.Id] = parentFile
			}
			appendFile(parentFile) // Recurse
		}

		// Find existing entry
		entry := fs.root.FindByGID(gfile.Id)
		// Store for later add
		if entry == nil {
			entry = fs.root.FileEntry(gfile) // Add New and retrieve
		}
		root.AddEntry(entry)
		// add File
	}

	for _, f := range fileList { // Ordered
		appendFile(f) // Check parent first
	}

	log.Println("Refresh done, update root")
	fs.root = root
	//fs.root.children = root.children

	log.Println("File count:", len(root.fileEntries))
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
		children := fs.root.ListByParentGID(fh.entry.GID)

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
func (fs *GDriveFS) SetInodeAttributes(ctx context.Context, op *fuseops.SetInodeAttributesOp) (err error) {

	// Hack to truncate file?

	if op.Size != nil {
		f := fs.root.FindByInode(op.Inode)

		if *op.Size != 0 { // We only allow truncate to 0
			return fuse.ENOSYS
		}
		// Delete and create another on truncate 0
		err = fs.client.Files.Delete(f.GFile.Id).Do() // XXX: Careful on this
		createdFile, err := fs.client.Files.Create(&drive.File{Parents: f.GFile.Parents, Name: f.GFile.Name}).Fields(fileFields).Do()
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

	entry := fs.root.LookupByGID(parentFile.GID, op.Name)

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

	existsFile := fs.root.LookupByGID(parentFile.GID, op.Name)
	//existsFile := parentFile.FindByName(op.Name, false)
	if existsFile != nil {
		return fuse.EEXIST
	}

	newGFile := &drive.File{
		Parents: []string{parentFile.GFile.Id},
		Name:    op.Name,
	}

	createdGFile, err := fs.client.Files.Create(newGFile).Fields(fileFields).Do()
	if err != nil {
		err = fuse.EINVAL
		return
	}
	entry := fs.root.FileEntry(createdGFile) // New Entry added // Or Return same?

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

	fileEntry := fs.root.LookupByGID(parentEntry.GID, op.Name)
	//fileEntry := parentEntry.FindByName(op.Name, false)
	if fileEntry == nil {
		return fuse.ENOATTR
	}
	err = fs.client.Files.Delete(fileEntry.GFile.Id).Do()
	if err != nil {
		return fuse.EIO
	}

	fs.root.RemoveEntry(fileEntry)
	//parentEntry.RemoveChild(fileEntry)

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
	createdGFile, err := fs.client.Files.Create(&drive.File{
		Parents:  []string{parentFile.GFile.Id},
		MimeType: "application/vnd.google-apps.folder",
		Name:     op.Name,
	}).Fields(fileFields).Do()
	if err != nil {
		return fuse.ENOATTR
	}
	entry := fs.root.FileEntry(createdGFile)
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
func (fs *GDriveFS) RmDir(ctx context.Context, op *fuseops.RmDirOp) (err error) {

	parentFile := fs.root.FindByInode(op.Parent)
	if parentFile == nil {
		return fuse.ENOENT
	}
	if parentFile.Inode == fuseops.RootInodeID {
		return syscall.EPERM
	}

	theFile := fs.root.LookupByGID(parentFile.GID, op.Name)
	//theFile := parentFile.FindByName(op.Name, false)

	err = fs.client.Files.Delete(theFile.GFile.Id).Do()
	if err != nil {
		return fuse.ENOTEMPTY
	}
	fs.root.RemoveEntry(theFile)

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

	ngFile := &drive.File{
		Name: op.NewName,
	}

	updateCall := fs.client.Files.Update(oldEntry.GID, ngFile).Fields(fileFields)
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
