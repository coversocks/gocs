package core

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
)

//Processor is interface for process connection
type Processor interface {
	ProcConn(raw io.ReadWriteCloser, target string) (err error)
}

//AyncProcessor is Processor impl, it will process connection by async
type AyncProcessor struct {
	Next Processor
}

//NewAyncProcessor will return new AyncProcessor
func NewAyncProcessor(next Processor) (proc *AyncProcessor) {
	proc = &AyncProcessor{
		Next: next,
	}
	return
}

//ProcConn will process connection async
func (a *AyncProcessor) ProcConn(raw io.ReadWriteCloser, target string) (err error) {
	go func() {
		err = a.Next.ProcConn(raw, target)
		if err != nil {
			DebugLog("AyncProcessor process connection %v for %v fail with %v", raw, target, err)
		} else {
			DebugLog("AyncProcessor process connection %v for %v is done", raw, target)
		}
		raw.Close()
	}()
	return
}

func (a *AyncProcessor) String() string {
	return "AyncProcessor"
}

//ProcConnDialer is ProcConn impl by dialer
type ProcConnDialer struct {
	Dialer
}

//NewProcConnDialer will return new ProcConnDialer
func NewProcConnDialer(dialer Dialer) (proc *ProcConnDialer) {
	proc = &ProcConnDialer{
		Dialer: dialer,
	}
	return
}

func copyClose(dst, src io.ReadWriteCloser, bufferSize int) {
	buf := make([]byte, bufferSize)
	_, err := io.CopyBuffer(dst, src, buf)
	DebugLog("ProcConnDialer connection %v is closed by %v", src, err)
	dst.Close()
}

//ProcConn process connection by dial
func (p *ProcConnDialer) ProcConn(raw io.ReadWriteCloser, target string) (err error) {
	conn, err := p.Dial(target)
	if err == nil {
		go copyClose(raw, conn, 512*1024)
		copyClose(conn, raw, 32*1024)
	}
	return
}

func (p *ProcConnDialer) String() string {
	return "ProcConnDialer"
}

//PortDistProcessor impl to Processor for distribute processor by target host port
type PortDistProcessor struct {
	handlers   map[string]Processor
	handlerLck sync.RWMutex
}

//NewPortDistProcessor will create new PortDistProcessor
func NewPortDistProcessor() (p *PortDistProcessor) {
	p = &PortDistProcessor{
		handlers:   map[string]Processor{},
		handlerLck: sync.RWMutex{},
	}
	return
}

//Add will add processor to handler list
func (p *PortDistProcessor) Add(port string, h Processor) {
	p.handlerLck.Lock()
	defer p.handlerLck.Unlock()
	p.handlers[port] = h
}

//ProcConn will process connection
func (p *PortDistProcessor) ProcConn(raw io.ReadWriteCloser, target string) (err error) {
	u, err := url.Parse(target)
	if err != nil {
		return
	}
	p.handlerLck.RLock()
	defer p.handlerLck.RUnlock()
	if h, ok := p.handlers[u.Port()]; ok {
		DebugLog("PortDistProcessor dist %v by %v to %v", raw, u.Port(), h)
		err = h.ProcConn(raw, target)
		return
	}
	if h, ok := p.handlers["*"]; ok {
		DebugLog("PortDistProcessor dist %v by %v to default %v", raw, u.Port(), h)
		err = h.ProcConn(raw, target)
		return
	}
	err = fmt.Errorf("processor is not exist for %v", target)
	return
}

func (p *PortDistProcessor) String() string {
	return "PortDistProcessor"
}

//SchemeDistProcessor impl to Processor for distribute processor by target scheme
type SchemeDistProcessor struct {
	handlers   map[string]Processor
	handlerLck sync.RWMutex
}

//NewSchemeDistProcessor will create new SchemeDistProcessor
func NewSchemeDistProcessor() (p *SchemeDistProcessor) {
	p = &SchemeDistProcessor{
		handlers:   map[string]Processor{},
		handlerLck: sync.RWMutex{},
	}
	return
}

//Add will add processor to handler list
func (s *SchemeDistProcessor) Add(scheme string, h Processor) {
	s.handlerLck.Lock()
	defer s.handlerLck.Unlock()
	s.handlers[scheme] = h
}

//ProcConn will process connection
func (s *SchemeDistProcessor) ProcConn(raw io.ReadWriteCloser, target string) (err error) {
	u, err := url.Parse(target)
	if err != nil {
		return
	}
	s.handlerLck.RLock()
	defer s.handlerLck.RUnlock()
	if h, ok := s.handlers[u.Scheme]; ok {
		DebugLog("SchemeDistProcessor dist %v by %v to %v", raw, u.Scheme, h)
		err = h.ProcConn(raw, target)
		return
	}
	err = fmt.Errorf("processor is not exist for %v", target)
	return
}

func (s *SchemeDistProcessor) String() string {
	return "SchemeDistProcessor"
}

//PACProcessor to impl Processor for pac
type PACProcessor struct {
	Proxy  Processor
	Direct Processor
	Check  func(h string) bool
}

//NewPACProcessor will create new PACProcessor
func NewPACProcessor(proxy, direct Processor) (pac *PACProcessor) {
	pac = &PACProcessor{
		Proxy:  proxy,
		Direct: direct,
	}
	return
}

//ProcConn will process connection
func (p *PACProcessor) ProcConn(r io.ReadWriteCloser, target string) (err error) {
	u, err := url.Parse(target)
	if err != nil {
		return
	}
	if u.Host == "proxy" || (p.Check != nil && p.Check(u.Hostname())) {
		DebugLog("PACProcessor follow proxy(%v) for %v", p.Proxy, r)
		err = p.Proxy.ProcConn(r, target)
	} else {
		DebugLog("PACProcessor follow direct(%v) for %v", p.Direct, r)
		err = p.Direct.ProcConn(r, target)
	}
	return
}

func (p *PACProcessor) String() string {
	return "PACProcessor"
}

//EchoProcessor is Processor impl for echo connection
type EchoProcessor struct {
}

//NewEchoProcessor will return new EchoProceessor
func NewEchoProcessor() (echo *EchoProcessor) {
	return &EchoProcessor{}
}

//ProcConn will process connection
func (e *EchoProcessor) ProcConn(r io.ReadWriteCloser, target string) (err error) {
	InfoLog("EchoProceessor process connection %v", target)
	_, err = io.Copy(r, r)
	r.Close()
	return
}

func (e *EchoProcessor) String() string {
	return "EchoProcessor"
}

//PrintProcessor is Processor impl for echo connection
type PrintProcessor struct {
	Next Processor
}

//NewPrintProcessor will return new EchoProceessor
func NewPrintProcessor(next Processor) (print *PrintProcessor) {
	return &PrintProcessor{Next: next}
}

//ProcConn will process connection
func (p *PrintProcessor) ProcConn(r io.ReadWriteCloser, target string) (err error) {
	conn := NewPrintConn(r)
	err = p.Next.ProcConn(conn, target)
	return
}

//RemoteByAddr will generate remote address by net.Addr
func RemoteByAddr(addr net.Addr) (remote string) {
	if udp, ok := addr.(*net.UDPAddr); ok {
		remote = fmt.Sprintf("udp://%v:%v", udp.IP, udp.Port)
	} else if tcp, ok := addr.(*net.TCPAddr); ok {
		remote = fmt.Sprintf("tcp://%v:%v", tcp.IP, tcp.Port)
	} else {
		remote = fmt.Sprintf("%v://%v", addr.Network(), addr)
	}
	return
}
