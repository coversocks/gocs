package netstack

import (
	"fmt"
	"io"
	"net"
	"reflect"
	"unsafe"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/buffer"
	"github.com/google/netstack/tcpip/header"
	"github.com/google/netstack/tcpip/stack"
)

//OutEndpoint impl tcpio.Endpoint by io out
type OutEndpoint struct {
	// mtu (maximum transmission unit) is the maximum size of a packet.
	mtu uint32

	// hdrSize specifies the link-layer header size. If set to 0, no header
	// is added/removed; otherwise an ethernet header is used.
	hdrSize int

	// addr is the address of the endpoint.
	addr tcpip.LinkAddress

	// caps holds the endpoint capabilities.
	caps stack.LinkEndpointCapabilities

	// gsoMaxSize is the maximum GSO packet size. It is zero if GSO is
	// disabled.
	gsoMaxSize uint32

	Dispatcher   stack.NetworkDispatcher
	Translations map[string]*tcpip.Error
	Out          io.WriteCloser
}

// OutOptions specify the details about the fd-based endpoint to be created.
type OutOptions struct {
	// MTU is the mtu to use for this endpoint.
	MTU uint32

	// EthernetHeader if true, indicates that the endpoint should read/write
	// ethernet frames instead of IP packets.
	EthernetHeader bool

	// Address is the link address for this endpoint. Only used if
	// EthernetHeader is true.
	Address string

	// SaveRestore if true, indicates that this NIC capability set should
	// include CapabilitySaveRestore
	SaveRestore bool

	// DisconnectOk if true, indicates that this NIC capability set should
	// include CapabilityDisconnectOk.
	DisconnectOk bool

	// GSOMaxSize is the maximum GSO packet size. It is zero if GSO is
	// disabled.
	GSOMaxSize uint32

	// SoftwareGSOEnabled indicates whether software GSO is enabled or not.
	SoftwareGSOEnabled bool

	// TXChecksumOffload if true, indicates that this endpoints capability
	// set should include CapabilityTXChecksumOffload.
	TXChecksumOffload bool

	// RXChecksumOffload if true, indicates that this endpoints capability
	// set should include CapabilityRXChecksumOffload.
	RXChecksumOffload bool

	// Out is output writer
	Out io.WriteCloser
}

// NewOutEndpoint creates a new io.WriteCloser base endpoint.
func NewOutEndpoint(opts *OutOptions) (out *OutEndpoint, err error) {
	// Parse the mac address.
	maddr, err := net.ParseMAC(opts.Address)
	if err != nil {
		return
	}
	caps := stack.LinkEndpointCapabilities(0)
	if opts.RXChecksumOffload {
		caps |= stack.CapabilityRXChecksumOffload
	}

	if opts.TXChecksumOffload {
		caps |= stack.CapabilityTXChecksumOffload
	}

	hdrSize := 0
	if opts.EthernetHeader {
		hdrSize = header.EthernetMinimumSize
		caps |= stack.CapabilityResolutionRequired
	}

	if opts.SaveRestore {
		caps |= stack.CapabilitySaveRestore
	}

	if opts.DisconnectOk {
		caps |= stack.CapabilityDisconnectOk
	}

	e := &OutEndpoint{
		mtu:     opts.MTU,
		caps:    caps,
		addr:    tcpip.LinkAddress(maddr),
		hdrSize: hdrSize,
		Out:     opts.Out,
	}
	return e, nil
}

// Attach launches the goroutine that reads packets from the file descriptor and
// dispatches them via the provided dispatcher.
func (e *OutEndpoint) Attach(dispatcher stack.NetworkDispatcher) {
	e.Dispatcher = dispatcher
}

// IsAttached implements stack.LinkEndpoint.IsAttached.
func (e *OutEndpoint) IsAttached() bool {
	return e.Dispatcher != nil
}

// MTU implements stack.LinkEndpoint.MTU. It returns the value initialized
// during construction.
func (e *OutEndpoint) MTU() uint32 {
	return e.mtu
}

// Capabilities implements stack.LinkEndpoint.Capabilities.
func (e *OutEndpoint) Capabilities() stack.LinkEndpointCapabilities {
	return e.caps
}

// MaxHeaderLength returns the maximum size of the link-layer header.
func (e *OutEndpoint) MaxHeaderLength() uint16 {
	return uint16(e.hdrSize)
}

// LinkAddress returns the link address of this endpoint.
func (e *OutEndpoint) LinkAddress() tcpip.LinkAddress {
	return e.addr
}

// Wait implements stack.LinkEndpoint.Wait.
func (e *OutEndpoint) Wait() {
}

// virtioNetHdr is declared in linux/virtio_net.h.
type virtioNetHdr struct {
	flags      uint8
	gsoType    uint8
	hdrLen     uint16
	gsoSize    uint16
	csumStart  uint16
	csumOffset uint16
}

// These constants are declared in linux/virtio_net.h.
const (
	_VIRTIO_NET_HDR_F_NEEDS_CSUM = 1
	_VIRTIO_NET_HDR_GSO_TCPV4    = 1
	_VIRTIO_NET_HDR_GSO_TCPV6    = 4
)

const virtioNetHdrSize = int(unsafe.Sizeof(virtioNetHdr{}))

func vnetHdrToByteSlice(hdr *virtioNetHdr) (slice []byte) {
	sh := (*reflect.SliceHeader)(unsafe.Pointer(&slice))
	sh.Data = uintptr(unsafe.Pointer(hdr))
	sh.Len = virtioNetHdrSize
	sh.Cap = virtioNetHdrSize
	return
}

// WritePacket writes outbound packets to the file descriptor. If it is not
// currently writable, the packet is dropped.
func (e *OutEndpoint) WritePacket(r *stack.Route, gso *stack.GSO, protocol tcpip.NetworkProtocolNumber, pkt tcpip.PacketBuffer) *tcpip.Error {
	if e.hdrSize > 0 {
		// Add ethernet header if needed.
		eth := header.Ethernet(pkt.Header.Prepend(header.EthernetMinimumSize))
		pkt.LinkHeader = buffer.View(eth)
		ethHdr := &header.EthernetFields{
			DstAddr: r.RemoteLinkAddress,
			Type:    protocol,
		}

		// Preserve the src address if it's set in the route.
		if r.LocalLinkAddress != "" {
			ethHdr.SrcAddr = r.LocalLinkAddress
		} else {
			ethHdr.SrcAddr = e.addr
		}
		eth.Encode(ethHdr)
	}

	if e.Capabilities()&stack.CapabilityHardwareGSO != 0 {
		vnetHdr := virtioNetHdr{}
		vnetHdrBuf := vnetHdrToByteSlice(&vnetHdr)
		if gso != nil {
			vnetHdr.hdrLen = uint16(pkt.Header.UsedLength())
			if gso.NeedsCsum {
				vnetHdr.flags = _VIRTIO_NET_HDR_F_NEEDS_CSUM
				vnetHdr.csumStart = header.EthernetMinimumSize + gso.L3HdrLen
				vnetHdr.csumOffset = gso.CsumOffset
			}
			if gso.Type != stack.GSONone && uint16(pkt.Data.Size()) > gso.MSS {
				switch gso.Type {
				case stack.GSOTCPv4:
					vnetHdr.gsoType = _VIRTIO_NET_HDR_GSO_TCPV4
				case stack.GSOTCPv6:
					vnetHdr.gsoType = _VIRTIO_NET_HDR_GSO_TCPV6
				default:
					panic(fmt.Sprintf("Unknown gso type: %v", gso.Type))
				}
				vnetHdr.gsoSize = gso.MSS
			}
		}
		return e.writeData(vnetHdrBuf, pkt.Header.View(), pkt.Data.ToView())
	}

	if pkt.Data.Size() == 0 {
		return e.writeData(pkt.Header.View())
	}

	return e.writeData(pkt.Header.View(), pkt.Data.ToView(), nil)
}

// WritePackets writes outbound packets to the file descriptor. If it is not
// currently writable, the packet is dropped.
func (e *OutEndpoint) WritePackets(r *stack.Route, gso *stack.GSO, hdrs []stack.PacketDescriptor, payload buffer.VectorisedView, protocol tcpip.NetworkProtocolNumber) (int, *tcpip.Error) {
	panic("not implement")
}

// WriteRawPacket implements stack.LinkEndpoint.WriteRawPacket.
func (e *OutEndpoint) WriteRawPacket(vv buffer.VectorisedView) *tcpip.Error {
	return e.writeData(vv.ToView())
}

func (e *OutEndpoint) writeData(data ...[]byte) *tcpip.Error {
	var all = data[0]
	for i, d := range data {
		if i < 1 {
			continue
		}
		all = append(all, d...)
	}
	fmt.Printf("OutEndpoint write %v\n", all)
	_, err := e.Out.Write(all)
	return e.TranslateErrno(err)
}

// GSOMaxSize returns the maximum GSO packet size.
func (e *OutEndpoint) GSOMaxSize() uint32 {
	return e.gsoMaxSize
}

// TranslateErrno will translate error to tcpip.Error
func (e *OutEndpoint) TranslateErrno(err error) *tcpip.Error {
	if err == nil {
		return nil
	}
	if nerr := e.Translations[err.Error()]; nerr != nil {
		return nerr
	}
	return tcpip.ErrInvalidEndpointState
}

//DeliverNetworkPacket will dispatches buf it.
func (e *OutEndpoint) DeliverNetworkPacket(buf []byte) bool {
	fmt.Printf("DeliverNetworkPacket data %v\n", buf)
	n := len(buf)
	if e.Capabilities()&stack.CapabilityHardwareGSO != 0 {
		// Skip virtioNetHdr which is added before each packet, it
		// isn't used and it isn't in a view.
		n -= virtioNetHdrSize
	}
	if n <= e.hdrSize {
		return false
	}

	var (
		p             tcpip.NetworkProtocolNumber
		remote, local tcpip.LinkAddress
		eth           header.Ethernet
	)
	if e.hdrSize > 0 {
		eth = header.Ethernet(buf[:header.EthernetMinimumSize])
		p = eth.Type()
		remote = eth.SourceAddress()
		local = eth.DestinationAddress()
	} else {
		// We don't get any indication of what the packet is, so try to guess
		// if it's an IPv4 or IPv6 packet.
		switch header.IPVersion(buf) {
		case header.IPv4Version:
			p = header.IPv4ProtocolNumber
		case header.IPv6Version:
			p = header.IPv6ProtocolNumber
		default:
			return true
		}
	}

	pkt := tcpip.PacketBuffer{
		Data:       buffer.NewVectorisedView(n, []buffer.View{buffer.View(buf)}),
		LinkHeader: buffer.View(eth),
	}
	pkt.Data.TrimFront(e.hdrSize)
	e.Dispatcher.DeliverNetworkPacket(e, remote, local, p, pkt)
	return true
}
