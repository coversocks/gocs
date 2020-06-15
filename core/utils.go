package core

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

//BasePipe is func to create os pipe
var BasePipe = os.Pipe

//PipedConn is connection piped read and write
type PipedConn struct {
	io.Reader
	rWriter io.Writer
	io.Writer
	wReader io.Reader
	Alias   string
}

//CreatePipeConn will create pipe connection
func CreatePipeConn() (a, b *PipedConn, err error) {
	aReader, bWriter, err := BasePipe()
	if err != nil {
		return
	}
	bReader, aWriter, err := BasePipe()
	if err != nil {
		bWriter.Close()
		return
	}
	a = &PipedConn{
		Reader:  aReader,
		rWriter: bWriter,
		Writer:  aWriter,
		wReader: bReader,
		Alias:   fmt.Sprintf("%v,%v", aReader, aWriter),
	}
	b = &PipedConn{
		Reader:  bReader,
		rWriter: aWriter,
		Writer:  bWriter,
		wReader: aReader,
		Alias:   fmt.Sprintf("%v,%v", bReader, bWriter),
	}
	return
}

//Close will close Reaer/Writer
func (p *PipedConn) Close() (err error) {
	if closer, ok := p.rWriter.(io.Closer); ok {
		err = closer.Close()
	}
	if closer, ok := p.Writer.(io.Closer); ok {
		xerr := closer.Close()
		if err == nil {
			err = xerr
		}
	}
	return
}

//Network is net.Addr impl
func (p *PipedConn) Network() string {
	return "Piped"
}

//LocalAddr is net.Conn impl
func (p *PipedConn) LocalAddr() net.Addr {
	return p
}

//RemoteAddr is net.Conn impl
func (p *PipedConn) RemoteAddr() net.Addr {
	return p
}

//SetDeadline is net.Conn impl
func (p *PipedConn) SetDeadline(t time.Time) error {
	return nil
}

//SetReadDeadline is net.Conn impl
func (p *PipedConn) SetReadDeadline(t time.Time) error {
	return nil
}

//SetWriteDeadline is net.Conn impl
func (p *PipedConn) SetWriteDeadline(t time.Time) error {
	return nil
}

func (p *PipedConn) String() string {
	return p.Alias
}

const (
	//LogLevelDebug is debug log level
	LogLevelDebug = 40
	//LogLevelInfo is info log level
	LogLevelInfo = 30
	//LogLevelWarn is warn log level
	LogLevelWarn = 20
	//LogLevelError is error log level
	LogLevelError = 10
)

var logLevel = LogLevelInfo

//SetLogLevel is set log level to l
func SetLogLevel(l int) {
	if l > 0 {
		logLevel = l
	}
}

//DebugLog is the debug level log
func DebugLog(format string, args ...interface{}) {
	if logLevel < LogLevelDebug {
		return
	}
	log.Output(2, fmt.Sprintf("D "+format, args...))
}

//InfoLog is the info level log
func InfoLog(format string, args ...interface{}) {
	if logLevel < LogLevelInfo {
		return
	}
	log.Output(2, fmt.Sprintf("I "+format, args...))
}

//WarnLog is the warn level log
func WarnLog(format string, args ...interface{}) {
	if logLevel < LogLevelWarn {
		return
	}
	log.Output(2, fmt.Sprintf("W "+format, args...))
}

//ErrorLog is the error level log
func ErrorLog(format string, args ...interface{}) {
	if logLevel < LogLevelError {
		return
	}
	log.Output(2, fmt.Sprintf("E "+format, args...))
}

//WriteJSON will marshal value to json and write to file
func WriteJSON(filename string, v interface{}) (err error) {
	data, err := json.MarshalIndent(v, "", "    ")
	if err == nil {
		err = ioutil.WriteFile(filename, data, os.ModePerm)
	}
	return
}

//ReadJSON will read file and unmarshal to value
func ReadJSON(filename string, v interface{}) (err error) {
	data, err := ioutil.ReadFile(filename)
	if err == nil {
		err = json.Unmarshal(data, v)
	}
	return
}

//SHA1 will get sha1 hash of data
func SHA1(data []byte) string {
	s := sha1.New()
	s.Write(data)
	return fmt.Sprintf("%x", s.Sum(nil))
}

//StringConn is an ReadWriteCloser for return  remote address info
type StringConn struct {
	Name string
	io.ReadWriteCloser
}

//NewStringConn will return new StringConn
func NewStringConn(raw io.ReadWriteCloser) *StringConn {
	return &StringConn{
		ReadWriteCloser: raw,
	}
}

func (s *StringConn) String() string {
	if len(s.Name) > 0 {
		return s.Name
	}
	return remoteAddr(s.ReadWriteCloser)
}

func remoteAddr(v interface{}) string {
	if wsc, ok := v.(*websocket.Conn); ok {
		return fmt.Sprintf("%v", wsc.RemoteAddr())
	}
	if netc, ok := v.(net.Conn); ok {
		return fmt.Sprintf("%v", netc.RemoteAddr())
	}
	return fmt.Sprintf("%v", v)
}

//ConnWrapper is wrapper for net.Conn by ReadWriteCloser
type ConnWrapper struct {
	io.ReadWriteCloser
}

//NewConnWrapper will create new ConnWrapper
func NewConnWrapper(base io.ReadWriteCloser) (wrapper *ConnWrapper) {
	return &ConnWrapper{ReadWriteCloser: base}
}

//Network impl net.Addr
func (c *ConnWrapper) Network() string {
	return "wrapper"
}

func (c *ConnWrapper) String() string {
	return fmt.Sprintf("%v", c.ReadWriteCloser)
}

//LocalAddr return then local network address
func (c *ConnWrapper) LocalAddr() net.Addr {
	return c
}

// RemoteAddr returns the remote network address.
func (c *ConnWrapper) RemoteAddr() net.Addr {
	return c
}

// SetDeadline impl net.Conn do nothing
func (c *ConnWrapper) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline impl net.Conn do nothing
func (c *ConnWrapper) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline impl net.Conn do nothing
func (c *ConnWrapper) SetWriteDeadline(t time.Time) error {
	return nil
}

//TCPKeepAliveListener is normal tcp listner for set tcp connection keep alive
type TCPKeepAliveListener struct {
	*net.TCPListener
}

//Accept will accept one connection
func (ln TCPKeepAliveListener) Accept() (net.Conn, error) {
	tc, err := ln.AcceptTCP()
	if err == nil {
		tc.SetKeepAlive(true)
		tc.SetKeepAlivePeriod(3 * time.Minute)
	}
	return tc, err
}

//Now get current timestamp
func Now() int64 {
	return time.Now().Local().UnixNano() / 1e6
}

//Statable is interface for load object state
type Statable interface {
	State() interface{}
}

//ReaderF is wrapper for io.Reader
type ReaderF func(p []byte) (n int, err error)

func (r ReaderF) Read(p []byte) (n int, err error) {
	n, err = r(p)
	return
}

type WriterF func(p []byte) (n int, err error)

func (w WriterF) Write(p []byte) (n int, err error) {
	n, err = w(p)
	return
}

type CloserF func() (err error)

func (c CloserF) Close() (err error) {
	err = c()
	return
}

//ChannelRWC is an io.ReadWriteCloser impl by channel
type ChannelRWC struct {
	Async       bool
	Retain      bool
	err         error
	closeLocker sync.RWMutex
	readQueued  chan []byte
	writeQueued chan []byte
}

//NewChannelRWC will return new ChannelRWC
func NewChannelRWC(async, retain bool, bufferSize int) (rwc *ChannelRWC) {
	rwc = &ChannelRWC{
		Async:       async,
		Retain:      retain,
		closeLocker: sync.RWMutex{},
		readQueued:  make(chan []byte, bufferSize),
		writeQueued: make(chan []byte, bufferSize),
	}
	return
}

//Pull will read data from write queue
func (c *ChannelRWC) Pull() (data []byte, err error) {
	c.closeLocker.RLock()
	if err != nil {
		c.closeLocker.RUnlock()
		err = c.err
		return
	}
	c.closeLocker.RUnlock()
	if c.Async {
		select {
		case data = <-c.writeQueued:
		default:
		}
	} else {
		data = <-c.writeQueued
	}
	if len(data) < 1 {
		err = c.err
	}
	return
}

//Push will send data to read queue
func (c *ChannelRWC) Push(data []byte) (err error) {
	c.closeLocker.RLock()
	if err != nil {
		c.closeLocker.RUnlock()
		err = c.err
		return
	}
	c.closeLocker.RUnlock()
	buf := data
	if c.Retain {
		buf = make([]byte, len(data))
		copy(buf, data)
	}
	c.readQueued <- buf
	return
}

func (c *ChannelRWC) Read(p []byte) (n int, err error) {
	c.closeLocker.RLock()
	if err != nil {
		c.closeLocker.RUnlock()
		err = c.err
		return
	}
	c.closeLocker.RUnlock()
	data := <-c.readQueued
	if len(data) < 1 {
		err = c.err
		return
	}
	if len(p) < len(data) {
		err = fmt.Errorf("buffer to small expect %v, but %v", len(data), len(p))
		return
	}
	n = copy(p, data)
	return
}

func (c *ChannelRWC) Write(p []byte) (n int, err error) {
	c.closeLocker.RLock()
	if err != nil {
		c.closeLocker.RUnlock()
		err = c.err
		return
	}
	c.closeLocker.RUnlock()
	buf := p
	if c.Retain {
		buf = make([]byte, len(p))
		copy(buf, p)
	}
	c.writeQueued <- buf
	n = len(buf)
	return
}

//Close will close the channel
func (c *ChannelRWC) Close() (err error) {
	c.closeLocker.RLock()
	if c.err != nil {
		c.closeLocker.RUnlock()
		err = c.err
		return
	}
	c.err = fmt.Errorf("closed")
	close(c.readQueued)
	close(c.writeQueued)
	c.closeLocker.RUnlock()
	return
}

// type HashConn struct {
// 	net.Conn
// 	Show          bool
// 	name          string
// 	readHasher    hash.Hash
// 	writeHash     hash.Hash
// 	readOut       io.WriteCloser
// 	writeOut      io.WriteCloser
// 	readc, writec uint64
// }

// func NewHashConn(base net.Conn, show bool, name string) (conn *HashConn) {
// 	conn = &HashConn{
// 		Conn:       base,
// 		Show:       show,
// 		name:       name,
// 		readHasher: sha1.New(),
// 		writeHash:  sha1.New(),
// 	}
// 	if conn.Show {
// 		var err error
// 		conn.readOut, err = os.OpenFile("/data/data/com.github.coversocks/files/"+name+"_r.dat", os.O_CREATE|os.O_WRONLY, os.ModePerm)
// 		if err != nil {
// 			panic(err)
// 		}
// 		conn.writeOut, err = os.OpenFile("/data/data/com.github.coversocks/files/"+name+"_w.dat", os.O_CREATE|os.O_WRONLY, os.ModePerm)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}
// 	return
// }

// func (h *HashConn) Read(p []byte) (n int, err error) {
// 	n, err = h.Conn.Read(p)
// 	if h.Show && err == nil {
// 		h.readHasher.Write(p[0:n])
// 	}
// 	if h.readOut != nil {
// 		h.readOut.Write(p[0:n])
// 	}
// 	atomic.AddUint64(&h.readc, uint64(n))
// 	return
// }

// func (h *HashConn) Write(p []byte) (n int, err error) {
// 	n, err = h.Conn.Write(p)
// 	if h.Show && err == nil {
// 		h.writeHash.Write(p[0:n])
// 	}
// 	if h.writeOut != nil {
// 		h.writeOut.Write(p[0:n])
// 	}
// 	atomic.AddUint64(&h.writec, uint64(n))
// 	return
// }

// //Close will close base Conn
// func (h *HashConn) Close() (err error) {
// 	err = h.Conn.Close()
// 	if h.Show {
// 		fmt.Printf("HashConn(%v) read(%v) hash:%x,write(%v) hash:%x\n", h.name, h.readc, h.readHasher.Sum(nil), h.writec, h.writeHash.Sum(nil))
// 	}
// 	if h.readOut != nil {
// 		h.readOut.Close()
// 		fmt.Printf("read out is closed %v\n", h.readOut)
// 	}
// 	if h.writeOut != nil {
// 		h.writeOut.Close()
// 		fmt.Printf("write out is closed %v\n", h.writeOut)
// 	}
// 	return
// }
