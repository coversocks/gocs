package core

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/codingeasygo/util/xdebug"
	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
)

const (
	//DefaultBufferSize is default buffer size
	DefaultBufferSize = 64 * 1024
)

// ChannelConn is cs connection
type ChannelConn struct {
	*frame.BaseReadWriteCloser
	Key         string
	LastUsed    time.Time
	Error       error
	BufferSize_ int
	Using       string
}

// NewChannelConn will return new channel
func NewChannelConn(raw io.ReadWriteCloser, bufferSize int) (conn *ChannelConn) {
	conn = &ChannelConn{
		BaseReadWriteCloser: frame.NewReadWriteCloser(frame.NewDefaultHeader(), raw, bufferSize),
		LastUsed:            time.Now(),
		BufferSize_:         bufferSize,
	}
	conn.Key = fmt.Sprintf("%p", conn)
	conn.SetLengthAdjustment(0)
	conn.SetLengthFieldLength(4)
	conn.SetLengthFieldMagic(1)
	conn.SetLengthFieldOffset(0)
	conn.SetDataOffset(5)
	conn.SetTimeout(0)
	return
}

func (c *ChannelConn) BufferSize() int {
	return c.BufferSize_
}

func (c *ChannelConn) ReadFrame() (cmd []byte, err error) {
	cmd, err = c.BaseReadWriteCloser.ReadFrame()
	if err != nil {
		c.Error = err
		return
	}
	offset := c.GetDataOffset()
	switch cmd[offset-1] {
	case CmdConnBack, CmdConnDial:
		if len(c.Using) > 0 {
			err = fmt.Errorf("dial command in using conn")
			c.Error = err
		}
	case CmdConnData:
		if len(c.Using) < 1 {
			err = fmt.Errorf("data command in not using conn")
			c.Error = err
		}
	case CmdConnClose:
		err = fmt.Errorf("%v", string(cmd[offset:]))
	default:
		err = fmt.Errorf("error command:%x", cmd[offset-1])
		c.Error = err
	}
	return
}

func (c *ChannelConn) WriteFrame(buffer []byte) (w int, err error) {
	offset := c.GetDataOffset()
	buffer[offset-1] = CmdConnData
	w, err = c.BaseReadWriteCloser.WriteFrame(buffer)
	if err != nil {
		c.Error = err
	}
	return
}

func (c *ChannelConn) WriteCommand(cmd byte, buffer []byte) (w int, err error) {
	offset := c.GetDataOffset()
	all := make([]byte, offset+len(buffer))
	all[offset-1] = cmd
	copy(all[offset:], buffer)
	w, err = c.BaseReadWriteCloser.WriteFrame(all)
	if err != nil {
		c.Error = err
	}
	return
}

func (c *ChannelConn) WriteTo(writer io.Writer) (w int64, err error) {
	w, err = frame.WriteTo(c, writer)
	return
}

func (c *ChannelConn) ReadFrom(reader io.Reader) (w int64, err error) {
	w, err = frame.ReadFrom(c, reader, c.BufferSize_)
	return
}

func (c *ChannelConn) Write(p []byte) (n int, err error) {
	n, err = frame.Write(c, p)
	return
}

func (c *ChannelConn) Read(p []byte) (n int, err error) {
	n, err = frame.Read(c, p)
	return
}

func (c *ChannelConn) Close() (err error) {
	if c.Error == nil {
		c.WriteCommand(CmdConnClose, []byte("closed"))
	}
	if c.Error != nil {
		err = c.BaseReadWriteCloser.Close()
	}
	return
}

func (c *ChannelConn) Ping() (err error) {
	_, err = c.WriteCommand(CmdConnDial, []byte("tcp://echo"))
	if err != nil {
		return
	}
	back, err := c.ReadFrame()
	if err != nil {
		return
	}
	backMessage := string(back[5:])
	if backMessage != "ok" {
		err = fmt.Errorf(backMessage)
		return
	}
	c.Using = "tcp://echo"
	buf := make([]byte, 8*1024)
	_, err = c.Write(buf)
	if err != nil {
		return
	}
	_, err = c.Read(buf)
	return
}

func (c *ChannelConn) LocalAddr() net.Addr {
	return c
}

func (c *ChannelConn) RemoteAddr() net.Addr {
	return c
}

func (c *ChannelConn) Network() string {
	return "cs"
}

func (c *ChannelConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *ChannelConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *ChannelConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (c *ChannelConn) String() string {
	return fmt.Sprintf("%v", c.BaseReadWriteCloser)
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

// Server is the main implementation for dark socks
type Server struct {
	BufferSize int
	Dialer     xio.PiperDialer
	conns      map[string]*ChannelConn
	connsLck   sync.RWMutex
}

// NewServer will create Server by buffer size and dialer
func NewServer(bufferSize int, dialer xio.PiperDialer) (server *Server) {
	server = &Server{
		BufferSize: bufferSize,
		Dialer:     dialer,
		conns:      map[string]*ChannelConn{},
		connsLck:   sync.RWMutex{},
	}
	return
}

// ProcConn will start process proxy connection
func (s *Server) ProcConn(raw io.ReadWriteCloser, key string) (err error) {
	conn := NewChannelConn(raw, s.BufferSize)
	s.connsLck.Lock()
	s.conns[conn.Key] = conn
	s.connsLck.Unlock()
	defer func() {
		s.connsLck.Lock()
		delete(s.conns, conn.Key)
		s.connsLck.Unlock()
	}()
	InfoLog("Server one channel is starting from %v", xio.RemoteAddr(raw))
	for {
		cmd, xerr := conn.ReadFrame()
		if xerr != nil {
			conn.Error = xerr
			break
		}
		if cmd[4] != CmdConnDial {
			WarnLog("Server connection from %v will be closed by expected dail command, but %x", conn, cmd[0])
			xerr = fmt.Errorf("protocol error")
			conn.Error = xerr
			break
		}
		targetURI := string(cmd[5:])
		DebugLog("Server receive dail connec to %v from %v", targetURI, conn)
		piper, xerr := s.Dialer.DialPiper(targetURI, s.BufferSize)
		if xerr != nil {
			InfoLog("Server dial to %v fail with %v", targetURI, xerr)
			conn.WriteCommand(CmdConnBack, []byte(xerr.Error()))
			continue
		}
		conn.WriteCommand(CmdConnBack, []byte("ok"))
		conn.Using = targetURI
		DebugLog("Server transfer is started by %v from %v to %v", targetURI, conn, piper)
		piper.PipeConn(conn, targetURI)
		DebugLog("Server transfer is stopped by %v from %v to %v", targetURI, conn, piper)
		conn.Using = ""
		piper.Close()
		if conn.Error != nil {
			break
		}
	}
	InfoLog("Server the channel(%v) is stopped by %v", xio.RemoteAddr(raw), conn.Error)
	conn.Close()
	err = conn.Error
	return
}

// Close will cose all connection
func (s *Server) Close() (err error) {
	s.connsLck.Lock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.connsLck.Unlock()
	return
}

// Client is normal client for implement dark socket protocl
type Client struct {
	*xhttp.Client
	conns       map[string]*ChannelConn
	idles       map[string]*ChannelConn
	connsLck    sync.RWMutex
	BufferSize  int
	MaxIdle     int
	KeepIdle    time.Duration
	TestDelay   time.Duration
	TickerDelay time.Duration
	Dialer      Dialer
	TryMax      int
	TryDelay    time.Duration
	connAccept  chan net.Conn
	running     bool
	exit        chan int
}

// NewClient will create client by buffer size and dialer
func NewClient(bufferSize int, dialer Dialer) (client *Client) {
	client = &Client{
		conns:       map[string]*ChannelConn{},
		idles:       map[string]*ChannelConn{},
		connsLck:    sync.RWMutex{},
		MaxIdle:     100,
		KeepIdle:    10 * time.Second,
		TestDelay:   30 * time.Second,
		TickerDelay: time.Second,
		BufferSize:  bufferSize,
		Dialer:      dialer,
		TryMax:      5,
		TryDelay:    500 * time.Millisecond,
		connAccept:  make(chan net.Conn, 8),
		exit:        make(chan int, 1),
	}
	raw := &http.Client{
		Transport: &http.Transport{
			Dial: client.httpDial,
		},
	}
	client.Client = xhttp.NewClient(raw)
	client.running = true
	go client.procLoop()
	return
}

// Close will close all proc connection
func (c *Client) Close() (err error) {
	c.running = false
	conns := []frame.ReadWriteCloser{}
	c.connsLck.Lock()
	for _, conn := range c.conns {
		conns = append(conns, conn)
	}
	c.connsLck.Unlock()
	for _, conn := range conns {
		conn.Close()
	}
	c.exit <- 1
	return
}

func (c *Client) httpDial(network, addr string) (conn net.Conn, err error) {
	proxy, conn, err := xio.CreatePipedConn()
	if err == nil {
		go c.PipeConn(proxy, network+"://"+addr)
	}
	return
}

func (c *Client) procLoop() {
	ticker := time.NewTicker(c.TickerDelay)
	running := true
	testLast := time.Time{}
	for running {
		select {
		case <-c.exit:
			running = false
		case <-ticker.C:
			c.timeoutConn()
			if time.Since(testLast) > c.TestDelay {
				c.testDialer()
				testLast = time.Now()
			}
		}
	}
}

func (c *Client) timeoutConn() {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Client timeout conn is panic by %v, the callstack is\n%v", perr, xdebug.CallStack())
		}
	}()
	c.connsLck.Lock()
	for key, conn := range c.idles {
		if time.Since(conn.LastUsed) > c.KeepIdle {
			DebugLog("Client remove one timeout channel in idle pool")
			conn.Close()
			delete(c.idles, key)
		}
	}
	c.connsLck.Unlock()
}

func (c *Client) testDialer() {
	defer func() {
		if perr := recover(); perr != nil {
			ErrorLog("Client test conn is panic by %v, the callstack is\n%v", perr, xdebug.CallStack())
		}
	}()
	tester, ok := c.Dialer.(Tester)
	if !ok {
		return
	}
	tester.Test("", func(raw io.ReadWriteCloser) (err error) {
		conn := NewChannelConn(raw, c.BufferSize)
		err = conn.Ping()
		return
	})
	// data, _ := json.MarshalIndent(result, " ", "  ")
	// InfoLog("Client test all dialer is done by \n%v", string(data))
}

// pullConn will return Conn in idle pool, if pool is empty, dial new by Dialer
func (c *Client) pullConn() (conn *ChannelConn, err error) {
	var key string
	c.connsLck.Lock()
	for key, conn = range c.idles {
		delete(c.idles, key)
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
	conn = NewChannelConn(raw, c.BufferSize)
	c.connsLck.Lock()
	c.conns[conn.Key] = conn
	c.connsLck.Unlock()
	return
}

// pushConn will push one Conn to idle pool
func (c *Client) pushConn(conn *ChannelConn) {
	if conn == nil {
		panic("conn is nil")
	}
	c.connsLck.Lock()
	defer c.connsLck.Unlock()
	if len(c.idles) >= c.MaxIdle {
		var oldestTime = time.Now()
		var oldest *ChannelConn
		for _, c := range c.idles {
			last := c.LastUsed
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
	delete(c.conns, conn.Key)
	c.idles[conn.Key] = conn
	DebugLog("Client push one channel to idle pool")
}

// PipeConn will start process proxy connection
func (c *Client) PipeConn(raw io.ReadWriteCloser, target string) (err error) {
	defer raw.Close()
	piper, err := c.DialPiper(target, c.BufferSize)
	if err == nil {
		err = piper.PipeConn(raw, target)
		piper.Close()
	}
	return
}

func (c *Client) DialBack(target string, bufferSize int) (conn *ChannelConn, message string, err error) {
	var back []byte
	for i := 0; i < c.TryMax; i++ {
		if i > 0 {
			time.Sleep(c.TryDelay)
		}
		conn, err = c.pullConn()
		if err != nil {
			InfoLog("Client try pull connection fail with %v", err)
			continue
		}
		DebugLog("Client try dial to %v", target)
		_, err = conn.WriteCommand(CmdConnDial, []byte(target))
		if err != nil {
			conn.Close()
			InfoLog("Client try %v dial to %v fail with %v, will retry after %v", i, target, err, c.TryDelay)
			continue
		}
		back, err = conn.ReadFrame()
		if err != nil {
			conn.Close()
			InfoLog("Client try %v dial to %v fail with %v, will retry after %v", i, target, err, c.TryDelay)
			continue
		}
		break
	}
	if err != nil {
		return
	}
	message = string(back[5:])
	return
}

func (c *Client) DialConn(target string, bufferSize int) (conn *ChannelConn, err error) {
	conn, backMessage, err := c.DialBack(target, bufferSize)
	if err == nil && backMessage != "ok" {
		err = fmt.Errorf("%v", backMessage)
		c.pushConn(conn)
		InfoLog("Client try dial to %v fail with %v", target, err)
	}
	return
}

// DialPiper is xio.PiperDialer implement for create xio.Piper on client
func (c *Client) DialPiper(target string, bufferSize int) (piper xio.Piper, err error) {
	conn, err := c.DialConn(target, bufferSize)
	if err != nil {
		return
	}
	conn.Using = target
	DebugLog("Client dial to %v success on %v", target, conn)
	piper = &piperConn{
		conn:   conn,
		client: c,
		piper:  xio.NewCopyPiper(conn, c.BufferSize),
	}
	return
}

type piperConn struct {
	conn   *ChannelConn
	client *Client
	piper  *xio.CopyPiper
}

func (p *piperConn) PipeConn(raw io.ReadWriteCloser, target string) (err error) {
	DebugLog("Client start transfer %v to %v for %v", xio.RemoteAddr(raw), p.conn, target)
	err = p.piper.PipeConn(raw, target)
	DebugLog("Client stop transfer %v to %v for %v", xio.RemoteAddr(raw), p.conn, target)
	return
}

func (p *piperConn) Close() (err error) {
	if p.conn.Error != nil {
		InfoLog("Client the channel(%v) is stopped by %v", p.conn, p.conn.Error)
		err = p.conn.Error
		p.conn.Close()
	} else {
		p.conn.Using = ""
		p.client.pushConn(p.conn)
	}
	return
}
