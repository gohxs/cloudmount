// +build linux

package main

//go:generate go get dev.hexasoftware.com/hxs/genversion
//go:generate genversion -package main -out version.go

import (
	"fmt"
	"log"
	"os"

	"github.com/gohxs/cloudmount/internal/core"

	"github.com/gohxs/cloudmount/internal/fs/dropboxfs"
	"github.com/gohxs/cloudmount/internal/fs/gdrivefs"
	"github.com/gohxs/cloudmount/internal/fs/megafs"
	"github.com/gohxs/prettylog"

	"os/exec"
)

var (
	//Name app name
	Name = "cloudmount"
)

func main() {
	// TODO: TEMP
	/*{
		// Globally insecure SSL for debugging
		r, _ := http.NewRequest("GET", "http://localhost", nil)
		cli := &http.Client{}
		cli.Do(r)
		tr := http.DefaultTransport.(*http.Transport)
		tr.TLSClientConfig.InsecureSkipVerify = true
	}*/

	prettylog.Global()

	fmt.Fprintf(os.Stderr, "%s-%s\n", Name, Version)
	// getClient
	c := core.New()

	// More will be added later
	c.Drivers["gdrive"] = gdrivefs.New
	c.Drivers["dropbox"] = dropboxfs.New
	c.Drivers["mega"] = megafs.New

	if err := parseFlags(&c.Config); err != nil {
		log.Fatalln(err)
	}

	err := c.Init() // Before daemon, because might require interactivity
	if err != nil {
		log.Println("Err:", err)
		return
	}
	fmt.Fprintf(os.Stderr, "%s on %s type %s\n", c.Config.Source, c.Config.Target, c.Config.Type)

	////////////////////////////////
	// Daemon
	/////////////////
	if !c.Config.Foreground {
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

	c.Mount()

}
