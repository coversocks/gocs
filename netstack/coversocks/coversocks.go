package coversocks

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/coversocks/gocs/netstack/dns"
	"github.com/coversocks/gocs/netstack/pcap"
	"github.com/coversocks/gocs/netstack/tcpip"

	"net/http"
	_ "net/http/pprof"

	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack"
)

func init() {
	go func() {
		http.HandleFunc("/debug/state", StateH)
		log.Println(http.ListenAndServe(":6060", nil))
	}()
	runtime.GOMAXPROCS(3)
}

//Hello will start inner debug and do nothing
func Hello() {

}

//StateH is current proxy state
func StateH(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	res := map[string]interface{}{}
	if netProxy == nil {
		res["status"] = "not started"
	} else {
		res["status"] = "ok"
		res["netstack"] = netProxy.State()
		if netMessage != nil {
			res["message"] = netMessage.State()
		}
	}
	data, _ := json.Marshal(res)
	w.Write(data)
}

var netQueue *core.ChannelRWC

//ReadQuque read outbound data from the netstack
func ReadQuque(buffer []byte, offset, length int) (n int) {
	b := buffer[offset : offset+length]
	data, err := netQueue.Pull()
	if err != nil {
		n = -1
		core.WarnLog("ReadQuque pull data fail with %v", err)
		return
	}
	if len(data) > length {
		err := fmt.Errorf("ReadQuque buffer is too small expected %v, but %v", len(data), length)
		core.WarnLog("ReadQuque copy data fail with %v", err)
		n = -2
		return
	}
	if len(data) > 0 {
		n = copy(b, data)
	}
	return
}

//WriteQuque write inbound data to the netstack
func WriteQuque(buffer []byte, offset, length int) (n int) {
	err := netQueue.Push(buffer[offset : offset+length])
	if err != nil {
		n = -1
		core.WarnLog("WriteQuque push data fail with %v", err)
		return
	}
	n = length
	return
}

var netFD int
var netRWC io.ReadWriteCloser
var netProxy *netstack.NetProxy
var netMessage *core.MessageDialer
var netRunning bool
var netLocker = sync.RWMutex{}

//BootstrapFD will bootstrap by config file path, n<0 return is fail, n==0 is success
func BootstrapFD(conf string, mtu, fd int, dump string) (res string) {
	core.InfoLog("fd bootstrap by conf:%v,mtu:%v,fd:%v,dump:%v", conf, mtu, fd, dump)
	netFile := os.NewFile(uintptr(fd), "FD")
	res = Bootstrap(conf, mtu, netFile, dump)
	return
}

//BootstrapQuque will bootstrap by config file path, n<0 return is fail, n==0 is success
func BootstrapQuque(conf string, mtu int, async, retain bool, dump string) (res string) {
	core.InfoLog("queue bootstrap by conf:%v,mtu:%v,dump:%v", conf, mtu, dump)
	netQueue = core.NewChannelRWC(async, retain, 1024)
	res = Bootstrap(conf, mtu, netQueue, dump)
	return
}

//Bootstrap will bootstrap by config file path and mtu and net stream
func Bootstrap(conf string, mtu int, rwc io.ReadWriteCloser, dump string) (res string) {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	targetRWC := rwc
	if len(dump) > 0 {
		var err error
		targetRWC, err = pcap.NewDumper(rwc, dump)
		if err != nil {
			res = err.Error()
			return
		}
	}
	netRWC = targetRWC
	netMessage = core.NewMessageDialer([]byte("m"), 512*1024)
	netProxy = netstack.NewNetProxy(netMessage)
	netProxy.MTU, netProxy.Timeout = mtu, 15*time.Second
	err := netProxy.Bootstrap(conf)
	if err != nil {
		res = err.Error()
		netMessage.Close()
		netMessage = nil
		return
	}
	if len(dump) < 1 && netProxy.ClientConf.LogLevel > core.LogLevelDebug {
		targetRWC = tcpip.NewPacketPrintRWC(true, "Net", rwc, false)
	}
	netProxy.SetWriter(targetRWC)
	core.InfoLog("NetProxy is bootstrap done")
	return
}

//Start the process
func Start() (res string) {
	if netRunning {
		res = "already started"
		return
	}
	netRunning = true
	core.InfoLog("NetProxy is starting")
	netProxy.StartProcessReader(netRWC)
	netProxy.StartProcessTimeout()
	return
}

//Stop the process
func Stop() (res string) {
	netLocker.Lock()
	defer netLocker.Unlock()
	if !netRunning || netProxy == nil {
		res = "not started"
		return
	}
	if netQueue != nil {
		netQueue.Close()
	}
	if netRWC != nil {
		netRWC.Close()
	}
	if netMessage != nil {
		netMessage.Close()
	}
	netProxy.Close()
	netProxy.Wait()
	core.InfoLog("NetProxy is stopped")
	netProxy = nil
	netMessage = nil
	netRWC = nil
	netQueue = nil
	netRunning = false
	return
}

//ProxySet will add proxy setting by key
func ProxySet(key string, proxy bool) (res string) {
	netLocker.RLock()
	defer netLocker.RUnlock()
	if netProxy == nil {
		res = "not started"
		return
	}
	if proxy {
		netProxy.NetProcessor.GFW.Set(key, dns.GfwProxy)
	} else {
		netProxy.NetProcessor.GFW.Set(key, dns.GfwLocal)
	}
	return
}

//ChangeMode will change proxy mode by global/auto
func ChangeMode(mode string) (res string) {
	netLocker.RLock()
	defer netLocker.RUnlock()
	if netProxy == nil {
		res = "not started"
		return
	}
	switch mode {
	case "global":
		netProxy.NetProcessor.GFW.Set("*", dns.GfwProxy)
	case "auto":
		netProxy.NetProcessor.GFW.Set("*", dns.GfwLocal)
	}
	return
}

//ProxyMode will return current proxy mode
func ProxyMode() (mode string) {
	netLocker.RLock()
	defer netLocker.RUnlock()
	if netProxy == nil {
		mode = "auto"
	} else if netProxy.NetProcessor.Get("*") == dns.GfwProxy {
		mode = "global"
	} else {
		mode = "auto"
	}
	return
}

//ReadMessage queue one message
func ReadMessage(b []byte) (n int) {
	netLocker.RLock()
	if netMessage == nil {
		n = -1
		netLocker.RUnlock()
		return
	}
	msg := netMessage
	netLocker.RUnlock()
	n, err := msg.Read(b)
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
	netLocker.RLock()
	if netMessage == nil {
		n = -1
		netLocker.RUnlock()
		return
	}
	msg := netMessage
	netLocker.RUnlock()
	n, err := msg.Write(b)
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

//TestWeb will test web request
func TestWeb(url, digest string) {
	fmt.Printf("testWeb start by url:%v,digest:%v\n", url, digest)
	var raw net.Conn
	client := &http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (conn net.Conn, err error) {
				conn, err = net.Dial(network, addr)
				return
			},
		},
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Printf("testWeb new fail with %v\n", err)
		return
	}
	res, err := client.Do(req)
	if err != nil {
		fmt.Printf("testWeb do fail with %v\n", err)
		return
	}
	defer res.Body.Close()
	h := sha1.New()
	_, err = io.Copy(h, res.Body)
	if err != nil {
		fmt.Printf("testWeb copy fail with %v\n", err)
		return
	}
	v := fmt.Sprintf("%x", h.Sum(nil))
	if v != digest {
		fmt.Printf("testWeb done fail by digest not match %v,%v\n", v, digest)
	} else {
		fmt.Printf("testWeb done success by %v\n", v)
	}
	if raw != nil {
		raw.Close()
	}
}
