package netstack

import (
	"io"
	"math/rand"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack/dns"
	"github.com/coversocks/gocs/netstack/tcpip"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

//NetProcessor is core.Processor, it will process dns resolver and split proxy/direct by pac
type NetProcessor struct {
	core.Processor
	*dns.GFW
	Resolver *dns.Processor
	Record   *dns.RecordProcessor
}

//NewNetProcessor will create new NetProcessor
func NewNetProcessor(dnsAsync bool, bufferSize int, proxy, direct core.Processor) (net *NetProcessor) {
	gfw := dns.NewGFW()
	pac := core.NewPACProcessor(proxy, direct)
	record := dns.NewRecordProcessor(pac)
	dns := dns.NewProcessor(dnsAsync, bufferSize, core.NewAyncProcessor(record), gfw.Find)
	port := core.NewPortDistProcessor()
	port.Add("53", dns)
	port.Add("137", direct)
	port.Add("*", pac)
	scheme := core.NewSchemeDistProcessor()
	scheme.Add("tcp", pac)
	scheme.Add("udp", port)
	net = &NetProcessor{Processor: scheme, GFW: gfw, Record: record, Resolver: dns}
	pac.Check = net.pacCheck
	return
}

func (n *NetProcessor) pacCheck(key string) bool {
	domain := n.Record.Value(key)
	if len(domain) > 0 {
		return n.GFW.IsProxy(domain)
	} else {
		return n.GFW.IsProxy(key)
	}
}

//State will return the state info
func (n *NetProcessor) State() (state interface{}) {
	state = map[string]interface{}{
		"recorder": n.Record.State(),
		"resolver": n.Resolver.State(),
	}
	return
}

//NetProxy is struct to process the netstack to proxy server
type NetProxy struct {
	*tcpip.Stack
	*NetProcessor
	Eth        bool
	MTU        int
	Conf       string
	Dialer     core.RawDialer
	Client     *gocs.Client
	ClientConf *gocs.ClientConf
	Timeout    time.Duration
	wg         sync.WaitGroup
	out        io.WriteCloser
	errout     io.WriteCloser
	proc       core.Processor
}

//NewNetProxy will return new NetProxy by configure file path, device mtu, net interface writer
func NewNetProxy(dialer core.RawDialer) (proxy *NetProxy) {
	proxy = &NetProxy{
		MTU:     1500,
		Dialer:  dialer,
		Timeout: time.Minute,
		wg:      sync.WaitGroup{},
	}
	return
}

//Bootstrap will init the netstack and proxy client
func (n *NetProxy) Bootstrap(confFile string) (err error) {
	n.Conf = confFile
	core.InfoLog("NetProxy bootstrap by conf:%v,mtu:%v", n.Conf, n.MTU)
	//
	//proxy processor init
	conf := gocs.ClientConf{Mode: "auto"}
	err = core.ReadJSON(n.Conf, &conf)
	if err != nil {
		core.ErrorLog("Client read configure fail with %v", err)
		return
	}
	core.SetLogLevel(conf.LogLevel)
	tcpip.SetLogLevel(conf.LogLevel)
	core.InfoLog("Client using config from %v", n.Conf)
	workdir, _ := filepath.Abs(filepath.Dir(n.Conf))
	if len(conf.WorkDir) > 0 && filepath.IsAbs(conf.WorkDir) {
		workdir = conf.WorkDir
	} else {
		workdir, _ = filepath.Abs(filepath.Join(workdir, conf.WorkDir))
	}
	client := &gocs.Client{Conf: conf, WorkDir: workdir, ConfPath: confFile}
	rules, err := client.ReadGfwRules()
	if err != nil {
		core.ErrorLog("Client read gfw rules fail with %v", err)
		return
	}
	wsDialer := core.NewWebsocketDialer()
	wsDialer.Dialer = n.Dialer
	err = client.Boostrap(wsDialer)
	if err != nil {
		core.ErrorLog("Client boostrap proxy client fail with %v", err)
		return
	}
	//
	//dns/pac processor init
	rawDialer := core.NewRawDialerWrapper(n.Dialer)
	// direct := core.NewAyncProcessor(core.NewProcConnDialer(rawDialer))
	direct := core.NewProcConnDialer(rawDialer)
	// proxy := core.NewAyncProcessor(client)
	net := NewNetProcessor(false, n.MTU*3, client, direct)
	net.GFW.Set(strings.Join(rules, "\n"), "dns://proxy")
	// net.GFW.Add("*", "dns://proxy")
	proc := core.NewAyncProcessor(net)
	// proc := core.NewAyncProcessor(core.NewProcConnDialer(rawDialer))
	//
	//netstack init
	s := tcpip.NewStack(n.Eth, n)
	s.MTU, s.Timeout = n.MTU, n.Timeout
	//
	//set env
	n.ClientConf, n.Client = &conf, client
	n.NetProcessor, n.proc = net, proc
	n.Stack = s
	return
}

//SetWriter will init the stack out
func (n *NetProxy) SetWriter(out io.WriteCloser) {
	n.out = out
	n.Stack.SetWriter(out)
}

//SetErrWriter will init the writer for error packet.
func (n *NetProxy) SetErrWriter(out io.WriteCloser) {
	n.errout = out
}

//Close close netstatck/client
func (n *NetProxy) Close() (err error) {
	core.InfoLog("NetProxy proxy is closing")
	n.Stack.Close()
	n.Client.Close()
	core.InfoLog("NetProxy proxy is closed")
	return
}

//State will return the state info
func (n *NetProxy) State() (state interface{}) {
	state = map[string]interface{}{
		"state":     n.Stack.State(),
		"processor": n.NetProcessor.State(),
	}
	return
}

//OnAccept will call when stack having income connection
func (n *NetProxy) OnAccept(conn net.Conn) {
	err := n.proc.ProcConn(conn, core.RemoteByAddr(conn.LocalAddr()))
	if err != nil {
		core.WarnLog("NetProxy process connection %v fail with %v", conn, err)
	}
}

//OnUnknow will call when stack having unknow income
func (n *NetProxy) OnUnknow(packet *tcpip.Packet) {
	// core.WarnLog("NetProxy receive unknow packet by %x", packet.Buffer)
}

//OnError will call when stack having process error
func (n *NetProxy) OnError(packet *tcpip.Packet) {
	if n.errout != nil {
		core.InfoLog("NetProxy saving error packet with %v size", len(packet.Buffer))
		n.errout.Write(packet.Buffer)
	}
}
