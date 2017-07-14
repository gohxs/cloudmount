package basefs

// Drive Common functions for CloudFS
// Like: //?? seems like defailt container
//    CreateFile
//    MkDir
//    Unlink
//    GetFile
//    UploadFile
//    Rename
//    Delete
type Driver interface {
	Upload(entry *FileEntry)
}
