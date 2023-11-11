package main

import (
	"fmt"
	"net"

	"github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/xio"
	"github.com/codingeasygo/util/xio/frame"
	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/udpgw"
)

func main() {
	core.SetLogLevel(core.LogLevelDebug)
	dns, _ := net.ResolveUDPAddr("udp4", "8.8.4.4:53")
	ss5 := socks.NewServer()
	ss5.BufferSize = 2048
	ss5.Dialer = xio.PiperDialerF(func(uri string, bufferSize int) (raw xio.Piper, err error) {
		fmt.Printf("dial to %v\n", uri)
		if uri == "tcp://127.0.0.1:10" {
			gw := udpgw.NewUDPGW()
			gw.DNS = dns
			udpgw := frame.NewBasePiper(gw, bufferSize)
			// udpgw.ByteOrder = binary.LittleEndian
			// udpgw.LengthFieldMagic = 0
			// udpgw.LengthFieldLength = 2
			// udpgw.LengthAdjustment = -2
			// udpgw.DataOffset = 2
			raw = xio.NewPrintPiper("UDPGW", udpgw)
		} else {
			raw, err = xio.DialNetPiper(uri, bufferSize)
		}
		return
	})
	ss5.Start(":6100")
	waiter := make(chan int)
	<-waiter
}
