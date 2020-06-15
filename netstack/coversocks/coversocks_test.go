package coversocks

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack/pcap"
	"github.com/coversocks/gocs/netstack/tcpip"

	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}

func TestRestart(t *testing.T) {
	for i := 0; i < 3; i++ {
		core.SetLogLevel(core.LogLevelDebug)
		tcpip.SetLogLevel(tcpip.LogLevelDebug)
		tcpip.SequenceGenerater = func() int {
			return 134020434
		}
		rawSender, err := pcap.NewReader("../testdata/test_tcp_client_close.pcap")
		if err != nil {
			t.Error(err)
			return
		}
		stackSender := tcpip.NewWaitReader(rawSender, 10*time.Millisecond)
		res := BootstrapQuque("../../default-client.json", 1500, true, true, "/tmp/dump.pcap")
		if len(res) > 0 {
			t.Error(res)
			return
		}
		netProxy.Stack.Delay = 100 * time.Millisecond
		netMessage.StartEcho(10240)
		waiter := make(chan int, 10240)
		go func() {
			buf := make([]byte, 10240)
			for {
				n := ReadQuque(buf, 0, 10240)
				if n < 0 {
					break
				}
				if n > 0 {
					fmt.Printf("Out write %v to %x\n", n, buf[0:n])
					waiter <- 1
				} else {
					time.Sleep(10 * time.Millisecond)
				}
			}
		}()
		go func() {
			buf := make([]byte, 10240)
			for {
				n, err := stackSender.Read(buf)
				if err != nil {
					netQueue.Close()
					break
				}
				WriteQuque(buf, 0, n)
			}
		}()
		res = Start()
		if len(res) > 0 {
			t.Error(res)
			return
		}
		time.Sleep(100 * time.Millisecond)
		// <-waiter
		res = Stop()
		if len(res) > 0 {
			t.Error(res)
			return
		}
		tcpip.SequenceGenerater = rand.Int
	}
}
