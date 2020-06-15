package tcpip

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"net/http"
	_ "net/http/pprof"

	"github.com/coversocks/gocs/netstack/pcap"
)

func init() {
	logLevel = LogLevelDebug
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}

func TestTCPServerClose(t *testing.T) {
	SequenceGenerater = func() int {
		return 134020434
	}
	rawSender, err := pcap.NewReader("../testdata/test_tcp_server_close.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	stackSender := NewWaitReader(rawSender, time.Millisecond)
	wg := sync.WaitGroup{}
	wg.Add(1)
	l := NewStack(false, nil)
	l.SetWriter(NewPacketPrintRWC(false, "Out", NewWrapRWC(ioutil.Discard), l.Eth))
	l.Handler = AcceperF(func(conn net.Conn, unknow *Packet) {
		go func() {
			err = conn.Close()
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}()
	})
	in := NewPacketPrintRWC(true, "In", NewWrapRWC(stackSender), l.Eth)
	err = l.ProcessReader(in)
	fmt.Printf("proc stop by %v\n", err)
	wg.Wait()
	rawSender.Close()
	SequenceGenerater = rand.Int
}

func TestTCPClientClose(t *testing.T) {
	SequenceGenerater = func() int {
		return 134020434
	}
	rawSender, err := pcap.NewReader("../testdata/test_tcp_client_close.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	stackSender := NewWaitReader(rawSender, 50*time.Millisecond)
	wg := sync.WaitGroup{}
	wg.Add(1)
	l := NewStack(false, nil)
	l.SetWriter(NewPacketPrintRWC(false, "Out", NewWrapRWC(ioutil.Discard), l.Eth))
	l.Handler = AcceperF(func(conn net.Conn, unknow *Packet) {
		go func() {
			buf := make([]byte, 1024)
			for i := 0; i < 100; i++ {
				n, err := conn.Read(buf)
				if i == 0 {
					if n != 5 || err != nil {
						t.Errorf("fail by %v,%v", n, err)
						break
					}
					conn.Write(buf[0:n])
					continue
				}
				if i == 1 {
					if err == nil {
						t.Errorf("fail by %v,%v", n, err)
					}
					break
				}
			}
			wg.Done()
		}()
	})
	in := NewPacketPrintRWC(true, "In", NewWrapRWC(stackSender), l.Eth)
	err = l.ProcessReader(in)
	fmt.Printf("proc stop by %v\n", err)
	wg.Wait()
	rawSender.Close()
}

func TestTCPClientRst(t *testing.T) {
	SequenceGenerater = func() int {
		return 1597969999
	}
	rawSender, err := pcap.NewReader("../testdata/test_tcp_client_rst.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	stackSender := NewWaitReader(rawSender, 100*time.Millisecond)
	wg := sync.WaitGroup{}
	wg.Add(1)
	l := NewStack(false, nil)
	l.SetWriter(NewPacketPrintRWC(false, "Out", NewWrapRWC(ioutil.Discard), l.Eth))
	l.Handler = AcceperF(func(conn net.Conn, unknow *Packet) {
		go func() {
			io.Copy(ioutil.Discard, conn)
			wg.Done()
		}()
	})
	in := NewPacketPrintRWC(true, "In", NewWrapRWC(stackSender), l.Eth)
	err = l.ProcessReader(in)
	fmt.Printf("proc stop by %v\n", err)
	wg.Wait()
	rawSender.Close()
}
func TestUDP(t *testing.T) {
	rawSender, err := pcap.NewReader("../testdata/test_udp.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	stackSender := NewWaitReader(rawSender, 100*time.Millisecond)
	wg := sync.WaitGroup{}
	wg.Add(1)
	l := NewStack(false, nil)
	l.SetWriter(NewPacketPrintRWC(true, "Out", NewWrapRWC(ioutil.Discard), l.Eth))
	l.Handler = AcceperF(func(conn net.Conn, unknow *Packet) {
		go func() {
			buf := make([]byte, 1024)
			for i := 0; i < 100; i++ {
				n, err := conn.Read(buf)
				if i == 0 {
					if n != 16 || err != nil {
						t.Errorf("fail by %v,%v,%v", n, err, string(buf[:n]))
						break
					}
					conn.Write(buf[0:n])
					continue
				}
				if i == 1 {
					if err == nil {
						t.Errorf("fail by %v,%v", n, err)
					}
					break
				}
			}
			wg.Done()
		}()
	})
	in := NewPacketPrintRWC(true, "In", NewWrapRWC(stackSender), l.Eth)
	err = l.ProcessReader(in)
	fmt.Printf("proc stop by %v\n", err)
	wg.Wait()
}

func TestPing(t *testing.T) {
	rawSender, err := pcap.NewReader("../testdata/test_ping.pcap")
	if err != nil {
		t.Error(err)
		return
	}
	stackSender := NewWaitReader(rawSender, 100*time.Millisecond)
	l := NewStack(false, nil)
	l.SetWriter(NewPacketPrintRWC(true, "Out", NewWrapRWC(ioutil.Discard), l.Eth))
	l.Handler = AcceperF(func(conn net.Conn, unknow *Packet) {
		panic("error")
	})
	in := NewPacketPrintRWC(true, "In", NewWrapRWC(stackSender), l.Eth)
	err = l.ProcessReader(in)
	fmt.Printf("proc stop by %v\n", err)
}
