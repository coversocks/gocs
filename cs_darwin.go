package gocs

import (
	"os"
	"os/exec"
)

var networksetupPath = ExecDir + "/networksetup-osx.sh"

func changeProxyModeNative(args ...string) (message string, err error) {
	out, err := exec.Command(networksetupPath, args...).CombinedOutput()
	message = string(out)
	return
}

var privoxyRunner *exec.Cmd
var privoxyPath = ExecDir + "/privoxy"

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
