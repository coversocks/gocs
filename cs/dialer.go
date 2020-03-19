package cs

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
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

//NetDialer is an implementation of Dialer by net
type NetDialer string

//Dial dial to remote by net
func (t NetDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	DebugLog("NetDialer %v dial to %v", t, remote)
	raw, err = net.Dial(string(t), remote)
	if err == nil {
		raw = NewStringConn(raw)
	}
	return
}

//WebsocketDialer is an implementation of Dialer by websocket
type WebsocketDialer string

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
		config.Dialer = &net.Dialer{}
		config.Dialer.Timeout = time.Duration(timeout) * time.Second
		raw, err = websocket.DialConfig(config)
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
