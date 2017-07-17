package dropboxfs

import (
	"bytes"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"github.com/dropbox/dropbox-sdk-go-unofficial/dropbox"
	dbfiles "github.com/dropbox/dropbox-sdk-go-unofficial/dropbox/files"
	"github.com/gohxs/cloudmount/internal/core"
	"github.com/gohxs/cloudmount/internal/coreutil"
	"github.com/gohxs/cloudmount/internal/fs/basefs"
	"github.com/gohxs/cloudmount/internal/oauth2util"
)

// Service basefs Service implementation
type Service struct {
	dbconfig    dropbox.Config
	savedCursor string
}

func NewService(coreConfig *core.Config) *Service {

	serviceConfig := Config{}
	log.Println("Initializing dropbox service")
	log.Println("Source config:", coreConfig.Source)

	err := coreutil.ParseConfig(coreConfig.Source, &serviceConfig)
	if err != nil {
		errlog.Fatalf("Unable to read <source>: %v", err)
	}
	config := &oauth2.Config{
		ClientID:     serviceConfig.ClientSecret.ClientID,
		ClientSecret: serviceConfig.ClientSecret.ClientSecret,
		RedirectURL:  "",
		Scopes:       []string{},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://www.dropbox.com/1/oauth2/authorize",
			TokenURL: "https://api.dropbox.com/1/oauth2/token",
		},
	}
	if serviceConfig.Auth == nil {
		tok := oauth2util.GetTokenFromWeb(config)
		serviceConfig.Auth = tok
		coreutil.SaveConfig(coreConfig.Source, &serviceConfig)
	}

	dbconfig := dropbox.Config{Token: serviceConfig.Auth.AccessToken}

	return &Service{dbconfig: dbconfig}

}

// Changes dropbox longpool changes
func (s *Service) Changes() ([]*basefs.Change, error) {
	fileService := dbfiles.New(s.dbconfig)
	if fileService == nil {
		log.Println("File service is nill")
		return nil, nil
	}

	if s.savedCursor == "" {
		res, err := fileService.ListFolderGetLatestCursor(&dbfiles.ListFolderArg{Path: "", Recursive: true})
		if err != nil {
			log.Println("Err:", err)
			return nil, err
		}
		s.savedCursor = res.Cursor
	}

	res, err := fileService.ListFolderLongpoll(dbfiles.NewListFolderLongpollArg(s.savedCursor))
	if err != nil {
		log.Println("Err in longpoll", err)
		return nil, err
	}

	if res.Changes == false {
		return nil, nil
	}

	ret := []*basefs.Change{}
	for {
		res, err := fileService.ListFolderContinue(dbfiles.NewListFolderContinueArg(s.savedCursor))
		if err != nil {
			return nil, err
		}
		for _, e := range res.Entries {
			var change *basefs.Change
			switch t := e.(type) {
			case *dbfiles.DeletedMetadata:
				change = &basefs.Change{t.PathLower, File(t), true}
			case *dbfiles.FileMetadata:
				change = &basefs.Change{t.PathLower, File(t), false}
			case *dbfiles.FolderMetadata:
				change = &basefs.Change{t.PathLower, File(t), false}
			}
			ret = append(ret, change)

		}
		if !res.HasMore {
			break
		}
	}
	{
		// Store new token
		res, err := fileService.ListFolderGetLatestCursor(&dbfiles.ListFolderArg{Path: "", Recursive: true})
		if err != nil {
			log.Println("Err:", err)
			return nil, err
		}
		s.savedCursor = res.Cursor
	}

	return ret, nil
}

// ListAll implementation
func (s *Service) ListAll() ([]*basefs.File, error) {
	fileService := dbfiles.New(s.dbconfig)
	// Some how list all files from Dropbox
	log.Println("Loading meta data")
	ret := []*basefs.File{}
	var err error
	var res *dbfiles.ListFolderResult

	res, err = fileService.ListFolder(&dbfiles.ListFolderArg{Recursive: true, Path: "", IncludeDeleted: false, IncludeMediaInfo: false})
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

	parentID := ""
	if parent != nil {
		parentID = parent.ID
	}
	if isDir {
		data, err := fileService.CreateFolder(&dbfiles.CreateFolderArg{
			Autorename: false,
			Path:       parentID + "/" + name,
		})
		if err != nil {
			return nil, err
		}
		return File(data), nil
	}

	newPath := parentID + "/" + name
	reader := bytes.NewBuffer([]byte{})
	data, err := fileService.Upload(&dbfiles.CommitInfo{
		Path:       newPath, // ???
		Autorename: false,
		Mode:       &dbfiles.WriteMode{Tagged: dropbox.Tagged{Tag: dbfiles.WriteModeOverwrite}},
	}, reader)
	if err != nil {
		log.Println("Upload Error:", err)
		return nil, err
	}

	return File(data), nil

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

	newParentID := ""
	if newParent != nil {
		newParentID = newParent.ID
	}

	res, err := fileService.Move(&dbfiles.RelocationArg{
		RelocationPath: dbfiles.RelocationPath{
			FromPath: file.ID,
			ToPath:   newParentID + "/" + name,
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

	// Common data

	var md dbfiles.Metadata
	switch t := metadata.(type) {
	case *dbfiles.FileMetadata:
		md = t.Metadata
		modifiedTime = t.ServerModified
		size = t.Size
	case *dbfiles.FolderMetadata:
		md = t.Metadata
		modifiedTime = time.Now()
		mode = os.FileMode(0755) | os.ModeDir
	//parentID = t.SharedFolderId
	case *dbfiles.DeletedMetadata:
		md = t.Metadata
	}

	ID = md.PathLower
	name = md.Name

	createdTime := modifiedTime // we dont have created time on dropbox?

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

	return file
}
