package core

import (
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/codingeasygo/util/converter"
	"github.com/codingeasygo/util/xio"
	"golang.org/x/net/websocket"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()
}

func TestWebsocketDialer(t *testing.T) {
	{ //auth test
		ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
			io.Copy(ws, ws)
		}))
		wsurl := strings.Replace(ts.URL, "http://", "ws://", 1)
		fmt.Println(wsurl)
		dialer := NewWebsocketDialer()
		raw, err := dialer.Dial(wsurl + "?username=x&password=123")
		if err != nil {
			t.Error(err)
			return
		}
		go func() {
			io.Copy(os.Stdout, raw)
		}()
		fmt.Fprintf(raw, "data\n")
		raw.Close()
	}
	{ //ws test
		ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
			io.Copy(ws, ws)
		}))
		wsurl := strings.Replace(ts.URL, "http://", "ws://", 1)
		fmt.Println(wsurl)
		dialer := NewWebsocketDialer()
		raw, err := dialer.Dial(wsurl)
		if err != nil {
			t.Error(err)
			return
		}
		go func() {
			io.Copy(os.Stdout, raw)
		}()
		fmt.Fprintf(raw, "data\n")
		raw.Close()
	}
	{ //wss test
		ts := httptest.NewTLSServer(websocket.Handler(func(ws *websocket.Conn) {
			io.Copy(ws, ws)
		}))
		wsurl := strings.Replace(ts.URL, "https://", "wss://", 1) + "?skip_verify=1"
		fmt.Println(wsurl)
		dialer := NewWebsocketDialer()
		raw, err := dialer.Dial(wsurl)
		if err != nil {
			t.Error(err)
			return
		}
		go func() {
			io.Copy(os.Stdout, raw)
		}()
		fmt.Fprintf(raw, "data\n")
		raw.Close()
	}
	{ //error test
		dialer := NewWebsocketDialer()
		_, err := dialer.Dial("://xxx")
		if err == nil {
			t.Error(err)
			return
		}
		_, err = dialer.Dial("ws://127.0.0.1:x")
		if err == nil {
			t.Error(err)
			return
		}
	}
	{ //tls error

	}
}

func TestNetDialer(t *testing.T) {
	ts := httptest.NewServer(nil)
	dialer := NewNetDialer("", "192.168.1.1:53")
	_, err := dialer.Dial("tcp://" + strings.TrimPrefix(ts.URL, "http://"))
	if err != nil {
		t.Error(err)
		return
	}
	_, err = dialer.Dial("dns://proxy")
	if err != nil {
		t.Error(err)
		return
	}
	_, err = dialer.Dial("127.0.0.1:x")
	if err == nil {
		t.Error("error")
		return
	}
	_, err = dialer.Dial("tcp://127.0.0.1:x")
	if err == nil {
		t.Error("error")
		return
	}
	_, err = dialer.DialPiper("tcp://", 1024)
	if err == nil {
		t.Error("error")
		return
	}
	piper, err := dialer.DialPiper("dns://proxy", 1024)
	if err != nil {
		t.Error(err)
		return
	}
	piper.Close()
	fmt.Println(dialer.String())
}

type TagRWC struct {
	Tag   string
	Panic bool
}

func (t *TagRWC) Read(p []byte) (l int, err error) {
	if t.Panic {
		panic("not")
	}
	return
}
func (t *TagRWC) Write(p []byte) (l int, err error) {
	if t.Panic {
		panic("not")
	}
	return
}
func (t *TagRWC) Close() (err error) {
	if t.Panic {
		panic("not")
	}
	return
}

func TestSortedDialerDial(t *testing.T) {
	willErr := 0
	dialer := NewSortedDialer(
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			if willErr == 1 || willErr == 3 {
				err = fmt.Errorf("error")
				return
			}
			time.Sleep(10 * time.Millisecond)
			raw = &TagRWC{Tag: "1", Panic: true}
			return
		}),
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			if willErr == 2 || willErr == 3 {
				err = fmt.Errorf("error")
				return
			}
			time.Sleep(1 * time.Millisecond)
			raw = &TagRWC{Tag: "2", Panic: true}
			return
		}),
	)
	dialer.RateTolerance = 0.05
	dialer.SortDelay = 1
	//
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "1" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	//
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "2" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	//
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "2" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	//
	willErr = 2
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "1" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	willErr = 0
	//
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "1" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	//
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "1" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	//
	willErr = 1
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "2" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	willErr = 0
	//
	fmt.Println(dialer.dialers, dialer.avgTime, dialer.errRate)
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "2" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
	fmt.Println(dialer.State())
}

type sortedTestDialer struct {
	Avg   Duration
	Error string
}

func (s *sortedTestDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	return
}

func (s *sortedTestDialer) Test(remote string, worker func(raw io.ReadWriteCloser) (err error)) (result *TestResult) {
	result = &TestResult{}
	result.Avg, result.Error = s.Avg, s.Error
	return
}

func TestSortedDialerTest(t *testing.T) {
	dialer := NewSortedDialer(
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			raw = &TagRWC{Tag: "1", Panic: false}
			return
		}),
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			raw = &TagRWC{Tag: "2", Panic: false}
			return
		}),
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			raw = &TagRWC{Tag: "3", Panic: false}
			return
		}),
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			err = fmt.Errorf("dial error 4")
			return
		}),
		&sortedTestDialer{
			Avg:   Duration(30 * time.Millisecond),
			Error: "",
		},
		&sortedTestDialer{
			Avg:   Duration(30 * time.Millisecond),
			Error: "error",
		},
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			err = fmt.Errorf("dial error 5")
			return
		}),
	)
	dialer.RateTolerance = 0.05
	dialer.SortDelay = 1
	dialer.TestMax = 6
	result := dialer.Test("", func(raw io.ReadWriteCloser) (err error) {
		switch raw.(*TagRWC).Tag {
		case "1":
			time.Sleep(20 * time.Millisecond)
		case "2":
			time.Sleep(10 * time.Millisecond)
		case "3":
			err = fmt.Errorf("test error 3")
		}
		return
	})
	fmt.Println(converter.JSON(result))
	if res, _ := dialer.Dial(""); res.(*TagRWC).Tag != "2" {
		t.Error(res)
		return
	}
	for dialer.sorting == 1 {
		time.Sleep(time.Millisecond)
	}
}

func TestPACDialer(t *testing.T) {
	var proxy *PACDialer
	echo := xio.NewEchoDialer()
	proxy = NewPACDialer(echo, nil)
	proxy.Mode = "global"
	raw, err := proxy.DialPiper("proxy", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	proxy = NewPACDialer(echo, nil)
	proxy.Mode = "auto"
	raw, err = proxy.DialPiper("tcp://proxy", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	proxy = NewPACDialer(nil, echo)
	proxy.Mode = ""
	raw, err = proxy.DialPiper("tcp://proxy", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	//
	fmt.Printf("%v\n", proxy.String())
	proxy.DialPiper("tcp://%2f", 1024)
}

func TestAutoPACDialer(t *testing.T) {
	var proxy *AutoPACDialer
	echo := xio.NewEchoDialer()
	directc := 0
	proxy = NewAutoPACDialer(echo, xio.PiperDialerF(func(target string, bufferSize int) (raw xio.Piper, err error) {
		if target == "tcp://proxy" && directc == 0 {
			err = fmt.Errorf("error")
			directc++
			return
		}
		if directc == 1 {
			panic("error")
		}
		return
	}))
	raw, err := proxy.DialPiper("tcp://proxy", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	raw, err = proxy.DialPiper("tcp://proxy", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	err = proxy.SaveCache("/tmp/pac.cache")
	if err != nil {
		t.Error(err)
		return
	}
	//load
	proxy = NewAutoPACDialer(echo, xio.PiperDialerF(func(target string, bufferSize int) (raw xio.Piper, err error) {
		panic("error")
	}))
	err = proxy.LoadCache("/tmp/pac.cache")
	if err != nil {
		t.Error(err)
		return
	}
	raw, err = proxy.DialPiper("tcp://proxy", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	//
	proxy = NewAutoPACDialer(nil, echo)
	raw, err = proxy.DialPiper("tcp://local", 1024)
	if err != nil || raw == nil {
		t.Error(err)
		return
	}
	//
	_, err = proxy.DialPiper("tcp://%2f", 1024)
	if err == nil {
		t.Error(err)
		return
	}
	fmt.Println(proxy.String())
}
