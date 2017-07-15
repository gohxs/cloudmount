// +build linux

package main

//go:generate go get dev.hexasoftware.com/hxs/genversion
//go:generate genversion -package main -out version.go

import (
	"fmt"
	"os"

	"os/exec"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/dropboxfs"
	"dev.hexasoftware.com/hxs/cloudmount/internal/fs/gdrivefs"
	"dev.hexasoftware.com/hxs/prettylog"
)

var (
	Name = "cloudmount"
	log  = prettylog.New(Name)
)

func main() {

	prettylog.Global()

	// getClient
	fmt.Fprintf(os.Stderr, "%s-%s\n", Name, Version)
	core := core.New()

	// More will be added later
	core.Drivers["gdrive"] = gdrivefs.New
	core.Drivers["dropbox"] = dropboxfs.New

	if err := parseFlags(&core.Config); err != nil {
		log.Fatalln(err)
	}

	err := core.Init() // Before daemon, because might require interactivity
	if err != nil {
		log.Println("Err:", err)
		return
	}

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
		//cmd.Stdout = os.Stdout
		//cmd.Stderr = os.Stderr
		cmd.Start()
		fmt.Println("[PID]", cmd.Process.Pid)
		os.Exit(0)
		return
	}

	core.Mount()

}
