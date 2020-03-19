package cs

import (
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func TestWebsocketDialer(t *testing.T) {
	{ //auth test
		ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
			io.Copy(ws, ws)
		}))
		wsurl := strings.Replace(ts.URL, "http://", "ws://", 1)
		fmt.Println(wsurl)
		dialer := WebsocketDialer("")
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
		dialer := WebsocketDialer("")
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
		dialer := WebsocketDialer("")
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
		dialer := WebsocketDialer("")
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
}

func TestNetDialer(t *testing.T) {
	dialer := NetDialer("tcp")
	_, err := dialer.Dial("127.0.0.1:80")
	if err != nil {
		t.Error("error")
		return
	}
	_, err = dialer.Dial("127.0.0.1:x")
	if err == nil {
		t.Error("error")
		return
	}
}

type TagRWC struct {
	Tag string
}

func (t *TagRWC) Read(p []byte) (l int, err error) {
	panic("not")
}
func (t *TagRWC) Write(p []byte) (l int, err error) {
	panic("not")
}
func (t *TagRWC) Close() (err error) {
	panic("not")
}

func TestSortedDialer(t *testing.T) {
	willErr := 0
	dialer := NewSortedDialer(
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			if willErr == 1 || willErr == 3 {
				err = fmt.Errorf("error")
				return
			}
			time.Sleep(10 * time.Millisecond)
			raw = &TagRWC{Tag: "1"}
			return
		}),
		DialerF(func(r string) (raw io.ReadWriteCloser, err error) {
			if willErr == 2 || willErr == 3 {
				err = fmt.Errorf("error")
				return
			}
			time.Sleep(1 * time.Millisecond)
			raw = &TagRWC{Tag: "2"}
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
}
