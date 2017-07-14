package gdrivefs

import (
	"time"

	drive "google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/basefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	log = prettylog.New("gdrivefs")
)

type GDriveFS struct {
	*basefs.BaseFS
	serviceConfig *Config
	nextRefresh   time.Time
}

func New(core *core.Core) core.DriverFS {

	fs := &GDriveFS{basefs.New(core), &Config{}, time.Now()}
	client := fs.initClient() // Init Oauth2 client

	fs.BaseFS.Client = client // This will be removed
	fs.BaseFS.Service = &gdriveService{client}

	return fs
}

func (fs *GDriveFS) Start() {
	go func() {
		fs.Refresh() // First load

		// Change reader loop
		startPageTokenRes, err := fs.Client.Changes.GetStartPageToken().Do()
		if err != nil {
			log.Println("GDrive err", err)
		}
		savedStartPageToken := startPageTokenRes.StartPageToken
		for {
			pageToken := savedStartPageToken
			for pageToken != "" {
				changesRes, err := fs.Client.Changes.List(pageToken).Fields(googleapi.Field("newStartPageToken,nextPageToken,changes(removed,fileId,file(" + fileFields + "))")).Do()
				if err != nil {
					log.Println("Err fetching changes", err)
					break
				}
				//log.Println("Changes:", len(changesRes.Changes))
				for _, c := range changesRes.Changes {
					_, entry := fs.Root.FindByGID(c.FileId)
					if c.Removed {
						if entry == nil {
							continue
						} else {
							fs.Root.RemoveEntry(entry)
						}
						continue
					}

					if entry != nil {
						entry.SetFile(&basefs.GFile{c.File}, fs.Config.UID, fs.Config.GID)
					} else {
						//Create new one
						fs.Root.FileEntry(c.File) // Creating new one
					}
				}
				if changesRes.NewStartPageToken != "" {
					savedStartPageToken = changesRes.NewStartPageToken
				}
				pageToken = changesRes.NextPageToken
			}

			time.Sleep(fs.Config.RefreshTime)
		}
	}()
}

const fileFields = googleapi.Field("id, name, size,mimeType, parents,createdTime,modifiedTime")
const gdFields = googleapi.Field("files(" + fileFields + ")")

func (fs *GDriveFS) Refresh() {

	fileList := []*drive.File{}
	fileMap := map[string]*drive.File{} // Temporary map by google drive fileID

	r, err := fs.Client.Files.List().
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
		r, err = fs.Client.Files.List().
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
	root := basefs.NewFileContainer(fs.BaseFS)
	var appendFile func(gfile *drive.File)
	appendFile = func(gfile *drive.File) {
		for _, pID := range gfile.Parents {
			parentFile, ok := fileMap[pID]
			if !ok {
				parentFile, err = fs.Client.Files.Get(pID).Do()
				if err != nil {
					log.Println("Error fetching single file:", err)
				}
				fileMap[parentFile.Id] = parentFile
			}
			appendFile(parentFile) // Recurse
		}

		// Find existing entry
		inode, entry := fs.Root.FindByGID(gfile.Id)
		// Store for later add
		if entry == nil {
			inode, entry = fs.Root.FileEntry(gfile) // Add New and retrieve
		}
		root.SetEntry(inode, entry)
		// add File
	}

	for _, f := range fileList { // Ordered
		appendFile(f) // Check parent first
	}

	log.Println("Refresh done, update root")
	fs.Root = root
	//fs.root.children = root.children

	log.Println("File count:", root.Count())
}
