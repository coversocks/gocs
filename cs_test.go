package gocs

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/coversocks/gocs/core"
)

func init() {
	go http.ListenAndServe(":6060", nil)
}

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
	wait := &sync.WaitGroup{}
	SetLogLevel(LogLevelDebug)
	var err error
	ts := httptest.NewServer(http.FileServer(http.Dir(".")))
	gfwListURL = fmt.Sprintf("%v/gfwlist.txt", ts.URL)
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
	wait.Add(1)
	go func() {
		WaitServer()
		wait.Done()
	}()
	err = StartClient("cs_test_client.json")
	if err != nil {
		t.Error(err)
		return
	}
	wait.Add(1)
	go func() {
		WaitClient()
		wait.Done()
	}()
	time.Sleep(300 * time.Millisecond)
	echo := xio.NewEchoDialer()
	server.Dialer.(*core.NetDialer).TCP = core.RawDialerF(func(network, address string) (net.Conn, error) {
		fmt.Printf("dial to %v://%v\n", network, address)
		if address == "echo" {
			return echo.Dial(network, address)
		}
		return net.Dial(network, address)
	})
	conn := newBufferConn()
	go func() {
		err = clientInstance.PipeConn(conn, "tcp://echo")
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
	//test forward
	testServer := http.Server{Addr: "127.0.0.1:11923", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "123")
	})}
	testListener, err := net.Listen("tcp", testServer.Addr)
	if err != nil {
		t.Error(err)
		return
	}
	defer testListener.Close()
	go testServer.Serve(testListener)
	res, err = xhttp.GetText("http://127.0.0.1:10923")
	if err != nil || res != "123" {
		t.Error(err)
		return
	}
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
	res, err = xhttp.GetText("http://127.0.0.1:11101/updateGfwlist")
	if err != nil || res != "ok" {
		t.Error(err)
		return
	}
	res, err = xhttp.GetText("http://127.0.0.1:11101/state")
	if err != nil || res == "" {
		t.Error(err)
		return
	}
	//
	//test error
	gfwListURL = "http://127.0.0.1:3211"
	_, err = xhttp.GetText("http://127.0.0.1:11101/updateGfwlist")
	if err == nil {
		t.Error(err)
		return
	}

	//
	c := clientInstance
	StopClient()
	StopServer()
	c.Client = nil
	c.UpdateGfwlist()
	fmt.Printf("client info %v\n", clientInstance)
	wait.Wait()
}

type bufferConn2 struct {
	uri    string
	sendc  int
	sended []byte
	recved []byte
	recv   chan int
}

func newBufferConn2() *bufferConn2 {
	return &bufferConn2{
		recv: make(chan int, 1),
	}
}

func (b *bufferConn2) Read(p []byte) (n int, err error) {
	switch b.sendc {
	case 0:
		n = copy(p, b.sended)
		b.sendc++
	default:
		<-b.recv
		err = fmt.Errorf("closed")
	}
	return
}

func (b *bufferConn2) Write(p []byte) (n int, err error) {
	t := make([]byte, len(p))
	n = copy(t, p)
	b.recved = t
	b.recv <- 1
	return
}

func (b *bufferConn2) Close() (err error) {
	return
}

func (b *bufferConn2) String() string {
	return b.uri
}

func TestMultiProxy(t *testing.T) {
	core.SetLogLevel(1)
	SetLogLevel(1)
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
	err = StartClient("cs_test_client.json")
	if err != nil {
		t.Error(err)
		return
	}
	defer func() {
		StopClient()
		StopServer()
	}()
	time.Sleep(300 * time.Millisecond)
	echo := xio.NewEchoDialer()
	server.Dialer.(*core.NetDialer).TCP = core.RawDialerF(func(network, address string) (net.Conn, error) {
		if address == "echo" {
			return echo.Dial(network, address)
		}
		return net.Dial(network, address)
	})

	runCase := func(i int) {
		data := fmt.Sprintf("data-%v", i)
		conn := newBufferConn2()
		conn.uri = data
		conn.sended = []byte(data)
		err := clientInstance.PipeConn(conn, "tcp://echo?a="+data)
		if string(conn.recved) != data {
			t.Errorf("error:%v,%v,%v", err, conn.recved, data)
			panic("error")
		}
	}
	begin := time.Now()
	runnerc := 100
	waitc := make(chan int, 10000)
	totalc := 10000
	waiter := sync.WaitGroup{}
	for k := 0; k < runnerc; k++ {
		waiter.Add(1)
		go func() {
			for {
				i := <-waitc
				if i < 0 {
					break
				}
				runCase(i)
			}
			waiter.Done()
			// fmt.Printf("runner is done\n\n")
		}()
	}
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < totalc; i++ {
		waitc <- i
	}
	for k := 0; k < runnerc; k++ {
		waitc <- -1
	}
	waiter.Wait()
	used := time.Since(begin)
	fmt.Printf("total:%v,used:%v,avg:%v\n", totalc, used, used/time.Duration(totalc))
}

func initProxy() (err error) {
	if server != nil {
		return
	}
	os.Mkdir("test_work_dir", os.ModePerm)
	exec.Command("cp", "-f", "abp.js", "test_work_dir/").CombinedOutput()
	exec.Command("cp", "-f", "gfwlist.txt", "test_work_dir/").CombinedOutput()
	defer os.RemoveAll("test_work_dir")
	networksetupPath = "echo"
	err = StartServer("cs_test_server.json")
	if err != nil {
		return
	}
	err = StartClient("cs_test_client.json")
	if err != nil {
		return
	}
	time.Sleep(300 * time.Millisecond)
	echo := xio.NewEchoDialer()
	server.Dialer.(*core.NetDialer).TCP = core.RawDialerF(func(network, address string) (net.Conn, error) {
		if address == "echo" {
			return echo.Dial(network, address)
		}
		return net.Dial(network, address)
	})
	return
}

func BenchmarkProxy(b *testing.B) {
	core.SetLogLevel(1)
	SetLogLevel(1)
	initProxy()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			conn := newBufferConn2()
			conn.sended = []byte("123")
			err := clientInstance.PipeConn(conn, "tcp://echo")
			if string(conn.recved) != "123" {
				b.Errorf("error:%v,%v", err, conn.recved)
				return
			}
		}
	})
	fmt.Printf("BenchmarkProxy is done\n")
}
