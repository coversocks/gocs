package udpgw

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/codingeasygo/util/xio/frame"
	"github.com/coversocks/gocs/core"
)

const UDPGW_CLIENT_FLAG_KEEPALIVE = (1 << 0)
const UDPGW_CLIENT_FLAG_REBIND = (1 << 1)
const UDPGW_CLIENT_FLAG_DNS = (1 << 2)
const UDPGW_CLIENT_FLAG_IPV6 = (1 << 3)

type UDPConn struct {
	raw    *net.UDPConn
	addr   *net.UDPAddr
	conid  uint16
	flags  uint8
	latest time.Time
}

type UDPGW struct {
	MTU      int
	DNS      *net.UDPAddr
	MaxConn  int
	connList map[uint16]*UDPConn
	connLock sync.RWMutex
	timeout  bool
}

func NewUDPGW() (gw *UDPGW) {
	gw = &UDPGW{
		MTU:      2048,
		MaxConn:  64,
		connList: map[uint16]*UDPConn{},
		connLock: sync.RWMutex{},
	}
	return
}

func (u *UDPGW) cloaseAllConn() {
	u.connLock.Lock()
	for connid, conn := range u.connList {
		conn.raw.Close()
		delete(u.connList, connid)
	}
	u.connLock.Unlock()
}

func (u *UDPGW) Close() (err error) {
	u.cloaseAllConn()
	u.StopTimeout()
	return
}

func (u *UDPGW) PipeConn(conn io.ReadWriteCloser, target string) (err error) {
	defer func() {
		conn.Close()
		u.cloaseAllConn()
		u.StopTimeout()
	}()
	rwc, ok := conn.(frame.ReadWriteCloser)
	if !ok {
		err = fmt.Errorf("conn is not frame.ReadWriteCloser")
		return
	}
	offset := rwc.GetDataOffset()
	for {
		data, xerr := rwc.ReadFrame()
		if xerr != nil {
			break
		}
		u.procData(rwc, data[offset:])
	}
	return
}

func (u *UDPGW) procData(piped frame.ReadWriteCloser, p []byte) (n int, err error) {
	flags := uint8(p[0])
	conid := binary.BigEndian.Uint16(p[1:])
	if flags&UDPGW_CLIENT_FLAG_KEEPALIVE == UDPGW_CLIENT_FLAG_KEEPALIVE {
		n = len(p)
		return
	}
	var addrIP net.IP
	var addrPort uint16
	var data []byte
	if flags&UDPGW_CLIENT_FLAG_IPV6 == UDPGW_CLIENT_FLAG_IPV6 {
		addrIP = net.IP(p[3:19])
		addrPort = binary.BigEndian.Uint16(p[19:21])
		data = p[21:]
	} else {
		addrIP = net.IP(p[3:7])
		addrPort = binary.BigEndian.Uint16(p[7:9])
		data = p[9:]
	}
	u.connLock.RLock()
	conn := u.connList[conid]
	u.connLock.RUnlock()
	if conn == nil {
		u.limitConn()
		var addr *net.UDPAddr
		if flags&UDPGW_CLIENT_FLAG_DNS == UDPGW_CLIENT_FLAG_DNS && u.DNS != nil {
			addr = u.DNS
		} else {
			addr = &net.UDPAddr{IP: addrIP, Port: int(addrPort)}
		}
		conn = &UDPConn{conid: conid, flags: flags, addr: addr, latest: time.Now()}
		conn.raw, err = net.DialUDP("udp", nil, addr)
		if err != nil {
			core.DebugLog("UDPGW udp dial to %v fail with %v", addr, err)
			return
		}
		core.DebugLog("UDPGW udp dial to %v success", addr)
		u.connLock.Lock()
		u.connList[conid] = conn
		u.connLock.Unlock()
		go u.procRead(piped, conn)
	}
	conn.latest = time.Now()
	n, err = conn.raw.Write(data)
	n += len(addrIP) + 5
	return
}

func (u *UDPGW) procRead(piped frame.ReadWriteCloser, conn *UDPConn) {
	defer func() {
		if perr := recover(); perr != nil {
			core.WarnLog("UDPGW process raw read is panic by %v", perr)
		}
		u.connLock.Lock()
		delete(u.connList, conn.conid)
		u.connLock.Unlock()
		conn.raw.Close()
		core.DebugLog("UDPGW udp to %v is closed", conn.addr)
	}()
	buffer := make([]byte, u.MTU+piped.GetDataOffset())
	offset := piped.GetDataOffset()
	if conn.flags&UDPGW_CLIENT_FLAG_IPV6 == UDPGW_CLIENT_FLAG_IPV6 {
		buffer[offset] = UDPGW_CLIENT_FLAG_IPV6
	} else {
		buffer[offset] = 0
	}
	if conn.flags&UDPGW_CLIENT_FLAG_DNS == UDPGW_CLIENT_FLAG_DNS {
		buffer[offset] |= UDPGW_CLIENT_FLAG_DNS
	}
	offset += 1
	binary.BigEndian.PutUint16(buffer[offset:], conn.conid)
	offset += 2
	offset += copy(buffer[offset:], conn.addr.IP)
	binary.BigEndian.PutUint16(buffer[offset:], uint16(conn.addr.Port))
	offset += 2
	for {
		n, err := conn.raw.Read(buffer[offset:])
		if err != nil {
			break
		}
		conn.latest = time.Now()
		_, err = piped.WriteFrame(buffer[:offset+n])
		if err != nil {
			break
		}
	}
}

func (u *UDPGW) StartTimeout(delay, timeout time.Duration) {
	if u.timeout {
		panic("running")
	}
	u.timeout = true
	go u.runTimeout(delay, timeout)
}

func (u *UDPGW) StopTimeout() {
	u.timeout = false
}

func (u *UDPGW) limitConn() {
	u.connLock.Lock()
	defer u.connLock.Unlock()
	if len(u.connList) >= u.MaxConn-1 {
		return
	}
	var oldest *UDPConn
	for _, conn := range u.connList {
		if oldest == nil || oldest.latest.After(conn.latest) {
			oldest = conn
		}
	}
	if oldest != nil {
		oldest.raw.Close()
		delete(u.connList, oldest.conid)
	}
}

func (u *UDPGW) runTimeout(delay, timeout time.Duration) {
	for u.timeout {
		u.procTimeout(timeout)
		time.Sleep(delay)
	}
}

func (u *UDPGW) procTimeout(timeout time.Duration) {
	defer func() {
		if perr := recover(); perr != nil {
			core.WarnLog("UDPGW process timeout is panic by %v", perr)
		}
	}()
	now := time.Now()
	u.connLock.Lock()
	for key, conn := range u.connList {
		if now.Sub(conn.latest) > timeout {
			conn.raw.Close()
			delete(u.connList, key)
		}
	}
	u.connLock.Unlock()
}
