package gocs

import (
	"fmt"
	"os"
	"os/exec"
)

var networksetupPath = ExecDir + "/networksetup-osx.sh"

func changeProxyModeNative(args ...string) (message string, err error) {
	out, err := exec.Command(networksetupPath, args...).CombinedOutput()
	message = string(out)
	return
}

var privoxyPath = ExecDir + "/privoxy"

func runPrivoxyNative(conf string) (err error) {
	runner := exec.Command(privoxyPath, "--no-daemon", conf)
	runner.Stderr = os.Stdout
	runner.Stdout = os.Stderr
	err = runner.Start()
	if err == nil {
		privoxyLock.Lock()
		privoxyRunner[fmt.Sprintf("%v", runner)] = runner
		privoxyLock.Unlock()
		err = runner.Wait()
		privoxyLock.Lock()
		delete(privoxyRunner, fmt.Sprintf("%v", runner))
		privoxyLock.Unlock()
	}
	return
}
