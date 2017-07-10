package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
)

func parseFlags(config *core.Config) (err error) {
	var mountoptsFlag string

	flag.StringVar(&config.CloudFSDriver, "t", "gdrive", "which cloud service to use [gdrive]")
	flag.BoolVar(&config.Daemonize, "d", false, "Run app in background")
	flag.BoolVar(&config.VerboseLog, "v", false, "Verbose log")
	flag.StringVar(&config.HomeDir, "w", config.HomeDir, "Work dir, path that holds configurations")

	flag.StringVar(&mountoptsFlag, "o", "", "-o [opts]  uid,gid")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] MOUNTPOINT\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
	}
	flag.Parse()

	if len(flag.Args()) < 1 {
		flag.Usage()
		//fmt.Println("Usage:\n gdrivemount [-d] [-v] MOUNTPOINT")
		return errors.New("Missing parameter")
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
