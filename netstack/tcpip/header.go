package tcpip

import (
	"fmt"
	"net"
	"strings"

	"github.com/google/netstack/tcpip"
	"github.com/google/netstack/tcpip/buffer"
	"github.com/google/netstack/tcpip/header"
)

type NotSupportedError struct {
	Version  int
	Protocol tcpip.TransportProtocolNumber
}

func (n *NotSupportedError) Error() string {
	return fmt.Sprintf("not supported version(%v) protocol(%v)", n.Version, n.Protocol)
}

type Packet struct {
	Buffer  []byte
	Eth     header.Ethernet
	Version int
	Proto   tcpip.TransportProtocolNumber
	IPv4    header.IPv4
	IPv6    header.IPv6
	ICMPv4  header.ICMPv4
	ICMPv6  header.ICMPv6
	TCP     header.TCP
	UDP     header.UDP
	Header  []byte
	Data    []byte
}

func Swap(to *Packet, options, playload int) (pk *Packet, err error) {
	pk, err = New(len(to.Eth) > 0, to.Version, to.Proto, options, playload)
	if err != nil {
		return
	}
	switch pk.Version {
	case header.IPv4Version:
		tos, _ := to.IPv4.TOS()
		pk.IPv4.Encode(&header.IPv4Fields{
			IHL:            header.IPv4MinimumSize,
			TOS:            tos,
			TotalLength:    uint16(len(pk.IPv4)),
			Flags:          to.IPv4.Flags(),
			FragmentOffset: to.IPv4.FragmentOffset(),
			TTL:            to.IPv4.TTL(),
			Protocol:       to.IPv4.Protocol(),
			SrcAddr:        to.IPv4.DestinationAddress(),
			DstAddr:        to.IPv4.SourceAddress(),
		})
	case header.IPv6Version:
		pk.IPv6.SetSourceAddress(to.IPv6.DestinationAddress())
		pk.IPv6.SetDestinationAddress(to.IPv6.SourceAddress())
	default:
		err = &NotSupportedError{Version: pk.Version, Protocol: 0}
	}
	return
}

func New(eth bool, version int, proto tcpip.TransportProtocolNumber, options, playload int) (pk *Packet, err error) {
	length := 0
	var ipOffset, protoOffset, playloadOffset int
	if eth {
		length += header.EthernetMinimumSize
	}
	ipOffset = length
	switch version {
	case header.IPv4Version:
		length += header.IPv4MinimumSize
		protoOffset = length
		switch proto {
		case header.ICMPv4ProtocolNumber:
			length += header.ICMPv4MinimumSize
		case header.TCPProtocolNumber:
			length += header.TCPMinimumSize + options
		case header.UDPProtocolNumber:
			length += header.UDPMinimumSize
		default:
			err = &NotSupportedError{Version: pk.Version, Protocol: proto}
		}
	case header.IPv6Version:
		length += header.IPv6MinimumSize
		protoOffset = length
		switch proto {
		case header.ICMPv6ProtocolNumber:
			length += header.ICMPv6MinimumSize
		case header.TCPProtocolNumber:
			length += header.TCPMinimumSize
		case header.UDPProtocolNumber:
			length += header.UDPMinimumSize
		default:
			err = &NotSupportedError{Version: pk.Version, Protocol: proto}
		}
	}
	if err != nil {
		return
	}
	playloadOffset = length
	length += playload
	buffer := make([]byte, length)
	pk = &Packet{Buffer: buffer, Header: buffer[:playloadOffset], Data: buffer[playloadOffset:]}
	if eth {
		pk.Eth = header.Ethernet(buffer)
	}
	pk.Version, pk.Proto = version, proto
	switch pk.Version {
	case header.IPv4Version:
		pk.IPv4 = header.IPv4(buffer[ipOffset:])
		switch proto {
		case header.ICMPv4ProtocolNumber:
			pk.ICMPv4 = header.ICMPv4(buffer[protoOffset:])
		case header.TCPProtocolNumber:
			pk.TCP = header.TCP(buffer[protoOffset:])
		case header.UDPProtocolNumber:
			pk.UDP = header.UDP(buffer[protoOffset:])
		}
	case header.IPv6Version:
		pk.IPv6 = header.IPv6(buffer[ipOffset:])
		switch proto {
		case header.ICMPv6ProtocolNumber:
			pk.ICMPv6 = header.ICMPv6(buffer[protoOffset:])
		case header.TCPProtocolNumber:
			pk.TCP = header.TCP(buffer[protoOffset:])
		case header.UDPProtocolNumber:
			pk.UDP = header.UDP(buffer[protoOffset:])
			pk.UDP.Encode(&header.UDPFields{Length: uint16(playload)})
		}
	}
	return
}

func Wrap(buffer []byte, eth bool) (pk *Packet, err error) {
	offset := 0
	pk = &Packet{Buffer: buffer}
	if eth {
		pk.Eth = header.Ethernet(buffer)
		offset = header.EthernetMinimumSize
	}
	pk.Version = header.IPVersion(buffer[offset:])
	switch pk.Version {
	case header.IPv4Version:
		pk.IPv4 = header.IPv4(buffer[offset:])
		offset += header.IPv4MinimumSize
		pk.Proto = pk.IPv4.TransportProtocol()
		data := pk.IPv4.Payload()
		switch pk.Proto {
		case header.ICMPv4ProtocolNumber:
			offset += header.ICMPv4MinimumSize
			pk.ICMPv4 = header.ICMPv4(data)
			pk.Data = pk.ICMPv4.Payload()
		case header.TCPProtocolNumber:
			offset += header.TCPMinimumSize
			pk.TCP = header.TCP(data)
			pk.Data = pk.TCP.Payload()
		case header.UDPProtocolNumber:
			offset += header.UDPMinimumSize
			pk.UDP = header.UDP(data)
			pk.Data = pk.UDP.Payload()
		default:
			err = &NotSupportedError{Version: pk.Version, Protocol: pk.Proto}
		}
	case header.IPv6Version:
		pk.IPv6 = header.IPv6(buffer[offset:])
		offset += header.IPv6MinimumSize
		pk.Proto = pk.IPv6.TransportProtocol()
		data := pk.IPv6.Payload()
		switch pk.Proto {
		case header.ICMPv6ProtocolNumber:
			offset += header.ICMPv6MinimumSize
			pk.ICMPv6 = header.ICMPv6(data)
			pk.Data = pk.ICMPv6.Payload()
		case header.TCPProtocolNumber:
			offset += header.TCPMinimumSize
			pk.TCP = header.TCP(data)
			pk.Data = pk.TCP.Payload()
		case header.UDPProtocolNumber:
			offset += header.UDPMinimumSize
			pk.UDP = header.UDP(data)
			pk.Data = pk.UDP.Payload()
		default:
			err = &NotSupportedError{Version: pk.Version, Protocol: pk.Proto}
		}
	default:
		err = &NotSupportedError{Version: pk.Version, Protocol: 0}
	}
	if err == nil {
		pk.Header = buffer[0:offset]
	}
	return
}

//LocalAddr will return the packet destionation address
func (p *Packet) LocalAddr() (addr net.Addr) {
	switch p.Version {
	case header.IPv4Version:
		proto := p.IPv4.TransportProtocol()
		switch proto {
		case header.ICMPv4ProtocolNumber:
			addr = &net.UDPAddr{
				IP: []byte(p.IPv4.DestinationAddress()),
			}
		case header.UDPProtocolNumber:
			addr = &net.UDPAddr{
				IP:   []byte(p.IPv4.DestinationAddress()),
				Port: int(p.UDP.DestinationPort()),
			}
		case header.TCPProtocolNumber:
			addr = &net.TCPAddr{
				IP:   []byte(p.IPv4.DestinationAddress()),
				Port: int(p.TCP.DestinationPort()),
			}
		}
	case header.IPv6Version:
		proto := p.IPv6.TransportProtocol()
		switch proto {
		case header.ICMPv4ProtocolNumber:
			addr = &net.UDPAddr{
				IP: []byte(p.IPv6.DestinationAddress()),
			}
		case header.UDPProtocolNumber:
			addr = &net.UDPAddr{
				IP:   []byte(p.IPv6.DestinationAddress()),
				Port: int(p.UDP.DestinationPort()),
			}
		case header.TCPProtocolNumber:
			addr = &net.TCPAddr{
				IP:   []byte(p.IPv6.DestinationAddress()),
				Port: int(p.TCP.DestinationPort()),
			}
		}
	}
	return
}

//RemoteAddr will return the packet source address
func (p *Packet) RemoteAddr() (addr net.Addr) {
	switch p.Version {
	case header.IPv4Version:
		proto := p.IPv4.TransportProtocol()
		switch proto {
		case header.ICMPv4ProtocolNumber:
			addr = &net.UDPAddr{
				IP: []byte(p.IPv4.SourceAddress()),
			}
		case header.UDPProtocolNumber:
			addr = &net.UDPAddr{
				IP:   []byte(p.IPv4.SourceAddress()),
				Port: int(p.UDP.SourcePort()),
			}
		case header.TCPProtocolNumber:
			addr = &net.TCPAddr{
				IP:   []byte(p.IPv4.SourceAddress()),
				Port: int(p.TCP.SourcePort()),
			}
		}
	case header.IPv6Version:
		proto := p.IPv6.TransportProtocol()
		switch proto {
		case header.ICMPv4ProtocolNumber:
			addr = &net.UDPAddr{
				IP: []byte(p.IPv6.SourceAddress()),
			}
		case header.UDPProtocolNumber:
			addr = &net.UDPAddr{
				IP:   []byte(p.IPv6.SourceAddress()),
				Port: int(p.UDP.SourcePort()),
			}
		case header.TCPProtocolNumber:
			addr = &net.TCPAddr{
				IP:   []byte(p.IPv6.SourceAddress()),
				Port: int(p.TCP.SourcePort()),
			}
		}
	}
	return
}

func (p *Packet) String() string {
	info := ""
	if len(p.Eth) > 0 {
		info += fmt.Sprintf("Ethernet II, Src: %v, Dst: %v\n", p.Eth.SourceAddress(), p.Eth.DestinationAddress())
	}
	switch p.Version {
	case header.IPv4Version:
		info += fmt.Sprintf("Internet Protocol Version %v, Src: %v, Dst: %v, Len: %v\n",
			p.Version, p.IPv4.SourceAddress(), p.IPv4.DestinationAddress(), p.IPv4.PayloadLength(),
		)
		switch p.IPv4.TransportProtocol() {
		case header.ICMPv4ProtocolNumber:
			info += fmt.Sprintf("Internet Control Message Protocol, Len: %v\n",
				len(p.ICMPv4.Payload()),
			)
		case header.TCPProtocolNumber:
			info += fmt.Sprintf("Transmission Control Protocol, Src Port: %v, Dst Port: %v, Len: %v\n",
				p.TCP.SourcePort(), p.TCP.DestinationPort(), len(p.TCP.Payload()),
			)
		case header.UDPProtocolNumber:
			info += fmt.Sprintf("User Datagram Protocol, Src Port: %v, Dst Port: %v, Len: %v\n",
				p.UDP.SourcePort(), p.UDP.DestinationPort(), len(p.UDP.Payload()),
			)
		}
	}
	info = strings.TrimSpace(info)
	return info
}

func (p *Packet) UpdateChecksum() {
	switch p.Version {
	case header.IPv4Version:
		switch p.Proto {
		case header.UDPProtocolNumber:
			// Calculate the udp checksum and set it.
			xsum := header.PseudoHeaderChecksum(p.Proto, p.IPv4.SourceAddress(), p.IPv4.DestinationAddress(), uint16(len(p.UDP)))
			xsum = header.Checksum(p.UDP.Payload(), xsum)
			p.UDP.SetChecksum(0)
			p.UDP.SetChecksum(^p.UDP.CalculateChecksum(xsum))
		case header.TCPProtocolNumber:
			// Calculate the tcp checksum and set it.
			xsum := header.PseudoHeaderChecksum(p.Proto, p.IPv4.SourceAddress(), p.IPv4.DestinationAddress(), uint16(len(p.TCP)))
			xsum = header.Checksum(p.TCP.Payload(), xsum)
			p.TCP.SetChecksum(0)
			p.TCP.SetChecksum(^p.TCP.CalculateChecksum(xsum))
		case header.ICMPv4ProtocolNumber:
			// Calculate the icmp checksum and set it.
			p.ICMPv4.SetChecksum(0)
			p.ICMPv4.SetChecksum(^header.Checksum(p.ICMPv4, 0))
		}
		// Calculate the ip checksum and set it.
		p.IPv4.SetChecksum(0)
		p.IPv4.SetChecksum(^p.IPv4.CalculateChecksum())

	}
}

func (p *Packet) Verify() (err error) {
	switch p.Version {
	case header.IPv4Version:
		switch p.Proto {
		case header.UDPProtocolNumber:
			// Calculate the udp checksum and set it.
			xsum := header.PseudoHeaderChecksum(p.Proto, p.IPv4.SourceAddress(), p.IPv4.DestinationAddress(), uint16(len(p.UDP)))
			xsum = header.Checksum(p.UDP.Payload(), xsum)
			having := p.UDP.Checksum()
			p.UDP.SetChecksum(0)
			current := ^p.UDP.CalculateChecksum(xsum)
			p.UDP.SetChecksum(having)
			if having != current {
				return fmt.Errorf("udp checksum fail with expected 0x%x, but 0x%x", having, current)
			}
		case header.TCPProtocolNumber:
			// Calculate the tcp checksum and set it.
			xsum := header.PseudoHeaderChecksum(p.Proto, p.IPv4.SourceAddress(), p.IPv4.DestinationAddress(), uint16(len(p.TCP)))
			xsum = header.Checksum(p.TCP.Payload(), xsum)
			having := p.TCP.Checksum()
			p.TCP.SetChecksum(0)
			current := ^p.TCP.CalculateChecksum(xsum)
			p.TCP.SetChecksum(having)
			if having != current {
				return fmt.Errorf("tcp checksum fail with expected 0x%x, but 0x%x", having, current)
			}
		case header.ICMPv4ProtocolNumber:
			view := buffer.NewVectorisedView(1, []buffer.View{buffer.View(p.ICMPv4.Payload())})
			having := p.ICMPv4.Checksum()
			p.ICMPv4.SetChecksum(0)
			current := ^header.ICMPv4Checksum(p.ICMPv4, view)
			p.ICMPv4.SetChecksum(having)
			if having != current {
				return fmt.Errorf("icmp checksum fail with expected 0x%x, but 0x%x", having, current)
			}
		}
		// Calculate the ip checksum and set it.
		having := p.IPv4.Checksum()
		p.IPv4.SetChecksum(0)
		current := ^p.IPv4.CalculateChecksum()
		p.IPv4.SetChecksum(having)
		if having != current {
			return fmt.Errorf("ip checksum fail with expected 0x%x, but 0x%x", having, current)
		}
	}
	return nil
}

func EncodeSynOptions(synOptions *header.TCPSynOptions) []byte {
	opts := make([]byte, header.TCPOptionsMaximumSize)
	offset := 0
	offset += header.EncodeMSSOption(uint32(synOptions.MSS), opts)

	if synOptions.WS >= 0 {
		offset += header.EncodeWSOption(3, opts[offset:])
	}
	if synOptions.TS {
		offset += header.EncodeTSOption(synOptions.TSVal, synOptions.TSEcr, opts[offset:])
	}

	if synOptions.SACKPermitted {
		offset += header.EncodeSACKPermittedOption(opts[offset:])
	}

	paddingToAdd := 4 - offset%4
	// Now add any padding bytes that might be required to quad align the
	// options.
	for i := offset; i < offset+paddingToAdd; i++ {
		opts[i] = header.TCPOptionNOP
	}
	offset += paddingToAdd
	return opts[:offset]
}

func (p *Packet) Clone() (packet *Packet) {
	buf := make([]byte, len(p.Buffer))
	copy(buf, p.Buffer)
	packet, _ = Wrap(buf, len(p.Eth) > 0)
	return
}
