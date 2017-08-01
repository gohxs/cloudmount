package megafs

import (
	"io"
	"io/ioutil"
	"os"
	"strings"

	"github.com/gohxs/cloudmount/internal/core"
	"github.com/gohxs/cloudmount/internal/coreutil"
	"github.com/gohxs/cloudmount/internal/fs/basefs"
	mega "github.com/t3rm1n4l/go-mega"
)

//Service gdrive service information
type Service struct {
	// raw client to access service
	megaCli *mega.Mega
	basefs  *basefs.BaseFS
}

//NewService creates and initializes a new GDrive service
func NewService(coreConfig *core.Config, basefs *basefs.BaseFS) *Service {

	serviceConfig := Config{}
	log.Println("Initializing", pname, "service")
	log.Println("Source config:", coreConfig.Source)

	err := coreutil.ParseConfig(coreConfig.Source, &serviceConfig)
	if err != nil {
		errlog.Fatalf("Unable to read <source>: %v", err)
	}
	// Initialize cloud service here
	m := mega.New()
	m.Login(serviceConfig.Credentials.Email, serviceConfig.Credentials.Password)

	return &Service{m, basefs}

}

//Changes populate a list with changes to be handled on basefs
func (s *Service) Changes() ([]*basefs.Change, error) { // Return a list of New file entries

	// It seems that the base lib caches entries and refreshes necessary, it should be fast to refresh all ?
	s.basefs.Refresh()
	return nil, nil
}

//ListAll lists all files recursively to cache locally
func (s *Service) ListAll() ([]*basefs.File, error) {
	ret := []*basefs.File{}

	rootNode := s.megaCli.FS.GetRoot()

	var addAll func(*mega.Node, string)
	addAll = func(n *mega.Node, pathstr string) {
		children, err := s.megaCli.FS.GetChildren(n)
		if err != nil {
			return
		}
		// Add to ret
		for _, childNode := range children {
			spath := pathstr + "/" + childNode.GetName()
			ret = append(ret, File(&MegaPath{Path: spath, Node: childNode}))
			if childNode.GetType() == mega.FOLDER {
				addAll(childNode, pathstr+"/"+childNode.GetName())
			}
		}
	}

	addAll(rootNode, "")

	return ret, nil

}

//Create create an entry in google drive
func (s *Service) Create(parent *basefs.File, name string, isDir bool) (*basefs.File, error) {
	log.Println("Creating ", name, "IsDir:", isDir)
	// implement CreateFile on service here and return a File struct

	parentID := ""
	var megaParent *mega.Node
	if parent == nil {
		megaParent = s.megaCli.FS.GetRoot()
	} else {
		parentID = parent.ID
		megaParent = parent.Data.(*MegaPath).Node
	}

	newName := parentID + "/" + name
	if isDir {
		newNode, err := s.megaCli.CreateDir(name, megaParent)
		if err != nil {
			return nil, err
		}

		return File(&MegaPath{Path: newName, Node: newNode}), nil
	}
	// Create the file
	/* srcpath */

	// Create tempFile, since mega package does not accept a reader
	f, err := ioutil.TempFile(os.TempDir(), "megafs")
	if err != nil {
		return nil, err
	}
	f.Close()

	progress := make(chan int, 1)
	// Upload empty file
	newNode, err := s.megaCli.UploadFile(f.Name(), megaParent, name, &progress)
	if err != nil {
		return nil, err
	}
	<-progress

	return File(&MegaPath{Path: newName, Node: newNode}), nil

}

//Upload a file
func (s *Service) Upload(reader io.Reader, file *basefs.File) (*basefs.File, error) {

	// Find parent, should have only one parent in mega
	parentEntry := s.basefs.Root.FindByID(file.Parents[0])
	megaPath := parentEntry.File.Data.(*MegaPath)
	megaParent := megaPath.Node

	//Special case
	upFile := reader.(*basefs.FileWrapper)

	progress := make(chan int, 1)
	newNode, err := s.megaCli.UploadFile(upFile.Name(), megaParent, file.Name, &progress)
	if err != nil {
		return nil, err
	}

	<-progress
	return File(&MegaPath{Path: megaPath.Path + "/" + newNode.GetName(), Node: newNode}), nil
}

//DownloadTo from gdrive to a writer
func (s *Service) DownloadTo(w io.Writer, file *basefs.File) error {

	downFile := w.(*basefs.FileWrapper)

	progress := make(chan int, 1)
	err := s.megaCli.DownloadFile(file.Data.(*MegaPath).Node, downFile.Name(), &progress)
	if err != nil {
		return err
	}
	<-progress
	for range progress {
	}
	return nil

}

//Move a file in drive
func (s *Service) Move(file *basefs.File, newParent *basefs.File, name string) (*basefs.File, error) {
	var megaParent *mega.Node
	newParentID := ""
	if newParent != nil {
		megaParent = newParent.Data.(*MegaPath).Node
		newParentID = newParent.ID
	} else {
		megaParent = s.megaCli.FS.GetRoot()
	}
	err := s.megaCli.Move(file.Data.(*MegaPath).Node, megaParent)
	if err != nil {
		return nil, err
	}
	// Change parent in file.Data or return new
	if file.Name != name {
		err := s.megaCli.Rename(file.Data.(*MegaPath).Node, name)
		if err != nil {
			return nil, err
		}
	}

	return File(&MegaPath{Path: newParentID + "/" + name, Node: file.Data.(*MegaPath).Node}), nil
}

//Delete file from drive
func (s *Service) Delete(file *basefs.File) error {
	err := s.megaCli.Delete(file.Data.(*MegaPath).Node, false)
	if err != nil {
		return err
	}

	return nil
}

// MegaPath files does not contain parents so we extract parents from Path
type MegaPath struct {
	Path string
	Node *mega.Node
}

//File converts a google drive File structure to baseFS
func File(mfile *MegaPath) *basefs.File {
	if mfile == nil {
		return nil
	}
	createdTime := mfile.Node.GetTimeStamp()
	modifiedTime := createdTime // This fs does not support modified?

	mode := os.FileMode(0644)
	if mfile.Node.GetType() == mega.FOLDER {
		mode = os.FileMode(0755) | os.ModeDir
	}

	// Like dropbox we define parents with path and Id with path
	pathParts := strings.Split(mfile.Path, "/")
	parentID := strings.Join(pathParts[:len(pathParts)-1], "/")
	parents := []string{}
	if parentID != "" {
		parents = []string{parentID}
	}

	file := &basefs.File{
		ID:           mfile.Path,
		Name:         mfile.Node.GetName(),
		Size:         uint64(mfile.Node.GetSize()),
		CreatedTime:  createdTime,
		ModifiedTime: modifiedTime,
		AccessedTime: modifiedTime,
		Mode:         mode,

		Parents: parents, // ?
		Data:    mfile,   // store original file
	}
	return file
}
