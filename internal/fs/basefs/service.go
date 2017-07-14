package basefs

import "io"

type Service interface {
	Truncate(file File) (File, error)
	Upload(reader io.Reader, file File) (File, error)
	DownloadTo(w io.Writer, file File) error
}
