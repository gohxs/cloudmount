package gdrivefs

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gohxs/cloudmount/internal/core"
	"github.com/gohxs/cloudmount/internal/coreutil"
	"github.com/gohxs/cloudmount/internal/fs/basefs"
	"github.com/gohxs/cloudmount/internal/oauth2util"

	"golang.org/x/oauth2"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const (
	fileFields = googleapi.Field("id, name,size,mimeType,parents,createdTime,modifiedTime,trashed")
	gdFields   = googleapi.Field("files(" + fileFields + ")")
)

//Service gdrive service information
type Service struct {
	client              *drive.Service
	serviceConfig       Config
	savedStartPageToken string
}

//NewService creates and initializes a new GDrive service
func NewService(coreConfig *core.Config) *Service {

	serviceConfig := Config{}
	log.Println("Initializing gdrive service")
	log.Println("Source config:", coreConfig.Source)

	err := coreutil.ParseConfig(coreConfig.Source, &serviceConfig)
	if err != nil {
		errlog.Fatalf("Unable to read <source>: %v", err)
	}
	config := &oauth2.Config{
		ClientID:     serviceConfig.ClientSecret.ClientID,
		ClientSecret: serviceConfig.ClientSecret.ClientSecret,
		RedirectURL:  "urn:ietf:wg:oauth:2.0:oob", //d.serviceConfig.ClientSecret.RedirectURIs[0],
		Scopes:       []string{drive.DriveScope},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://accounts.google.com/o/oauth2/auth",  //d.serviceConfig.ClientSecret.AuthURI,
			TokenURL: "https://accounts.google.com/o/oauth2/token", //d.serviceConfig.ClientSecret.TokenURI,
		},
	}
	if serviceConfig.Auth == nil {
		tok := oauth2util.GetTokenFromWeb(config)
		serviceConfig.Auth = tok
		coreutil.SaveConfig(coreConfig.Source, &serviceConfig)
	}

	client := config.Client(oauth2.NoContext, serviceConfig.Auth)
	driveCli, err := drive.New(client)
	if err != nil {
		errlog.Fatalf("Unable to retrieve drive Client: %v", err)
	}

	return &Service{client: driveCli, serviceConfig: serviceConfig}

}

//Changes populate a list with changes to be handled on basefs
func (s *Service) Changes() ([]*basefs.Change, error) { // Return a list of New file entries
	if s.savedStartPageToken == "" {
		startPageTokenRes, err := s.client.Changes.GetStartPageToken().Do()
		if err != nil {
			log.Println("GDrive err", err)
		}
		s.savedStartPageToken = startPageTokenRes.StartPageToken
	}

	ret := []*basefs.Change{}
	pageToken := s.savedStartPageToken
	for pageToken != "" {
		changesRes, err := s.client.Changes.List(pageToken).Fields(googleapi.Field("newStartPageToken,nextPageToken,changes(removed,fileId,file(" + fileFields + "))")).Do()
		if err != nil {
			log.Println("Err fetching changes", err)
			break
		}
		//log.Println("Changes:", len(changesRes.Changes))
		for _, c := range changesRes.Changes {
			remove := c.Removed
			if c.File != nil && c.File.Trashed { // Might not be removed but instead trashed
				remove = true
			}
			change := &basefs.Change{ID: c.FileId, File: File(c.File), Remove: remove}
			ret = append(ret, change) // Convert to our changes
		}
		if changesRes.NewStartPageToken != "" {
			s.savedStartPageToken = changesRes.NewStartPageToken
		}
		pageToken = changesRes.NextPageToken
	}
	return ret, nil
}

//ListAll lists all files recursively to cache locally
func (s *Service) ListAll() ([]*basefs.File, error) {
	fileList := []*drive.File{}
	// Service list ALL ???
	fileMap := map[string]*drive.File{} // Temporary map by google drive fileID

	r, err := s.client.Files.List().
		OrderBy("createdTime").
		PageSize(1000).
		SupportsTeamDrives(true).
		IncludeTeamDriveItems(true).
		Fields(googleapi.Field("nextPageToken"), gdFields).
		Do()
	if err != nil {
		// Sometimes gdrive returns error 500 randomly
		errlog.Println("GDrive ERR:", err)
		return s.ListAll() // retry ??
		//return nil, err
	}

	fileList = append(fileList, r.Files...)

	// Rest of the pages
	for r.NextPageToken != "" {
		r, err = s.client.Files.List().
			OrderBy("createdTime").
			PageToken(r.NextPageToken).
			Fields(googleapi.Field("nextPageToken"), gdFields).
			Do()
		if err != nil {
			errlog.Println("GDrive ERR:", err)
			return s.ListAll() // retry
			//return nil, err
		}
		fileList = append(fileList, r.Files...)
	}
	log.Println("Total entries:", len(fileList))

	// Cache ID for faster retrieval, might not be necessary
	for _, f := range fileList { // Temporary lookup Cache
		fileMap[f.Id] = f
	}

	// All fetched

	files := []*basefs.File{}
	// Create clean fileList
	var appendFile func(gfile *drive.File)
	appendFile = func(gfile *drive.File) {
		if gfile.Trashed {
			return
		}
		for _, pID := range gfile.Parents {
			parentFile, ok := fileMap[pID]
			if !ok {
				parentFile, err = s.client.Files.Get(pID).Do()
				if err != nil {
					log.Println("Error fetching single file:", err)
				}
				fileMap[parentFile.Id] = parentFile
				appendFile(parentFile) // Recurse
			}
		}
		// Do not append directly
		files = append(files, File(gfile)) // Add converted file
	}

	for _, f := range fileList { // Ordered
		appendFile(f) // Check parent first
	}

	log.Println("File count:", len(files))

	return files, nil

}

//Create create an entry in google drive
func (s *Service) Create(parent *basefs.File, name string, isDir bool) (*basefs.File, error) {
	if parent == nil {
		return nil, basefs.ErrPermission
	}

	newGFile := &drive.File{
		Parents: []string{parent.ID},
		Name:    name,
	}
	if isDir {
		newGFile.MimeType = "application/vnd.google-apps.folder"
	}
	// Could be transformed to CreateFile in continer
	createdGFile, err := s.client.Files.Create(newGFile).Fields(fileFields).Do()
	if err != nil {
		log.Println("err", err)
		return nil, err
	}

	return File(createdGFile), nil

}

//Upload a file
func (s *Service) Upload(reader io.Reader, file *basefs.File) (*basefs.File, error) {
	ngFile := &drive.File{}
	up := s.client.Files.Update(file.ID, ngFile)
	upFile, err := up.Media(reader).Fields(fileFields).Do()
	if err != nil {
		return nil, err
	}

	return File(upFile), nil
}

//DownloadTo from gdrive to a writer
func (s *Service) DownloadTo(w io.Writer, file *basefs.File) error {

	var res *http.Response
	var err error
	// TODO :Place this in service Download
	gfile := file.Data.(*drive.File)
	// Export GDocs (Special google doc documents needs to be exported make a config somewhere for this)
	switch gfile.MimeType { // Make this somewhat optional special case
	case "application/vnd.google-apps.document":
		log.Println("Mimes", s.serviceConfig.Mime)
		targetMime := s.serviceConfig.Mime[gfile.MimeType]
		log.Println("Mime before default is:", targetMime)
		if targetMime == "" {
			targetMime = "text/plain"
		}
		log.Println("Exporting doc as:", targetMime)

		res, err = s.client.Files.Export(gfile.Id, targetMime).Download()
	case "application/vnd.google-apps.spreadsheet":
		res, err = s.client.Files.Export(gfile.Id, "text/csv").Download()
	default:
		res, err = s.client.Files.Get(gfile.Id).Download()
	}

	if err != nil {
		log.Println("Error from GDrive API", err, "Mimetype:", gfile.MimeType)
		return err
	}
	defer res.Body.Close()
	io.Copy(w, res.Body)

	return nil
}

//Move a file in drive
func (s *Service) Move(file *basefs.File, newParent *basefs.File, name string) (*basefs.File, error) {
	/*if newParent == nil {
		return nil, basefs.ErrPermission
	}*/
	ngFile := &drive.File{
		Name: name,
	}

	updateCall := s.client.Files.Update(file.ID, ngFile).Fields(fileFields)

	if !file.HasParent(newParent) {
		for _, pgid := range file.Parents {
			updateCall.RemoveParents(pgid) // Remove all parents??
		}
		if newParent != nil {
			updateCall.AddParents(newParent.ID)
		}
	}
	updatedFile, err := updateCall.Do()

	return File(updatedFile), err
}

//Delete file from drive
func (s *Service) Delete(file *basefs.File) error {
	// PRevent removing from root?
	err := s.client.Files.Delete(file.ID).Do()
	if err != nil {
		return err
	}
	return nil
}

//File converts a google drive File structure to baseFS
func File(gfile *drive.File) *basefs.File {
	if gfile == nil {
		return nil
	}

	createdTime, _ := time.Parse(time.RFC3339, gfile.CreatedTime)
	modifiedTime, _ := time.Parse(time.RFC3339, gfile.ModifiedTime)

	mode := os.FileMode(0644)
	if gfile.MimeType == "application/vnd.google-apps.folder" {
		mode = os.FileMode(0755) | os.ModeDir
	}

	file := &basefs.File{
		ID:           gfile.Id,
		Name:         gfile.Name,
		Size:         uint64(gfile.Size),
		CreatedTime:  createdTime,
		ModifiedTime: modifiedTime,
		AccessedTime: modifiedTime,
		Mode:         mode,

		Parents: gfile.Parents,
		Data:    gfile, // Extra gfile
	}
	return file
}
