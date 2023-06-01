package cmds

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/cheggaaa/pb"
	"github.com/urfave/cli"
)

var InstallFlag = cli.Command{
	Name:    "install",
	Aliases: []string{"i"},
	Usage:   fmt.Sprintf("Install binary from https://gobuild.io"),
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "debug",
			Usage: "Enable debug log",
		},
	},
	Action: InstallAction,
}

var GOBIN string

// Determine binary path
func init() {
	var paths []string
	gobin := os.Getenv("GOBIN")
	if gobin != "" {
		paths = filepath.SplitList(gobin)
	}
	if len(paths) > 0 {
		GOBIN = paths[0]
		return
	}
	paths = filepath.SplitList(os.Getenv("GOPATH"))
	if len(paths) > 0 {
		GOBIN = filepath.Join(paths[0], "bin")
		return
	}
	log.Fatal("Make sure you set env GOPATH or GOBIN")

	os.ExpandEnv("$HOME/.gopack/src")
}

func getInsPath(names ...string) string {
	base := os.ExpandEnv("$HOME/.gopack/")
	names = append([]string{base}, names...)
	target := filepath.Join(names...)
	os.MkdirAll(filepath.Dir(target), 0755)
	return target
}

// http://dn-gobuild5.qbox.me/gorelease/gorelease/gorelease/master/darwin-amd64/gorelease.zip
// Need to update
func downloadSource(name string) (dest string, err error) {
	parts := strings.Split(name, "/")
	if len(parts) != 2 {
		return "", fmt.Errorf("name: %s can only contains on /", name)
	}

	owner, repo := parts[0], parts[1]
	osarch := runtime.GOOS + "-" + runtime.GOARCH
	url := fmt.Sprintf("http://dn-gobuild5.qbox.me/gorelease/%s/%s/master/%s/%s.zip",
		owner, repo, osarch, repo)

	prompt("Downloading %v", url)
	log.Println("download:", url)
	dest = getInsPath("src", fmt.Sprintf("%s.zip", repo))

	resp, err := http.Get(url)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		err = errors.New("Http status not 200")
		return
	}

	lengthStr := resp.Header.Get("Content-Length")
	if lengthStr == "" {
		err = errors.New("Content length is empty")
		return
	}
	length, _ := strconv.Atoi(lengthStr)
	bar := pb.New(length).SetUnits(pb.U_BYTES)

	outFd, err := os.Create(dest)
	if err != nil {
		return
	}
	defer outFd.Close()

	bar.Start()
	reader := bar.NewProxyReader(resp.Body)
	_, err = io.Copy(outFd, reader)
	return
}

func deployPackage(pkgName, path string, binDir string) error {
	cmd := exec.Command("unzip", "-o", "-d", getInsPath("opt", pkgName), path)
	log.Println("zip command:", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	baseName := filepath.Base(pkgName)
	symlink := filepath.Join(GOBIN, baseName)

	prompt("Symlink %v", symlink)
	// for linux and darwin
	os.Remove(symlink)

	// TODO: need to resolve multi binaries
	return os.Symlink(getInsPath("opt", pkgName, filepath.Base(pkgName)), symlink)
}

func prompt(format string, args ...interface{}) {
	format = "==> " + strings.TrimPrefix(format, "\n") + "\n"
	fmt.Printf(format, args...)
}

func InstallAction(c *cli.Context) {
	if c.Bool("debug") {
		log.SetOutput(os.Stderr)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	if len(c.Args()) < 1 {
		log.Fatal("Need at lease one argument")
	}
	// log.Println(GOBIN)
	pkgName := c.Args().First()
	// TODO: use myname for now
	if len(strings.Split(pkgName, "/")) == 1 {
		pkgName = "gobuild-official/" + pkgName
	}

	prompt("Repository %v", pkgName)
	dest, err := downloadSource(pkgName)
	if err != nil {
		log.Fatal(err)
	}

	err = deployPackage(pkgName, dest, GOBIN)
	if err != nil {
		log.Fatal(err)
	}

	prompt("Program [%s] installed", pkgName)
}
