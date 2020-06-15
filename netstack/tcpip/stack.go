package tcpip

import (
	"fmt"
	"io"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/google/netstack/tcpip/header"
)

//DroppedError is netstack packet dropped error
type DroppedError struct {
	Message string
	Type    string
}

//NewDroppedError will return new error by type and message
func NewDroppedError(t, format string, args ...interface{}) (err *DroppedError) {
	err = &DroppedError{Type: t, Message: fmt.Sprintf(format, args...)}
	return
}

func (d *DroppedError) Error() string {
	return d.String()
}

func (d *DroppedError) String() string {
	return fmt.Sprintf("%v:%v", d.Type, d.Message)
}

const (
	//ConnStatusCreated is the connection status after created
	ConnStatusCreated = 0
	//ConnStatusConnecting is the connection status when connecting
	ConnStatusConnecting = 100
	//ConnStatusConnected is the connection status after connected
	ConnStatusConnected = 200
	//ConnStatusClosing is the connection status when connecting
	ConnStatusClosing = 300
	//ConnStatusClosed is the connection status after closed
	ConnStatusClosed = 400
)

func statusInfo(status int) string {
	switch status {
	case ConnStatusCreated:
		return "created"
	case ConnStatusConnecting:
		return "connecting"
	case ConnStatusConnected:
		return "connected"
	case ConnStatusClosing:
		return "closing"
	case ConnStatusClosed:
		return "closed"
	default:
		return "unknow"
	}
}

//SequenceGenerater is the tcp sequence number generater
var SequenceGenerater = rand.Int

//Conn is net.Conn impl for udp connection
type Conn struct {
	*Packet     //accept packet
	Key         string
	err         error
	latest      time.Time
	status      int
	stack       *Stack
	receiver    chan []byte
	closeLocker sync.RWMutex
	mss         int

	//tcp
	acked  uint32
	recved uint32
	sended uint32
}

//NewConn will create new Conn
func NewConn(stack *Stack, key string, packet *Packet) (conn *Conn) {
	conn = &Conn{
		Key:         key,
		receiver:    make(chan []byte, 10240),
		closeLocker: sync.RWMutex{},
		latest:      time.Now(),
		stack:       stack,
		Packet:      packet.Clone(),
		mss:         stack.MTU - 100,
	}
	return
}

func (c *Conn) Read(p []byte) (n int, err error) {
	c.closeLocker.RLock()
	if c.status > ConnStatusConnected {
		err = fmt.Errorf(statusInfo(c.status))
		c.closeLocker.RUnlock()
		return
	}
	c.closeLocker.RUnlock()
	c.latest = time.Now()
	data := <-c.receiver
	if data == nil {
		err = c.err
		return
	}
	if len(p) < len(data) {
		panic("buffer too small")
	}
	n = copy(p, data)
	return
}

func (c *Conn) Write(p []byte) (n int, err error) {
	w := 0
	l := len(p)
	for n < l {
		if l-n > c.mss {
			w, err = c.rawWrite(p[n : n+c.mss])
		} else {
			w, err = c.rawWrite(p[n:l])
		}
		if err != nil {
			break
		}
		n += w
	}
	return
}

func (c *Conn) rawWrite(p []byte) (n int, err error) {
	c.closeLocker.RLock()
	if c.status != ConnStatusConnected {
		err = fmt.Errorf(statusInfo(c.status))
		c.closeLocker.RUnlock()
		return
	}
	c.closeLocker.RUnlock()
	c.latest = time.Now()
	if len(p) > c.mss {
		err = fmt.Errorf("data is too large to write expect <%v, but %v", c.mss, len(p))
		return
	}
	pk, err := Swap(c.Packet, 0, len(p))
	if err != nil {
		return
	}
	n = copy(pk.Data, p)
	switch pk.Proto {
	case header.TCPProtocolNumber:
		pk.TCP.Encode(&header.TCPFields{
			SrcPort:    c.Packet.TCP.DestinationPort(),
			DstPort:    c.Packet.TCP.SourcePort(),
			SeqNum:     c.sended,
			AckNum:     c.recved,
			DataOffset: header.TCPMinimumSize,
			Flags:      header.TCPFlagPsh | header.TCPFlagAck,
			WindowSize: c.Packet.TCP.WindowSize(),
		})
		c.sended += uint32(len(pk.Data))
		if c.sended-c.acked >= uint32(c.Packet.TCP.WindowSize()) {
			time.Sleep(10 * time.Microsecond)
		}
	case header.UDPProtocolNumber:
		pk.UDP.Encode(&header.UDPFields{
			SrcPort: c.Packet.UDP.DestinationPort(),
			DstPort: c.Packet.UDP.SourcePort(),
			Length:  uint16(header.UDPMinimumSize + len(pk.Data)),
		})
	default:
		panic("not supported")
	}
	pk.UpdateChecksum()
	err = c.stack.sendPacket(pk)
	time.Sleep(time.Microsecond)
	return
}

//Close will close udp connection
func (c *Conn) Close() (err error) {
	DebugLog("Conn connection %v is closing by local", c)
	err = c.closeStart()
	return
}

func (c *Conn) closeStart() (err error) {
	c.closeLocker.RLock()
	if c.status > ConnStatusConnected {
		err = fmt.Errorf(statusInfo(c.status))
		c.closeLocker.RUnlock()
		return
	}
	switch c.Proto {
	case header.TCPProtocolNumber:
		c.status = ConnStatusClosing
		c.closeLocker.RUnlock()
		err = c.procFinAck(c.Packet)
	case header.UDPProtocolNumber:
		c.status = ConnStatusConnected
		c.closeLocker.RUnlock()
		c.stack.removeConn(c)
		c.closeFinised()
	}
	return
}

func (c *Conn) closeFinised() {
	c.closeLocker.Lock()
	if c.err == nil {
		c.err = fmt.Errorf("closed")
	}
	c.status = ConnStatusClosed
	close(c.receiver)
	c.closeLocker.Unlock()
	DebugLog("Conn connection %v close is finished", c)
}

func (c *Conn) String() string {
	net := ""
	if c.Proto == header.TCPProtocolNumber {
		net = "tcp"
	} else {
		net = "udp"
	}
	return fmt.Sprintf("%v %v <-> %v", net, c.LocalAddr(), c.RemoteAddr())
}

func (c *Conn) procSync(received *Packet) (err error) {
	synOptions := header.ParseSynOptions(received.TCP.Options(), false)
	if synOptions.MSS > 0 {
		c.mss = int(synOptions.MSS)
	}
	c.latest = time.Now()
	opts := EncodeSynOptions(&header.TCPSynOptions{
		MSS: uint16(c.stack.MTU - 100),
	})
	pk, err := Swap(c.Packet, len(opts), 0)
	if err != nil {
		return
	}
	// options := header.ParseSynOptions(received.TCP.Options(), false)
	c.sended = uint32(SequenceGenerater())
	c.status = ConnStatusConnecting
	c.recved = received.TCP.SequenceNumber() + 1
	pk.TCP.Encode(&header.TCPFields{
		SrcPort:    c.Packet.TCP.DestinationPort(),
		DstPort:    c.Packet.TCP.SourcePort(),
		SeqNum:     c.sended,
		AckNum:     c.recved,
		DataOffset: header.TCPMinimumSize + uint8(len(opts)),
		Flags:      header.TCPFlagSyn | header.TCPFlagAck,
		WindowSize: c.Packet.TCP.WindowSize(),
	})
	copy(pk.TCP.Options(), opts)
	c.sended++
	pk.UpdateChecksum()
	err = c.stack.sendPacket(pk)
	DebugLog("Conn connection %v is accepting", c)
	return
}

func (c *Conn) procReset(received *Packet) (err error) {
	err = c.stack.procReset(c.Packet, c.sended, c.recved)
	c.stack.removeConn(c)
	c.closeFinised()
	return
}

func (c *Conn) procKeepAlive(received *Packet) (err error) {
	pk, err := Swap(c.Packet, 0, 0)
	if err != nil {
		return
	}
	pk.TCP.Encode(&header.TCPFields{
		SrcPort:    c.Packet.TCP.DestinationPort(),
		DstPort:    c.Packet.TCP.SourcePort(),
		SeqNum:     c.sended - 1,
		AckNum:     c.recved,
		DataOffset: header.TCPMinimumSize,
		Flags:      header.TCPFlagAck,
		WindowSize: c.Packet.TCP.WindowSize(),
	})
	pk.UpdateChecksum()
	err = c.stack.sendPacket(pk)
	return
}

func (c *Conn) procReceive(received *Packet) (err error) {
	c.latest = time.Now()
	seq := received.TCP.SequenceNumber()
	ack := received.TCP.AckNumber()
	data := received.TCP.Payload()
	if c.status == ConnStatusConnecting {
		if seq != c.recved || ack != c.sended {
			WarnLog("conn connection %v will reset with seq/ack not correct, seq:%v!=%v, ack:%v!=%v", c, seq, c.recved, ack, c.sended)
			err = c.procReset(received)
			return
		}
		c.status = ConnStatusConnected
		DebugLog("Conn connection %v is connected", c)
		c.stack.Handler.OnAccept(c)
	}
	if ack > c.sended {
		WarnLog("conn connection %v will reset with seq/ack not correct, seq:%v!=%v, ack:%v!=%v", c, seq, c.recved, ack, c.sended)
		err = c.procReset(received)
		return
	}
	if ack > 0 {
		c.acked = ack
	}
	// DebugLog("Conn connection %v receive seq:%v/%v,ack:%v/%v,data:%v", c, seq, c.recved, ack, c.sended, len(data))
	if seq < c.recved {
		if c.recved-seq == 1 {
			err = c.procKeepAlive(received)
			return
		}
		//already received
		err = NewDroppedError("dropped", "already received seq:%v/%v", seq, c.recved)
		DebugLog("Conn connection %v having already received seq:%v/%v,ack:%v/%v,data:%v", c, seq, c.recved, ack, c.sended, len(data))
		return
	}
	if len(data) < 1 {
		return
	}
	buf := make([]byte, len(data))
	copy(buf, data)
	c.receiver <- buf
	c.procAck(received, uint32(len(data)))
	return
}

func (c *Conn) procAck(received *Packet, added uint32) (err error) {
	c.latest = time.Now()
	pk, err := Swap(c.Packet, 0, 0)
	if err != nil {
		return
	}
	c.recved += added
	pk.TCP.Encode(&header.TCPFields{
		SrcPort:    c.Packet.TCP.DestinationPort(),
		DstPort:    c.Packet.TCP.SourcePort(),
		SeqNum:     c.sended,
		AckNum:     c.recved,
		DataOffset: header.TCPMinimumSize,
		Flags:      header.TCPFlagAck,
		WindowSize: c.Packet.TCP.WindowSize(),
	})
	pk.UpdateChecksum()
	err = c.stack.sendPacket(pk)
	return
}

func (c *Conn) procFinAck(received *Packet) (err error) {
	c.latest = time.Now()
	pk, err := Swap(c.Packet, 0, 0)
	if err != nil {
		return
	}
	pk.TCP.Encode(&header.TCPFields{
		SrcPort:    c.Packet.TCP.DestinationPort(),
		DstPort:    c.Packet.TCP.SourcePort(),
		SeqNum:     c.sended,
		AckNum:     c.recved,
		DataOffset: header.TCPMinimumSize,
		Flags:      header.TCPFlagAck | header.TCPFlagFin,
		WindowSize: c.Packet.TCP.WindowSize(),
	})
	c.sended++
	pk.UpdateChecksum()
	err = c.stack.sendPacket(pk)
	return
}

//SetDeadline is impl for net.Conn
func (c *Conn) SetDeadline(t time.Time) error {
	return nil
}

//SetReadDeadline is impl for net.Conn
func (c *Conn) SetReadDeadline(t time.Time) error {
	return nil
}

//SetWriteDeadline is impl for net.Conn
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return nil
}

//Handler is interface to handler the stack action
type Handler interface {
	//OnAccept will call when stack having income connection
	OnAccept(conn net.Conn)
	//OnUnknow will call when stack having unknow income
	OnUnknow(packet *Packet)
	//OnError will call when stack process error
	OnError(packet *Packet)
}

//AcceperF is Handler impl by func
type AcceperF func(conn net.Conn, unknow *Packet)

//OnAccept will call when stack having income connection
func (a AcceperF) OnAccept(conn net.Conn) {
	a(conn, nil)
}

//OnError will call when stack process error
func (a AcceperF) OnError(packet *Packet) {
}

//OnUnknow will call when stack having unknow income
func (a AcceperF) OnUnknow(packet *Packet) {
	a(nil, packet)
}

//Stack is a netstack impl
type Stack struct {
	lck     sync.RWMutex
	udps    map[string]*Conn
	tcps    map[string]*Conn
	running bool
	out     io.WriteCloser
	waiter  sync.WaitGroup
	Handler Handler
	Eth     bool
	MTU     int
	Timeout time.Duration
	Delay   time.Duration
}

//NewStack will create new Stack
func NewStack(eth bool, handler Handler) (s *Stack) {
	s = &Stack{
		MTU:     1500,
		Eth:     eth,
		udps:    map[string]*Conn{},
		tcps:    map[string]*Conn{},
		lck:     sync.RWMutex{},
		waiter:  sync.WaitGroup{},
		Handler: handler,
		Timeout: 15 * time.Second,
		Delay:   3 * time.Second,
	}
	return
}

func (s *Stack) removeConn(c *Conn) {
	s.lck.Lock()
	if c.Proto == header.UDPProtocolNumber {
		delete(s.udps, c.Key)
	} else {
		delete(s.tcps, c.Key)
	}
	s.lck.Unlock()
}

func (s *Stack) sendPacket(packet *Packet) (err error) {
	_, err = s.out.Write(packet.Buffer)
	return
}

//ProcessReader will read input packet from reader and deliver frame to netstack
func (s *Stack) ProcessReader(in io.Reader) (err error) {
	InfoLog("Stack process reader(%v) is starting", in)
	var n int
	buffer := make([]byte, s.MTU)
	s.running = true
	for s.running {
		n, err = in.Read(buffer)
		if err != nil {
			break
		}
		err = s.ProcessBuffer(buffer, 0, n)
		if err != nil {
			WarnLog("Stack process packet fail with %v", err)
			continue
		}
	}
	InfoLog("Stack process reader is done by %v", err)
	s.waiter.Done()
	return
}

//StartProcessReader will start reader processor runner
func (s *Stack) StartProcessReader(in io.Reader) {
	if s.running {
		return
	}
	s.waiter.Add(1)
	go s.ProcessReader(in)
}

//ProcessBuffer will deliver buffer to netstack and send to connection
func (s *Stack) ProcessBuffer(buffer []byte, offset, length int) (err error) {
	packet, err := Wrap(buffer[offset:length], s.Eth)
	if err != nil {
		return
	}
	err = s.ProcessPacket(packet)
	return
}

//ProcessPacket will deliver packet to netstack and send to connection
func (s *Stack) ProcessPacket(packet *Packet) (err error) {
	switch packet.Version {
	case header.IPv4Version:
		err = s.procIPv4(packet)
	case header.IPv6Version:
		fallthrough
	default:
		err = NewDroppedError("dropped", "not suppored ip version %v", packet.Version)
	}
	if err != nil {
		s.Handler.OnError(packet)
	}
	return
}

//ProcessTimeout will close all timeout connection
func (s *Stack) ProcessTimeout() {
	conns := []*Conn{}
	s.lck.Lock()
	now := time.Now()
	for _, c := range s.udps {
		if now.Sub(c.latest) > s.Timeout {
			conns = append(conns, c)
		}
	}
	for _, c := range s.tcps {
		if now.Sub(c.latest) > s.Timeout {
			conns = append(conns, c)
		}
	}
	s.lck.Unlock()
	if len(conns) > 0 {
		DebugLog("Stack process check timeout having %v connection will be closed", len(conns))
	}
	for _, c := range conns {
		DebugLog("Conn connection %v is closing by timeout", c)
		c.closeStart()
	}
}

//StartProcessTimeout will start timeout runner
func (s *Stack) StartProcessTimeout() {
	if s.running {
		return
	}
	s.waiter.Add(1)
	s.running = true
	go func() {
		InfoLog("Statck the timeout processor is starting")
		for s.running {
			s.ProcessTimeout()
			time.Sleep(s.Delay)
		}
		InfoLog("Statck the timeout processor is stopped")
		s.waiter.Done()
	}()
}

//Wait the runner done
func (s *Stack) Wait() {
	s.waiter.Wait()
}

func (s *Stack) procIPv4(packet *Packet) (err error) {
	checksum := packet.IPv4.Checksum()
	packet.IPv4.SetChecksum(0)
	if checksum != ^packet.IPv4.CalculateChecksum() {
		err = fmt.Errorf("checksum fail")
		return
	}
	packet.IPv4.SetChecksum(checksum)
	switch packet.IPv4.TransportProtocol() {
	case header.ICMPv4ProtocolNumber:
		err = s.procICMP(packet)
	case header.TCPProtocolNumber:
		err = s.procTCP(packet)
	case header.UDPProtocolNumber:
		err = s.procUDP(packet, checksum)
	default:
		err = NewDroppedError("dropped", "not suppored")
	}
	return
}

func (s *Stack) procUnreachable(packet *Packet) (err error) {
	key := fmt.Sprintf("%v-%v", packet.LocalAddr(), packet.RemoteAddr())
	DebugLog("Stack receive icmp dst unreable for connection %v", key)
	s.lck.RLock()
	session, ok := s.udps[key]
	s.lck.RUnlock()
	if ok {
		DebugLog("Stack connection %v is closing by remote", session)
		session.closeStart()
	}
	return
}

func (s *Stack) procICMP(packet *Packet) (err error) {
	switch packet.Version {
	case header.IPv4Version:
		switch packet.ICMPv4.Type() {
		case header.ICMPv4Echo:
			src, dst := packet.IPv4.SourceAddress(), packet.IPv4.DestinationAddress()
			packet.IPv4.SetSourceAddress(dst)
			packet.IPv4.SetDestinationAddress(src)
			packet.ICMPv4.SetType(header.ICMPv4EchoReply)
			packet.UpdateChecksum()
			err = s.sendPacket(packet)
		case header.ICMPv4DstUnreachable:
			info, err := Wrap(packet.ICMPv4.Payload(), false)
			if err == nil {
				err = s.procUnreachable(info)
			}
		default:
			err = NewDroppedError("dropped", "not suppored icmp type %v", packet.ICMPv4.Type())
			if s.Handler != nil {
				s.Handler.OnUnknow(packet)
			}
		}
	default:
		err = NewDroppedError("dropped", "not suppored ip version %v", packet.Version)
	}
	return
}

func (s *Stack) procUDP(packet *Packet, partialChecksum uint16) (err error) {
	key := fmt.Sprintf("%v-%v", packet.LocalAddr(), packet.RemoteAddr())
	s.lck.Lock()
	session, ok := s.udps[key]
	if !ok {
		session = NewConn(s, key, packet)
		s.udps[key] = session
	}
	s.lck.Unlock()
	if !ok {
		DebugLog("Stack connection %v is connected", session)
		session.status = ConnStatusConnected
		s.Handler.OnAccept(session)
	}
	session.closeLocker.RLock()
	if session.err == nil {
		buf := make([]byte, len(packet.Data))
		copy(buf, packet.Data)
		session.receiver <- buf
	}
	session.closeLocker.RUnlock()
	return
}

func (s *Stack) procReset(packet *Packet, seq, ack uint32) (err error) {
	pk, err := Swap(packet, 0, 0)
	if err != nil {
		return
	}
	pk.TCP.Encode(&header.TCPFields{
		SrcPort:    packet.TCP.DestinationPort(),
		DstPort:    packet.TCP.SourcePort(),
		SeqNum:     seq,
		AckNum:     ack,
		DataOffset: header.TCPMinimumSize,
		Flags:      header.TCPFlagRst,
		WindowSize: packet.TCP.WindowSize(),
	})
	pk.UpdateChecksum()
	err = s.sendPacket(pk)
	return
}

func (s *Stack) procTCP(packet *Packet) (err error) {
	key := fmt.Sprintf("%v-%v", packet.LocalAddr(), packet.RemoteAddr())
	flags := packet.TCP.Flags()
	if (flags & header.TCPFlagRst) == header.TCPFlagRst {
		s.lck.Lock()
		conn, ok := s.tcps[key]
		delete(s.tcps, key)
		s.lck.Unlock()
		if ok {
			conn.closeFinised()
			s.removeConn(conn)
		}
		err = s.procReset(packet, packet.TCP.AckNumber(), packet.TCP.SequenceNumber()+1)
		return
	}
	if (flags & header.TCPFlagSyn) == header.TCPFlagSyn {
		s.lck.Lock()
		conn, ok := s.tcps[key]
		if !ok {
			conn = NewConn(s, key, packet)
			s.tcps[key] = conn
		}
		s.lck.Unlock()
		if ok {
			err = conn.procReset(packet)
		} else {
			err = conn.procSync(packet)
		}
		return
	}
	if (flags & header.TCPFlagFin) == header.TCPFlagFin {
		s.lck.RLock()
		conn, ok := s.tcps[key]
		s.lck.RUnlock()
		if ok {
			if conn.status <= ConnStatusConnected {
				DebugLog("Stack connection %v is closing by remote", conn)
				err = conn.closeStart()
			} else {
				err = conn.procAck(packet, 1)
				s.removeConn(conn)
				conn.closeFinised()
			}
		} else {
			err = s.procReset(packet, packet.TCP.AckNumber(), packet.TCP.SequenceNumber()+1)
		}
		return
	}
	if (flags & header.TCPFlagAck) == header.TCPFlagAck {
		s.lck.RLock()
		conn, ok := s.tcps[key]
		s.lck.RUnlock()
		if ok {
			if conn.status == ConnStatusClosing {
				s.removeConn(conn)
				conn.closeFinised()
			} else {
				err = conn.procReceive(packet)
			}
		} else {
			err = s.procReset(packet, packet.TCP.AckNumber(), packet.TCP.SequenceNumber()+1)
		}
		return
	}
	if s.Handler != nil {
		s.Handler.OnUnknow(packet)
	}
	err = NewDroppedError("dropped", "unknow flags %v", flags)
	return
}

//SetWriter will init the stack out
func (s *Stack) SetWriter(out io.WriteCloser) {
	s.out = out
}

//State will return current status info
func (s *Stack) State() (state interface{}) {
	udps := map[string]interface{}{}
	tcps := map[string]interface{}{}
	s.lck.RLock()
	for key, c := range s.udps {
		udps[key] = map[string]interface{}{
			"latest": c.latest,
			"status": statusInfo(c.status),
		}
	}
	for key, c := range s.tcps {
		tcps[key] = map[string]interface{}{
			"latest": c.latest,
			"status": statusInfo(c.status),
		}
	}
	s.lck.RUnlock()
	state = map[string]interface{}{
		"tcps": tcps,
		"udps": udps,
	}
	return
}

//Close will close the acceptor
func (s *Stack) Close() (err error) {
	s.running = false
	return
}
