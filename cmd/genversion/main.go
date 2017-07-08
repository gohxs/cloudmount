package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

func main() {
	var pkg string
	var dst string
	flag.StringVar(&pkg, "package", "", "Set package name")
	flag.StringVar(&dst, "out", "", "Output file")
	flag.Parse()

	if pkg == "" {
		log.Println("Missing argument -package")
		flag.Usage()
		return
	}
	if dst == "" {
		log.Println("Missing argument -out")
		flag.Usage()
		return
	}

	tag, err := exec.Command("git", "describe", "--tags").Output()
	if err != nil {
		log.Fatal("Error Getting tag", err)
	}
	re := regexp.MustCompile(`\r?\n`)
	vtag := re.ReplaceAll(tag, []byte(" "))

	version := fmt.Sprintf("%s - built: %s", strings.TrimSpace(string(vtag)), time.Now().UTC().Format("2006-01-02 15:04:05 UTC"))

	fmt.Printf("Version: %s\n", version)

	f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Println("Error opening file", err)
		return
	}

	fmt.Fprintf(f, "package %s\n", pkg)

	fmt.Fprintln(f, "\nconst (")
	fmt.Fprintf(f, "  //Version contains version of the package\n")
	fmt.Fprintf(f, "  Version = \"%s\"\n", version)
	fmt.Fprintln(f, ")")

	f.Sync()
	f.Close()

}
