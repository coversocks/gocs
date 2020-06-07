package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"

	_ "net/http/pprof"
)

func init() {
	go http.ListenAndServe("localhost:6060", nil)
}

func TestConn(t *testing.T) {
	//
	{ //one frame
		data1 := []byte("one")
		buf := make([]byte, 4+len(data1))
		binary.BigEndian.PutUint32(buf, uint32(4+len(data1)))
		copy(buf[4:], data1)
		raw := bytes.NewBuffer(buf)
		proc := NewBaseConn(raw, 256*1024)
		cmd, err := proc.ReadCmd()
		if err != nil || !bytes.Equal(cmd, data1) {
			t.Error(err)
			return
		}
		_, err = proc.ReadCmd()
		if err != io.EOF {
			t.Error(err)
			return
		}
	}
	{ //one frame splice
		data1 := []byte("one")
		r, w, _ := os.Pipe()
		wait := sync.WaitGroup{}
		wait.Add(1)
		go func() {
			proc := NewBaseConn(r, 256*1024)
			cmd, err := proc.ReadCmd()
			if err != nil || !bytes.Equal(cmd, data1) {
				t.Error(err)
				return
			}
			_, err = proc.ReadCmd()
			if err != io.EOF {
				t.Error(err)
				return
			}
			wait.Done()
		}()
		buf := make([]byte, uint32(4+len(data1)))
		binary.BigEndian.PutUint32(buf, uint32(4+len(data1)))
		copy(buf[4:], data1)
		w.Write(buf[0:3])
		time.Sleep(time.Millisecond)
		w.Write(buf[3:])
		time.Sleep(time.Millisecond)
		w.Close()
		time.Sleep(time.Millisecond)
		wait.Wait()
	}
	//
	{ //two frame
		data1 := []byte("two1")
		data2 := []byte("two2")
		buf := make([]byte, 8+len(data1)+len(data2))
		binary.BigEndian.PutUint32(buf, uint32(4+len(data1)))
		copy(buf[4:], data1)
		binary.BigEndian.PutUint32(buf[4+len(data1):], uint32(4+len(data2)))
		copy(buf[8+len(data1):], data2)
		raw := bytes.NewBuffer(buf)
		//
		proc := NewBaseConn(raw, 256*1024)
		cmd, err := proc.ReadCmd()
		if err != nil || !bytes.Equal(cmd, data1) {
			t.Error(err)
			return
		}
		cmd, err = proc.ReadCmd()
		if err != nil || !bytes.Equal(cmd, data2) {
			t.Error(err)
			return
		}
		_, err = proc.ReadCmd()
		if err != io.EOF {
			t.Error(err)
			return
		}
	}
	//
	{ //two frame splice
		data1 := []byte("splice1")
		data2 := []byte("splice2")
		r, w, _ := os.Pipe()
		wait := sync.WaitGroup{}
		wait.Add(1)
		go func() {
			proc := NewBaseConn(r, 256*1024)
			cmd, err := proc.ReadCmd()
			if err != nil || !bytes.Equal(cmd, data1) {
				t.Error(err)
				return
			}
			cmd, err = proc.ReadCmd()
			if err != nil || !bytes.Equal(cmd, data2) {
				t.Error(err)
				return
			}
			_, err = proc.ReadCmd()
			if err != io.EOF {
				t.Error(err)
				return
			}
			wait.Done()
		}()
		buf := make([]byte, 1024)
		binary.BigEndian.PutUint32(buf, uint32(4+len(data1)))
		copy(buf[4:], data1)
		binary.BigEndian.PutUint32(buf[4+len(data1):], uint32(4+len(data2)))
		copy(buf[8+len(data1):], data2[:1])
		w.Write(buf[:8+len(data1)+1])
		time.Sleep(time.Millisecond)
		w.Write(data2[1:])
		time.Sleep(time.Millisecond)
		w.Close()
		time.Sleep(time.Millisecond)
		wait.Wait()
	}
	{ //test too large
		buf := make([]byte, 1024)
		binary.BigEndian.PutUint32(buf, 1000000)
		proc := NewBaseConn(bytes.NewBuffer(buf), 1024)
		_, err := proc.ReadCmd()
		if err == nil {
			t.Error(err)
			return
		}
	}
	{ //test string
		wsRaw := &websocket.Conn{}
		fmt.Printf("%v\n", NewBaseConn(wsRaw, 1024))
		netRaw := &net.TCPConn{}
		fmt.Printf("%v\n", NewBaseConn(netRaw, 1024))
	}
}

func TestDarkSocket(t *testing.T) {
	SetLogLevel(LogLevelDebug)
	//
	buf := make([]byte, 1024)
	wait := sync.WaitGroup{}
	{ //normal process
		//
		serverChannel, clientChannel, _ := CreatePipeConn()
		serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
		serverConn, serverRemote, _ := CreatePipeConn()
		serverConn.Alias, serverRemote.Alias = "ServerConn", "ServerRemote"
		clientConn, clientRemote, _ := CreatePipeConn()
		clientConn.Alias, clientRemote.Alias = "ClientConn", "ClientRemote"
		//
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = serverConn
			DebugLog("test server dial to %v success", remote)
			return
		}))
		wait.Add(1)
		go func() {
			server.ProcConn(NewBaseConn(serverChannel, server.BufferSize))
			wait.Done()
		}()
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = clientChannel
			DebugLog("test client dial to %v success", remote)
			return
		}))
		wait.Add(1)
		go func() {
			client.ProcConn(clientConn, "test")
			wait.Done()
		}()
		//
		serverRemoteData := []byte("server")
		serverRemote.Write(serverRemoteData)
		readed, _ := clientRemote.Read(buf)
		if readed != len(serverRemoteData) || !bytes.Equal(serverRemoteData, buf[:readed]) {
			t.Errorf("error:%v,%v", readed, string(buf[:readed]))
			return
		}
		//
		clientemoteData := []byte("client")
		clientRemote.Write(clientemoteData)
		readed, _ = serverRemote.Read(buf)
		if readed != len(clientemoteData) || !bytes.Equal(clientemoteData, buf[:readed]) {
			t.Errorf("error:%v,%v", readed, string(buf[:readed]))
			return
		}
		//
		serverRemote.Close()
		time.Sleep(time.Millisecond)
		client.Close()
		time.Sleep(time.Millisecond)
		serverChannel.Close()
		fmt.Println("---->normal process done")
	}
	{ //http process
		//
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "ok")
		}))
		serverChannel, clientChannel, _ := CreatePipeConn()
		serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
		serverConn, serverRemote, _ := CreatePipeConn()
		serverConn.Alias, serverRemote.Alias = "ServerConn", "ServerRemote"
		//
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			addr := strings.TrimPrefix(httpServer.URL, "http://")
			raw, err = net.Dial("tcp", addr)
			DebugLog("test server dial to %v", addr)
			return
		}))
		wait.Add(1)
		go func() {
			server.ProcConn(NewBaseConn(serverChannel, server.BufferSize))
			wait.Done()
		}()
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = clientChannel
			DebugLog("test client dial to %v success", remote)
			return
		}))
		res, err := client.HTTPGet("http://xxxx")
		if err != nil || string(res) != "ok" {
			t.Error(err)
			return
		}
		fmt.Println(err, string(res))
		//
		time.Sleep(time.Millisecond)
		fmt.Println("closing server remote")
		serverRemote.Close()
		time.Sleep(time.Millisecond)
		fmt.Println("closing server channel")
		serverChannel.Close()
		fmt.Println("---->http process done")
	}
	{ //server dial error
		//
		serverChannel, clientChannel, _ := CreatePipeConn()
		serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
		serverConn, serverRemote, _ := CreatePipeConn()
		serverConn.Alias, serverRemote.Alias = "ServerConn", "ServerRemote"
		clientConn, clientRemote, _ := CreatePipeConn()
		clientConn.Alias, clientRemote.Alias = "ClientConn", "ClientRemote"
		//
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			if remote == "error" {
				err = fmt.Errorf("dail to %v fail with mock error", remote)
			} else {
				raw = serverConn
			}
			return
		}))
		wait.Add(1)
		go func() {
			server.ProcConn(NewBaseConn(serverChannel, server.BufferSize))
			wait.Done()
		}()
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = clientChannel
			DebugLog("test client dial to %v success", remote)
			return
		}))
		wait.Add(1)
		go func() {
			client.ProcConn(clientConn, "test")
			wait.Done()
		}()
		//
		serverRemoteData := []byte("server")
		serverRemote.Write(serverRemoteData)
		readed, _ := clientRemote.Read(buf)
		if readed != len(serverRemoteData) || !bytes.Equal(serverRemoteData, buf[:readed]) {
			t.Errorf("error:%v,%v", readed, string(buf[:readed]))
			return
		}
		serverRemote.Close()
		time.Sleep(time.Millisecond)
		//
		err := client.ProcConn(clientConn, "error")
		if err == nil {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", err)
		//
		time.Sleep(time.Millisecond)
		serverChannel.Close()
		fmt.Println("---->server dial error done")
	}
	{ //client dialer error
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			err = fmt.Errorf("error")
			return
		}))
		clientConn := NewErrMockConn(1, 1)
		err := client.ProcConn(clientConn, "test")
		if err == nil || err.Error() != "error" {
			t.Error(err)
			return
		}
		fmt.Println("---->client dial error done")
	}
	{ //server proc error
		remoteConn := NewErrMockConn(1, 1)
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = remoteConn
			return
		}))
		//
		procConn1 := NewErrMockConn(1, 1)
		buf1 := make([]byte, 8)
		binary.BigEndian.PutUint32(buf1, 8)
		copy(buf1[4:], []byte("abcd"))
		procConn1.ReadData <- buf1
		err := server.ProcConn(NewBaseConn(procConn1, server.BufferSize))
		if err == nil {
			t.Error(err)
			return
		}
		//
		procConn2 := NewErrMockConn(1, 2)
		buf2 := make([]byte, 8)
		binary.BigEndian.PutUint32(buf2, 8)
		buf2[4] = CmdConnDial
		copy(buf2[5:], []byte("abc"))
		procConn2.ReadData <- buf2
		procConn2.ReadErrC = 2
		err = server.ProcConn(NewBaseConn(procConn2, server.BufferSize))
		if err == nil {
			t.Error(err)
			return
		}
		fmt.Println("---->server proc error done")
	}
	{ //client proc error
		//write channel command error
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			remoteConn := NewErrMockConn(1, 1)
			remoteConn.WriteErrC = 1
			raw = remoteConn
			return
		}))
		clientConn := NewErrMockConn(1, 1)
		err := client.ProcConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		//read channe command error
		client = NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			remoteConn := NewErrMockConn(1, 2)
			remoteConn.ReadErrC = 1
			raw = remoteConn
			return
		}))
		clientConn = NewErrMockConn(1, 2)
		err = client.ProcConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		//channe command error
		client = NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			remoteConn := NewErrMockConn(1, 2)
			buf2 := make([]byte, 8)
			binary.BigEndian.PutUint32(buf2, 8)
			buf2[4] = 0
			copy(buf2[5:], []byte("abc"))
			remoteConn.ReadData <- buf2
			raw = remoteConn
			return
		}))
		clientConn = NewErrMockConn(1, 2)
		err = client.ProcConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		//data command error
		client = NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			remoteConn := NewErrMockConn(1, 2)
			buf2 := make([]byte, 8)
			binary.BigEndian.PutUint32(buf2, 7)
			buf2[4] = CmdConnBack
			copy(buf2[5:], []byte("ok"))
			remoteConn.ReadData <- buf2
			remoteConn.ReadErrC = 2
			raw = remoteConn
			return
		}))
		clientConn = NewErrMockConn(1, 2)
		err = client.ProcConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		// err := client.ProcConn(clientConn, "test")
		// if err == nil || err.Error() != "error" {
		// 	t.Error(err)
		// 	return
		// }
		fmt.Println("---->client proc error done")
	}
	//
	wait.Wait()

}

type ErrMockConn struct {
	ReadData  chan []byte
	ReadErrC  int
	readed    int
	WriteData chan []byte
	WriteErrC int
	writed    int
}

func NewErrMockConn(r, w int) (conn *ErrMockConn) {
	conn = &ErrMockConn{
		ReadData:  make(chan []byte, r),
		WriteData: make(chan []byte, w),
	}
	return
}

func (e *ErrMockConn) Read(p []byte) (n int, err error) {
	e.readed++
	if e.readed == e.ReadErrC {
		err = fmt.Errorf("mock error")
		return
	}
	data := <-e.ReadData
	if data == nil {
		err = fmt.Errorf("closed")
		return
	}
	n = copy(p, data)
	return
}

func (e *ErrMockConn) Write(p []byte) (n int, err error) {
	defer func() {
		message := recover()
		if message != nil {
			err = fmt.Errorf("%v", message)
		}
	}()
	e.writed++
	if e.writed == e.WriteErrC {
		err = fmt.Errorf("mock error")
		return
	}
	e.WriteData <- p
	n = len(p)
	return
}

func (e *ErrMockConn) Close() (err error) {
	defer func() {
		message := recover()
		if message != nil {
			err = fmt.Errorf("%v", message)
		}
	}()
	close(e.ReadData)
	close(e.WriteData)
	return
}

func (e *ErrMockConn) Reset() {
	e.readed = 0
	e.ReadErrC = 0
	e.writed = 0
	e.WriteErrC = 0
}

func TestCopyRemote2ChannelErr(t *testing.T) {
	var target, conn *ErrMockConn
	var err error
	{
		//
		//target read error
		target = NewErrMockConn(1, 1)
		conn = NewErrMockConn(1, 2)
		target.ReadErrC = 1
		err = copyRemote2Channel(1024, NewBaseConn(conn, 1024), target)
		fmt.Println("xxxx")
		<-conn.WriteData
		if err == nil {
			t.Error(err)
			return
		}
	}
	{
		//
		//conn write error
		target = NewErrMockConn(1, 1)
		conn = NewErrMockConn(1, 2)
		conn.WriteErrC = 1
		target.ReadData <- []byte("test")
		err = copyRemote2Channel(1024, NewBaseConn(conn, 1024), target)
		if err == nil {
			t.Error(err)
			return
		}
	}
}

func TestCopyChannel2RemoteErr(t *testing.T) {
	var target, conn *ErrMockConn
	var err error
	{
		//
		//conn read error
		target = NewErrMockConn(1, 1)
		conn = NewErrMockConn(1, 1)
		conn.ReadErrC = 1
		err = copyChannel2Remote(NewBaseConn(conn, 1024), target)
		if err == nil {
			t.Error(err)
			return
		}
	}
	{
		//
		//conn command error
		target = NewErrMockConn(1, 1)
		conn = NewErrMockConn(1, 1)
		buf := make([]byte, 8)
		binary.BigEndian.PutUint32(buf, 8)
		copy(buf[4:], []byte("abcd"))
		conn.ReadData <- buf
		err = copyChannel2Remote(NewBaseConn(conn, 1024), target)
		if err == nil {
			t.Error(err)
			return
		}
	}
}
