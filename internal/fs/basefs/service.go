package basefs

import "io"

type Service interface {
	ListAll() ([]*File, error)
	Create(parent *File, name string, isDir bool) (*File, error)
	//Truncate(file *File) (*File, error)
	Upload(reader io.Reader, file *File) (*File, error)
	DownloadTo(w io.Writer, file *File) error
	Move(file *File, newParent *File, name string) (*File, error)
	Delete(file *File) error
}
