package core

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xtime"
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

//DialPiper is xio.PiperDialer implement by dial raw by net dailer
func (d DialerF) DialPiper(target string, bufferSize int) (piper xio.Piper, err error) {
	raw, err := d(target)
	if err == nil {
		piper = xio.NewCopyPiper(raw, bufferSize)
	}
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

//AliasURI is interface to find really uri
type AliasURI interface {
	Find(target string) (string, error)
}

//MapAliasURI is implement alias uri
type MapAliasURI map[string]string

//Find really uri
func (m MapAliasURI) Find(target string) (remote string, err error) {
	u, err := url.Parse(target)
	if err != nil {
		return
	}
	remote, ok := m[u.Host]
	if !ok {
		remote = target
	}
	return
}

//NetDialer is an implementation of Dialer by net
type NetDialer struct {
	DNS   string
	UDP   RawDialer
	TCP   RawDialer
	Alias AliasURI
}

//NewNetDialer will create new NetDialer
func NewNetDialer(bind, dns string) (dialer *NetDialer) {
	udp := &net.Dialer{
		Timeout: 5 * time.Second,
	}
	if len(bind) > 0 {
		udp.LocalAddr = &net.UDPAddr{
			IP: net.ParseIP(bind),
		}
	}
	tcp := &net.Dialer{
		Timeout: 5 * time.Second,
	}
	if len(bind) > 0 {
		tcp.LocalAddr = &net.TCPAddr{
			IP: net.ParseIP(bind),
		}
	}
	dialer = &NetDialer{
		DNS: dns,
		UDP: udp,
		TCP: tcp,
	}
	return
}

//Dial dial to remote by net
func (n *NetDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	target := remote
	if n.Alias != nil {
		remote, err = n.Alias.Find(target)
	}
	if err != nil {
		return
	}
	DebugLog("NetDialer dial to %v, really is %v", target, remote)
	if strings.Contains(remote, "://") {
		var u *url.URL
		u, err = url.Parse(remote)
		if err != nil {
			return
		}
		switch u.Scheme {
		case "echo":
			raw = xio.NewEchoConn()
		case "dns":
			raw, err = n.UDP.Dial("udp", n.DNS)
			// if err == nil {
			// 	raw = NewCountConn(raw, &n.udpRead, &n.udpWrite)
			// }
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
		raw = xio.NewStringConn(raw)
		DebugLog("NetDialer dial to %v success", remote)
	} else {
		DebugLog("NetDialer dial to %v fail with %v", remote, err)
	}
	return
}

//DialPiper is xio.PiperDialer implement by dial raw by net dailer
func (n *NetDialer) DialPiper(target string, bufferSize int) (piper xio.Piper, err error) {
	raw, err := n.Dial(target)
	if err == nil {
		piper = xio.NewCopyPiper(raw, bufferSize)
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
		begin := xtime.Now()
		s.tryCount[i]++
		raw, err = dialer.Dial(remote)
		if err == nil {
			used := xtime.Now() - begin
			s.usedTime[i] += used
			s.avgTime[i] = s.usedTime[i] / (s.tryCount[i] - s.errCount[i])
			s.errRate[i] = float32(s.errCount[i]) / float32(s.tryCount[i])
			break
		}
		s.errCount[i]++
		s.errRate[i] = float32(s.errCount[i]) / float32(s.tryCount[i])
	}
	s.sortLock.RUnlock()
	if atomic.CompareAndSwapInt32(&s.sorting, 0, 1) && xtime.Now()-s.sortTime > s.SortDelay {
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

// //AsyncConn is an io.ReadWriteCloser impl, it will wait the base connection is connected
// type AsyncConn struct {
// 	Err  error
// 	Next io.ReadWriteCloser
// 	lck  sync.RWMutex
// }

// //NewAsyncConn will return new AsyncConn
// func NewAsyncConn() (conn *AsyncConn) {
// 	conn = &AsyncConn{
// 		lck: sync.RWMutex{},
// 	}
// 	return
// }

// func (a *AsyncConn) Read(p []byte) (n int, err error) {
// 	a.lck.RLock()
// 	if a.Err == nil {
// 		n, err = a.Next.Read(p)
// 	} else {
// 		err = a.Err
// 	}
// 	a.lck.RUnlock()
// 	return
// }

// func (a *AsyncConn) Write(p []byte) (n int, err error) {
// 	a.lck.RLock()
// 	if a.Err == nil {
// 		n, err = a.Next.Write(p)
// 	} else {
// 		err = a.Err
// 	}
// 	a.lck.RUnlock()
// 	return
// }

// //Close will close connection
// func (a *AsyncConn) Close() (err error) {
// 	a.lck.RLock()
// 	if a.Err == nil {
// 		err = a.Next.Close()
// 	}
// 	a.lck.RUnlock()
// 	return
// }

// //lock for connection, read/write/close will be locked
// //it should be call befer dial return
// func (a *AsyncConn) lock() {
// 	a.lck.Lock()
// }

// //unlocak for next connection is connected
// func (a *AsyncConn) unlock(next io.ReadWriteCloser, err error) {
// 	a.Next = next
// 	a.Err = err
// 	a.lck.Unlock()
// }

// //AsyncDialer is Dialer impl, it will dial remote connection by async
// type AsyncDialer struct {
// 	Next Dialer
// }

// //NewAsyncDialer will return new AsyncDialer
// func NewAsyncDialer(next Dialer) (dialer *AsyncDialer) {
// 	dialer = &AsyncDialer{
// 		Next: next,
// 	}
// 	return
// }

// //Dial to remote and get the connection
// func (a *AsyncDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
// 	conn := NewAsyncConn()
// 	conn.lock() //lock for wait connect
// 	go func() {
// 		next, cerr := a.Next.Dial(remote)
// 		if cerr != nil {
// 			DebugLog("AsyncDialer dial to %v fail with %v", remote, cerr)
// 		}
// 		conn.unlock(next, cerr) //unlock for connected
// 	}()
// 	raw = conn
// 	return
// }

// type PrintDialer struct {
// 	Dialer
// }

// func NewPrintDialer(base Dialer) (dialer *PrintDialer) {
// 	dialer = &PrintDialer{
// 		Dialer: base,
// 	}
// 	return
// }

// func (m *PrintDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
// 	base, err := m.Dialer.Dial(remote)
// 	if err == nil {
// 		raw = xio.NewPrintConn("Print", base)
// 	}
// 	return
// }

//PACDialer to impl xio.PiperDialer for pac
type PACDialer struct {
	Mode   string
	Proxy  xio.PiperDialer
	Direct xio.PiperDialer
	Check  func(h string) bool
}

//NewPACDialer will create new PACDialer
func NewPACDialer(proxy, direct xio.PiperDialer) (pac *PACDialer) {
	pac = &PACDialer{
		Proxy:  proxy,
		Direct: direct,
	}
	return
}

//DialPiper will dial by pac
func (p *PACDialer) DialPiper(target string, bufferSize int) (raw xio.Piper, err error) {
	u, err := url.Parse(target)
	if err != nil {
		return
	}
	proxy := false
	switch p.Mode {
	case "global":
		proxy = true
	case "auto":
		proxy = u.Host == "proxy" || (p.Check != nil && p.Check(u.Hostname()))
	default:
		proxy = false
	}
	if proxy {
		DebugLog("PACDialer(%v) follow proxy(%v) by %v", p.Mode, p.Proxy, target)
		raw, err = p.Proxy.DialPiper(target, bufferSize)
	} else {
		DebugLog("PACDialer(%v) follow direct(%v) by %v", p.Mode, p.Direct, target)
		raw, err = p.Direct.DialPiper(target, bufferSize)
	}
	return
}

func (p *PACDialer) String() string {
	return "PACDialer"
}

//AutoPACDialer prover dialer follow dial proxy auto when dial direct is error
type AutoPACDialer struct {
	Proxy     xio.PiperDialer
	Direct    xio.PiperDialer
	cache     map[string]int //cache for using proxy cache
	cacheLock sync.RWMutex
}

//NewAutoPACDialer will create new AutoPACDialer
func NewAutoPACDialer(proxy, direct xio.PiperDialer) (pac *AutoPACDialer) {
	pac = &AutoPACDialer{
		Proxy:     proxy,
		Direct:    direct,
		cache:     map[string]int{},
		cacheLock: sync.RWMutex{},
	}
	return
}

//SaveCache will save cache to filename
func (p *AutoPACDialer) SaveCache(filename string) (err error) {
	p.cacheLock.RLock()
	err = WriteJSON(filename, p.cache)
	p.cacheLock.RUnlock()
	InfoLog("AutoPACDialer save cache to %v with %v", filename, err)
	return
}

//LoadCache will load cache from filename
func (p *AutoPACDialer) LoadCache(filename string) (err error) {
	p.cacheLock.Lock()
	err = ReadJSON(filename, &p.cache)
	p.cacheLock.Unlock()
	InfoLog("AutoPACDialer load cache from %v with %v", filename, err)
	return
}

//DialPiper will dial by pac
func (p *AutoPACDialer) DialPiper(target string, bufferSize int) (raw xio.Piper, err error) {
	hostname := target
	if strings.Contains(target, "://") {
		var u *url.URL
		u, err = url.Parse(target)
		if err != nil {
			return
		}
		hostname = u.Hostname()
	}
	proxy := 0
	ok := false
	p.cacheLock.RLock()
	proxy, ok = p.cache[hostname]
	p.cacheLock.RUnlock()
	if proxy < 1 {
		DebugLog("AutoPACDialer try follow direct(%v) by %v", p.Direct, target)
		raw, err = p.Direct.DialPiper(target, bufferSize)
		if err == nil {
			return
		}
	}
	DebugLog("AutoPACDialer try follow proxy(%v) by %v", p.Proxy, target)
	raw, err = p.Proxy.DialPiper(target, bufferSize)
	if err == nil && !ok {
		p.cacheLock.Lock()
		p.cache[hostname] = 1
		p.cacheLock.Unlock()
		InfoLog("AutoPACDialer add %v to proxy cache", hostname)
	}
	return
}

func (p *AutoPACDialer) String() string {
	return "AutoPACDialer"
}
