package core

import (
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	//DefaultBufferSize is default buffer size
	DefaultBufferSize = 64 * 1024
)

//Conn is interface for read/write raw connection by command mode
type Conn interface {
	ID() uint64
	ReadCmd() (cmd []byte, err error)
	WriteCmd(cmd []byte) (w int, err error)
	Close() (err error)
	SetErr(err error)
	PreErr() (err error)
	LastUsed() (latest time.Time)
}

//connSequence is the sequence to create Conn.ID()
var connSequence uint64

//BaseConn imple read/write raw connection by command mode
type BaseConn struct {
	id     uint64
	buf    []byte
	offset uint32
	length uint32
	raw    io.ReadWriter
	latest time.Time
	Err    error
}

//NewBaseConn will create new Conn by raw reader/writer and buffer size
func NewBaseConn(raw io.ReadWriter, bufferSize int) (conn *BaseConn) {
	conn = &BaseConn{
		id:  atomic.AddUint64(&connSequence, 1),
		buf: make([]byte, bufferSize),
		raw: raw,
	}
	return
}

//ID will reture connection id
func (b *BaseConn) ID() uint64 {
	return b.id
}

//readMore will read more data to buffer
func (b *BaseConn) readMore() (err error) {
	readed, err := b.raw.Read(b.buf[b.offset+b.length:])
	if err == nil {
		b.length += uint32(readed)
	}
	b.latest = time.Now()
	return
}

//ReadCmd will read raw reader as command mode.
//the commad protocol is:lenght(4 byte)+data
func (b *BaseConn) ReadCmd() (cmd []byte, err error) {
	more := b.length < 5
	for {
		if more {
			err = b.readMore()
			if err != nil {
				break
			}
			if b.length < 5 {
				continue
			}
		}
		b.buf[b.offset] = 0
		frameLength := binary.BigEndian.Uint32(b.buf[b.offset:])
		if frameLength > uint32(len(b.buf)) {
			err = fmt.Errorf("frame too large")
			break
		}
		if b.length < frameLength {
			more = true
			if b.offset > 0 {
				copy(b.buf[0:], b.buf[b.offset:b.offset+b.length])
				b.offset = 0
			}
			continue
		}
		cmd = b.buf[b.offset+4 : b.offset+frameLength]
		b.offset += frameLength
		b.length -= frameLength
		more = b.length <= 4
		if b.length < 1 {
			b.offset = 0
		}
		break
	}
	return
}

func (b *BaseConn) Read(p []byte) (n int, err error) {
	data, err := b.ReadCmd()
	if err == nil {
		n = copy(p, data)
	}
	return
}

//WriteCmd will write data by command mode
func (b *BaseConn) WriteCmd(cmd []byte) (w int, err error) {
	binary.BigEndian.PutUint32(cmd, uint32(len(cmd)))
	cmd[0] = byte(rand.Intn(255))
	w, err = b.raw.Write(cmd)
	// if err == nil {
	// 	fmt.Printf("Cmd Write %v %v\n", len(cmd), cmd)
	// }
	b.latest = time.Now()
	return
}

func (b *BaseConn) Write(p []byte) (n int, err error) {
	// fmt.Printf("Conn Begin %v %v\n", len(p), p)
	buf := make([]byte, len(p)+4)
	copy(buf[4:], p)
	n = len(buf)
	// fmt.Printf("Conn Write %v,%v %v\n", len(buf), len(p), buf)
	_, err = b.WriteCmd(buf)
	return
}

//LastUsed will return last used time
func (b *BaseConn) LastUsed() (latest time.Time) {
	latest = b.latest
	return
}

//Close will check raw if io.Closer and close it
func (b *BaseConn) Close() (err error) {
	if closer, ok := b.raw.(io.Closer); ok {
		err = closer.Close()
	}
	b.Err = fmt.Errorf("closed")
	return
}

func (b *BaseConn) String() string {
	return remoteAddr(b.raw)
}

//SetErr will mark error
func (b *BaseConn) SetErr(err error) {
	b.Err = err
}

//PreErr will get prefix error
func (b *BaseConn) PreErr() (err error) {
	err = b.Err
	return
}

//copyRemote2Channel will read target connection data and write to channel connection by command mode
func copyRemote2Channel(bufferSize int, conn Conn, target io.ReadWriteCloser) (err error) {
	var readed int
	buf := make([]byte, bufferSize)
	for {
		readed, err = target.Read(buf[5:])
		if err != nil {
			break
		}
		// fmt.Printf("R2C:%v->%v:%v\n", target, conn, readed)
		buf[4] = CmdConnData
		_, err = conn.WriteCmd(buf[:readed+5])
		if err != nil {
			conn.SetErr(err)
			break
		}
	}
	target.Close()
	if conn.PreErr() == nil {
		conn.WriteCmd(append([]byte{0, 0, 0, 0, CmdConnClose}, []byte(err.Error())...))
	}
	return
}

//copyChannel2Remote will read channel connection data by command mode and write to target
func copyChannel2Remote(conn Conn, target io.ReadWriteCloser) (err error) {
	var cmd []byte
	for {
		cmd, err = conn.ReadCmd()
		if err != nil {
			conn.SetErr(err)
			break
		}
		// fmt.Printf("C2R:%v->%v:%v\n", conn, target, len(cmd))
		switch cmd[0] {
		case CmdConnData:
			_, err = target.Write(cmd[1:])
		case CmdConnClose:
			err = fmt.Errorf("%v", string(cmd[1:]))
		default:
			err = fmt.Errorf("error command:%x", cmd[0])
			conn.SetErr(err)
		}
		if err != nil {
			break
		}
	}
	target.Close()
	return
}

type handlerErrConn struct {
	*BaseConn
	Closer func()
	closed bool
}

func (h *handlerErrConn) Close() error {
	if !h.closed {
		h.closed = true
		h.Closer()
	}
	return h.BaseConn.Close()
}

const (
	//CmdConnDial is cs protocol command for dial connection
	CmdConnDial = 0x10
	//CmdConnBack is cs protocol command for dial connection back
	CmdConnBack = 0x20
	//CmdConnData is cs protocol command for transfer data
	CmdConnData = 0x30
	//CmdConnClose is cs protocol command for connection close
	CmdConnClose = 0x40
)

//Server is the main implementation for dark socks
type Server struct {
	BufferSize int
	Dialer     Dialer
}

//NewServer will create Server by buffer size and dialer
func NewServer(bufferSize int, dialer Dialer) (server *Server) {
	server = &Server{
		BufferSize: bufferSize,
		Dialer:     dialer,
	}
	return
}

//ProcConn will start process proxy connection
func (s *Server) ProcConn(conn Conn) (err error) {
	InfoLog("Server one channel is starting from %v", conn)
	for {
		cmd, xerr := conn.ReadCmd()
		if xerr != nil {
			conn.SetErr(xerr)
			break
		}
		if cmd[0] != CmdConnDial {
			WarnLog("Server connection from %v will be closed by expected dail command, but %x", conn, cmd[0])
			xerr = fmt.Errorf("protocol error")
			conn.SetErr(xerr)
			break
		}
		targetURI := string(cmd[1:])
		DebugLog("Server receive dail connec to %v from %v", targetURI, conn)
		target, xerr := s.Dialer.Dial(targetURI)
		if xerr != nil {
			InfoLog("Server dial to %v fail with %v", targetURI, xerr)
			conn.WriteCmd(append([]byte{0, 0, 0, 0, CmdConnBack}, []byte(xerr.Error())...))
			continue
		}
		conn.WriteCmd(append([]byte{0, 0, 0, 0, CmdConnBack}, []byte("ok")...))
		DebugLog("Server transfer is started from %v to %v", conn, target)
		s.procRemote(conn, target)
		DebugLog("Server transfer is stopped from %v to %v", conn, target)
		if conn.PreErr() != nil {
			break
		}
	}
	conn.Close()
	InfoLog("Server the channel(%v) is stopped by %v", conn, conn.PreErr())
	err = conn.PreErr()
	return
}

//procRemote will transfer data between channel and target
func (s *Server) procRemote(conn Conn, target io.ReadWriteCloser) (err error) {
	wait := sync.WaitGroup{}
	wait.Add(1)
	go func() {
		copyChannel2Remote(conn, target)
		wait.Done()
	}()
	err = copyRemote2Channel(s.BufferSize, conn, target)
	wait.Wait()
	return
}

//Client is normal client for implement dark socket protocl
type Client struct {
	conns      map[uint64]Conn
	idles      map[uint64]Conn
	connsLck   sync.RWMutex
	BufferSize int
	MaxIdle    int
	Dialer     Dialer
	HTTPClient *http.Client
}

//NewClient will create client by buffer size and dialer
func NewClient(bufferSize int, dialer Dialer) (client *Client) {
	client = &Client{
		conns:      map[uint64]Conn{},
		idles:      map[uint64]Conn{},
		connsLck:   sync.RWMutex{},
		MaxIdle:    100,
		BufferSize: bufferSize,
		Dialer:     dialer,
	}
	client.HTTPClient = &http.Client{
		Transport: &http.Transport{
			Dial: client.httpDial,
		},
	}
	return
}

//Close will close all proc connection
func (c *Client) Close() (err error) {
	conns := []Conn{}
	c.connsLck.Lock()
	for _, conn := range c.conns {
		conns = append(conns, conn)
	}
	c.connsLck.Unlock()
	for _, conn := range conns {
		conn.Close()
	}
	return
}

func (c *Client) httpDial(network, addr string) (conn net.Conn, err error) {
	proxy, conn, err := CreatePipeConn()
	if err == nil {
		go c.ProcConn(proxy, addr)
	}
	return
}

//pullConn will return Conn in idle pool, if pool is empty, dial new by Dialer
func (c *Client) pullConn() (conn Conn, err error) {
	c.connsLck.Lock()
	for _, conn = range c.idles {
		delete(c.idles, conn.ID())
		break
	}
	c.connsLck.Unlock()
	if conn != nil {
		DebugLog("Client pull one connection from idel pool")
		return
	}
	raw, err := c.Dialer.Dial("")
	if err != nil {
		return
	}
	baseConn := NewBaseConn(raw, c.BufferSize)
	c.connsLck.Lock()
	conn = &handlerErrConn{
		BaseConn: baseConn,
		Closer: func() {
			c.connsLck.Lock()
			delete(c.conns, baseConn.ID())
			c.connsLck.Unlock()
		},
	}
	c.conns[conn.ID()] = conn
	c.connsLck.Unlock()
	return
}

//pushConn will push one Conn to idle pool
func (c *Client) pushConn(conn Conn) {
	c.connsLck.Lock()
	if len(c.idles) > c.MaxIdle {
		var oldestTime = time.Now()
		var oldest Conn
		for _, c := range c.idles {
			last := c.LastUsed()
			if oldestTime.Sub(last) > 0 {
				oldestTime = last
				oldest = c
			}
		}
		if oldest != nil {
			DebugLog("Client remove one oldest channel for idle pool is full")
			oldest.Close()
		}
	}
	c.idles[conn.ID()] = conn
	c.connsLck.Unlock()
	DebugLog("Client push one channel to idle pool")
}

//ProcConn will start process proxy connection
func (c *Client) ProcConn(raw io.ReadWriteCloser, target string) (err error) {
	defer raw.Close()
	conn, err := c.pullConn()
	if err != nil {
		return
	}
	DebugLog("Client try proxy %v to %v for %v", raw, conn, target)
	_, err = conn.WriteCmd(append([]byte{0, 0, 0, 0, CmdConnDial}, []byte(target)...))
	if err != nil {
		conn.SetErr(err)
		conn.Close()
		DebugLog("Client try proxy %v to %v for %v fail with %v", raw, conn, target, err)
		return
	}
	back, err := conn.ReadCmd()
	if err != nil {
		conn.SetErr(err)
		conn.Close()
		DebugLog("Client try proxy %v to %v for %v fail with %v", raw, conn, target, err)
		return
	}
	if back[0] != CmdConnBack {
		err = fmt.Errorf("protocol error, expected back command, but %x", back[0])
		WarnLog("Client will close connection(%v) by %v", conn, err)
		conn.SetErr(err)
		conn.Close()
		DebugLog("Client try proxy %v to %v for %v fail with %v", raw, conn, target, err)
		return
	}
	backMessage := string(back[1:])
	if backMessage != "ok" {
		err = fmt.Errorf("%v", backMessage)
		c.pushConn(conn)
		DebugLog("Client try proxy %v to %v for %v fail with %v", raw, conn, target, err)
		return
	}
	DebugLog("Client start transfer %v to %v for %v", raw, conn, target)
	c.procRemote(conn, raw)
	DebugLog("Client stop transfer %v to %v for %v", raw, conn, target)
	if conn.PreErr() != nil {
		InfoLog("Client the channel(%v) is stopped by %v", conn, conn.PreErr())
		err = conn.PreErr()
		conn.Close()
	} else {
		c.pushConn(conn)
	}
	return
}

//procRemote will tansfer data between channel Conn and target connection.
func (c *Client) procRemote(conn Conn, target io.ReadWriteCloser) (err error) {
	wait := sync.WaitGroup{}
	wait.Add(1)
	go func() {
		copyChannel2Remote(conn, target)
		wait.Done()
	}()
	err = copyRemote2Channel(c.BufferSize, conn, target)
	wait.Wait()
	return
}

//HTTPGet will do http get request by proxy
func (c *Client) HTTPGet(uri string) (data []byte, err error) {
	resp, err := c.HTTPClient.Get(uri)
	if err == nil {
		data, err = ioutil.ReadAll(resp.Body)
		resp.Body.Close()
	}
	return
}
