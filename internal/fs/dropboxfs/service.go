package dropboxfs

import (
	"io"
	"os"
	"strings"
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	dbfiles "github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
)

// Service basefs Service implementation
type Service struct {
	dbconfig dropbox.Config
}

func (s *Service) Changes() error {
	log.Println("Checking changes")
	fileService := dbfiles.New(s.dbconfig)
	if fileService == nil {
		log.Println("File service is nill")
		return nil
	}

	res, err := fileService.ListFolderLongpoll(dbfiles.NewListFolderLongpollArg(""))
	if err != nil {
		log.Println("Err in longpoll", err)
		return nil
	}
	log.Println("Has Changes:", res.Changes)
	log.Println("Backoff:", res.Backoff)

	return nil
}

// ListAll implementation
func (s *Service) ListAll() ([]*basefs.File, error) {
	fileService := dbfiles.New(s.dbconfig)
	// Some how list all files from Dropbox
	log.Println("Loading meta data")
	ret := []*basefs.File{}
	var err error
	var res *dbfiles.ListFolderResult

	res, err = fileService.ListFolder(&dbfiles.ListFolderArg{Recursive: true, Path: "/test", IncludeDeleted: false, IncludeMediaInfo: false})
	if err != nil {
		log.Println("Error listing:", err)
		return nil, err
	}
	log.Println("Loaded: res.Entries", len(res.Entries))
	for _, e := range res.Entries {
		ret = append(ret, File(e))
	}

	for res.HasMore {
		res, err = fileService.ListFolderContinue(&dbfiles.ListFolderContinueArg{Cursor: res.Cursor})
		log.Println("Loaded: res.Entries", len(res.Entries))
		for _, e := range res.Entries {
			ret = append(ret, File(e))
		}
	}

	return ret, nil
}

// Create file implementation
func (s *Service) Create(parent *basefs.File, name string, isDir bool) (*basefs.File, error) {
	fileService := dbfiles.New(s.dbconfig)

	if isDir {
		data, err := fileService.CreateFolder(&dbfiles.CreateFolderArg{
			Autorename: false,
			Path:       parent.ID + "/" + name,
		})
		if err != nil {
			return nil, err
		}
		return File(data), nil
	}
	newPath := parent.ID + "/" + name
	// For file we create Local entry but do nothing, since dropbox does not upload local empty files
	metadata := &dbfiles.FileMetadata{}

	metadata.Id = newPath
	metadata.Name = name
	metadata.ServerModified = time.Now().UTC()
	metadata.PathLower = strings.ToLower(newPath)

	return File(metadata), nil
}

// Upload file implementation
func (s *Service) Upload(reader io.Reader, file *basefs.File) (*basefs.File, error) {
	fileService := dbfiles.New(s.dbconfig)

	data, err := fileService.Upload(&dbfiles.CommitInfo{
		Path:       file.ID, // ???
		Autorename: false,
		Mode:       &dbfiles.WriteMode{Tagged: dropbox.Tagged{Tag: dbfiles.WriteModeOverwrite}},
		//ClientModified: time.Now().UTC(),
	}, reader.(io.Reader))
	if err != nil {
		log.Println("Upload Error:", err)
		return nil, err
	}

	return File(data), nil
}

// DownloadTo implementation
func (s *Service) DownloadTo(w io.Writer, file *basefs.File) error {
	fileService := dbfiles.New(s.dbconfig)

	_, content, err := fileService.Download(&dbfiles.DownloadArg{Path: file.ID})
	if err != nil {
		return err
	}

	defer content.Close()
	io.Copy(w, content)

	return nil
}

// Move and Rename file implementation
func (s *Service) Move(file *basefs.File, newParent *basefs.File, name string) (*basefs.File, error) {
	fileService := dbfiles.New(s.dbconfig)

	res, err := fileService.Move(&dbfiles.RelocationArg{
		RelocationPath: dbfiles.RelocationPath{
			FromPath: file.ID,
			ToPath:   newParent.ID + "/" + name,
		},
	})
	if err != nil {
		return nil, err
	}

	return File(res), nil
}

// Delete deletes a file entry (including Dir)
func (s *Service) Delete(file *basefs.File) error {
	fileService := dbfiles.New(s.dbconfig)

	_, err := fileService.Delete(&dbfiles.DeleteArg{Path: file.ID})
	if err != nil {
		return err
	}
	return nil
}

// File Metadata to File Converter
func File(metadata dbfiles.IsMetadata) *basefs.File {

	var ID string
	var name string
	var modifiedTime time.Time
	var mode = os.FileMode(0644)
	var size uint64

	//var parentID string

	switch t := metadata.(type) {
	case *dbfiles.FileMetadata:
		ID = t.PathLower
		name = t.Name
		modifiedTime = t.ServerModified
		size = t.Size
	case *dbfiles.FolderMetadata:
		ID = t.PathLower
		name = t.Name
		mode = os.FileMode(0755) | os.ModeDir
		modifiedTime = time.Now()
		//parentID = t.SharedFolderId
	}

	createdTime := modifiedTime

	//log.Println("ParentID:", parentID)

	pathParts := strings.Split(ID, "/")
	parentID := strings.Join(pathParts[:len(pathParts)-1], "/")
	parents := []string{}
	if parentID != "" {
		parents = []string{parentID}
	}

	file := &basefs.File{
		ID:           ID,
		Name:         name,
		Parents:      parents,
		Size:         size,
		CreatedTime:  createdTime,
		ModifiedTime: modifiedTime,
		AccessedTime: modifiedTime,
		Mode:         mode,
	}
	//log.Println("FileID is:", file.ID)

	return file
}
