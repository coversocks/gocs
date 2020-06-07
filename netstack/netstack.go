package netstack

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/dns"
	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/buffer"
	"github.com/google/netstack/tcpip/header"
	"github.com/google/netstack/tcpip/network/arp"
	"github.com/google/netstack/tcpip/network/ipv4"
	"github.com/google/netstack/tcpip/network/ipv6"
	"github.com/google/netstack/tcpip/stack"
	"github.com/google/netstack/tcpip/transport/tcp"
	"github.com/google/netstack/tcpip/transport/udp"
	"github.com/google/netstack/waiter"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

//StringError is Error for interface info
type StringError struct {
	Err interface{}
}

//NewStringError will create new StringError
func NewStringError(err interface{}) *StringError {
	return &StringError{Err: err}
}

func (s *StringError) Error() string {
	return fmt.Sprintf("%v", s.Err)
}

//FullAddress is an wrapper for net.Addr
type FullAddress struct {
	Net  string
	End  tcpip.Endpoint
	Addr *tcpip.FullAddress
}

//NewFullAddress will create new FullAddress
func NewFullAddress(net string, end tcpip.Endpoint, faddr *tcpip.FullAddress) (addr *FullAddress) {
	addr = &FullAddress{
		Net:  net,
		End:  end,
		Addr: faddr,
	}
	return
}

//Network return the tcp/udp
func (f *FullAddress) Network() string {
	return f.Net
}

//String return string info
func (f *FullAddress) String() string {
	return fmt.Sprintf("%v:%v", f.Addr.Addr, f.Addr.Port)
}

type readFrom interface {
	ReadFrom(addr, to *tcpip.FullAddress) (buffer.View, tcpip.ControlMessages, *tcpip.Error)
}

//UDPConn is net.Conn impl for udp connection
type UDPConn struct {
	Retain bool //whether retain writed data
	Local  *tcpip.FullAddress
	Remote *tcpip.FullAddress
	End    tcpip.Endpoint
	lck    sync.RWMutex
	err    error
	recv   chan []byte
	closed func(u *UDPConn)
	latest time.Time
}

//NewUDPConn will create new UDPConn
func NewUDPConn(local, remote *tcpip.FullAddress, end tcpip.Endpoint, retain bool) (conn *UDPConn) {
	conn = &UDPConn{
		Local:  local,
		Remote: remote,
		End:    end,
		Retain: retain,
		lck:    sync.RWMutex{},
		recv:   make(chan []byte, 100),
		latest: time.Now(),
	}
	return
}

func (u *UDPConn) Read(p []byte) (n int, err error) {
	u.latest = time.Now()
	u.lck.RLock()
	if u.err != nil {
		err = u.err
		return
	}
	u.lck.RUnlock()
	data := <-u.recv
	if data == nil {
		err = u.err
		return
	}
	if len(p) < len(data) {
		panic("buffer too small")
	}
	n = copy(p, data)
	return
}

func (u *UDPConn) Write(p []byte) (n int, err error) {
	u.latest = time.Now()
	u.lck.RLock()
	if u.err != nil {
		err = u.err
		return
	}
	u.lck.RUnlock()
	buf := p
	if u.Retain {
		buf = make([]byte, len(p))
		copy(buf, p)
	}
	w, _, e := u.End.Write(tcpip.SlicePayload(buffer.View(buf)), tcpip.WriteOptions{From: u.Local, To: u.Remote})
	if e != nil {
		err = NewStringError(e)
	}
	n = int(w)
	return
}

//Close will close udp connection
func (u *UDPConn) Close() (err error) {
	err = u.closeByError(fmt.Errorf("closed"))
	return
}

func (u *UDPConn) closeByError(e error) (err error) {
	u.lck.Lock()
	if u.err != nil {
		u.lck.Unlock()
		return
	}
	u.err = e
	u.lck.Unlock()
	close(u.recv)
	core.DebugLog("UDPConn connection %v is closed by %v", u, e)
	return
}

// LocalAddr returns the local network address.
func (u *UDPConn) LocalAddr() net.Addr {
	return NewFullAddress("udp", u.End, u.Local)
}

// RemoteAddr returns the remote network address.
func (u *UDPConn) RemoteAddr() net.Addr {
	return NewFullAddress("udp", u.End, u.Remote)
}

// SetDeadline for impl net.Conn, do nothing
func (u *UDPConn) SetDeadline(ti time.Time) error {
	return nil
}

// SetReadDeadline for impl net.Conn, do nothing
func (u *UDPConn) SetReadDeadline(ti time.Time) error {
	return nil
}

// SetWriteDeadline for impl net.Conn, do nothing
func (u *UDPConn) SetWriteDeadline(ti time.Time) error {
	return nil
}

func (u *UDPConn) String() string {
	return fmt.Sprintf("%v:%v <-> %v:%v", u.Local.Addr, u.Local.Port, u.Remote.Addr, u.Remote.Port)
}

//TCPConn is io.ReadWriteClose impl for netstack connection
type TCPConn struct {
	Retain bool //whether retain writed data
	Waiter *waiter.Queue
	End    tcpip.Endpoint
	Local  *tcpip.FullAddress
	Remote *tcpip.FullAddress
	entry  *waiter.Entry
	notify chan struct{}
}

//NewTCPConn will create new TCPConn
func NewTCPConn(wq *waiter.Queue, end tcpip.Endpoint, retain bool) (conn *TCPConn) {
	waitEntry, notifyCh := waiter.NewChannelEntry(nil)
	wq.EventRegister(&waitEntry, waiter.EventIn)
	local, _ := end.GetLocalAddress()
	remote, _ := end.GetRemoteAddress()
	conn = &TCPConn{
		Waiter: wq,
		End:    end,
		Retain: retain,
		Local:  &local,
		Remote: &remote,
		entry:  &waitEntry,
		notify: notifyCh,
	}
	return
}

func (t *TCPConn) Read(p []byte) (n int, err error) {
	for {
		data, _, rerr := t.End.Read(nil)
		if rerr != nil {
			if rerr == tcpip.ErrWouldBlock {
				<-t.notify
				continue
			}
			err = NewStringError(rerr)
			break
		}
		if len(p) < len(data) {
			panic("buffer too small")
		}
		n = copy(p, data)
		break
	}
	return
}

func (t *TCPConn) Write(p []byte) (n int, err error) {
	buf := p
	if t.Retain {
		buf = make([]byte, len(p))
		copy(buf, p)
	}
	w, _, e := t.End.Write(tcpip.SlicePayload(buffer.View(buf)), tcpip.WriteOptions{})
	if e != nil {
		err = NewStringError(e)
	}
	n = int(w)
	return
}

//Close will close one tcp connection
func (t *TCPConn) Close() (err error) {
	t.Waiter.EventUnregister(t.entry)
	t.End.Close()
	return
}

// LocalAddr returns the local network address.
func (t *TCPConn) LocalAddr() net.Addr {
	local, _ := t.End.GetLocalAddress()
	return NewFullAddress("tcp", t.End, &local)
}

// RemoteAddr returns the remote network address.
func (t *TCPConn) RemoteAddr() net.Addr {
	remote, _ := t.End.GetRemoteAddress()
	return NewFullAddress("tcp", t.End, &remote)
}

// SetDeadline for impl net.Conn, do nothing
func (t *TCPConn) SetDeadline(ti time.Time) error {
	return nil
}

// SetReadDeadline for impl net.Conn, do nothing
func (t *TCPConn) SetReadDeadline(ti time.Time) error {
	return nil
}

// SetWriteDeadline for impl net.Conn, do nothing
func (t *TCPConn) SetWriteDeadline(ti time.Time) error {
	return nil
}

func (t *TCPConn) String() string {
	return fmt.Sprintf("%v:%v <-> %v:%v", t.Local.Addr, t.Local.Port, t.Remote.Addr, t.Remote.Port)
}

//Listener is a netstack tcp listener
type Listener struct {
	net     string
	Retain  bool //whether retain writed data
	Waiter  *waiter.Queue
	End     tcpip.Endpoint
	entry   *waiter.Entry
	notify  chan struct{}
	udps    map[string]*UDPConn
	lck     sync.RWMutex
	running bool
	Next    core.Processor
	Timeout time.Duration
}

//NewListener will create new Listener
func NewListener(net string, wq *waiter.Queue, end tcpip.Endpoint, retain bool, next core.Processor) (l *Listener) {
	waitEntry, notifyCh := waiter.NewChannelEntry(nil)
	wq.EventRegister(&waitEntry, waiter.EventIn)
	l = &Listener{
		net:     net,
		Waiter:  wq,
		End:     end,
		Retain:  retain,
		entry:   &waitEntry,
		notify:  notifyCh,
		udps:    map[string]*UDPConn{},
		lck:     sync.RWMutex{},
		Next:    next,
		Timeout: 15 * time.Second,
	}
	return
}

func (l *Listener) remoteUDPSession(u *UDPConn) {
	key := fmt.Sprintf("%v:%v-%v:%v", u.Local.Addr, u.Local.Port, u.Remote.Addr, u.Remote.Port)
	l.lck.Lock()
	delete(l.udps, key)
	l.lck.Unlock()
}

//Bind will bind the address
func (l *Listener) Bind(addr *tcpip.FullAddress) (err error) {
	nerr := l.End.Bind(*addr)
	if nerr != nil {
		err = NewStringError(nerr)
	}
	return
}

//Listen will listen the address
func (l *Listener) Listen(backlog int) (err error) {
	nerr := l.End.Listen(backlog)
	if nerr != nil {
		err = NewStringError(nerr)
	}
	return
}

//Accept will accept the tcp connection and return the ne connection
func (l *Listener) Accept() (conn net.Conn, err error) {
	if l.net == "udp" {
		conn, err = l.udpAccept()
	} else {
		conn, err = l.tcpAccept()
	}
	return
}

func (l *Listener) udpAccept() (conn net.Conn, err error) {
	end := l.End.(readFrom)
	for {
		var addr, to tcpip.FullAddress
		data, _, rerr := end.ReadFrom(&addr, &to)
		if rerr != nil {
			if rerr == tcpip.ErrWouldBlock {
				<-l.notify
				continue
			}
			err = NewStringError(rerr)
			break
		}
		key := fmt.Sprintf("%v:%v-%v:%v", addr.Addr, addr.Port, to.Addr, to.Port)
		l.lck.Lock()
		session, ok := l.udps[key]
		if !ok {
			session = NewUDPConn(&to, &addr, l.End, l.Retain)
			session.closed = l.remoteUDPSession
			l.udps[key] = session
			conn = session
		}
		l.lck.Unlock()
		session.recv <- data
		if conn != nil { //found one new session
			core.DebugLog("Listener accept on %v connection %v", l.net, conn)
			break
		}
	}
	return
}

func (l *Listener) tcpAccept() (conn net.Conn, err error) {
	for {
		end, wq, nerr := l.End.Accept()
		if nerr != nil {
			if nerr == tcpip.ErrWouldBlock {
				<-l.notify
				continue
			}
			err = NewStringError(nerr)
		} else {
			conn = NewTCPConn(wq, end, l.Retain)
			core.DebugLog("Listener accept on %v connection %v", l.net, conn)
		}
		break
	}
	return
}

func (l *Listener) TimeoutConnection() (err error) {
	if l.net != "udp" {
		return
	}
	l.lck.RLock()
	defer l.lck.RUnlock()
	now := time.Now()
	for _, udp := range l.udps {
		if now.Sub(udp.latest) > l.Timeout {
			udp.closeByError(fmt.Errorf("time out"))
		}
	}
	return
}

//Close will close the acceptor
func (l *Listener) Close() (err error) {
	l.running = false
	l.Waiter.EventUnregister(l.entry)
	l.End.Close()
	return
}

//Addr will return the listner local address
func (l *Listener) Addr() net.Addr {
	local, _ := l.End.GetLocalAddress()
	return NewFullAddress(l.net, l.End, &local)
}

//LoopProc will accept connection by base listener and do ProcConn by connection local address
func (l *Listener) LoopProc() (err error) {
	core.InfoLog("Listener start loop proc net:%v,address:%v,retain:%v,next:%v", l.net, l.Addr(), l.Retain, l.Next)
	l.running = true
	wg := sync.WaitGroup{}
	if l.net == "udp" {
		wg.Add(1)
		go func() {
			for l.running {
				l.TimeoutConnection()
				time.Sleep(3 * time.Second)
			}
			wg.Done()
		}()
	}
	var conn net.Conn
	for {
		conn, err = l.Accept()
		if err != nil {
			break
		}
		perr := l.Next.ProcConn(conn, l.net+"://"+conn.LocalAddr().String())
		if perr != nil {
			core.DebugLog("ListenerProcessor proc connection(%v) fail with %v", conn, perr)
		}
	}
	wg.Wait()
	return
}

//NetProcessor is core.Processor, it will process dns resolver and split proxy/direct by pac
type NetProcessor struct {
	core.Processor
	*dns.GFW
	Record *dns.RecordProcessor
}

//NewNetProcessor will create new NetProcessor
func NewNetProcessor(proxy, direct core.Processor) (net *NetProcessor) {
	gfw := dns.NewGFW()
	pac := core.NewPACProcessor(proxy, direct)
	record := dns.NewRecordProcessor(pac)
	dns := dns.NewProcessor(record, gfw.Find)
	port := core.NewPortDistProcessor()
	port.Add("53", dns)
	port.Add("*", pac)
	scheme := core.NewSchemeDistProcessor()
	scheme.Add("tcp", pac)
	scheme.Add("udp", port)
	net = &NetProcessor{Processor: scheme, GFW: gfw, Record: record}
	pac.Check = net.pacCheck
	return
}

func (n *NetProcessor) pacCheck(key string) bool {
	return n.GFW.IsProxy(n.Record.Value(key))
}

//Stack is netstack to init netstack and process listener
type Stack struct {
	*stack.Stack
	Retain  bool
	Address tcpip.Address
	Protoco tcpip.NetworkProtocolNumber
	Link    stack.LinkEndpoint
	Waiter  *waiter.Queue
	Next    core.Processor
}

//NewStack will create new Stack
func NewStack(retain bool, next core.Processor) (s *Stack) {
	s = &Stack{
		Address: header.IPv4Any,
		Protoco: ipv4.ProtocolNumber,
		Waiter:  &waiter.Queue{},
		Retain:  retain,
		Next:    next,
	}
	return
}

//Bootstrap will start the net stack
func (s *Stack) Bootstrap(linkEP stack.LinkEndpoint) (err error) {
	core.InfoLog("Stack bootstrap by addres:%v,protoco:%v,retain:%v,next:%v", s.Address, s.Protoco, s.Retain, s.Next)
	// Create the stack with ip and tcp protocols, then add a tun-based
	// NIC and address.
	raw := stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocol{ipv4.NewProtocol(), ipv6.NewProtocol(), arp.NewProtocol()},
		TransportProtocols: []stack.TransportProtocol{tcp.NewProtocol(), udp.NewProtocol()},
	})
	nerr := raw.CreateNIC(1, linkEP)
	if nerr != nil {
		err = NewStringError(nerr)
		return
	}
	nerr = raw.AddAddress(1, s.Protoco, s.Address)
	if nerr != nil {
		err = NewStringError(nerr)
		return
	}
	nerr = raw.AddAddress(1, arp.ProtocolNumber, arp.ProtocolAddress)
	if nerr != nil {
		err = NewStringError(nerr)
		return
	}
	subnet, err := tcpip.NewSubnet(
		tcpip.Address(strings.Repeat("\x00", len(s.Address))),
		tcpip.AddressMask(strings.Repeat("\x00", len(s.Address))),
	)
	if err != nil {
		return
	}
	// Add default route.
	raw.SetRouteTable([]tcpip.Route{
		{
			Destination: subnet,
			NIC:         1,
		},
	})
	s.Link = linkEP
	s.Stack = raw
	return
}

//CreateListener will create the listener by stack
func (s *Stack) CreateListener(net string) (l *Listener, err error) {
	var protoco tcpip.TransportProtocolNumber
	if net == "tcp" {
		protoco = tcp.ProtocolNumber
	} else if net == "udp" {
		protoco = udp.ProtocolNumber
	} else {
		err = fmt.Errorf("not supported net %v", net)
		return
	}
	end, nerr := s.NewEndpoint(protoco, s.Protoco, s.Waiter)
	if nerr != nil {
		err = NewStringError(nerr)
		return
	}
	l = NewListener(net, s.Waiter, end, s.Retain, s.Next)
	core.InfoLog("Stack create listener by net:%v,retain:%v,next:%v", net, s.Retain, s.Next)
	return
}

//NetProxy is struct to process the netstack to proxy server
type NetProxy struct {
	Conf       string
	MTU        uint32
	Netif      io.WriteCloser
	Dialer     core.RawDialer
	Stack      *Stack
	Link       *OutEndpoint
	Client     *gocs.Client
	ClientConf *gocs.ClientConf
	Processor  *NetProcessor
	TCP        *Listener
	UDP        *Listener
	wg         sync.WaitGroup
}

//NewNetProxy will return new NetProxy by configure file path, device mtu, net interface writer
func NewNetProxy(conf string, mtu uint32, netif io.WriteCloser, dialer core.RawDialer) (proxy *NetProxy) {
	proxy = &NetProxy{
		Conf:   conf,
		MTU:    mtu,
		Netif:  netif,
		Dialer: dialer,
		wg:     sync.WaitGroup{},
	}
	return
}

//Bootstrap will init the netstack and proxy client
func (n *NetProxy) Bootstrap() (err error) {
	core.InfoLog("NetProxy bootstrap by conf:%v,mtu:%v,netif:%v", n.Conf, n.MTU, n.Netif)
	//
	//proxy processor init
	conf := gocs.ClientConf{Mode: "auto"}
	err = core.ReadJSON(n.Conf, &conf)
	if err != nil {
		core.ErrorLog("Client read configure fail with %v", err)
		return
	}
	core.SetLogLevel(conf.LogLevel)
	core.InfoLog("Client using config from %v", n.Conf)
	workdir, _ := filepath.Abs(filepath.Dir(n.Conf))
	if len(conf.WorkDir) > 0 && filepath.IsAbs(conf.WorkDir) {
		workdir = conf.WorkDir
	} else {
		workdir, _ = filepath.Abs(filepath.Join(workdir, conf.WorkDir))
	}
	client := &gocs.Client{Conf: conf, WorkDir: workdir}
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
	direct := core.NewAyncProcessor(core.NewProcConnDialer(rawDialer))
	proxy := core.NewAyncProcessor(client)
	processor := NewNetProcessor(proxy, direct)
	processor.GFW.Add(strings.Join(rules, "\n"), "dns://proxy")
	//
	//netstack init
	s := NewStack(true, processor)
	linkEP, err := NewOutEndpoint(&OutOptions{
		MTU:            n.MTU,
		EthernetHeader: false,
		Address:        "aa:00:01:01:01:01",
		Out:            n.Netif,
	})
	if err != nil {
		core.ErrorLog("Client create link endpoint fail with %v", err)
		client.Close()
		return
	}
	err = s.Bootstrap(linkEP)
	if err != nil {
		core.ErrorLog("Client create link endpoint fail with %v", err)
		client.Close()
		return
	}
	//
	ltcp, err := s.CreateListener("tcp")
	if err == nil {
		err = ltcp.Bind(&tcpip.FullAddress{})
		if err == nil {
			err = ltcp.Listen(10)
		}
	}
	if err != nil {
		core.ErrorLog("Client create tcp listener fail with %v", err)
		client.Close()
		s.Close()
		return
	}
	ludp, err := s.CreateListener("udp")
	if err == nil {
		err = ludp.Bind(&tcpip.FullAddress{})
	}
	if err != nil {
		core.ErrorLog("Client create tcp listener fail with %v", err)
		client.Close()
		s.Close()
		return
	}
	n.ClientConf, n.Client = &conf, client
	n.Processor = processor
	n.Stack, n.Link = s, linkEP
	n.TCP, n.UDP = ltcp, ludp
	return
}

//DeliverNetworkPacket will deliver network package to dispatcher
func (n *NetProxy) DeliverNetworkPacket(buf []byte) bool {
	return n.Link.DeliverNetworkPacket(buf)
}

//Proc will run the tcp/udp listener
func (n *NetProxy) Proc() {
	core.InfoLog("NetProxy process is starting")
	n.wg.Add(2)
	go func() {
		err := n.TCP.LoopProc()
		core.InfoLog("NetProxy tcp loop proc is stopped by %v", err)
		n.wg.Done()
	}()
	err := n.UDP.LoopProc()
	core.InfoLog("NetProxy udp loop proc is stopped by %v", err)
	n.wg.Done()
	core.InfoLog("NetProxy process is stopped")
}

//Close close netstatck/client
func (n *NetProxy) Close() {
	core.InfoLog("NetProxy proxy is closing")
	n.TCP.Close()
	n.UDP.Close()
	n.Stack.Close()
	n.Client.Close()
	n.wg.Wait()
	core.InfoLog("NetProxy proxy is closed")
}
