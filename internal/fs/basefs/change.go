package basefs

//Change information retrieved by API deltas
type Change struct {
	ID     string
	File   *File
	Remove bool
}
