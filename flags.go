package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
)

func parseFlags(config *core.Config) (err error) {
	var mountoptsFlag string

	flag.StringVar(&config.Type, "t", config.Type, "which cloud service to use [gdrive]")
	flag.BoolVar(&config.Daemonize, "d", false, "Run app in background")
	flag.BoolVar(&config.VerboseLog, "v", false, "Verbose log")
	flag.StringVar(&config.HomeDir, "w", config.HomeDir, "Work dir, path that holds configurations")
	flag.DurationVar(&config.RefreshTime, "r", config.RefreshTime, "Timed cloud synchronization interval [if applied]")

	flag.StringVar(&mountoptsFlag, "o", "", "uid,gid ex: -o uid=1000,gid=0 ")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [<source>] <directory>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Source: can be json/yaml configuration file usually with credentials or cloud specific configuration\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
	}
	flag.Parse()

	if flag.NArg() < 1 {
		flag.Usage()
		//fmt.Println("Usage:\n gdrivemount [-d] [-v] <SRC/CONFIG> <MOUNTPOINT>")
		return errors.New("Missing parameter")
	}
	if flag.NArg() == 1 {
		config.Source = filepath.Join(config.HomeDir, config.Type+".yaml")
		config.Target = flag.Arg(0)
	} else {
		config.Source = flag.Arg(0)
		config.Target = flag.Arg(1)
	}
	/////////////////////////////////////
	// Parse mount opts
	/////////////////
	pmountopts := strings.Split(mountoptsFlag, ",")
	mountopts := map[string]string{}
	for _, v := range pmountopts {
		if keyindex := strings.Index(v, "="); keyindex != -1 {
			key := strings.TrimSpace(v[:keyindex])
			value := strings.TrimSpace(v[keyindex+1:])
			mountopts[key] = value
		}
	}

	/////////////////////////////////////
	// Use mount opts
	///////////////
	uidStr, ok := mountopts["uid"]
	if ok {
		uid, err := strconv.Atoi(uidStr)
		if err != nil {
			panic(err)
		}
		config.UID = uint32(uid)
	}

	gidStr, ok := mountopts["gid"]
	if ok {
		gid, err := strconv.Atoi(gidStr)
		if err != nil {
			panic(err)
		}
		config.GID = uint32(gid)
	}
	return
}
