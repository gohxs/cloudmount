package basefs

type Change struct {
	ID     string
	File   *File
	Remove bool
}
