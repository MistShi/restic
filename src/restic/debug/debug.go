// +build debug

package debug

import (
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"restic/fs"
	"runtime"
	"strings"

	"restic/errors"
)

var opts struct {
	logger *log.Logger
	funcs  map[string]bool
	files  map[string]bool
}

// make sure that all the initialization happens before the init() functions
// are called, cf https://golang.org/ref/spec#Package_initialization
var _ = initDebug()

func initDebug() bool {
	initDebugLogger()
	initDebugTags()

	fmt.Fprintf(os.Stderr, "debug enabled\n")

	return true
}

func initDebugLogger() {
	debugfile := os.Getenv("DEBUG_LOG")
	if debugfile == "" {
		return
	}

	fmt.Fprintf(os.Stderr, "debug log file %v\n", debugfile)

	f, err := fs.OpenFile(debugfile, os.O_WRONLY|os.O_APPEND, 0600)

	if err == nil {
		_, err = f.Seek(2, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to seek to the end of %v: %v\n", debugfile, err)
			os.Exit(3)
		}
	}

	if err != nil && os.IsNotExist(errors.Cause(err)) {
		f, err = fs.OpenFile(debugfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to open debug log file: %v\n", err)
		os.Exit(2)
	}

	opts.logger = log.New(f, "", log.LstdFlags)
}

func parseFilter(envname string, pad func(string) string) map[string]bool {
	filter := make(map[string]bool)

	env := os.Getenv(envname)
	if env == "" {
		return filter
	}

	for _, fn := range strings.Split(env, ",") {
		t := pad(strings.TrimSpace(fn))
		val := true
		if t[0] == '-' {
			val = false
			t = t[1:]
		} else if t[0] == '+' {
			val = true
			t = t[1:]
		}

		// test pattern
		_, err := path.Match(t, "")
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid pattern %q: %v\n", t, err)
			os.Exit(5)
		}

		filter[t] = val
	}

	return filter
}

func padFunc(s string) string {
	if s == "all" {
		return s
	}

	return s
}

func padFile(s string) string {
	if s == "all" {
		return s
	}

	if !strings.Contains(s, "/") {
		s = "*/" + s
	}

	if !strings.Contains(s, ":") {
		s = s + ":*"
	}

	return s
}

func initDebugTags() {
	opts.funcs = parseFilter("DEBUG_FUNCS", padFunc)
	opts.files = parseFilter("DEBUG_FILES", padFile)
}

// taken from https://github.com/VividCortex/trace
func goroutineNum() int {
	b := make([]byte, 20)
	runtime.Stack(b, false)
	var num int

	fmt.Sscanf(string(b), "goroutine %d ", &num)
	return num
}

// taken from https://github.com/VividCortex/trace
func getPosition() (fn, dir, file string, line int) {
	pc, file, line, ok := runtime.Caller(2)
	if !ok {
		return "", "", "", 0
	}

	dirname, filename := filepath.Base(filepath.Dir(file)), filepath.Base(file)

	Func := runtime.FuncForPC(pc)

	return path.Base(Func.Name()), dirname, filename, line
}

func checkFilter(filter map[string]bool, key string) bool {
	// check if key is enabled directly
	if v, ok := filter[key]; ok {
		return v
	}

	// check for globbing
	for k, v := range filter {
		if m, _ := path.Match(k, key); m {
			return v
		}
	}

	// check if tag "all" is enabled
	if v, ok := filter["all"]; ok && v {
		return true
	}

	return false
}

// Log prints a message to the debug log (if debug is enabled).
func Log(f string, args ...interface{}) {
	fn, dir, file, line := getPosition()
	goroutine := goroutineNum()

	if len(f) == 0 || f[len(f)-1] != '\n' {
		f += "\n"
	}

	pos := fmt.Sprintf("%s/%s:%d", dir, file, line)

	formatString := fmt.Sprintf("%s\t%s\t%d\t%s", pos, fn, goroutine, f)

	dbgprint := func() {
		fmt.Fprintf(os.Stderr, formatString, args...)
	}

	if opts.logger != nil {
		opts.logger.Printf(formatString, args...)
	}

	if checkFilter(opts.files, fmt.Sprintf("%s/%s:%d", dir, file, line)) {
		dbgprint()
		return
	}

	if checkFilter(opts.funcs, fn) {
		dbgprint()
	}
}
