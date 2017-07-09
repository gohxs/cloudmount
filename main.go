// +build linux

package main

//go:generate go run cmd/genversion/main.go -package main -out version.go

import (
	"fmt"
	"os"

	"os/exec"

	"dev.hexasoftware.com/hxs/cloudmount/cloudfs"

	"dev.hexasoftware.com/hxs/prettylog"

	_ "dev.hexasoftware.com/hxs/cloudmount/fs/gdrivefs"
	//_ "github.com/icattlecoder/godaemon" // No reason
)

var (
	Name = "cloudmount"
	log  = prettylog.New("main")
)

func main() {

	prettylog.Global()
	// getClient
	fmt.Printf("%s-%s\n", Name, Version)

	core := core.New()

	core.Drivers["gdrive"] = gdrivefs.New

	core.Init()
	// Register drivers here too
	core.ParseFlags()

	core.Start()

	///////////////////////////////
	// cloud drive Type
	/////////////////
	/*f, ok := core.Drivers[clouddriveFlag] // there can be some interaction before daemon
	if !ok {
		log.Fatal("FileSystem not supported")
	}
	driveFS := f()*/

	////////////////////////////////
	// Daemon
	/////////////////
	if daemonizeFlag {
		subArgs := []string{}
		for _, arg := range os.Args[1:] {
			if arg == "-d" { // ignore daemon flag
				continue
			}
			subArgs = append(subArgs, arg)
		}

		cmd := exec.Command(os.Args[0], subArgs...)
		cmd.Start()
		fmt.Println("[PID]", cmd.Process.Pid)
		os.Exit(0)
		return
	}

}
