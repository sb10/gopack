package main

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	sh "github.com/codeskyblue/go-sh"
	"github.com/gobuild/log"
	"github.com/urfave/cli"
)

func shExecString(sess *sh.Session, command string) error {
	if runtime.GOOS == "windows" {
		return sess.Command("cmd", "/c", command).Run()
	}
	return sess.Command("bash", "-c", command).Run()
}

func findFiles(path string, depth int, skips []*regexp.Regexp) ([]string, error) {
	baseNumSeps := strings.Count(path, string(os.PathSeparator))
	var files []string
	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Debugf("filewalk: %s", err)
			return nil
		}
		if info.IsDir() {
			pathDepth := strings.Count(path, string(os.PathSeparator)) - baseNumSeps
			//log.Println("Skip", info, pathDepth, depth)
			if pathDepth > depth {
				return filepath.SkipDir
			}
		}
		name := info.Name()
		isSkip := false
		for _, skip := range skips {
			if skip.MatchString(name) || skip.MatchString(path) {
				isSkip = true
				break
			}
		}
		if isSkip {
			log.Debug("skip:", path)
		}
		if !isSkip {
			files = append(files, path)
		}
		if isSkip && info.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	return files, err
}

func actionPack(c *cli.Context) {
	var goos, goarch = c.String("os"), c.String("arch")
	//var depth = c.Int("depth")
	var output = c.String("output")
	var gom = c.String("gom")
	var nobuild = c.Bool("nobuild")
	var adds = c.StringSlice("add")
	var rmflag = c.Bool("rm")

	if c.GlobalBool("debug") {
		log.SetOutputLevel(log.Ldebug)
	}
	if c.Bool("quiet") {
		log.SetOutputLevel(log.Lwarn)
	}

	log.Debugf("os: %s, arch: %s", goos, goarch)

	var err error
	defer func() {
		if err != nil {
			log.Fatal(err)
		}
	}()
	sess := sh.NewSession()
	sess.SetEnv("GOOS", goos)
	sess.SetEnv("GOARCH", goarch)
	sess.ShowCMD = !c.Bool("quiet")

	gomarr := strings.Fields(gom)
	if len(gomarr) >= 1 {
		sess.Alias("go", gomarr[0], gomarr[1:]...)
	}

	// parse yaml
	pcfg := DefaultPcfg
	log.Debug("config:", pcfg)

	var skips []*regexp.Regexp
	for _, str := range pcfg.Excludes {
		skips = append(skips, regexp.MustCompile("^"+str+"$"))
	}
	var needs []*regexp.Regexp
	for _, str := range pcfg.Includes {
		needs = append(needs, regexp.MustCompile("^"+str+"$"))
	}

	os.MkdirAll(filepath.Dir(output), 0755)

	var z Archiver
	hasExt := func(ext string) bool { return strings.HasSuffix(output, ext) }
	switch {
	case hasExt(".zip"):
		log.Println("zip format")
		z, err = CreateZip(output)
	case hasExt(".tar"):
		log.Println("tar format")
		z, err = CreateTar(output)
	case hasExt(".tgz"):
		fallthrough
	case hasExt(".tar.gz"):
		log.Println("tar.gz format")
		z, err = CreateTgz(output)
	default:
		log.Println("unsupport file archive format")
		os.Exit(1)
	}
	if err != nil {
		return
	}

	var files = []string{}
	var buildFiles = pcfg.Settings.Outfiles
	// build source
	if !nobuild {
		targetDir := sanitizedName(pcfg.Settings.TargetDir)
		if !sh.Test("dir", targetDir) {
			os.MkdirAll(targetDir, 0755)
		}

		//gobin := filepath.Join(pwd, sanitizedName(pcfg.Settings.TargetDir))
		//sess.SetEnv("GOBIN", gobin)
		//log.Debugf("set env GOBIN=%s", gobin)
		/*
			symdir := filepath.Join(targetDir, goos+"_"+goarch)
			if err = os.Symlink(gobin, symdir); err != nil {
				log.Fatalf("os symlink error: %s", err)
			}
		*/
		//defer os.Remove(symdir)
		// opts := []string{"install", "-v"}
		// opts = append(opts, strings.Fields(pcfg.Settings.Addopts)...) // TODO: here need to use shell args parse lib
		//if pcfg.Settings.Build == "" {
		//pcfg.Settings.Build = DEFAULT_BUILD
		//}

		for _, command := range pcfg.Script {
			if err = shExecString(sess, command); err != nil {
				return
			}
		}
		//os.Remove(symdir) // I have to do it twice

		if len(buildFiles) == 0 {
			cwd, _ := os.Getwd()
			program := filepath.Base(cwd)
			buildFiles = append(buildFiles, program)
		}

		for _, filename := range buildFiles {
			if goos == "windows" {
				filename += ".exe"
			}
			files = append(files, filename)
		}
	}

	log.Debug("archive files")
	depth := pcfg.Depth
	for _, filename := range pcfg.Includes {
		fs, err := findFiles(filename, depth, skips)
		if err != nil {
			return
		}
		files = append(files, fs...)
	}

	// adds - parse by cli
	files = append(files, adds...)
	uniqset := make(map[string]bool, len(files))
	for _, file := range files {
		file = sanitizedName(file)
		if uniqset[file] {
			continue
		}
		uniqset[file] = true
		log.Infof("zip add file: %v", file)
		if err = z.Add(file); err != nil {
			return
		}
	}
	log.Info("finish archive file")
	err = z.Close()

	if rmflag {
		for _, filename := range buildFiles {
			if goos == "windows" {
				filename += ".exe"
			}
			os.Remove(filename)
		}
	}
}
