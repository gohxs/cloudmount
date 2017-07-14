package gdrivefs

import (
	"io"
	"net/http"

	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"

	"google.golang.org/api/drive/v3"
)

type gdriveService struct {
	client *drive.Service
}

func (s *gdriveService) Truncate(file basefs.File) (basefs.File, error) {
	err := s.client.Files.Delete(file.ID()).Do() // XXX: Careful on this
	createdFile, err := s.client.Files.Create(&drive.File{Parents: file.Parents(), Name: file.Name()}).Fields(fileFields).Do()
	if err != nil {
		return nil, err
	}

	return &basefs.GFile{createdFile}, nil

}

func (s *gdriveService) Upload(reader io.Reader, file basefs.File) (basefs.File, error) {
	ngFile := &drive.File{}
	up := s.client.Files.Update(file.ID(), ngFile)
	upFile, err := up.Media(reader).Fields(fileFields).Do()
	if err != nil {
		return nil, err
	}

	return &basefs.GFile{upFile}, nil
}

func (s *gdriveService) DownloadTo(w io.Writer, file basefs.File) error {

	var res *http.Response
	var err error
	// TODO :Place this in service Download
	gfile := file.(*basefs.GFile).File
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
