package udpgw

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	_ "net/http/pprof"
	"testing"
	"time"

	"github.com/codingeasygo/util/xhttp"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
	"github.com/coversocks/gocs/core"
)

func init() {
	http.HandleFunc("/debug/udpgw", StateH)
	go http.ListenAndServe(":6061", nil)
}

func TestUDPGW(t *testing.T) {
	core.SetLogLevel(core.LogLevelDebug)
	//ipv4
	lv4, _ := net.ListenUDP("udp4", nil)
	addrv4 := lv4.LocalAddr().(*net.UDPAddr)
	datav4 := make([]byte, 1024)
	lenv4 := 0
	binary.BigEndian.PutUint16(datav4[1:], 1)
	lenv4 += 3
	lenv4 += copy(datav4[lenv4:], addrv4.IP)
	binary.BigEndian.PutUint16(datav4[lenv4:], uint16(addrv4.Port))
	lenv4 += 2
	lenv4 += copy(datav4[lenv4:], []byte("abc"))
	go func() {
		buf := make([]byte, 1024)
		for {
			n, from, _ := lv4.ReadFromUDP(buf)
			lv4.WriteToUDP(buf[0:n], from)
		}
	}()
	//ipv6
	lv6, _ := net.ListenUDP("udp6", nil)
	addrv6 := lv6.LocalAddr().(*net.UDPAddr)
	datav6 := make([]byte, 1024)
	lenv6 := 0
	datav6[0] = UDPGW_CLIENT_FLAG_IPV6
	binary.BigEndian.PutUint16(datav6[1:], 2)
	lenv6 += 3
	lenv6 += copy(datav6[lenv6:], addrv6.IP)
	binary.BigEndian.PutUint16(datav6[lenv6:], uint16(addrv6.Port))
	lenv6 += 2
	lenv6 += copy(datav6[lenv6:], []byte("abc"))
	go func() {
		buf := make([]byte, 1024)
		for {
			n, from, _ := lv6.ReadFromUDP(buf)
			lv6.WriteToUDP(buf[0:n], from)
		}
	}()
	a, b, _ := xio.Pipe()
	gw := NewUDPGW()
	gw.DNS = addrv4
	go gw.PipeConn(frame.NewReadWriteCloser(frame.NewDefaultHeader(), b, 1024), "tcp://localhost")
	//
	sender := frame.NewReadWriteCloser(frame.NewDefaultHeader(), a, 1024)
	var back []byte

	//ipv4
	for i := 0; i < 100; i++ {
		binary.BigEndian.PutUint16(datav4[1:], uint16(i))
		sender.Write(datav4[0:lenv4])
		back, _ = sender.ReadFrame()
		if !bytes.Equal(back[4:], datav4[:lenv4]) {
			fmt.Printf("back->%v,%v\n", back[4:], datav4[:lenv4])
			t.Error("error")
			return
		}
	}

	//ipv6
	sender.Write(datav6[0:lenv6])
	back, _ = sender.ReadFrame()
	if !bytes.Equal(back[4:], datav6[:lenv6]) {
		fmt.Printf("back->%v,%v\n", back[4:], datav6[:lenv6])
		t.Error("error")
		return
	}
	//state
	ts := httptest.NewServer(http.HandlerFunc(StateH))
	xhttp.GetText("%v", ts.URL)
	//timeout
	StartTimeout(time.Millisecond, 10*time.Millisecond)
	time.Sleep(100 * time.Millisecond)
	if len(gw.connList) > 0 {
		t.Error("error")
		return
	}
	StopTimeout()

	//dns
	datav4[0] = UDPGW_CLIENT_FLAG_DNS
	binary.BigEndian.PutUint16(datav4[1:], 3)
	sender.Write(datav4[0:lenv4])
	back, _ = sender.ReadFrame()
	if !bytes.Equal(back[4:], datav4[:lenv4]) {
		fmt.Printf("back->%v,%v\n", back[4:], datav4[:lenv4])
		t.Error("error")
		return
	}

	time.Sleep(1 * time.Second)

	//close
	sender.Close()
	gw.Close()

	time.Sleep(100 * time.Millisecond)
}
