// +build linux

package main

//go:generate go run cmd/genversion/main.go -package main -out version.go

import (
	"fmt"
	"os"

	"os/exec"

	"dev.hexasoftware.com/hxs/prettylog"

	"dev.hexasoftware.com/hxs/cloudmount/cloudfs"
	"dev.hexasoftware.com/hxs/cloudmount/fs/gdrivefs"
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

	core := cloudfs.New()
	core.Drivers["gdrive"] = gdrivefs.New

	err := core.Init()
	if err != nil {
		log.Println("Err:", err)
		return
	}
	// Register drivers here too
	////////////////////////////////
	// Daemon
	/////////////////
	if core.Config.Daemonize {
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

	core.Mount()

}
