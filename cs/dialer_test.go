package cs

import (
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

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
