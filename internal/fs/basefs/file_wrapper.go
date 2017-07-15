package basefs

// Unfortunately Dropbox/net/http closes the file after sending (even if it is io.Reader only) and in this case we still need it locally open

import "os"

type fileWrapper struct {
	*os.File
}

func (f *fileWrapper) Close() error {
	//panic("I don't want anyone to close this file")
	// Ignore closers
	return nil
}
func (f *fileWrapper) RealClose() error {
	return f.File.Close()
}
