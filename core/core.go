package core

import (
	"os/user"
	"path/filepath"
	"strconv"
)

var (
	Drivers = map[string]DriverFactory{}
	Config  ConfigData
)

type ConfigData struct {
	WorkDir string
	UID     uint32 // Mount UID
	GID     uint32 // Mount GID
}

// TODO Friendly panics
func init() {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		panic(err)
	}
	gid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		panic(gid)
	}

	Config = ConfigData{
		WorkDir: filepath.Join(usr.HomeDir, ".cloudmount"),
		UID:     uint32(uid),
		GID:     uint32(gid),
	}

}
