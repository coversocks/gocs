package gocs

import (
	"os"
	"os/exec"
	"path/filepath"
)

var networksetupPath = filepath.Join(execDir(), "networksetup-linux.sh")

func changeProxyModeNative(args ...string) (message string, err error) {
	out, err := exec.Command(networksetupPath, args...).CombinedOutput()
	message = string(out)
	return
}

var privoxyRunner *exec.Cmd
var privoxyPath = filepath.Join(execDir(), "privoxy")

func runPrivoxyNative(conf string) (err error) {
	privoxyRunner = exec.Command(privoxyPath, "--no-daemon", conf)
	privoxyRunner.Stderr = os.Stdout
	privoxyRunner.Stdout = os.Stderr
	err = privoxyRunner.Start()
	if err == nil {
		err = privoxyRunner.Wait()
	}
	privoxyRunner = nil
	return
}
