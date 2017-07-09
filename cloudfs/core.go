package core

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"dev.hexasoftware.com/hxs/prettylog"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

type Core struct {
	Config  Config
	Drivers map[string]DriverFactory
}

type Config struct {
	Daemonize     bool
	CloudFSDriver string
	VerboseLog    bool

	HomeDir string
	UID     uint32 // Mount UID
	GID     uint32 // Mount GID
}

// New create a New cloudmount core
func New() *Core {
	return &Core{Drivers: map[string]DriverFactory{}}
}

func (c *Core) Init() {
	// TODO: friendly panics
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

	// Defaults
	c.Config = Config{
		HomeDir:    filepath.Join(usr.HomeDir, ".cloudmount"),
		UID:        uint32(uid),
		GID:        uint32(gid),
		VerboseLog: false,
		Daemonize:  false,
	}

}

func (c *Core) ParseFlags() {
	var mountoptsFlag string

	flag.StringVar(&c.Config.CloudFSDriver, "t", "gdrive", "which cloud service to use [gdrive]")
	flag.BoolVar(&c.Config.Daemonize, "d", false, "Run app in background")
	flag.BoolVar(&c.Config.VerboseLog, "v", false, "Verbose log")
	flag.StringVar(&c.Config.HomeDir, "h", c.Config.HomeDir, "Path that holds configurations")

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
		return
	}
	/////////////////////////////////////
	// Parse mount opts
	/////////////////
	pmountopts := strings.Split(mountoptsFlag, ",")
	mountopts := map[string]string{}
	for _, v := range pmountopts {
		keypart := strings.Split(v, "=")
		if len(keypart) != 2 {
			continue
		}
		mountopts[keypart[0]] = keypart[1]
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
		c.Config.UID = uint32(uid)
	}
}

func (c *Core) Start() {

	cloudfs, ok := c.Drivers[c.Config.CloudFSDriver]
	if !ok {
		log.Fatal("CloudFS not supported")
	}
	driveFS := cloudfs() // Constructor?

	// Start driveFS somehow

	//////////////
	// Server
	/////////
	ctx := context.Background()
	server := fuseutil.NewFileSystemServer(driveFS)
	mountPath := flag.Arg(0)

	var err error
	var mfs *fuse.MountedFileSystem

	if c.Config.VerboseLog {
		mfs, err = fuse.Mount(mountPath, server, &fuse.MountConfig{DebugLogger: prettylog.New("fuse"), ErrorLogger: prettylog.New("fuse-err")})
	} else {
		mfs, err = fuse.Mount(mountPath, server, &fuse.MountConfig{})
	}
	if err != nil {
		log.Fatal("Failed mounting path", flag.Arg(0))
	}

	// Signal handling to refresh Drives
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range sigs {
			log.Println("Signal:", sig)
			switch sig {
			case syscall.SIGUSR1:
				log.Println("Manually Refresh drive")
				go driveFS.Refresh()
			case syscall.SIGHUP:
				log.Println("GC")
				mem := runtime.MemStats{}
				runtime.ReadMemStats(&mem)
				log.Printf("Mem: %.2fMB", float64(mem.Alloc)/1024/1024)
				runtime.GC()

				runtime.ReadMemStats(&mem)
				log.Printf("After gc: Mem: %.2fMB", float64(mem.Alloc)/1024/1024)

			case os.Interrupt:
				log.Println("Graceful unmount")
				fuse.Unmount(mountPath)
				os.Exit(1)
			case syscall.SIGTERM:
				log.Println("Graceful unmount")
				fuse.Unmount(mountPath)
				os.Exit(1)
			}

		}
	}()

	if err := mfs.Join(ctx); err != nil {
		log.Fatalf("Joining: %v", err)
	}

}
