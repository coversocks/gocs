package core

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
)

const (
	//DefaultBufferSize is default buffer size
	DefaultBufferSize = 64 * 1024
)

//ChannelConn is cs connection
type ChannelConn struct {
	*frame.BaseReadWriteCloser
	Key        string
	LastUsed   time.Time
	Error      error
	BufferSize int
	Using      string
}

//NewChannelConn will return new channel
func NewChannelConn(raw io.ReadWriteCloser, bufferSize int) (conn *ChannelConn) {
	conn = &ChannelConn{
		BaseReadWriteCloser: frame.NewReadWriteCloser(raw, bufferSize),
		LastUsed:            time.Now(),
		BufferSize:          bufferSize,
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
			err = fmt.Errorf("dial in using")
			c.Error = err
		}
	case CmdConnData:
		if len(c.Using) < 1 {
			err = fmt.Errorf("data in not using")
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
	w, err = frame.ReadFrom(c, reader, c.BufferSize)
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

//Server is the main implementation for dark socks
type Server struct {
	BufferSize int
	Dialer     xio.PiperDialer
	conns      map[string]*ChannelConn
	connsLck   sync.RWMutex
}

//NewServer will create Server by buffer size and dialer
func NewServer(bufferSize int, dialer xio.PiperDialer) (server *Server) {
	server = &Server{
		BufferSize: bufferSize,
		Dialer:     dialer,
		conns:      map[string]*ChannelConn{},
		connsLck:   sync.RWMutex{},
	}
	return
}

//ProcConn will start process proxy connection
func (s *Server) ProcConn(raw io.ReadWriteCloser) (err error) {
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
		DebugLog("Server transfer is started from %v to %v", conn, piper)
		piper.PipeConn(conn, targetURI)
		DebugLog("Server transfer is stopped from %v to %v", conn, piper)
		piper.Close()
		conn.Using = ""
		if conn.Error != nil {
			break
		}
	}
	InfoLog("Server the channel(%v) is stopped by %v", xio.RemoteAddr(raw), conn.Error)
	conn.Close()
	err = conn.Error
	return
}

//Close will cose all connection
func (s *Server) Close() (err error) {
	s.connsLck.Lock()
	for _, conn := range s.conns {
		conn.Close()
	}
	s.connsLck.Unlock()
	return
}

//Client is normal client for implement dark socket protocl
type Client struct {
	*xhttp.Client
	conns      map[string]*ChannelConn
	idles      map[string]*ChannelConn
	connsLck   sync.RWMutex
	BufferSize int
	MaxIdle    int
	Dialer     Dialer
	TryMax     int
	TryDelay   time.Duration
}

//NewClient will create client by buffer size and dialer
func NewClient(bufferSize int, dialer Dialer) (client *Client) {
	client = &Client{
		conns:      map[string]*ChannelConn{},
		idles:      map[string]*ChannelConn{},
		connsLck:   sync.RWMutex{},
		MaxIdle:    100,
		BufferSize: bufferSize,
		Dialer:     dialer,
		TryMax:     5,
		TryDelay:   500 * time.Millisecond,
	}
	raw := &http.Client{
		Transport: &http.Transport{
			Dial: client.httpDial,
		},
	}
	client.Client = xhttp.NewClient(raw)
	return
}

//Close will close all proc connection
func (c *Client) Close() (err error) {
	conns := []frame.ReadWriteCloser{}
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
	proxy, conn, err := xio.CreatePipedConn()
	if err == nil {
		go c.PipeConn(proxy, network+"://"+addr)
	}
	return
}

//pullConn will return Conn in idle pool, if pool is empty, dial new by Dialer
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

//pushConn will push one Conn to idle pool
func (c *Client) pushConn(conn *ChannelConn) {
	c.connsLck.Lock()
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
	c.connsLck.Unlock()
	DebugLog("Client push one channel to idle pool")
}

//PipeConn will start process proxy connection
func (c *Client) PipeConn(raw io.ReadWriteCloser, target string) (err error) {
	defer raw.Close()
	piper, err := c.DialPiper(target, c.BufferSize)
	if err == nil {
		err = piper.PipeConn(raw, target)
		piper.Close()
	}
	return
}

//DialPiper is xio.PiperDialer implement for create xio.Piper on client
func (c *Client) DialPiper(target string, bufferSize int) (piper xio.Piper, err error) {
	var conn *ChannelConn
	var back []byte
	for i := 0; i < c.TryMax; i++ {
		if i > 0 {
			time.Sleep(c.TryDelay)
		}
		conn, err = c.pullConn()
		if err != nil {
			DebugLog("Client try pull connection fail with %v", err)
			continue
		}
		DebugLog("Client try dial to %v", target)
		_, err = conn.WriteCommand(CmdConnDial, []byte(target))
		if err != nil {
			conn.Close()
			DebugLog("Client try %v dial to %v fail with %v, will retry after %v", i, target, err, c.TryDelay)
			continue
		}
		back, err = conn.ReadFrame()
		if err != nil {
			conn.Close()
			DebugLog("Client try %v dial to %v fail with %v, will retry after %v", i, target, err, c.TryDelay)
			continue
		}
		break
	}
	if err != nil {
		return
	}
	backMessage := string(back[5:])
	if backMessage != "ok" {
		err = fmt.Errorf("%v", backMessage)
		c.pushConn(conn)
		DebugLog("Client try dial to %v fail with %v", target, err)
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
