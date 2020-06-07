package gocs

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

func sysproxyPath() string {
	var runner = filepath.Join(execDir(), "sysproxy.exe")
	if runtime.GOARCH == "amd64" {
		runner = filepath.Join(execDir(), "sysproxy64.exe")
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

var privoxyRunner *exec.Cmd
var privoxyPath = filepath.Join(execDir(), "privoxy.exe")

func runPrivoxyNative(conf string) (err error) {
	privoxyRunner = exec.Command(privoxyPath, conf)
	privoxyRunner.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	privoxyRunner.Stderr = os.Stdout
	privoxyRunner.Stdout = os.Stderr
	err = privoxyRunner.Start()
	if err == nil {
		err = privoxyRunner.Wait()
	}
	privoxyRunner = nil
	return
}
