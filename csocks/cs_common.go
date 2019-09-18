package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

var workDir = workDir_()

func workDir_() (dir string) {
	home, _ := os.UserHomeDir()
	dir = filepath.Join(home, ".coversocks")
	// if _, err := os.Stat(dir); err != nil {
	os.MkdirAll(dir, os.ModePerm)
	// }
	return
}

func execDir() (dir string) {
	dir, _ = exec.LookPath(os.Args[0])
	dir = filepath.Dir(dir)
	dir, _ = filepath.Abs(dir)
	return
}
