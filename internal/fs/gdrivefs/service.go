package gdrivefs

import (
	"io"
	"net/http"
	"os"
	"time"

	"github.com/jacobsa/fuse"

	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
)

const (
	fileFields = googleapi.Field("id, name, size,mimeType, parents,createdTime,modifiedTime")
	gdFields   = googleapi.Field("files(" + fileFields + ")")
)

type Service struct {
	client              *drive.Service
	savedStartPageToken string
}

func (s *Service) Changes() ([]*drive.Change, error) { // Return a list of New file entries
	if s.savedStartPageToken == "" {
		startPageTokenRes, err := s.client.Changes.GetStartPageToken().Do()
		if err != nil {
			log.Println("GDrive err", err)
		}
		s.savedStartPageToken = startPageTokenRes.StartPageToken
	}

	ret := []*drive.Change{}
	pageToken := s.savedStartPageToken
	for pageToken != "" {
		changesRes, err := s.client.Changes.List(pageToken).Fields(googleapi.Field("newStartPageToken,nextPageToken,changes(removed,fileId,file(" + fileFields + "))")).Do()
		if err != nil {
			log.Println("Err fetching changes", err)
			break
		}
		//log.Println("Changes:", len(changesRes.Changes))
		for _, c := range changesRes.Changes {
			ret = append(ret, c) // Convert to our changes
		}
		if changesRes.NewStartPageToken != "" {
			s.savedStartPageToken = changesRes.NewStartPageToken
		}
		pageToken = changesRes.NextPageToken
	}
	return ret, nil
}

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
		log.Println("GDrive ERR:", err)
		return s.ListAll() // retry
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
			log.Println("GDrive ERR:", err)
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

func (s *Service) Create(parent *basefs.File, name string, isDir bool) (*basefs.File, error) {
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
		return nil, fuse.EINVAL
	}

	return File(createdGFile), nil

}

func (s *Service) Upload(reader io.Reader, file *basefs.File) (*basefs.File, error) {
	ngFile := &drive.File{}
	up := s.client.Files.Update(file.ID, ngFile)
	upFile, err := up.Media(reader).Fields(fileFields).Do()
	if err != nil {
		return nil, err
	}

	return File(upFile), nil
}

func (s *Service) DownloadTo(w io.Writer, file *basefs.File) error {

	var res *http.Response
	var err error
	// TODO :Place this in service Download
	gfile := file.Data.(*drive.File)
	// Export GDocs (Special google doc documents needs to be exported make a config somewhere for this)
	switch gfile.MimeType { // Make this somewhat optional special case
	case "application/vnd.google-apps.document":
		res, err = s.client.Files.Export(gfile.Id, "text/plain").Download()
	case "application/vnd.google-apps.spreadsheet":
		res, err = s.client.Files.Export(gfile.Id, "text/csv").Download()
	default:
		res, err = s.client.Files.Get(gfile.Id).Download()
	}

	if err != nil {
		log.Println("Error from GDrive API", err)
		return err
	}
	defer res.Body.Close()
	io.Copy(w, res.Body)

	return nil
}

func (s *Service) Move(file *basefs.File, newParent *basefs.File, name string) (*basefs.File, error) {

	ngFile := &drive.File{
		Name: name,
	}

	updateCall := s.client.Files.Update(file.ID, ngFile).Fields(fileFields)

	if !file.HasParent(newParent) {
		for _, pgid := range file.Parents {
			updateCall.RemoveParents(pgid) // Remove all parents??
		}
		updateCall.AddParents(newParent.ID)
	}
	//	}
	/*if oldParentFile != newParentFile {
		updateCall.RemoveParents(oldParentFile.GID)
		updateCall.AddParents(newParentFile.GID)
	}*/
	updatedFile, err := updateCall.Do()

	return File(updatedFile), err
}

func (s *Service) Delete(file *basefs.File) error {
	err := s.client.Files.Delete(file.ID).Do()
	if err != nil {
		return err
	}
	return nil
}

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
