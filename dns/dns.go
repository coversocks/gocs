package dns

import (
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/coversocks/gocs/core"
	"github.com/miekg/dns"
)

const (
	//GfwProxy is GFW target for proxy
	GfwProxy = "dns://proxy"
	//GfwLocal is GFW target for local
	GfwLocal = "dns://local"
)

//GFW impl check if domain in gfw list
type GFW struct {
	list map[string]string
	lck  sync.RWMutex
}

//NewGFW will create new GFWList
func NewGFW() (gfw *GFW) {
	gfw = &GFW{
		list: map[string]string{
			"*": GfwLocal,
		},
		lck: sync.RWMutex{},
	}
	return
}

//Add list
func (g *GFW) Add(list, target string) {
	g.lck.Lock()
	defer g.lck.Unlock()
	g.list[list] = target
}

//IsProxy return true, if domain target is dns://proxy
func (g *GFW) IsProxy(domain string) bool {
	return g.Find(domain) == GfwProxy
}

//Find domain target
func (g *GFW) Find(domain string) (target string) {
	g.lck.RLock()
	defer g.lck.RUnlock()
	domain = strings.Trim(domain, " \t.")
	if len(domain) < 1 {
		target = g.list["*"]
		return
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		target = g.check(parts...)
	} else {
		n := len(parts) - 1
		for i := 0; i < n; i++ {
			target = g.check(parts[i:]...)
			if len(target) > 0 {
				break
			}
		}
	}
	if len(target) < 1 {
		target = g.list["*"]
	}
	return
}

func (g *GFW) check(parts ...string) (target string) {
	ptxt := fmt.Sprintf("(?m)^[\\|\\.\\@]*(http://)?(https://)?%v$", strings.Join(parts, "\\."))
	pattern, err := regexp.Compile(ptxt)
	if err == nil {
		for key, val := range g.list {
			if len(pattern.FindString(key)) > 0 {
				target = val
				break
			}
		}
	}
	return
}

func (g *GFW) String() string {
	return "GFW"
}

//Conn impl the  connection for read/write  message
type Conn struct {
	p          *Processor
	key        string
	base       io.ReadWriteCloser
	readQueued chan []byte
	closed     bool
	lck        sync.RWMutex
}

//NewConn will create new Conn
func NewConn(p *Processor, key string, base io.ReadWriteCloser) (conn *Conn) {
	conn = &Conn{
		p:          p,
		key:        key,
		base:       base,
		readQueued: make(chan []byte, 1024),
		lck:        sync.RWMutex{},
	}
	return
}

func (c *Conn) Read(p []byte) (n int, err error) {
	c.lck.RLock()
	if c.closed {
		c.lck.RUnlock()
		return
	}
	c.lck.RUnlock()
	data := <-c.readQueued
	if data == nil {
		err = fmt.Errorf("closed")
		return
	}
	if len(data) > len(p) {
		err = fmt.Errorf("buffer is too small")
		return
	}
	n = copy(p, data)
	// fmt.Printf("Conn(%p).Read---->%v,%v\n", d, n, data)
	return
}

func (c *Conn) Write(p []byte) (n int, err error) {
	c.lck.RLock()
	if c.closed {
		c.lck.RUnlock()
		return
	}
	c.lck.RUnlock()
	n, err = c.base.Write(p)
	return
}

//Close will close the connection
func (c *Conn) Close() (err error) {
	c.lck.Lock()
	defer c.lck.Unlock()
	if c.closed {
		err = fmt.Errorf("closed")
		return
	}
	c.closed = true
	close(c.readQueued)
	c.p.close(c)
	return
}

func (c *Conn) String() string {
	return fmt.Sprintf("Conn(%v)", c.base)
}

//Processor impl to core.Processor for process  connection
type Processor struct {
	conns    map[string]*Conn
	connsLck sync.RWMutex
	Next     core.Processor
	Target   func(domain string) string
}

//NewProcessor will create new Processor
func NewProcessor(next core.Processor, target func(domain string) string) (p *Processor) {
	p = &Processor{
		conns:    map[string]*Conn{},
		connsLck: sync.RWMutex{},
		Next:     next,
		Target:   target,
	}
	return
}

//ProcConn will process connection
func (p *Processor) ProcConn(r io.ReadWriteCloser, target string) (err error) {
	core.DebugLog("Processor proc for %v", r)
	go p.proc(r)
	return
}

func (p *Processor) close(c *Conn) {
	p.connsLck.Lock()
	delete(p.conns, c.key)
	p.connsLck.Unlock()
}

func (p *Processor) proc(r io.ReadWriteCloser) {
	core.DebugLog("Processor dns runner is starting for %v", r)
	var n int
	var err error
	for {
		buf := make([]byte, 4096)
		n, err = r.Read(buf)
		if err != nil {
			break
		}
		msg := new(dns.Msg)
		err = msg.Unpack(buf[0:n])
		if err != nil {
			break
		}
		var target = p.Target(msg.Question[0].Name)
		var key = fmt.Sprintf("%p-%v", r, target)
		p.connsLck.RLock()
		conn, ok := p.conns[key]
		p.connsLck.RUnlock()
		if !ok {
			conn = NewConn(p, key, r)
			err = p.Next.ProcConn(conn, target)
			if err != nil {
				//drop it
				core.DebugLog("Processor dns runner proc %v fail with %v", r, err)
				continue
			}
			p.connsLck.Lock()
			p.conns[key] = conn
			p.connsLck.Unlock()
		}
		conn.readQueued <- buf[0:n]
	}
	r.Close()
	core.DebugLog("Processor dns runner is stopped for %v", r)
}

func (p *Processor) String() string {
	return "Processor(DNS)"
}

//RecordConn is  connection for recording  response
type RecordConn struct {
	p    *RecordProcessor
	base io.ReadWriteCloser
}

//NewRecordConn will create new RecordConn
func NewRecordConn(p *RecordProcessor, base io.ReadWriteCloser) (conn *RecordConn) {
	conn = &RecordConn{
		p:    p,
		base: base,
	}
	return
}

func (r *RecordConn) Read(p []byte) (n int, err error) {
	n, err = r.base.Read(p)
	return
}

func (r *RecordConn) Write(p []byte) (n int, err error) {
	msg := new(dns.Msg)
	if xerr := msg.Unpack(p); xerr == nil && len(msg.Answer) > 0 {
		for _, answer := range msg.Answer {
			if a, ok := answer.(*dns.A); ok {
				core.DebugLog("Record recoding %v->%v", a.Hdr.Name, a.A)
				r.p.Record(a.A.String(), a.Hdr.Name)
			}
		}
	}
	n, err = r.base.Write(p)
	return
}

//Close will close base connection
func (r *RecordConn) Close() (err error) {
	err = r.base.Close()
	core.DebugLog("%v is closed", r)
	return
}

func (r *RecordConn) String() string {
	return fmt.Sprintf("RecordConn(%v)", r.base)
}

//RecordProcessor to impl processor for record  response
type RecordProcessor struct {
	Next   core.Processor
	allIP  map[string]string
	allLck sync.RWMutex
}

//NewRecordProcessor will create new RecordProcessor
func NewRecordProcessor(next core.Processor) (r *RecordProcessor) {
	r = &RecordProcessor{
		Next:   next,
		allIP:  map[string]string{},
		allLck: sync.RWMutex{},
	}
	return
}

//Record the key and value
func (r *RecordProcessor) Record(key, val string) {
	r.allLck.Lock()
	r.allIP[key] = val
	r.allLck.Unlock()
}

//IsRecorded will check if key is recorded
func (r *RecordProcessor) IsRecorded(key string) (ok bool) {
	r.allLck.RLock()
	_, ok = r.allIP[key]
	r.allLck.RUnlock()
	return
}

//Value will return record value by key
func (r *RecordProcessor) Value(key string) (val string) {
	r.allLck.RLock()
	val, _ = r.allIP[key]
	r.allLck.RUnlock()
	return
}

//Clear all recorded
func (r *RecordProcessor) Clear() {
	r.allLck.Lock()
	r.allIP = map[string]string{}
	r.allLck.Unlock()
}

//ProcConn will process connection
func (r *RecordProcessor) ProcConn(base io.ReadWriteCloser, target string) (err error) {
	core.DebugLog("RecordProcessor proc %v", base)
	err = r.Next.ProcConn(NewRecordConn(r, base), target)
	return
}

func (r *RecordProcessor) String() string {
	return "RecordProcessor"
}
