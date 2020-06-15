package main

import (
	"bufio"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/netstack"
	"github.com/coversocks/gocs/netstack/pcap"
	"github.com/coversocks/gocs/netstack/tcpip"
	"github.com/google/netstack/tcpip/link/rawfile"
	"github.com/google/netstack/tcpip/link/tun"

	_ "net/http/pprof"
)

func init() {
	go func() {
		http.ListenAndServe(":6060", nil)
	}()
}

var record *pcap.Writer

//FD is file descriptor read/write/close
type FD int

func (f FD) Write(p []byte) (n int, err error) {
	n = len(p)
	nerr := rawfile.NonBlockingWrite(int(f), p)
	if nerr != nil {
		err = tcpip.NewValueError(nerr)
	}
	fmt.Printf("FD write %v by %x\n", n, p)
	return
}

func (f FD) Read(p []byte) (n int, err error) {
	n, nerr := rawfile.BlockingRead(int(f), p)
	if nerr != nil {
		err = tcpip.NewValueError(nerr)
	}
	return
}

//Close will close file descriptor
func (f FD) Close() (err error) {
	return
}

func main() {
	var tunName, task, unknow, bind, conf, dump, errdump string
	var debug int
	var count bool
	flag.StringVar(&tunName, "dev", "tap2", "the tap device")
	flag.StringVar(&task, "task", "echo", "the runner action")
	flag.StringVar(&unknow, "unknow", "", "the unknow record file")
	flag.StringVar(&bind, "bind", "", "the bind address for vpn")
	flag.StringVar(&conf, "conf", "default-client.json", "the configure file")
	flag.StringVar(&dump, "dump", "dump.pcap", "the pcap dump file")
	flag.StringVar(&errdump, "err", "", "the pcap dump file to save error packet")
	flag.IntVar(&debug, "debug", 0, "show debug")
	flag.BoolVar(&count, "count", false, "show count")
	flag.Parse()
	fmt.Printf("ntest start by tun:%v,task:%v,unknow:%v,debug:%v\n", tunName, task, unknow, debug)
	tcpip.SetLogLevel(tcpip.LogLevelDebug)
	mtu, err := rawfile.GetMTU(tunName)
	if err != nil {
		panic(err)
	}
	fd, err := tun.Open(tunName)
	if err != nil {
		panic(err)
	}
	fdio := os.NewFile(uintptr(fd), tunName)
	if len(unknow) > 0 {
		record, err = pcap.NewWriter(unknow)
		if err != nil {
			panic(err)
		}
	}
	if task == "dump" {
		out, err := pcap.NewWriter(dump)
		if err != nil {
			panic(err)
		}
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			out.Close()
			fdio.Close()
		}()
		buf := make([]byte, mtu*2)
		var n int
		for {
			n, err = fdio.Read(buf)
			if err != nil {
				break
			}
			_, err = out.Write(buf[0:n])
			if err != nil {
				break
			}
		}
		fmt.Printf("done by %v\n", err)
		return
	}
	if task == "write" {
		data, _ := hex.DecodeString("45100028000040004006e293ac120006ac1200020064ee365f3f16567537d839500400e5a52d0000")
		packet, _ := tcpip.Wrap(data, false)
		srcAddr, dstAddr := packet.IPv4.SourceAddress(), packet.IPv4.DestinationAddress()
		srcPort, dstPort := packet.TCP.SourcePort(), packet.TCP.DestinationPort()
		packet.IPv4.SetSourceAddress(dstAddr)
		packet.IPv4.SetDestinationAddress(srcAddr)
		packet.TCP.SetSourcePort(dstPort)
		packet.TCP.SetDestinationPort(srcPort)
		packet.UpdateChecksum()
		fdio.Write(packet.Buffer)
		return
	}
	if task == "vpn" {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		var errWriter io.WriteCloser
		if len(errdump) > 0 {
			errWriter, err = pcap.NewWriter(errdump)
			if err != nil {
				panic(err)
			}
		}
		dialer := core.NewNetDialer(bind, "114.114.114.114:53")
		netProxy := netstack.NewNetProxy(core.NewDialerWrapper(dialer))
		netProxy.MTU = int(mtu)
		err = netProxy.Bootstrap(conf)
		if err != nil {
			panic(err)
		}
		netProxy.SetWriter(tcpip.NewPacketPrintRWC(debug == 2, "Out", fdio, netProxy.Eth))
		if errWriter != nil {
			netProxy.SetErrWriter(errWriter)
		}
		netProxy.StartProcessTimeout()
		go func() {
			<-c
			if errWriter != nil {
				errWriter.Close()
			}
			fdio.Close()
		}()
		netProxy.ProcessReader(tcpip.NewPacketPrintRWC(debug == 2, "In", fdio, netProxy.Eth))
		return
	}
	l := tcpip.NewStack(false, nil)
	l.MTU = int(mtu)
	l.SetWriter(tcpip.NewPacketPrintRWC(debug == 2, "Out", fdio, l.Eth))
	in := tcpip.NewPacketPrintRWC(debug == 2, "In", fdio, l.Eth)
	unknowHandler := func(packet *tcpip.Packet) {
		if record != nil {
			record.Write(packet.Buffer)
		}
	}
	var totalRecved, totalSended uint64
	if count {
		go func() {
			for {
				recved, sended := totalRecved, totalSended
				time.Sleep(time.Second)
				fmt.Printf("current recved:%v/%v,sended:%v/%v\n",
					totalRecved-recved, time.Second, totalSended-sended, time.Second)
			}
		}()
	}
	switch task {
	case "close":
		l.Handler = tcpip.AcceperF(func(conn net.Conn, unknow *tcpip.Packet) {
			if unknow != nil {
				unknowHandler(unknow)
				return
			}
			go func() {
				time.Sleep(2 * time.Second)
				conn.Close()
				fmt.Printf("close stop by %v\n", err)
			}()
		})
	case "recv":
		l.Handler = tcpip.AcceperF(func(conn net.Conn, unknow *tcpip.Packet) {
			if unknow != nil {
				unknowHandler(unknow)
				return
			}
			go func() {
				raw := tcpip.NewPrintRWC(debug == 1, "Conn", conn)
				buf := make([]byte, 16*1024)
				for {
					n, err := raw.Read(buf)
					if err != nil {
						break
					}
					totalRecved += uint64(n)
				}
				fmt.Printf("recv stop by %v\n", err)
			}()
		})
	case "send":
		l.Handler = tcpip.AcceperF(func(conn net.Conn, unknow *tcpip.Packet) {
			if unknow != nil {
				unknowHandler(unknow)
				return
			}
			go func() {
				raw := tcpip.NewPrintRWC(debug == 1, "Conn", conn)
				i := 0
				for {
					n, err := fmt.Fprintf(raw, "send----->%v\n", i)
					if err != nil {
						break
					}
					totalSended += uint64(n)
					i++
				}
				fmt.Printf("send stop by %v\n", err)
			}()
		})
	case "echo":
		l.Handler = tcpip.AcceperF(func(conn net.Conn, unknow *tcpip.Packet) {
			if unknow != nil {
				unknowHandler(unknow)
				return
			}
			go func() {
				raw := tcpip.NewPrintRWC(debug == 1, "Conn", conn)
				buf := make([]byte, 16*1024)
				for {
					n, err := raw.Read(buf)
					if err != nil {
						break
					}
					_, err = raw.Write(buf[0:n])
					if err != nil {
						break
					}
					totalRecved += uint64(n)
					totalSended += uint64(n)
				}
				fmt.Printf("conn stop by %v\n", err)
			}()
		})
	case "http":
		listener := tcpip.NewChannelListener()
		httpMux := http.NewServeMux()
		var echoCount uint64
		go func() {
			for {
				lastc := echoCount
				time.Sleep(time.Second)
				fmt.Printf("echo speed by %v/%v\n", echoCount-lastc, time.Second)
			}
		}()
		httpMux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
			all, err := ioutil.ReadAll(r.Body)
			if err != nil {
				fmt.Printf("echo fail by %v\n", err)
				return
			}
			_, err = w.Write(all)
			if err != nil {
				fmt.Printf("echo fail by %v\n", err)
				return
			}
			atomic.AddUint64(&echoCount, 1)
			// fmt.Printf("echo success by %v\n", writed)
		})
		httpSrv := http.Server{Handler: httpMux}
		go httpSrv.Serve(listener)
		l.Handler = tcpip.AcceperF(func(conn net.Conn, unknow *tcpip.Packet) {
			if unknow != nil {
				unknowHandler(unknow)
				return
			}
			rwc := tcpip.NewWrapRWC(conn)
			rwc.Reader = bufio.NewReaderSize(conn, 1024*32)
			listener.Channel <- rwc
		})
	default:
		panic(task)
	}
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		<-c
		if record != nil {
			record.Close()
			record = nil
		}
		fdio.Close()
	}()
	err = l.ProcessReader(in)
	fmt.Printf("proc stop by %v\n", err)
}
