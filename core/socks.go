package core

import (
	"fmt"
	"io"
	"net"
)

//SocksProxy is an implementation of socks5 proxy
type SocksProxy struct {
	net.Listener
	Dialer       func(uri string, raw io.ReadWriteCloser) (sid uint64, err error)
	HTTPUpstream string
}

//NewSocksProxy will return new SocksProxy
func NewSocksProxy() (socks *SocksProxy) {
	socks = &SocksProxy{}
	return
}

//Listen the address
func (s *SocksProxy) Listen(addr string) (err error) {
	s.Listener, err = net.Listen("tcp", addr)
	if err == nil {
		InfoLog("SocksProxy listen socks5 proxy on %v", addr)
	}
	return
}

//Run proxy listener
func (s *SocksProxy) Run() (err error) {
	if s.Listener != nil {
		s.loopAccept(s.Listener)
	}
	return
}

func (s *SocksProxy) loopAccept(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			break
		}
		go s.procConn(conn)
	}
}

func (s *SocksProxy) procConn(conn net.Conn) {
	var err error
	DebugLog("SocksProxy proxy connection from %v", conn.RemoteAddr())
	defer func() {
		if err != nil {
			DebugLog("SocksProxy proxy connection from %v is done with %v", conn.RemoteAddr(), err)
			conn.Close()
		}
	}()
	buf := make([]byte, 1024*64)
	//
	//Procedure method
	err = fullBuf(conn, buf, 2)
	if err != nil {
		return
	}
	if buf[0] != 0x05 {
		if len(s.HTTPUpstream) < 1 {
			err = fmt.Errorf("only ver 0x05 is supported, but %x", buf[0])
			return
		}
		DebugLog("SocksProxy proxy connection to http upstream(%v) from %v", s.HTTPUpstream, conn.RemoteAddr())
		var up net.Conn
		up, err = net.Dial("tcp", s.HTTPUpstream)
		if err != nil {
			return
		}
		go io.Copy(conn, up)
		up.Write(buf[:2])
		buf = nil
		_, err = io.Copy(up, conn)
		return
	}
	err = fullBuf(conn, buf[2:], uint32(buf[1]))
	if err != nil {
		return
	}
	_, err = conn.Write([]byte{0x05, 0x00})
	if err != nil {
		return
	}
	//
	//Procedure request
	err = fullBuf(conn, buf, 5)
	if err != nil {
		return
	}
	if buf[0] != 0x05 {
		err = fmt.Errorf("only ver 0x05 is supported, but %x", buf[0])
		return
	}
	var uri string
	switch buf[3] {
	case 0x01:
		err = fullBuf(conn, buf[5:], 5)
		if err == nil {
			remote := fmt.Sprintf("%v.%v.%v.%v", buf[4], buf[5], buf[6], buf[7])
			port := uint16(buf[8])*256 + uint16(buf[9])
			uri = fmt.Sprintf("%v:%v", remote, port)
		}
	case 0x03:
		err = fullBuf(conn, buf[5:], uint32(buf[4]+2))
		if err == nil {
			remote := string(buf[5 : buf[4]+5])
			port := uint16(buf[buf[4]+5])*256 + uint16(buf[buf[4]+6])
			uri = fmt.Sprintf("%v:%v", remote, port)
		}
	default:
		err = fmt.Errorf("ATYP %v is not supported", buf[3])
		return
	}
	DebugLog("SocksProxy start dial to %v on %v", uri, conn.RemoteAddr())
	// if err != nil {
	// 	buf[0], buf[1], buf[2], buf[3] = 0x05, 0x04, 0x00, 0x01
	// 	buf[4], buf[5], buf[6], buf[7] = 0x00, 0x00, 0x00, 0x00
	// 	buf[8], buf[9] = 0x00, 0x00
	// 	buf[1] = 0x01
	// 	conn.Write(buf[:10])
	// 	InfoLog("SocksProxy dial to %v on %v fail with %v", uri, conn.RemoteAddr(), err)
	// 	pending.Close()
	// 	return
	// }
	buf[0], buf[1], buf[2], buf[3] = 0x05, 0x00, 0x00, 0x01
	buf[4], buf[5], buf[6], buf[7] = 0x00, 0x00, 0x00, 0x00
	buf[8], buf[9] = 0x00, 0x00
	_, err = conn.Write(buf[:10])
	if err == nil {
		_, err = s.Dialer(uri, NewStringConn(conn))
	}
}

func fullBuf(r io.Reader, p []byte, length uint32) error {
	all := uint32(0)
	buf := p[:length]
	for {
		readed, err := r.Read(buf)
		if err != nil {
			return err
		}
		all += uint32(readed)
		if all < length {
			buf = p[all:]
			continue
		} else {
			break
		}
	}
	return nil
}
