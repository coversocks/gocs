package core

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"regexp"
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

type Duration time.Duration

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) (err error) {
	val, err := time.ParseDuration(string(b))
	if err == nil {
		*d = Duration(val)
	}
	return
}

func (d *Duration) String() string {
	return time.Duration(*d).String()
}

type TestResult struct {
	Min     Duration               `json:"min"`
	Max     Duration               `json:"max"`
	Avg     Duration               `json:"avg"`
	Success int64                  `json:"success"`
	Fail    int64                  `json:"fail"`
	Error   string                 `json:"error"`
	Info    map[string]interface{} `json:"info"`
}

//Tester is interface for test raw connect by worker
type Tester interface {
	Test(remote string, worker func(raw io.ReadWriteCloser) (err error)) (result *TestResult)
}

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
		if u.Hostname() == "echo" {
			raw = xio.NewEchoConn()
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
	return
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
	keys          []string
	dialers       map[string]Dialer
	avgTime       map[string]int64
	usedTime      map[string]int64
	tryCount      map[string]int64
	errCount      map[string]int64
	errRate       map[string]float32
	sorting       int32
	sortTime      int64
	sortLock      sync.RWMutex
	Name          string
	RateTolerance float32
	SortDelay     int64
	TestMax       int
}

//NewSortedDialer will new sorted dialer by sub dialer
func NewSortedDialer(dialers ...Dialer) (dialer *SortedDialer) {
	dialer = &SortedDialer{
		dialers:       map[string]Dialer{},
		avgTime:       map[string]int64{},
		usedTime:      map[string]int64{},
		tryCount:      map[string]int64{},
		errCount:      map[string]int64{},
		errRate:       map[string]float32{},
		sortLock:      sync.RWMutex{},
		RateTolerance: 0.15,
		SortDelay:     5000,
		TestMax:       3,
	}
	for _, d := range dialers {
		key := fmt.Sprintf("%v", d)
		dialer.keys = append(dialer.keys, key)
		dialer.dialers[key] = d
	}
	return
}

//Dial impl the Dialer
func (s *SortedDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	err = fmt.Errorf("not dialer")
	for _, key := range s.keys {
		s.sortLock.Lock()
		dialer := s.dialers[key]
		begin := xtime.Now()
		s.tryCount[key]++
		s.sortLock.Unlock()
		raw, err = dialer.Dial(remote)
		if err == nil {
			s.sortLock.Lock()
			used := xtime.Now() - begin
			s.usedTime[key] += used
			s.avgTime[key] = s.usedTime[key] / (s.tryCount[key] - s.errCount[key])
			s.errRate[key] = float32(s.errCount[key]) / float32(s.tryCount[key])
			s.sortLock.Unlock()
			break
		}
		s.sortLock.Lock()
		s.errCount[key]++
		s.errRate[key] = float32(s.errCount[key]) / float32(s.tryCount[key])
		s.sortLock.Unlock()
	}
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
	iKey, jKey := s.keys[i], s.keys[j]
	if s.errRate[iKey] < s.errRate[jKey] {
		r = s.errRate[jKey]-s.errRate[iKey] > s.RateTolerance || s.avgTime[iKey] < s.avgTime[jKey]
	} else {
		r = s.errRate[iKey]-s.errRate[jKey] < s.RateTolerance && s.avgTime[iKey] < s.avgTime[jKey]
	}
	return
}

// Swap swaps the elements with indexes i and j.
func (s *SortedDialer) Swap(i, j int) {
	s.keys[i], s.keys[j] = s.keys[j], s.keys[i]
}

// Test will test and sort dialer
func (s *SortedDialer) Test(remote string, worker func(raw io.ReadWriteCloser) (err error)) (result *TestResult) {
	result = &TestResult{}
	allKeys := s.keys
	if s.TestMax > 0 && len(allKeys) > s.TestMax {
		allKeys = allKeys[0:s.TestMax]
	}
	allDialer := map[string]Dialer{}
	s.sortLock.RLock()
	for _, key := range allKeys {
		allDialer[key] = s.dialers[key]
	}
	s.sortLock.RUnlock()
	allUsed := map[string]Duration{}
	allFail := map[string]string{}
	allInfo := map[string]*TestResult{}
	for key, dialer := range allDialer {
		if tester, ok := dialer.(Tester); ok {
			keyResult := tester.Test(remote, worker)
			allInfo[key] = keyResult
			if len(keyResult.Error) > 0 {
				allFail[key] = keyResult.Error
				result.Error = keyResult.Error
				continue
			}
			allUsed[key] = keyResult.Avg
		} else {
			begin := time.Now()
			raw, err := dialer.Dial(remote)
			if err != nil {
				allFail[key] = err.Error()
				result.Error = err.Error()
				continue
			}
			err = worker(raw)
			if err != nil {
				allFail[key] = err.Error()
				result.Error = err.Error()
				raw.Close()
				continue
			}
			allUsed[key] = Duration(time.Since(begin))
			raw.Close()
		}
	}
	s.sortLock.Lock()
	totalUsed := Duration(0)
	result.Min = Duration(time.Minute)
	result.Info = map[string]interface{}{}
	for key := range allDialer {
		fail := allFail[key]
		used := allUsed[key]
		s.tryCount[key]++
		if len(fail) > 0 {
			s.errCount[key]++
			s.errRate[key] = float32(s.errCount[key]) / float32(s.tryCount[key])
			result.Fail++
			result.Info[key] = fail
		} else {
			s.usedTime[key] += time.Duration(used).Milliseconds()
			s.avgTime[key] = s.usedTime[key] / (s.tryCount[key] - s.errCount[key])
			s.errRate[key] = float32(s.errCount[key]) / float32(s.tryCount[key])
			result.Success++
			if result.Min > used {
				result.Min = used
			}
			if result.Max < used {
				result.Max = used
			}
			totalUsed += used
			result.Info[key] = used.String()
		}
		if allInfo[key] != nil {
			result.Info[key] = allInfo[key]
		}
	}
	if result.Success > 0 {
		result.Avg = totalUsed / Duration(result.Success)
	}
	sort.Sort(s)
	s.sortLock.Unlock()
	return
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

func (s *SortedDialer) String() string {
	return s.Name
}

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

type SkipDialer struct {
	Proxy       xio.PiperDialer
	Direct      xio.PiperDialer
	skipMatcher []*regexp.Regexp
}

func NewSkipDialer(proxy xio.PiperDialer) (dialer *SkipDialer) {
	dialer = &SkipDialer{
		Proxy:  proxy,
		Direct: NewNetDialer("", ""),
	}
	return
}

func (s *SkipDialer) AddSkip(skips ...string) (err error) {
	var matcher *regexp.Regexp
	var matchers []*regexp.Regexp
	for _, skip := range skips {
		matcher, err = regexp.Compile(skip)
		if err != nil {
			break
		}
		matchers = append(matchers, matcher)
	}
	if err == nil {
		s.skipMatcher = append(s.skipMatcher, matchers...)
	}
	return
}

func (s *SkipDialer) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	for _, matcher := range s.skipMatcher {
		if matcher.MatchString(uri) {
			raw, err = s.Direct.DialPiper(uri, bufferSize)
			return
		}
	}
	raw, err = s.Proxy.DialPiper(uri, bufferSize)
	return
}
