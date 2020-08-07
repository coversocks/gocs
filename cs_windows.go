package gocs

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
)

func sysproxyPath() string {
	var runner = ExecDir + "\\sysproxy.exe"
	if runtime.GOARCH == "amd64" {
		runner = ExecDir + "\\sysproxy64.exe"
	}
	return runner
}

var networksetupPath = sysproxyPath()

func changeProxyModeNative(args ...string) (message string, err error) {
	var cmd *exec.Cmd
	switch args[0] {
	case "auto":
		cmd = exec.Command(networksetupPath, "pac", args[1])
	case "global":
		cmd = exec.Command(networksetupPath, "global", args[1]+":"+args[2])
	default:
		cmd = exec.Command(networksetupPath, "off")
	}
	out, err := cmd.CombinedOutput()
	message = string(out)
	return
}

var privoxyPath = ExecDir + "\\privoxy.exe"

func runPrivoxyNative(conf string) (err error) {
	runner := exec.Command(privoxyPath, conf)
	runner.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
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
