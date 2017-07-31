package basefs

// Unfortunately Dropbox/net/http closes the file after sending (even if it is io.Reader only) and in this case we still need it locally open

import "os"

//FileWrapper helper to prevent http.Post to close my files
type FileWrapper struct {
	*os.File
}

//Close ignore close
func (f *FileWrapper) Close() error {
	// Ignore closers
	return nil
}

//RealClose to be called internally to close the file
func (f *FileWrapper) RealClose() error {
	return f.File.Close()
}
