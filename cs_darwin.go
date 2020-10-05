package gocs

import (
	"os/exec"
)

var networksetupPath = ExecDir + "/networksetup-osx.sh"

func changeProxyModeNative(args ...string) (message string, err error) {
	out, err := exec.Command(networksetupPath, args...).CombinedOutput()
	message = string(out)
	return
}
