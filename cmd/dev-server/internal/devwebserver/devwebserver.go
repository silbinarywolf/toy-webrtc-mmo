package devwebserver

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/tools/go/packages"
)

const (
	packagePath = "github.com/silbinarywolf/toy-webrtc-mmo/cmd/dev-server/internal/devwebserver"
)

var flagSet = flag.NewFlagSet("serve", flag.ExitOnError)

// Serve will serve a build of the application to the web browser.
// This function will block until exit.
func Serve() {
	tags := flagSet.String("tags", "", "a list of build tags to consider satisfied during the build")
	//verbose := flagSet.Bool("verbose", false, "verbose")

	// Setup
	args := Arguments{}
	args.Port = ":8080"
	args.Directory = "."
	if tags != nil {
		args.Tags = *tags
	}
	arguments = args

	// Validation of settings
	dir := args.Directory
	if dir != "." {
		panic("Specifying a custom directory is not currently supported.")
	}

	// Get default resources
	var err error
	wasmJSPath, err = getDefaultWasmJSPath(args.Directory)
	if err != nil {
		panic(err)
	}
	//fmt.Printf("wasm_exec.js: %s\n", wasmJSPath)
	indexHTMLPath, err = getDefaultIndexHTMLPath(args.Directory)
	if err != nil {
		panic(err)
	}
	//fmt.Printf("index.html: %s\n", indexHTMLPath)

	// Start server
	fmt.Printf("Listening on http://localhost%s...\n", args.Port)
	http.HandleFunc("/", handle)
	//shared.OpenBrowser("http://localhost" + args.Port)
	if err := http.ListenAndServe(args.Port, nil); err != nil {
		panic(err)
	}
}

var wasmJSPath string

var indexHTMLPath string

var (
	arguments    Arguments
	tmpOutputDir = ""
)

type Arguments struct {
	Port      string // :8080
	Directory string // .
	Tags      string // ie. "debug"
}

func handle(w http.ResponseWriter, r *http.Request) {
	output, err := ensureTmpOutputDir()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dir := arguments.Directory
	tags := arguments.Tags

	// Get path and package
	upath := r.URL.Path[1:]
	pkg := filepath.Dir(upath)
	fpath := filepath.Join(".", filepath.Base(upath))
	if strings.HasSuffix(r.URL.Path, "/") {
		fpath = filepath.Join(fpath, "index.html")
	}

	parts := strings.Split(upath, "/")
	isAsset := len(parts) > 0 && parts[0] == "asset"

	if isAsset {
		// Load asset
		log.Print("serving asset: " + upath)

		// todo(Jake): 2018-12-30
		// Improve this so when "data" folder support
		// is added, this allows any filetype from the "data" folder.
		switch ext := filepath.Ext(upath); ext {
		case ".ttf",
			".data",
			".json":
			http.ServeFile(w, r, upath)
		}
		return
	}

	switch filepath.Base(fpath) {
	case "index.html":
		log.Print("serving index.html: " + indexHTMLPath)
		http.ServeFile(w, r, indexHTMLPath)
	case "wasm_exec.js":
		log.Print("serving index.html: " + wasmJSPath)
		http.ServeFile(w, r, wasmJSPath)
		return
	case "main.wasm":
		if _, err := os.Stat(fpath); os.IsNotExist(err) {
			// go build
			args := []string{"build", "-o", filepath.Join(output, "main.wasm")}
			if tags != "" {
				args = append(args, "-tags", tags)
			}
			args = append(args, pkg)
			log.Print("go ", strings.Join(args, " "))
			cmdBuild := exec.Command(gobin(), args...)
			cmdBuild.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
			cmdBuild.Dir = dir
			out, err := cmdBuild.CombinedOutput()
			if err != nil {
				log.Print(err)
				log.Print(string(out))
				http.Error(w, string(out), http.StatusInternalServerError)
				return
			}
			if len(out) > 0 {
				log.Print(string(out))
			}

			f, err := os.Open(filepath.Join(output, "main.wasm"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer f.Close()
			http.ServeContent(w, r, "main.wasm", time.Now(), f)
			return
		}
	}
}

func gobin() string {
	return filepath.Join(runtime.GOROOT(), "bin", "go")
}

func ensureTmpOutputDir() (string, error) {
	if tmpOutputDir != "" {
		return tmpOutputDir, nil
	}

	tmp, err := ioutil.TempDir("", "")
	if err != nil {
		return "", err
	}
	tmpOutputDir = tmp
	return tmpOutputDir, nil
}

var (
	cmdDir string
	cmdErr error
)

func computeCmdSourceDir(gameDir string) (string, error) {
	if cmdDir == "" && cmdErr == nil {
		cmdDir, cmdErr = computeCmdSourceDirUncached(gameDir)
	}
	return cmdDir, cmdErr
}

func computeCmdSourceDirUncached(gameDir string) (string, error) {
	currentDir, err := filepath.Abs(gameDir)
	if err != nil {
		return "", err
	}
	cfg := &packages.Config{
		Dir: currentDir,
	}
	pkgs, err := packages.Load(cfg, packagePath)
	if err != nil {
		return "", err
	}
	if len(pkgs) == 0 {
		return "", errors.New("Unable to find package: " + packagePath)
	}
	pkg := pkgs[0]
	if len(pkg.GoFiles) == 0 {
		return "", errors.New("Cannot find *.go files in:" + currentDir)
	}
	dir := filepath.Dir(pkg.GoFiles[0])
	return dir, nil
}

func getDefaultWasmJSPath(gameDir string) (string, error) {
	const baseName = "wasm_exec.js"
	// Look for user-override
	/* {
		dir := gameDir + "/html/" + baseName
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			return dir, nil
		}
	} */
	// Look for engine default
	dir, err := computeCmdSourceDir(gameDir)
	if err != nil {
		return "", err
	}
	dir = dir + "/" + baseName
	return dir, nil
}

func getDefaultIndexHTMLPath(gameDir string) (string, error) {
	const baseName = "index.html"
	// Look for user-override
	/* {
		dir := gameDir + "/html/" + baseName
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			return dir, nil
		}
	} */
	// Look for engine default
	dir, err := computeCmdSourceDir(gameDir)
	if err != nil {
		return "", err
	}
	dir = dir + "/" + baseName
	return dir, nil
}
