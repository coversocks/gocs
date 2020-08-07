package gocs

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/codingeasygo/util/xhttp"
	"github.com/coversocks/gocs/core"
)

type bufferConn struct {
	recv chan []byte
	send chan []byte
}

func newBufferConn() *bufferConn {
	return &bufferConn{
		send: make(chan []byte, 100),
		recv: make(chan []byte, 100),
	}
}

func (b *bufferConn) Read(p []byte) (n int, err error) {
	fmt.Printf("start read bytes\n")
	d := <-b.send
	n = copy(p, d)
	fmt.Printf("send %v bytes is done\n", n)
	return
}

func (b *bufferConn) Write(p []byte) (n int, err error) {
	fmt.Printf("start write bytes\n")
	buf := make([]byte, len(p))
	n = copy(buf, p)
	b.recv <- buf
	fmt.Printf("recv %v bytes is done\n", n)
	return
}

func (b *bufferConn) Close() (err error) {
	return
}

func TestProxy(t *testing.T) {
	os.Mkdir("test_work_dir", os.ModePerm)
	exec.Command("cp", "-f", "abp.js", "test_work_dir/").CombinedOutput()
	exec.Command("cp", "-f", "gfwlist.txt", "test_work_dir/").CombinedOutput()
	defer os.RemoveAll("test_work_dir")
	networksetupPath = "echo"
	privoxyPath = "echo"
	go func() {
		StartServer("cs_test_server.json")
	}()
	go func() {
		StartClient("cs_test_client.json")
	}()
	defer func() {
		StopClient()
		StopServer()
	}()
	time.Sleep(300 * time.Millisecond)
	echo := core.NewEchoDialer()
	serverDialer.TCP = core.RawDialerF(func(network, address string) (net.Conn, error) {
		fmt.Printf("dial to %v://%v\n", network, address)
		if address == "echo" {
			return echo.Dial(network, address)
		}
		return net.Dial(network, address)
	})
	var err error
	conn := newBufferConn()
	go func() {
		err = client.ProcConn(conn, "tcp://echo")
		if err != nil {
			t.Error(err)
			return
		}
	}()
	conn.send <- []byte("123")
	data := <-conn.recv
	if string(data) != "123" {
		t.Errorf("error:%v", data)
		return
	}
	var res string
	//test pac
	res, err = xhttp.GetText("http://127.0.0.1:11101/pac.js")
	if err != nil || !strings.Contains(res, "FindProxyForURL") {
		t.Error(err)
		return
	}
	//test proxy mode
	res, err = xhttp.GetText("http://127.0.0.1:11101/changeProxyMode?mode=auto")
	if err != nil || res != "ok" {
		t.Error(err)
		return
	}
	res, err = xhttp.GetText("http://127.0.0.1:11101/changeProxyMode?mode=global")
	if err != nil || res != "ok" {
		t.Error(err)
		return
	}
}
