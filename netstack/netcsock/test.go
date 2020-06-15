package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack"
	"github.com/google/netstack/tcpip/link/rawfile"
	"github.com/google/netstack/tcpip/link/tun"

	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()
}

//FD is file descriptor read/write/close
type FD int

func (f FD) Write(p []byte) (n int, err error) {
	n = len(p)
	nerr := rawfile.NonBlockingWrite(int(f), p)
	if nerr != nil {
		err = netstack.NewStringError(nerr)
	}
	return
}

func (f FD) Read(p []byte) (n int, err error) {
	n, nerr := rawfile.BlockingRead(int(f), p)
	if nerr != nil {
		err = netstack.NewStringError(nerr)
	}
	return
}

//Close will close file descriptor
func (f FD) Close() (err error) {
	return
}

//EchoDialer is dialer test
type EchoDialer struct {
}

//Dial dail one raw connection
func (e *EchoDialer) Dial(network, address string) (c net.Conn, err error) {
	if address == "114.114.114.114:53" {
		c, err = net.Dial(network, address)
	} else {
		c, err = core.NewEchoConn()
	}
	return
}

func main() {
	core.SetLogLevel(core.LogLevelDebug)
	tunName := os.Args[1]
	mtu, err := rawfile.GetMTU(tunName)
	if err != nil {
		panic(err)
	}

	fmt.Printf("mtu--->%v\n", mtu)

	fd, err := tun.Open(tunName)
	if err != nil {
		panic(err)
	}
	if err := syscall.SetNonblock(fd, true); err != nil {
		panic(err)
	}
	netif := FD(fd)
	udpDialer := &net.Dialer{
		LocalAddr: &net.UDPAddr{IP: net.ParseIP("10.211.55.23")},
		Timeout:   5 * time.Second,
	}
	tcpDialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{IP: net.ParseIP("10.211.55.23")},
		Timeout:   5 * time.Second,
	}
	rawDialer := core.NewNetDialer("114.114.114.114:53")
	rawDialer.UDP = udpDialer
	rawDialer.TCP = tcpDialer
	proxy := netstack.NewNetProxy("csocks.json", mtu, netif, core.NewDialerWrapper(rawDialer))
	err = proxy.Bootstrap()
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			buf := make([]byte, proxy.MTU-100)
			n, err := rawfile.BlockingRead(fd, buf)
			if err != nil {
				break
			}
			proxy.DeliverNetworkPacket(buf[:n])
		}
	}()
	// proxy.Processor.GFW.Add("||google.com", "dns://proxy")
	proxy.Proc()
}
