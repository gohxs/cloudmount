package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"dev.hexasoftware.com/hxs/cloudmount/internal/core"
	"dev.hexasoftware.com/hxs/cloudmount/internal/coreutil"
)

func parseFlags(config *core.Config) (err error) {
	var mountoptsFlag string

	flag.StringVar(&config.Type, "t", config.Type, "which cloud service to use [gdrive]")
	flag.BoolVar(&config.Daemonize, "d", false, "Run app in background")
	flag.BoolVar(&config.VerboseLog, "v", false, "Verbose log")
	flag.BoolVar(&config.Verbose2Log, "vv", false, "Extra Verbose log")
	flag.StringVar(&config.HomeDir, "w", config.HomeDir, "Work dir, path that holds configurations")
	flag.DurationVar(&config.RefreshTime, "r", config.RefreshTime, "Timed cloud synchronization interval [if applied]")

	flag.StringVar(&mountoptsFlag, "o", "", fmt.Sprintf("%v", config.Options))

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [options] [<source>] <directory>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Source: can be json/yaml configuration file usually with credentials or cloud specific configuration\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n")
	}
	flag.Parse()

	fileExt := filepath.Ext(os.Args[0])
	if fileExt != "" {
		config.Type = fileExt[1:]
	}

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

	if config.Verbose2Log {
		config.VerboseLog = true
	}

	// Read fs type from config file
	sourceType := struct {
		Type string `json:"type"`
	}{}
	coreutil.ParseConfig(config.Source, &sourceType)
	if sourceType.Type != "" {
		if config.Type != "" && sourceType.Type != config.Type {
			log.Fatalf("ERR: service mismatch <source> specifies '%s' while flag -t is '%s'", sourceType.Type, config.Type)
		}
		config.Type = sourceType.Type
	}

	if config.Type == "" {
		log.Fatalf("ERR: Missing -t param, unknown file system")
	}

	err = coreutil.ParseOptions(mountoptsFlag, &config.Options)
	if err != nil {
		log.Fatal("ERR: Invalid syntax parsing mount options")
	}
	return
}
