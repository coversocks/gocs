package main

import (
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/net/websocket"
)

func TestParseListenAddr(t *testing.T) {
	addrs, err := parseListenAddr("xxx:1")
	if err != nil || len(addrs) != 1 {
		t.Error(err)
		return
	}
	addrs, err = parseListenAddr("xxx:1-2")
	if err != nil || len(addrs) != 2 {
		t.Error(err)
		return
	}
	//
	//test error
	_, err = parseListenAddr("xxx")
	if err == nil {
		t.Error(err)
		return
	}
	_, err = parseListenAddr("xxx:x")
	if err == nil {
		t.Error(err)
		return
	}
	_, err = parseListenAddr("xxx:1-x")
	if err == nil {
		t.Error(err)
		return
	}
}

func TestClientConf(t *testing.T) {
	buf := make([]byte, 1024)
	ts := httptest.NewServer(websocket.Handler(func(ws *websocket.Conn) {
		io.Copy(ws, ws)
	}))
	conf := &ClientConf{
		Servers: []*ClientServerConf{
			&ClientServerConf{
				Enable:   false,
				Address:  []string{"wss://127.0.0.1:x"},
				Username: "x",
				Password: "x",
			},
			&ClientServerConf{
				Enable:   false,
				Address:  []string{"ws://" + strings.TrimPrefix(ts.URL, "http://") + "?a=1"},
				Username: "x",
				Password: "x",
			},
			&ClientServerConf{
				Enable:   false,
				Address:  []string{"ws://" + strings.TrimPrefix(ts.URL, "http://")},
				Username: "x",
				Password: "x",
			},
		},
	}
	conf.Boostrap()
	//not server
	_, err := conf.Dialer.Dial("")
	if err == nil {
		t.Error(err)
		return
	}
	//err server
	conf.Servers[0].Enable = true
	_, err = conf.Dialer.Dial("")
	if err == nil {
		t.Error(err)
		return
	}
	conf.Servers[0].Enable = false
	//having argument
	conf.Servers[1].Enable = true
	conf.Boostrap()
	raw, err := conf.Dialer.Dial("")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(raw, "%v", "abc")
	readed, _ := raw.Read(buf)
	fmt.Println(buf[:readed])
	raw.Close()
	conf.Servers[1].Enable = false
	//not argument
	conf.Servers[2].Enable = true
	conf.Boostrap()
	raw, err = conf.Dialer.Dial("")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(raw, "%v", "abc")
	readed, _ = raw.Read(buf)
	fmt.Println(buf[:readed])
	raw.Close()
	conf.Servers[2].Enable = false
}
