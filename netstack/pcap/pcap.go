// Package pcap provider reader for pcap file
// for more info: https://wiki.wireshark.org/Development/LibpcapFileFormat
package pcap

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/coversocks/gocs/core"
)

//FileHeader global file header
type FileHeader struct {
	MagicNumber  uint32 /* magic number */
	VersionMajor uint16 /* major version number */
	VersionMinor uint16 /* minor version number */
	Thiszone     int32  /* GMT to local correction */
	Sigfigs      uint32 /* accuracy of timestamps */
	Snaplen      uint32 /* max length of captured packets, in octets */
	Network      uint32 /* data link type */
}

//PacketHeader packet header
type PacketHeader struct {
	TsSec   uint32 /* timestamp seconds */
	TsUsec  uint32 /* timestamp microseconds */
	InclLen uint32 /* number of octets of packet saved in file */
	OrigLen uint32 /* actual length of packet */
}

//Encode header to data
func (p *PacketHeader) Encode() (data []byte) {
	data = make([]byte, 16)
	binary.LittleEndian.PutUint32(data[0:4], p.TsSec)
	binary.LittleEndian.PutUint32(data[4:8], p.TsUsec)
	binary.LittleEndian.PutUint32(data[8:12], p.InclLen)
	binary.LittleEndian.PutUint32(data[12:16], p.OrigLen)
	return
}

//Reader is impl reader pcap file
type Reader struct {
	File   *os.File
	Header FileHeader
	Limit  int
	readed int
}

//NewReader will create new Reader by filename
func NewReader(filename string) (reader *Reader, err error) {
	reader = &Reader{}
	reader.File, err = os.Open(filename)
	if err == nil {
		err = binary.Read(reader.File, binary.LittleEndian, &reader.Header)
	}
	return
}

//ReadPacket will ead next packet. returns header,data,error
func (r *Reader) ReadPacket() (header *PacketHeader, data []byte, err error) {
	if r.Limit > 0 && r.readed >= r.Limit {
		err = io.EOF
		return
	}
	r.readed++
	header = &PacketHeader{}
	err = binary.Read(r.File, binary.LittleEndian, header)
	if err == nil {
		data = make([]byte, header.InclLen)
		err = binary.Read(r.File, binary.LittleEndian, &data)
	}
	return
}

func (r *Reader) Read(p []byte) (n int, err error) {
	_, data, err := r.ReadPacket()
	if err != nil {
		return
	}
	if len(data) > len(p) {
		err = fmt.Errorf("buffer is too small, expected %v, but %v", len(data), len(p))
		return
	}
	n = copy(p, data)
	return
}

//Close pcap file
func (r *Reader) Close() error {
	return r.File.Close()
}

//Writer is pcap writer
type Writer struct {
	filename string
	File     *os.File
	Header   FileHeader
	Limit    int
	writed   int
	closed   bool
	locker   sync.RWMutex
}

//NewWriter will create new Reader by filename
func NewWriter(filename string) (w *Writer, err error) {
	w = &Writer{filename: filename, locker: sync.RWMutex{}}
	w.Header.MagicNumber = 0xa1b2c3d4
	w.Header.VersionMajor = 2
	w.Header.VersionMinor = 4
	w.Header.Snaplen = 262144
	w.Header.Network = 101
	w.File, err = os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.ModePerm)
	if err == nil {
		err = binary.Write(w.File, binary.LittleEndian, &w.Header)
	}
	return
}

//WritePacket will ead next packet. returns header,data,error
func (w *Writer) WritePacket(header *PacketHeader, data []byte) (err error) {
	if w.Limit > 0 && w.writed >= w.Limit {
		err = io.EOF
		return
	}
	w.locker.Lock()
	defer w.locker.Unlock()
	if w.closed {
		err = fmt.Errorf("closed")
		return
	}
	w.writed++
	_, err = w.File.Write(header.Encode())
	if err == nil {
		_, err = w.File.Write(data)
	}
	return
}

func (w *Writer) Write(p []byte) (n int, err error) {
	now := time.Now().Local().UnixNano() / 1e6
	err = w.WritePacket(&PacketHeader{
		TsSec:   uint32(now / 1000),
		TsUsec:  uint32(now % 1000),
		InclLen: uint32(len(p)),
		OrigLen: uint32(len(p)),
	}, p)
	return
}

//Close pcap file
func (w *Writer) Close() (err error) {
	w.locker.Lock()
	defer w.locker.Unlock()
	if w.closed {
		err = fmt.Errorf("closed")
		return
	}
	w.closed = true
	err = w.File.Close()
	return
}

//Dumper is pcap dumper to record packet to file
type Dumper struct {
	Base io.ReadWriteCloser
	Out  *Writer
}

//NewDumper will create new Dumper
func NewDumper(base io.ReadWriteCloser, filename string) (dumper *Dumper, err error) {
	dumper = &Dumper{Base: base}
	dumper.Out, err = NewWriter(filename)
	return
}

func (d *Dumper) Read(p []byte) (n int, err error) {
	n, err = d.Base.Read(p)
	if n > 0 && err == nil {
		core.DebugLog("Dumper record in pacakge with %v bytes", n)
		d.Out.Write(p[0:n])
	}
	return
}

func (d *Dumper) Write(p []byte) (n int, err error) {
	n, err = d.Base.Write(p)
	core.DebugLog("Dumper record out pacakge with %v bytes", n)
	d.Out.Write(p)
	return
}

//Close will close the dumper Writer and close base
func (d *Dumper) Close() (err error) {
	d.Out.Close()
	err = d.Base.Close()
	return
}

func (d *Dumper) String() string {
	return fmt.Sprintf("Dumper(%v)", d.Base)
}
