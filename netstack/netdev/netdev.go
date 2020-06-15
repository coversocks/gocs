package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"time"

	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()
}

func verify(buf []byte) bool {
	var hex byte
	for i := 0; i < len(buf)-1; i++ {
		hex += buf[i]
	}
	if hex == buf[len(buf)-1] {
		return true
	}
	panic(fmt.Sprintf("having %v, expect %v", buf[len(buf)-1], hex))
}

func tohash(p []byte) []byte {
	buf := make([]byte, len(p)+1)
	var hex byte
	for _, b := range p {
		hex += b
	}
	copy(buf, p)
	buf[len(p)] = hex
	verify(buf)
	return buf
}

var netOut io.WriteCloser

type netOutWriter struct {
}

func (n *netOutWriter) Write(p []byte) (l int, err error) {
	if netOut != nil {
		// fmt.Printf("writing len:%v\n", len(p))
		// fmt.Printf("Out %v %v\n", len(p), p)
		buf := tohash(p)
		l, err = netOut.Write(buf)
	}
	return
}

func (n *netOutWriter) Close() (err error) {
	return
}

func main() {
	udpDialer := &net.Dialer{
		// LocalAddr: &net.UDPAddr{IP: net.ParseIP("10.211.55.23")},
		Timeout: 5 * time.Second,
	}
	tcpDialer := &net.Dialer{
		// LocalAddr: &net.TCPAddr{IP: net.ParseIP("10.211.55.23")},
		Timeout: 5 * time.Second,
	}
	rawDialer := core.NewNetDialer("114.114.114.114:53")
	rawDialer.UDP = udpDialer
	rawDialer.TCP = tcpDialer
	// connDialer := core.NewPrintDialer(rawDialer)
	proxy := netstack.NewNetProxy("csocks.json", 1500, &netOutWriter{}, core.NewDialerWrapper(rawDialer))
	err := proxy.Bootstrap()
	if err != nil {
		panic(err)
	}
	go proxy.Proc()
	if os.Args[1] == "file" {
		recorded, err := os.OpenFile("data.dat", os.O_RDONLY, os.ModePerm)
		if err != nil {
			panic(err)
		}
		reader := core.NewBaseConn(recorded, 10240)
		go func() {
			buf := make([]byte, 15000)
			for {
				n, err := reader.Read(buf)
				if err != nil {
					break
				}
				// fmt.Printf("read %v,%v\n", n, buf[0:n])
				proxy.DeliverNetworkPacket(buf[0:n])
			}
		}()
		c := make(chan int, 1)
		<-c
	}
	if os.Args[1] == "udp" {
		l, err := net.Listen("tcp", ":8091")
		if err != nil {
			panic(err)
		}
		record, err := os.OpenFile("data.dat", os.O_CREATE|os.O_WRONLY, os.ModePerm)
		if err != nil {
			panic(err)
		}
		go func() {
			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, os.Kill)
			<-c
			record.Close()
			fmt.Printf("recorder is closed\n")
			os.Exit(1)
		}()
		for {
			conn, err := l.Accept()
			if err != nil {
				panic(err)
			}
			fmt.Printf("accept from %v\n", conn.RemoteAddr())
			reader := core.NewBaseConn(&FileDataRecorder{ReadWriteCloser: conn, Out: record}, 10240)
			netOut = reader
			go func() {
				buf := make([]byte, 15000)
				for {
					n, err := reader.Read(buf)
					if err != nil {
						break
					}
					// fmt.Printf("read %v,%v\n", n, buf[0:n])
					proxy.DeliverNetworkPacket(buf[0:n])
				}
			}()
		}
	}
}

type FileDataRecorder struct {
	io.ReadWriteCloser
	Out io.WriteCloser
}

func (r *FileDataRecorder) Read(p []byte) (n int, err error) {
	n, err = r.ReadWriteCloser.Read(p)
	if err == nil {
		r.Out.Write(p[0:n])
	}
	return
}
