package gocs

import (
	"os"
	"os/exec"
	"testing"

	"github.com/codingeasygo/util/runner"
	"github.com/coversocks/gocs/core"
)

func TestServerError(t *testing.T) {
	server := NewServer("", ServerConf{}, core.NewNetDialer("", ""))
	server.httpStart()
	server.httpsStart()
}

func TestRestart(t *testing.T) {
	var err error
	os.Mkdir("test_work_dir", os.ModePerm)
	exec.Command("cp", "-f", "abp.js", "test_work_dir/").CombinedOutput()
	exec.Command("cp", "-f", "gfwlist.txt", "test_work_dir/").CombinedOutput()
	defer os.RemoveAll("test_work_dir")
	networksetupPath = "echo"
	err = StartServer("cs_test_server.json")
	if err != nil {
		t.Error(err)
		return
	}
	server.Conf.HTTPSGen = 0
	err = server.ProcRestart()
	if err != runner.ErrNotTask {
		t.Error("error")
		return
	}
	server.Conf.HTTPSGen = 1
	err = server.ProcRestart()
	if err != nil {
		t.Error("error")
		return
	}
	StopServer()
}
