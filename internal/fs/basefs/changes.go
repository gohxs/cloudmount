package basefs

type Change struct {
	File    *File
	deleted bool
}
