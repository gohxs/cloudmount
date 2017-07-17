package core

import (
	"context"
	"flag"
	glog "log"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"dev.hexasoftware.com/hxs/cloudmount/internal/coreutil"
	"dev.hexasoftware.com/hxs/prettylog"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

var (
	pname  = "cloudmount"
	log    = prettylog.Dummy()
	errlog = prettylog.New(pname + "-err")
)

// Core struct
type Core struct {
	Config  Config
	Drivers map[string]DriverFactory

	CurrentFS DriverFS
}

// New create a New cloudmount core
func New() *Core {

	// TODO: friendly panics
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	var uid, gid uint32
	err = coreutil.StringAssign(usr.Uid, &uid)
	if err != nil {
		panic(err)
	}
	err = coreutil.StringAssign(usr.Gid, &gid)
	if err != nil {
		panic(err)
	}

	return &Core{
		Drivers: map[string]DriverFactory{},
		Config: Config{
			Daemonize:   false,
			Type:        "",
			VerboseLog:  false,
			RefreshTime: 5 * time.Second,
			HomeDir:     filepath.Join(usr.HomeDir, ".cloudmount"),
			Source:      filepath.Join(usr.HomeDir, ".cloudmount", "gdrive.yaml"),

			// Defaults at least
			Options: Options{
				UID:      uint32(uid),
				GID:      uint32(gid),
				Readonly: false,
			},
		},
	}

}

// Init to be run after configuration
func (c *Core) Init() (err error) {

	if c.Config.VerboseLog {
		log = prettylog.New(pname)
	}

	fsFactory, ok := c.Drivers[c.Config.Type]
	if !ok {
		errlog.Fatal("CloudFS not supported")
	}

	c.CurrentFS = fsFactory(c) // Factory

	return
}

func (c *Core) Mount() {

	// Start Selected driveFS
	c.CurrentFS.Start() // Should not block
	//////////////
	// Server
	/////////
	ctx := context.Background()
	server := fuseutil.NewFileSystemServer(c.CurrentFS)
	mountPath := c.Config.Target

	var err error
	var mfs *fuse.MountedFileSystem

	fsname := c.Config.Source

	var dbgLogger *glog.Logger
	var errLogger *glog.Logger
	if c.Config.Verbose2Log { // Extra verbose
		dbgLogger = prettylog.New("fuse")
		errLogger = prettylog.New("fuse-err")
	}

	mfs, err = fuse.Mount(mountPath, server, &fuse.MountConfig{
		VolumeName: "cloudmount",
		//Options:     coreutil.OptionMap(c.Config.Options),
		FSName:      fsname,
		DebugLogger: dbgLogger,
		ErrorLogger: errLogger,
		ReadOnly:    c.Config.Options.Readonly,
	})
	if err != nil {
		errlog.Fatal("Failed mounting path ", flag.Arg(0), err)
	}

	// Signal handling to refresh Drives
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, syscall.SIGUSR1, syscall.SIGHUP, syscall.SIGINT, os.Interrupt, syscall.SIGTERM)
	go func() {
		for sig := range sigs {
			log.Println("Signal:", sig)
			switch sig {
			//case syscall.SIGUSR1:
			//log.Println("Manually Refresh drive")
			//go c.CurrentFS.Refresh()
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
		errlog.Fatalf("Joining: %v", err)
	}

}
