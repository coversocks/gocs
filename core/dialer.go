package core

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/net/websocket"
)

//Dialer is interface for dial raw connect by string
type Dialer interface {
	Dial(remote string) (raw io.ReadWriteCloser, err error)
}

//DialerF is an the implementation of Dialer by func
type DialerF func(remote string) (raw io.ReadWriteCloser, err error)

//Dial dial to remote by func
func (d DialerF) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	raw, err = d(remote)
	return
}

//RawDialer is an interface to dial raw conenction
type RawDialer interface {
	Dial(network, address string) (net.Conn, error)
}

//RawDialerF is an the implementation of RawDialer by func
type RawDialerF func(network, address string) (net.Conn, error)

//Dial dial to remote by func
func (d RawDialerF) Dial(network, address string) (raw net.Conn, err error) {
	raw, err = d(network, address)
	return
}

//RawDialerWrapper is wrapper to Dialer and RawDialer
type RawDialerWrapper struct {
	RawDialer
}

//NewRawDialerWrapper will create wrapper
func NewRawDialerWrapper(d RawDialer) (dialer *RawDialerWrapper) {
	dialer = &RawDialerWrapper{RawDialer: d}
	return
}

//Dial to remote
func (r *RawDialerWrapper) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	target, err := url.Parse(remote)
	if err != nil {
		return
	}
	raw, err = r.RawDialer.Dial(target.Scheme, target.Host)
	return
}

//DialerWrapper is wrapper to Dialer and RawDialer
type DialerWrapper struct {
	Dialer
}

//NewDialerWrapper will create wrapper
func NewDialerWrapper(d Dialer) (dialer *DialerWrapper) {
	dialer = &DialerWrapper{Dialer: d}
	return
}

//Dial to remote
func (d *DialerWrapper) Dial(network, address string) (conn net.Conn, err error) {
	raw, err := d.Dialer.Dial(network + "://" + network)
	if err == nil {
		conn = NewConnWrapper(raw)
	}
	return
}

//NetDialer is an implementation of Dialer by net
type NetDialer struct {
	DNS string
	UDP RawDialer
	TCP RawDialer
}

//NewNetDialer will create new NetDialer
func NewNetDialer(dns string) (dialer *NetDialer) {
	dialer = &NetDialer{
		DNS: dns,
		UDP: &net.Dialer{
			Timeout: 5 * time.Second,
		},
		TCP: &net.Dialer{
			Timeout: 5 * time.Second,
		},
	}
	return
}

//Dial dial to remote by net
func (n *NetDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	DebugLog("NetDialer dial to %v", remote)
	if strings.Contains(remote, "://") {
		var u *url.URL
		u, err = url.Parse(remote)
		if err != nil {
			return
		}
		switch u.Scheme {
		case "dns":
			raw, err = n.UDP.Dial("udp", n.DNS)
		case "udp":
			raw, err = n.UDP.Dial("udp", u.Host)
		case "tcp":
			raw, err = n.TCP.Dial("tcp", u.Host)
		default:
			err = fmt.Errorf("not supported scheme %v", u.Scheme)
		}
	} else {
		//for old version
		raw, err = n.TCP.Dial("tcp", remote)
	}
	if err == nil {
		raw = NewStringConn(raw)
		DebugLog("NetDialer dial to %v success", remote)
	} else {
		DebugLog("NetDialer dial to %v fail with %v", remote, err)
	}
	return
}

func (n *NetDialer) String() string {
	return "NetDialer"
}

//WebsocketDialer is an implementation of Dialer by websocket
type WebsocketDialer struct {
	Dialer RawDialer
}

//NewWebsocketDialer will create new WebsocketDialer
func NewWebsocketDialer() (dialer *WebsocketDialer) {
	dialer = &WebsocketDialer{
		Dialer: &net.Dialer{},
	}
	return
}

//Dial dial to remote by websocket
func (w WebsocketDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	targetURL, err := url.Parse(remote)
	if err != nil {
		return
	}
	username, password := targetURL.Query().Get("username"), targetURL.Query().Get("password")
	skipVerify := targetURL.Query().Get("skip_verify") == "1"
	timeout, _ := strconv.ParseUint(targetURL.Query().Get("timeout"), 10, 32)
	if timeout < 1 {
		timeout = 5
	}
	var origin string
	if targetURL.Scheme == "wss" {
		origin = fmt.Sprintf("https://%v", targetURL.Host)
	} else {
		origin = fmt.Sprintf("http://%v", targetURL.Host)
	}
	config, err := websocket.NewConfig(targetURL.String(), origin)
	if err == nil {
		if len(username) > 0 && len(password) > 0 {
			config.Header.Set("Authorization", "Basic "+basicAuth(username, password))
		}
		config.TlsConfig = &tls.Config{}
		config.TlsConfig.InsecureSkipVerify = skipVerify
		raw, err = w.dial(config, time.Duration(timeout)*time.Second)
	}
	if err == nil {
		InfoLog("WebsocketDialer dial to %v success", remote)
	} else {
		InfoLog("WebsocketDialer dial to %v fail with %v", remote, err)
	}
	return
}

var portMap = map[string]string{
	"ws":  "80",
	"wss": "443",
}

func parseAuthority(location *url.URL) string {
	if _, ok := portMap[location.Scheme]; ok {
		if _, _, err := net.SplitHostPort(location.Host); err != nil {
			return net.JoinHostPort(location.Host, portMap[location.Scheme])
		}
	}
	return location.Host
}

func tlsHandshake(rawConn net.Conn, timeout time.Duration, config *tls.Config) (conn *tls.Conn, err error) {
	errChannel := make(chan error, 2)
	time.AfterFunc(timeout, func() {
		errChannel <- fmt.Errorf("timeout")
	})
	conn = tls.Client(rawConn, config)
	go func() {
		errChannel <- conn.Handshake()
	}()
	err = <-errChannel
	return conn, nil
}

func (w WebsocketDialer) dial(config *websocket.Config, timeout time.Duration) (conn net.Conn, err error) {
	remote := parseAuthority(config.Location)
	rawConn, err := w.Dialer.Dial("tcp", remote)
	if err == nil {
		if config.Location.Scheme == "wss" {
			conn, err = tlsHandshake(rawConn, timeout, config.TlsConfig)
		} else {
			conn = rawConn
		}
		if err == nil {
			conn, err = websocket.NewClient(config, conn)
		}
		if err != nil {
			rawConn.Close()
		}
	}
	return
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

//SortedDialer will auto sort the dialer by used time/error rate
type SortedDialer struct {
	dialers       []Dialer
	avgTime       []int64
	usedTime      []int64
	tryCount      []int64
	errCount      []int64
	errRate       []float32
	sorting       int32
	sortTime      int64
	sortLock      sync.RWMutex
	RateTolerance float32
	SortDelay     int64
}

//NewSortedDialer will new sorted dialer by sub dialer
func NewSortedDialer(dialers ...Dialer) (dialer *SortedDialer) {
	dialer = &SortedDialer{
		dialers:       dialers,
		avgTime:       make([]int64, len(dialers)),
		usedTime:      make([]int64, len(dialers)),
		tryCount:      make([]int64, len(dialers)),
		errCount:      make([]int64, len(dialers)),
		errRate:       make([]float32, len(dialers)),
		sortLock:      sync.RWMutex{},
		RateTolerance: 0.15,
		SortDelay:     5000,
	}
	return
}

//Dial impl the Dialer
func (s *SortedDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	err = fmt.Errorf("not dialer")
	s.sortLock.RLock()
	for i, dialer := range s.dialers {
		begin := Now()
		s.tryCount[i]++
		raw, err = dialer.Dial(remote)
		if err == nil {
			used := Now() - begin
			s.usedTime[i] += used
			s.avgTime[i] = s.usedTime[i] / (s.tryCount[i] - s.errCount[i])
			s.errRate[i] = float32(s.errCount[i]) / float32(s.tryCount[i])
			break
		}
		s.errCount[i]++
		s.errRate[i] = float32(s.errCount[i]) / float32(s.tryCount[i])
	}
	s.sortLock.RUnlock()
	if atomic.CompareAndSwapInt32(&s.sorting, 0, 1) && Now()-s.sortTime > s.SortDelay {
		go func() {
			s.sortLock.Lock()
			sort.Sort(s)
			s.sortLock.Unlock()
			s.sorting = 0
		}()
	}
	return
}

// Len is the number of elements in the collection.
func (s *SortedDialer) Len() int {
	return len(s.dialers)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s *SortedDialer) Less(i, j int) (r bool) {
	if s.errRate[i] < s.errRate[j] {
		r = s.errRate[j]-s.errRate[i] > s.RateTolerance || s.avgTime[i] < s.avgTime[j]
	} else {
		r = s.errRate[i]-s.errRate[j] < s.RateTolerance && s.avgTime[i] < s.avgTime[j]
	}
	return
}

// Swap swaps the elements with indexes i and j.
func (s *SortedDialer) Swap(i, j int) {
	s.dialers[i], s.dialers[j] = s.dialers[j], s.dialers[i]
	s.avgTime[i], s.avgTime[j] = s.avgTime[j], s.avgTime[i]
	s.usedTime[i], s.usedTime[j] = s.usedTime[j], s.usedTime[i]
	s.tryCount[i], s.tryCount[j] = s.tryCount[j], s.tryCount[i]
	s.errCount[i], s.errCount[j] = s.errCount[j], s.errCount[i]
	s.errRate[i], s.errRate[j] = s.errRate[j], s.errRate[i]
}

//State will return current dialer state
func (s *SortedDialer) State() interface{} {
	s.sortLock.RLock()
	res := []map[string]interface{}{}
	for i, dialer := range s.dialers {
		res = append(res, map[string]interface{}{
			"name":      fmt.Sprintf("%v", dialer),
			"avg_time":  s.avgTime[i],
			"used_time": s.usedTime[i],
			"try_count": s.tryCount[i],
			"err_count": s.errCount[i],
			"err_rate":  s.errRate[i],
		})
	}
	s.sortLock.RUnlock()
	return res
}

//AsyncConn is an io.ReadWriteCloser impl, it will wait the base connection is connected
type AsyncConn struct {
	Err  error
	Next io.ReadWriteCloser
	lck  sync.RWMutex
}

//NewAsyncConn will return new AsyncConn
func NewAsyncConn() (conn *AsyncConn) {
	conn = &AsyncConn{
		lck: sync.RWMutex{},
	}
	return
}

func (a *AsyncConn) Read(p []byte) (n int, err error) {
	a.lck.RLock()
	if a.Err == nil {
		n, err = a.Next.Read(p)
	} else {
		err = a.Err
	}
	a.lck.RUnlock()
	return
}

func (a *AsyncConn) Write(p []byte) (n int, err error) {
	a.lck.RLock()
	if a.Err == nil {
		n, err = a.Next.Write(p)
	} else {
		err = a.Err
	}
	a.lck.RUnlock()
	return
}

//Close will close connection
func (a *AsyncConn) Close() (err error) {
	a.lck.RLock()
	if a.Err == nil {
		err = a.Next.Close()
	}
	a.lck.RUnlock()
	return
}

//lock for connection, read/write/close will be locked
//it should be call befer dial return
func (a *AsyncConn) lock() {
	a.lck.Lock()
}

//unlocak for next connection is connected
func (a *AsyncConn) unlock(next io.ReadWriteCloser, err error) {
	a.Next = next
	a.Err = err
	a.lck.Unlock()
}

//AsyncDialer is Dialer impl, it will dial remote connection by async
type AsyncDialer struct {
	Next Dialer
}

//NewAsyncDialer will return new AsyncDialer
func NewAsyncDialer(next Dialer) (dialer *AsyncDialer) {
	dialer = &AsyncDialer{
		Next: next,
	}
	return
}

//Dial to remote and get the connection
func (a *AsyncDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	conn := NewAsyncConn()
	conn.lock() //lock for wait connect
	go func() {
		next, cerr := a.Next.Dial(remote)
		if cerr != nil {
			DebugLog("AsyncDialer dial to %v fail with %v", remote, cerr)
		}
		conn.unlock(next, cerr) //unlock for connected
	}()
	raw = conn
	return
}

//EchoConn is net.Conn impl by os.Pipe
type EchoConn struct {
	r, w *os.File
}

//NewEchoConn will return new echo connection
func NewEchoConn() (conn *EchoConn, err error) {
	conn = &EchoConn{}
	conn.r, conn.w, err = os.Pipe()
	return
}

func (e *EchoConn) Read(b []byte) (n int, err error) {
	n, err = e.r.Read(b)
	return
}

func (e *EchoConn) Write(b []byte) (n int, err error) {
	n, err = e.w.Write(b)
	return
}

// Close closes the connection.
func (e *EchoConn) Close() error {
	return e.r.Close()
}

// LocalAddr returns the local network address.
func (e *EchoConn) LocalAddr() net.Addr {
	return nil
}

// RemoteAddr returns the remote network address.
func (e *EchoConn) RemoteAddr() net.Addr {
	return nil
}

// SetDeadline for net.Conn
func (e *EchoConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline for net.Conn
func (e *EchoConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline for net.Conn
func (e *EchoConn) SetWriteDeadline(t time.Time) error {
	return nil
}

//EchoDialer is dialer test
type EchoDialer struct {
}

//NewEchoDialer will return new EchoDialer
func NewEchoDialer() (dialer *EchoDialer) {
	dialer = &EchoDialer{}
	return
}

//Dial dail one raw connection
func (e *EchoDialer) Dial(network, address string) (c net.Conn, err error) {
	c, err = NewEchoConn()
	return
}

const (
	MessageHeadConn  = 10
	MessageHeadBack  = 20
	MessageHeadData  = 30
	MessageHeadClose = 40
)

var messageConnSequence uint64

//MessageConn impl the  MessageConnection for read/write  message
type MessageConn struct {
	cid        uint64
	dialer     *MessageDialer
	remote     string
	connected  chan string
	readQueued chan []byte
	closed     bool
	lck        sync.RWMutex
}

//NewMessageConn will create new MessageConn
func NewMessageConn(dialer *MessageDialer, remote string) (conn *MessageConn) {
	conn = &MessageConn{
		dialer:     dialer,
		remote:     remote,
		cid:        atomic.AddUint64(&messageConnSequence, 1),
		connected:  make(chan string, 1),
		readQueued: make(chan []byte, 1024),
		lck:        sync.RWMutex{},
	}
	return
}

func (m *MessageConn) Read(p []byte) (n int, err error) {
	m.lck.RLock()
	if m.closed {
		m.lck.RUnlock()
		err = fmt.Errorf("closed")
		return
	}
	m.lck.RUnlock()
	data := <-m.readQueued
	if data == nil {
		err = fmt.Errorf("closed")
		return
	}
	if len(data) > len(p) {
		err = fmt.Errorf("buffer is too small")
		return
	}
	n = copy(p, data)
	return
}

func (m *MessageConn) Write(p []byte) (n int, err error) {
	m.lck.RLock()
	if m.closed {
		m.lck.RUnlock()
		err = fmt.Errorf("closed")
		return
	}
	m.lck.RUnlock()
	data := make([]byte, len(m.dialer.Header)+8+len(p))
	copy(data, m.dialer.Header)
	binary.BigEndian.PutUint64(data[len(m.dialer.Header):], m.cid)
	data[len(m.dialer.Header)+8] = MessageHeadData
	n = copy(data[len(m.dialer.Header)+9:], p)
	m.dialer.Message <- data
	return
}

//Connect send connect message
func (m *MessageConn) Connect() (err error) {
	m.lck.RLock()
	if m.closed {
		m.lck.RUnlock()
		err = fmt.Errorf("closed")
		return
	}
	m.lck.RUnlock()
	data := make([]byte, len(m.dialer.Header)+9+len(m.remote))
	copy(data, m.dialer.Header)
	binary.BigEndian.PutUint64(data[len(m.dialer.Header):], m.cid)
	data[len(m.dialer.Header)+8] = MessageHeadConn
	copy(data[len(m.dialer.Header)+9:], []byte(m.remote))
	m.dialer.Message <- data
	message := <-m.connected
	if len(message) > 0 {
		err = fmt.Errorf("%v", message)
	}
	return
}

//Close will close the MessageConnection
func (m *MessageConn) Close() (err error) {
	err = m.closeOnly()
	m.dialer.closeConn(m)
	return
}

func (m *MessageConn) closeOnly() (err error) {
	m.lck.Lock()
	defer m.lck.Unlock()
	if m.closed {
		err = fmt.Errorf("closed")
		return
	}
	m.closed = true
	close(m.readQueued)
	close(m.connected)
	return
}

// LocalAddr returns the local network address.
func (m *MessageConn) LocalAddr() net.Addr {
	return m
}

// RemoteAddr returns the remote network address.
func (m *MessageConn) RemoteAddr() net.Addr {
	return m
}

// SetDeadline for net.Conn
func (m *MessageConn) SetDeadline(t time.Time) error {
	return nil
}

// SetReadDeadline for net.Conn
func (m *MessageConn) SetReadDeadline(t time.Time) error {
	return nil
}

// SetWriteDeadline for net.Conn
func (m *MessageConn) SetWriteDeadline(t time.Time) error {
	return nil
}

//Network impl net.Addr
func (m *MessageConn) Network() string {
	return "message"
}

func (m *MessageConn) String() string {
	return m.remote
}

//MessageDialer is dialer impl for message
type MessageDialer struct {
	Header   []byte
	Message  chan []byte
	conns    map[uint64]*MessageConn
	connsLck sync.RWMutex
	closed   bool
}

//NewMessageDialer will create new MessageDialer
func NewMessageDialer(header []byte) (dialer *MessageDialer) {
	dialer = &MessageDialer{
		Header:   header,
		Message:  make(chan []byte, 1024),
		conns:    map[uint64]*MessageConn{},
		connsLck: sync.RWMutex{},
	}
	return
}

//Dial dail one raw connection
func (m *MessageDialer) Dial(network, address string) (c net.Conn, err error) {
	InfoLog("MessageDialer dial to %v://%v\n", network, address)
	m.connsLck.Lock()
	if m.closed {
		err = fmt.Errorf("closed")
		m.connsLck.Unlock()
		return
	}
	conn := NewMessageConn(m, network+"://"+address)
	m.conns[conn.cid] = conn
	m.connsLck.Unlock()
	err = conn.Connect()
	if err == nil {
		c = conn
	}
	return
}

func (m *MessageDialer) closeConn(c *MessageConn) {
	m.connsLck.Lock()
	defer m.connsLck.Unlock()
	delete(m.conns, c.cid)
}

//Deliver deliver message to connection
func (m *MessageDialer) Write(b []byte) (n int, err error) {
	head := len(m.Header)
	if len(b) < head+8 {
		err = fmt.Errorf("invalid data")
		return
	}
	cid := binary.BigEndian.Uint64(b[head:])
	m.connsLck.RLock()
	if m.closed {
		err = fmt.Errorf("closed")
		m.connsLck.RUnlock()
		return
	}
	conn, ok := m.conns[cid]
	m.connsLck.RUnlock()
	if !ok {
		err = fmt.Errorf("connection not exist")
		return
	}
	cmd := b[head+8]
	switch cmd {
	case MessageHeadData:
		conn.readQueued <- b[head+9:]
	case MessageHeadBack:
		conn.connected <- string(b[head+9:])
	case MessageHeadClose:
		conn.Close()
	default:
		err = fmt.Errorf("unknow command(%v)", b[head+8])
	}
	n = len(b)
	return
}

func (m *MessageDialer) Read(b []byte) (n int, err error) {
	m.connsLck.RLock()
	if m.closed {
		err = fmt.Errorf("closed")
		m.connsLck.RUnlock()
		return
	}
	m.connsLck.RUnlock()
	data := <-m.Message
	if data == nil {
		err = fmt.Errorf("closed")
		return
	}
	if len(data) > len(b) {
		err = fmt.Errorf("buffer is too small")
		return
	}
	n = copy(b, data)
	return
}

//Close all connection
func (m *MessageDialer) Close() (err error) {
	m.connsLck.Lock()
	defer m.connsLck.Unlock()
	m.closed = true
	for cid, conn := range m.conns {
		conn.closeOnly()
		delete(m.conns, cid)
	}
	close(m.Message)
	return
}

//HeadDistWriteCloser is WriteCloser by head
type HeadDistWriteCloser struct {
	ws  map[byte]io.WriteCloser
	lck sync.RWMutex
}

//NewHeadDistWriteCloser will return new HeadDistWriteCloser
func NewHeadDistWriteCloser() (writer *HeadDistWriteCloser) {
	writer = &HeadDistWriteCloser{
		ws:  map[byte]io.WriteCloser{},
		lck: sync.RWMutex{},
	}
	return
}

//Add WriteCloser to list
func (h *HeadDistWriteCloser) Add(m byte, w io.WriteCloser) {
	h.lck.Lock()
	defer h.lck.Unlock()
	h.ws[m] = w
}

func (h *HeadDistWriteCloser) Write(b []byte) (n int, err error) {
	h.lck.RLock()
	writer, ok := h.ws[b[0]]
	h.lck.RUnlock()
	if ok {
		n, err = writer.Write(b)
	} else {
		err = fmt.Errorf("writer not exist by %v", b[0])
	}
	return
}

//Close will close all connection
func (h *HeadDistWriteCloser) Close() (err error) {
	h.lck.RLock()
	for _, w := range h.ws {
		w.Close()
	}
	h.lck.RUnlock()
	return
}
