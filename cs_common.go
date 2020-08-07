package gocs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// func workDir_() (dir string) {
// 	home, _ := os.UserHomeDir()
// 	dir = filepath.Join(home, ".coversocks")
// 	// if _, err := os.Stat(dir); err != nil {
// 	os.MkdirAll(dir, os.ModePerm)
// 	// }
// 	return
// }

//ExecDir is executor directory
var ExecDir string = execDir()

func execDir() (dir string) {
	dir, _ = exec.LookPath(os.Args[0])
	dir = filepath.Dir(dir)
	dir, _ = filepath.Abs(dir)
	return
}

// var exitf = os.Exit

func parsePortAddr(prefix, addr, subffix string) (addrs []string, err error) {
	var start, end int64
	parts := strings.Split(addr, ",")
	for _, part := range parts {
		ports := strings.SplitN(part, "-", 2)
		start, err = strconv.ParseInt(ports[0], 10, 32)
		if err != nil {
			return
		}
		end = start
		if len(ports) > 1 {
			end, err = strconv.ParseInt(ports[1], 10, 32)
			if err != nil {
				return
			}
		}
		for i := start; i <= end; i++ {
			addrs = append(addrs, fmt.Sprintf("%v%v%v", prefix, i, subffix))
		}
	}
	return
}

var privoxyRunner = map[string]*exec.Cmd{}
var privoxyLock = sync.RWMutex{}
