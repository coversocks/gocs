package coversocks

import (
	"io"
	"os"

	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack"
)

var netReader *os.File
var netWriter *os.File
var netProxy *netstack.NetProxy
var netMessage *core.MessageDialer

//Bootstrap will bootstrap by config file path, n<0 return is fail, n==0 is success
func Bootstrap(conf string, mtu int) (n int) {
	var err error
	netReader, netWriter, err = os.Pipe()
	if err != nil {
		n = -10
		core.ErrorLog("mobile create pipe fail with %v", err)
		return
	}
	netMessage = core.NewMessageDialer([]byte("m"))
	netProxy = netstack.NewNetProxy(conf, uint32(mtu), netWriter, netMessage)
	err = netProxy.Bootstrap()
	if err != nil {
		n = -10
		core.ErrorLog("mobile bootstrap fail with %v", err)
	}
	return
}

//Start the process
func Start() {
	go netProxy.Proc()
}

//Stop the process
func Stop() {

}

//ReadNet read outbound data from the netstack
func ReadNet(b []byte) (n int) {
	n, err := netReader.Read(b)
	if err == nil {
		return
	}
	core.WarnLog("NetRead read fail with %v", err)
	if err == io.EOF {
		n = -1
	} else {
		n = -99
	}
	return
}

//WriteNet write inbound data to the netstack
func WriteNet(b []byte) (n int) {
	if netProxy.DeliverNetworkPacket(b) {
		n = len(b)
	}
	return
}

//ReadMessage queue one message
func ReadMessage(b []byte) (n int) {
	n, err := netMessage.Read(b)
	if err == nil {
		return
	}
	core.WarnLog("ReadMessage read fail with %v", err)
	if err == io.EOF {
		n = -1
	} else {
		n = -99
	}
	return
}

//WriteMessage done messsage
func WriteMessage(b []byte) (n int) {
	n, err := netMessage.Write(b)
	if err == nil {
		return
	}
	core.WarnLog("ReadMessage write fail with %v", err)
	if err == io.EOF {
		n = -1
	} else {
		n = -99
	}
	return
}
