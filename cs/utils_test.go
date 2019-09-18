package cs

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestPipeConn(t *testing.T) {
	a, b, err := CreatePipeConn()
	if err != nil {
		t.Error(err)
		return
	}
	wait := sync.WaitGroup{}
	wait.Add(2)
	go func() {
		buf := bytes.NewBuffer(nil)
		io.Copy(buf, a)
		fmt.Printf("A:%v\n", string(buf.Bytes()))
		wait.Done()
	}()
	go func() {
		buf := bytes.NewBuffer(nil)
		io.Copy(buf, b)
		fmt.Printf("B:%v\n", string(buf.Bytes()))
		wait.Done()
	}()
	fmt.Fprintf(a, "a message")
	fmt.Fprintf(b, "b message")
	time.Sleep(time.Millisecond)
	a.Close()
	wait.Wait()
	//
	a.SetDeadline(time.Now())
	a.SetReadDeadline(time.Now())
	a.SetWriteDeadline(time.Now())
	a.LocalAddr().Network()
	//
	//test error
	var callc = 0
	BasePipe = func() (r, w *os.File, err error) {
		callc++
		if callc > 1 {
			err = fmt.Errorf("error")
		} else {
			r, w, err = os.Pipe()
		}
		return
	}
	_, _, err = CreatePipeConn()
	if err == nil {
		t.Error(err)
		return
	}
	_, _, err = CreatePipeConn()
	if err == nil {
		t.Error(err)
		return
	}
}

func TestPipeConnClose(t *testing.T) {
	BasePipe = os.Pipe
	wait := make(chan int)
	//
	a, b, err := CreatePipeConn()
	if err != nil {
		t.Error(err)
		return
	}
	go func() {
		buf := make([]byte, 1024)
		a.Read(buf)
		wait <- 1
	}()
	a.Close()
	b.Close()
	<-wait
	//
	a, b, err = CreatePipeConn()
	if err != nil {
		t.Error(err)
		return
	}
	go func() {
		buf := make([]byte, 1024)
		a.Read(buf)
		wait <- 1
	}()
	b.Close()
	a.Close()
	<-wait
}

func TestLog(t *testing.T) {
	//
	SetLogLevel(LogLevelDebug)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(LogLevelInfo)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(LogLevelWarn)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(LogLevelError)
	DebugLog("debug")
	InfoLog("info")
	WarnLog("warn")
	ErrorLog("error")
	//
	SetLogLevel(1)
	ErrorLog("error")
}

func TestSHA(t *testing.T) {
	fmt.Println(SHA1([]byte("123")))
}

func TestStringConn(t *testing.T) {
	{ //test string
		wsRaw := &websocket.Conn{}
		fmt.Printf("%v\n", NewStringConn(wsRaw))
		netRaw := &net.TCPConn{}
		fmt.Printf("%v\n", NewStringConn(netRaw))
		fmt.Printf("%v\n", NewStringConn(os.Stdout))
		conn := NewStringConn(os.Stdout)
		conn.Name = "stdout"
		fmt.Printf("%v\n", conn)
	}
}

func TestTCPKeepAliveListener(t *testing.T) {
	wait := make(chan int, 1)
	listner, _ := net.Listen("tcp", ":0")
	l := &TCPKeepAliveListener{TCPListener: listner.(*net.TCPListener)}
	go func() {
		c, err := l.Accept()
		if err != nil {
			t.Error(err)
			return
		}
		ioutil.ReadAll(c)
		wait <- 1
	}()
	conn, err := net.Dial("tcp", listner.Addr().String())
	if err != nil {
		t.Error(err)
		return
	}
	conn.Close()
	<-wait
}

func TestReadJSON(t *testing.T) {
	err := ReadJSON("ssdkfcsfk", nil)
	if err == nil {
		t.Error(err)
		return
	}
}
