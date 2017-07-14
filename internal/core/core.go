package core

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"dev.hexasoftware.com/hxs/prettylog"

	"github.com/jacobsa/fuse"
	"github.com/jacobsa/fuse/fuseutil"
)

var (
	log = prettylog.New("cloudmount")
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

	uid, err := strconv.Atoi(usr.Uid)
	if err != nil {
		panic(err)
	}
	gid, err := strconv.Atoi(usr.Gid)
	if err != nil {
		panic(gid)
	}

	return &Core{
		Drivers: map[string]DriverFactory{},
		Config: Config{
			Daemonize:   false,
			Type:        "gdrive",
			VerboseLog:  false,
			RefreshTime: 5 * time.Second,
			HomeDir:     filepath.Join(usr.HomeDir, ".cloudmount"),
			Source:      filepath.Join(usr.HomeDir, ".cloudmount", "gdrive.json"),
			UID:         uint32(uid),
			GID:         uint32(gid),
		},
	}

}

// Init to be run after configuration
func (c *Core) Init() (err error) {

	fsFactory, ok := c.Drivers[c.Config.Type]
	if !ok {
		log.Fatal("CloudFS not supported")
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

	if c.Config.VerboseLog {
		mfs, err = fuse.Mount(mountPath, server, &fuse.MountConfig{DebugLogger: prettylog.New("fuse"), ErrorLogger: prettylog.New("fuse-err")})
	} else {
		mfs, err = fuse.Mount(mountPath, server, &fuse.MountConfig{})
	}
	if err != nil {
		log.Fatal("Failed mounting path", flag.Arg(0), err)
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
		log.Fatalf("Joining: %v", err)
	}

}
