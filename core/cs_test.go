package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codingeasygo/util/xio"

	"net/http/httptest"
	_ "net/http/pprof"
)

func init() {
	SetLogLevel(LogLevelDebug)
	go http.ListenAndServe("localhost:6061", nil)
}

func TestCoversocket(t *testing.T) {
	buf := make([]byte, 1024)
	wait := sync.WaitGroup{}
	{ //normal process
		//
		serverChannel, clientChannel, _ := xio.CreatePipedConn()
		serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
		serverConn, serverRemote, _ := xio.CreatePipedConn()
		serverConn.Alias, serverRemote.Alias = "ServerConn", "ServerRemote"
		clientConn, clientRemote, _ := xio.CreatePipedConn()
		clientConn.Alias, clientRemote.Alias = "ClientConn", "ClientRemote"
		//
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = serverConn
			DebugLog("test server dial to %v success", remote)
			return
		}))
		wait.Add(1)
		go func() {
			server.ProcConn(serverChannel, "")
			wait.Done()
		}()
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = clientChannel
			DebugLog("test client dial to %v success", remote)
			return
		}))
		client.MaxIdle = 0
		wait.Add(1)
		go func() {
			client.PipeConn(clientConn, "test")
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
		server.Close()
		wait.Wait()
		fmt.Println("---->normal process done")
	}
	{ //push pull
		var n int
		message := "test..."
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			DebugLog("test server dial to %v success", remote)
			raw = xio.NewPipedChan()
			return
		}))
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			serverChannel, clientChannel, _ := xio.CreatePipedConn()
			serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
			raw = clientChannel
			wait.Add(1)
			go func() {
				server.ProcConn(serverChannel, "")
				wait.Done()
			}()
			DebugLog("test client dial to %v success", remote)
			return
		}))
		client.MaxIdle = 0
		client.TickerDelay = 1 * time.Millisecond
		//
		clientConn1, clientRemote1, _ := xio.CreatePipedConn()
		clientConn1.Alias, clientRemote1.Alias = "ClientConn1", "ClientRemote1"
		wait.Add(1)
		go func() {
			client.PipeConn(clientConn1, "test")
			wait.Done()
		}()

		clientRemote1.Write([]byte(message))
		n, _ = clientRemote1.Read(buf)
		if message != string(buf[0:n]) {
			t.Error("error")
			return
		}
		//
		clientConn2, clientRemote2, _ := xio.CreatePipedConn()
		clientConn2.Alias, clientRemote2.Alias = "ClientConn", "ClientRemote"
		wait.Add(1)
		go func() {
			client.PipeConn(clientConn2, "test")
			wait.Done()
		}()
		clientRemote2.Write([]byte(message))
		clientRemote2.Read(buf)
		if message != string(buf[0:n]) {
			t.Error("error")
			return
		}
		//
		clientRemote1.Close()
		time.Sleep(20 * time.Millisecond)
		client.Close()
		server.Close()
		wait.Wait()
		fmt.Println("---->push pull done")
	}
	{ //conn timeout
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			DebugLog("test server dial to %v success", remote)
			raw = xio.NewPipedChan()
			return
		}))
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			serverChannel, clientChannel, _ := xio.CreatePipedConn()
			serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
			raw = clientChannel
			wait.Add(1)
			go func() {
				server.ProcConn(serverChannel, "")
				wait.Done()
			}()
			DebugLog("test client dial to %v success", remote)
			return
		}))
		client.MaxIdle = 0
		client.TickerDelay = 1 * time.Millisecond
		client.KeepIdle = 10 * time.Millisecond
		//
		clientConn1, clientRemote1, _ := xio.CreatePipedConn()
		clientConn1.Alias, clientRemote1.Alias = "ClientConn1", "ClientRemote1"
		wait.Add(1)
		go func() {
			client.PipeConn(clientConn1, "test")
			wait.Done()
		}()
		clientRemote1.Close()
		time.Sleep(150 * time.Millisecond)
		client.Close()
		server.Close()
		wait.Wait()
		fmt.Println("---->conn timeout done")
	}
	{ //test dialer
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			DebugLog("test server dial to %v success", remote)
			raw = xio.NewPipedChan()
			return
		}))
		dialer := NewSortedDialer(DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			serverChannel, clientChannel, _ := xio.CreatePipedConn()
			serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
			raw = clientChannel
			wait.Add(1)
			go func() {
				server.ProcConn(serverChannel, "")
				wait.Done()
			}()
			DebugLog("test client dial to %v success", remote)
			return
		}))
		client := NewClient(256*1024, dialer)
		client.MaxIdle = 0
		client.TickerDelay = 1 * time.Millisecond
		client.KeepIdle = 10 * time.Millisecond
		//
		clientConn1, clientRemote1, _ := xio.CreatePipedConn()
		clientConn1.Alias, clientRemote1.Alias = "ClientConn1", "ClientRemote1"
		wait.Add(1)
		go func() {
			client.PipeConn(clientConn1, "test")
			wait.Done()
		}()
		clientRemote1.Close()
		time.Sleep(150 * time.Millisecond)
		client.Close()
		server.Close()
		wait.Wait()
		fmt.Println("---->test dialer done")
	}
	{ //http process
		//
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "ok")
		}))
		serverChannel, clientChannel, _ := xio.CreatePipedConn()
		serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
		serverConn, serverRemote, _ := xio.CreatePipedConn()
		serverConn.Alias, serverRemote.Alias = "ServerConn", "ServerRemote"
		//
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			addr := strings.TrimPrefix(httpServer.URL, "http://")
			raw, err = net.Dial("tcp", addr)
			DebugLog("test server dial to %v by %v", addr, err)
			return
		}))
		wait.Add(1)
		go func() {
			server.ProcConn(serverChannel, "")
			wait.Done()
		}()
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = clientChannel
			DebugLog("test client dial to %v success", remote)
			return
		}))
		res, err := client.GetText("http://xxxx")
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
		wait.Wait()
		fmt.Println("---->http process done")
	}
	{ //server dial error
		//
		serverChannel, clientChannel, _ := xio.CreatePipedConn()
		serverChannel.Alias, clientChannel.Alias = "ServerChannel", "ClientChannel"
		serverConn, serverRemote, _ := xio.CreatePipedConn()
		serverConn.Alias, serverRemote.Alias = "ServerConn", "ServerRemote"
		clientConn, clientRemote, _ := xio.CreatePipedConn()
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
			server.ProcConn(serverChannel, "")
			wait.Done()
		}()
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = clientChannel
			DebugLog("test client dial to %v success", remote)
			return
		}))
		client.TryDelay = time.Millisecond
		wait.Add(1)
		go func() {
			client.PipeConn(clientConn, "test")
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
		err := client.PipeConn(clientConn, "error")
		if err == nil {
			t.Error(err)
			return
		}
		fmt.Printf("%v\n", err)
		//
		time.Sleep(time.Millisecond)
		serverChannel.Close()
		wait.Wait()
		fmt.Println("---->server dial error done")
	}
	{ //client dialer error
		client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			err = fmt.Errorf("error")
			return
		}))
		client.TryDelay = time.Millisecond
		clientConn := NewErrMockConn(1, 1)
		err := client.PipeConn(clientConn, "test")
		if err == nil || err.Error() != "error" {
			t.Error(err)
			return
		}
		wait.Wait()
		fmt.Println("---->client dial error done")
	}
	{ //server proc error
		var err error
		remoteConn := NewErrMockConn(1, 1)
		server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			raw = remoteConn
			return
		}))
		//read command fail
		procConn1 := NewErrMockConn(1, 1)
		buf1 := make([]byte, 8)
		binary.BigEndian.PutUint32(buf1, 8)
		buf1[4] = 0
		copy(buf1[5:], []byte("abc"))
		procConn1.ReadData <- buf1
		err = server.ProcConn(procConn1, "")
		if err == nil {
			t.Error(err)
			return
		}
		//not dial command fail
		procConn2 := NewErrMockConn(1, 1)
		buf2 := make([]byte, 8)
		binary.BigEndian.PutUint32(buf2, 8)
		buf2[4] = CmdConnBack
		copy(buf2[5:], []byte("abc"))
		procConn2.ReadData <- buf2
		err = server.ProcConn(procConn2, "")
		if err == nil {
			t.Error(err)
			return
		}
		wait.Wait()
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
		client.TryDelay = time.Millisecond
		clientConn := NewErrMockConn(1, 1)
		err := client.PipeConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		fmt.Printf("read channel command error done\n")
		//read channel command error
		client = NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
			remoteConn := NewErrMockConn(1, 2)
			remoteConn.ReadErrC = 1
			raw = remoteConn
			return
		}))
		client.TryDelay = time.Millisecond
		clientConn = NewErrMockConn(1, 2)
		err = client.PipeConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		fmt.Printf("read channel command error done\n")
		//dial back command error
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
		client.TryDelay = time.Millisecond
		clientConn = NewErrMockConn(1, 2)
		err = client.PipeConn(clientConn, "test")
		if err == nil {
			t.Error(err)
			return
		}
		wait.Wait()
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

type chanBuffer struct {
	wait   chan int
	readc  int
	sended []byte
	recved []byte
	closed uint32
}

func newChanBuffer() (buffer *chanBuffer) {
	buffer = &chanBuffer{
		wait: make(chan int, 1),
	}
	return
}

func (c *chanBuffer) Read(p []byte) (n int, err error) {
	if atomic.LoadUint32(&c.closed) < 1 {
		switch c.readc {
		case 0:
			n = copy(p, c.sended)
			// fmt.Printf("read data---->\n")
			c.readc++
		default:
			<-c.wait
			err = io.EOF
			// fmt.Printf("read eof---->\n")
		}
	} else {
		err = fmt.Errorf("closed")
	}
	return
}

func (c *chanBuffer) Write(p []byte) (n int, err error) {
	if atomic.LoadUint32(&c.closed) < 1 {
		buf := make([]byte, len(p))
		n = copy(buf, p)
		c.recved = buf
		c.wait <- 1
		// fmt.Printf("write---->\n")
	} else {
		err = fmt.Errorf("closed")
	}
	return
}

func (c *chanBuffer) Close() (err error) {
	atomic.StoreUint32(&c.closed, 1)
	// fmt.Printf("closed---->\n")
	return
}

func (c *chanBuffer) String() string {
	return "chanBuffer"
}

func BenchmarkCoversocksConn(b *testing.B) {
	logLevel = -1
	server := NewServer(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
		raw = xio.NewEchoConn()
		return
	}))
	client := NewClient(256*1024, DialerF(func(remote string) (raw io.ReadWriteCloser, err error) {
		serverChannel, raw, err := xio.CreatePipedConn()
		if err == nil {
			go server.ProcConn(serverChannel, "")
		}
		return
	}))
	var runc int64
	var errc int64
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			uri := fmt.Sprintf("data-%v", atomic.AddInt64(&runc, 1))
			conn := newChanBuffer()
			conn.sended = []byte(uri)
			err := client.PipeConn(conn, uri)
			if err != nil && err != io.EOF {
				atomic.AddInt64(&errc, 1)
			}
			if !bytes.Equal(conn.sended, conn.recved) {
				b.Errorf("error,\nsended:%x\nrecved:%x\n", conn.sended, conn.recved)
			}
		}
	})
	client.Close()
	server.Close()
}
