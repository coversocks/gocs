package core

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"net/http"
	_ "net/http/pprof"

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
	dialer := NewNetDialer("", "114.114.114.114:53")
	_, err := dialer.Dial("tcp://127.0.0.1:80")
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
	fmt.Println(dialer.State())
}

func TestMessageDialer(t *testing.T) {
	wg := sync.WaitGroup{}
	dialer := NewMessageDialer([]byte("m"), 1024)
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := dialer.Read(buf)
			if err != nil {
				break
			}
			switch buf[9] {
			case MessageHeadConn:
				remote := string(buf[10:n])
				if remote == "tcp://test" {
					fmt.Printf("dialer conn %v\n", string(buf[10:n]))
					buf[9] = MessageHeadBack
					dialer.Write(buf[0:10])
				} else {
					buf[9] = MessageHeadBack
					l := copy(buf[10:], "testerror")
					dialer.Write(buf[0 : 10+l])
				}
			case MessageHeadData:
				fmt.Printf("dialer recv %v\n", string(buf[10:n]))
				dialer.Write(buf[0:n])
			}
		}
		wg.Done()
	}()
	//test read write
	{
		conn, err := dialer.Dial("tcp", "test")
		if err != nil {
			t.Error(err)
			return
		}
		var sended, recved int
		go func() {
			for {
				buf := make([]byte, 1024)
				n, err := conn.Read(buf)
				if err != nil {
					break
				}
				recved += n
				fmt.Printf("conn recv %v\n", string(buf[:n]))
			}
			wg.Done()
		}()
		for i := 0; i < 10; i++ {
			n, err := fmt.Fprintf(conn, "data-%v\n", i*i*i)
			if err != nil {
				break
			}
			sended += n
			fmt.Printf("conn send %v\n", sended)
		}
		for recved < sended {
			time.Sleep(10 * time.Millisecond)
		}
		if sended != recved {
			t.Errorf("sended:%v,recved:%v", sended, recved)
			return
		}
		wg.Add(1)
		buf := make([]byte, 10)
		copy(buf, dialer.Header)
		binary.BigEndian.PutUint64(buf[1:], conn.(*MessageConn).cid)
		buf[9] = MessageHeadClose
		dialer.Write(buf)
		wg.Wait()
	}
	//test read buffer too small
	{
		conn, err := dialer.Dial("tcp", "test")
		if err != nil {
			t.Error(err)
			return
		}
		go func() {
			for {
				buf := make([]byte, 10)
				_, err := conn.(*MessageConn).rawRead(buf)
				if err != nil {
					break
				}
				t.Error("error recved")
			}
			wg.Done()
		}()
		wg.Add(1)
		fmt.Fprintf(conn, "datadatadatadata-%v\n", 1000)
		wg.Wait()
	}
	//
	//test dial error
	_, err := dialer.Dial("tcp", "error")
	if err == nil {
		t.Error(err)
		return
	}
	//
	//test close all
	conn, err := dialer.Dial("tcp", "test")
	if err != nil {
		t.Error(err)
		return
	}
	go func() {
		for {
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err != nil {
				break
			}
			fmt.Printf("conn recv %v\n", string(buf[:n]))
		}
		wg.Done()
	}()
	wg.Add(2)
	dialer.Close()
	wg.Wait()
	//
	//test error
	err = conn.(*MessageConn).Connect()
	if err == nil || err.Error() != "closed" {
		t.Error(err)
		return
	}
	_, err = conn.Write(nil)
	if err == nil || err.Error() != "closed" {
		t.Error(err)
		return
	}
	err = conn.Close()
	if err == nil || err.Error() != "closed" {
		t.Error(err)
		return
	}
	//
	//test dialer error
	_, err = dialer.Dial("tcp", "test")
	if err == nil || err.Error() != "closed" {
		t.Error("nil")
		return
	}
	_, err = dialer.Write(make([]byte, 1024))
	if err == nil || err.Error() != "closed" {
		t.Error("nil")
		return
	}
	_, err = dialer.Write(make([]byte, 1))
	if err == nil || err.Error() != "invalid data" {
		t.Error("nil")
		return
	}
	dialer2 := NewMessageDialer([]byte("m"), 1024)
	dialer2.Message <- make([]byte, 10240)
	_, err = dialer2.Read(make([]byte, 100))
	if err == nil || !strings.HasPrefix(err.Error(), "buffer is too small") {
		t.Error(err)
		return
	}
	_, err = dialer2.Write(make([]byte, 1024))
	if err == nil || err.Error() != "connection not exist" {
		t.Error(err)
		return
	}
	dialer2.conns[0] = nil
	_, err = dialer2.Write(make([]byte, 1024))
	if err == nil || err.Error() != "unknow command(0)" {
		t.Error(err)
		return
	}
}

func TestMessageDialeEchor(t *testing.T) {
	dialer := NewMessageDialer([]byte("m"), 1024)
	dialer.StartEcho(10240)
	conn, err := dialer.Dial("tcp", "eco")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(conn, "testing-%v", 100)
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil || string(buf[0:n]) != "testing-100" {
		t.Error(err)
		return
	}
	dialer.Close()
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("error")
		return
	}
}
