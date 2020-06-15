package tcpip

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/google/netstack/tcpip/header"
)

type ValueError struct {
	Value interface{}
}

func NewValueError(v interface{}) (err error) {
	err = &ValueError{Value: v}
	return
}

func (v *ValueError) Error() string {
	return fmt.Sprintf("%v", v.Value)
}

func (v *ValueError) String() string {
	return fmt.Sprintf("%v", v.Value)
}

type WrapRWC struct {
	Base   interface{}
	Reader interface{}
	Writer interface{}
	Closer interface{}
}

func NewWrapRWC(base interface{}) (rwc *WrapRWC) {
	rwc = &WrapRWC{
		Base:   base,
		Reader: base,
		Writer: base,
		Closer: base,
	}
	return
}

func (w *WrapRWC) Read(b []byte) (n int, err error) {
	if reader, ok := w.Reader.(io.Reader); ok {
		n, err = reader.Read(b)
	} else {
		err = fmt.Errorf("base is not io.Reader")
	}
	return
}

func (w *WrapRWC) Write(b []byte) (n int, err error) {
	if writer, ok := w.Writer.(io.Writer); ok {
		n, err = writer.Write(b)
	} else {
		err = fmt.Errorf("base is not io.Writer")
	}
	return
}

func (w *WrapRWC) Close() (err error) {
	if closer, ok := w.Base.(io.Closer); ok {
		err = closer.Close()
	} else {
		err = fmt.Errorf("base is not io.Closer")
	}
	return
}

func (w *WrapRWC) LocalAddr() net.Addr {
	if conn, ok := w.Base.(net.Conn); ok {
		return conn.LocalAddr()
	}
	return w
}

func (w *WrapRWC) RemoteAddr() net.Addr {
	if conn, ok := w.Base.(net.Conn); ok {
		return conn.RemoteAddr()
	}
	return w
}

func (w *WrapRWC) SetDeadline(t time.Time) error {
	if conn, ok := w.Base.(net.Conn); ok {
		return conn.SetDeadline(t)
	}
	return nil
}

func (w *WrapRWC) SetReadDeadline(t time.Time) error {
	if conn, ok := w.Base.(net.Conn); ok {
		return conn.SetReadDeadline(t)
	}
	return nil
}

func (w *WrapRWC) SetWriteDeadline(t time.Time) error {
	if conn, ok := w.Base.(net.Conn); ok {
		return conn.SetWriteDeadline(t)
	}
	return nil
}

func (w *WrapRWC) Network() string {
	return "wrap"
}

func (w *WrapRWC) String() string {
	return "wrap"
}

type PrintRWC struct {
	Enable bool
	Name   string
	Base   io.ReadWriteCloser
}

func NewPrintRWC(enable bool, name string, base io.ReadWriteCloser) (rwc *PrintRWC) {
	rwc = &PrintRWC{
		Enable: enable,
		Name:   name,
		Base:   base,
	}
	return
}

func (p *PrintRWC) Read(b []byte) (n int, err error) {
	n, err = p.Base.Read(b)
	if p.Enable && err == nil {
		fmt.Printf("%v read %v by %x\n", p.Name, n, b[0:n])
	}
	return
}

func (p *PrintRWC) Write(b []byte) (n int, err error) {
	n, err = p.Base.Write(b)
	if p.Enable && err == nil {
		fmt.Printf("%v write %v by %x\n", p.Name, n, b)
	}
	return
}

func (p *PrintRWC) Close() (err error) {
	err = p.Base.Close()
	if p.Enable {
		fmt.Printf("%v close with %v\n", p.Name, err)
	}
	return
}

type PacketPrintRWC struct {
	Name   string
	Eth    bool
	Enable bool
	io.ReadWriteCloser
}

func NewPacketPrintRWC(enable bool, name string, base io.ReadWriteCloser, eth bool) (rwc *PacketPrintRWC) {
	rwc = &PacketPrintRWC{Enable: enable, Name: name, Eth: eth, ReadWriteCloser: base}
	return
}

func (p *PacketPrintRWC) Read(b []byte) (n int, err error) {
	n, err = p.ReadWriteCloser.Read(b)
	if p.Enable && err == nil {
		p.print("Read", b[:n])
	}
	return
}

func (p *PacketPrintRWC) Write(b []byte) (n int, err error) {
	n, err = p.ReadWriteCloser.Write(b)
	if p.Enable && err == nil {
		p.print("Write", b)
	}
	return
}

func (p *PacketPrintRWC) print(sub string, b []byte) {
	packet, err := Wrap(b, p.Eth)
	if err != nil {
		InfoLog("%v.%v wrap data fail with %v by %x", p.Name, sub, err, b)
	} else {
		if packet.Proto == header.TCPProtocolNumber {
			srcAddr := packet.IPv4.SourceAddress()
			dstAddr := packet.IPv4.DestinationAddress()
			srcPort := packet.TCP.SourcePort()
			dstPort := packet.TCP.DestinationPort()
			seqNum := packet.TCP.SequenceNumber()
			ackNum := packet.TCP.AckNumber()
			syn := packet.TCP.Flags() & header.TCPFlagSyn
			ack := packet.TCP.Flags() & header.TCPFlagAck
			psh := packet.TCP.Flags() & header.TCPFlagPsh
			fin := packet.TCP.Flags() & header.TCPFlagFin
			rst := packet.TCP.Flags() & header.TCPFlagRst
			data := len(packet.TCP.Payload())
			InfoLog(`
%v.%v Packet
	src:%v:%v
	dst:%v:%v
	seq:%v
	ack:%v
	syn:%v
	ack:%v
	psh:%v
	fin:%v
	rst:%v
	data:%v
`, p.Name, sub, srcAddr, srcPort, dstAddr, dstPort, seqNum, ackNum, syn, ack, psh, fin, rst, data)
		}
	}
}

type ChannelListener struct {
	Channel chan net.Conn
}

func NewChannelListener() (listner *ChannelListener) {
	listner = &ChannelListener{
		Channel: make(chan net.Conn, 1024),
	}
	return
}

func (c *ChannelListener) Accept() (conn net.Conn, err error) {
	conn = <-c.Channel
	if conn == nil {
		err = fmt.Errorf("closed")
	}
	return
}

func (c *ChannelListener) Close() error {
	close(c.Channel)
	return nil
}

func (c *ChannelListener) Addr() net.Addr {
	return c
}

func (c *ChannelListener) Network() string {
	return "channel"
}

func (c *ChannelListener) String() string {
	return "string"
}

type WaitReader struct {
	Base io.Reader
	Wait time.Duration
}

func NewWaitReader(base io.Reader, wait time.Duration) (reader *WaitReader) {
	reader = &WaitReader{
		Base: base,
		Wait: wait,
	}
	return
}

func (w *WaitReader) Read(p []byte) (n int, err error) {
	if w.Wait > 0 {
		time.Sleep(w.Wait)
	}
	n, err = w.Base.Read(p)
	return
}

func (w *WaitReader) Close() (err error) {
	if closer, ok := w.Base.(io.Closer); ok {
		err = closer.Close()
	}
	return
}
