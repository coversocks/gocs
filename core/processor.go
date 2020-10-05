package core

// //OnReceivedF is function for data received
// type OnReceivedF func(io.ReadWriteCloser, []byte) (err error)

// //OnClosedF is function for connection closed
// type OnClosedF func(io.ReadWriteCloser) (err error)

// //ThroughReadeCloser is interface for sync read function
// type ThroughReadeCloser interface {
// 	Throughable() bool
// 	OnReceived(f OnReceivedF) (err error)
// 	OnClosed(f OnClosedF) (err error)
// }

// //AyncPiper is Processor impl, it will process connection by async
// type AyncPiper struct {
// 	Next xio.Piper
// }

// //NewAyncPiper will return new AyncPiper
// func NewAyncPiper(next xio.Piper) (proc *AyncPiper) {
// 	proc = &AyncPiper{
// 		Next: next,
// 	}
// 	return
// }

// //PipeConn will process connection async
// func (a *AyncPiper) PipeConn(raw io.ReadWriteCloser, target string) (err error) {
// 	go func() {
// 		err = a.Next.PipeConn(raw, target)
// 		if err != nil {
// 			DebugLog("AyncPiper pipe connection %v for %v fail with %v", raw, target, err)
// 		} else {
// 			DebugLog("AyncPiper pipe connection %v for %v is done", raw, target)
// 		}
// 		raw.Close()
// 	}()
// 	return
// }

// func (a *AyncPiper) String() string {
// 	return "AyncPiper"
// }

// //ProcConnDialer is ProcConn impl by dialer
// type ProcConnDialer struct {
// 	Dialer
// 	Through bool
// }

// //NewProcConnDialer will return new ProcConnDialer
// func NewProcConnDialer(through bool, dialer Dialer) (proc *ProcConnDialer) {
// 	proc = &ProcConnDialer{
// 		Through: through,
// 		Dialer:  dialer,
// 	}
// 	return
// }

// func copyClose(dst, src io.ReadWriteCloser, bufferSize int) {
// 	buf := make([]byte, bufferSize)
// 	_, err := io.CopyBuffer(dst, src, buf)
// 	DebugLog("ProcConnDialer connection %v is closed by %v", src, err)
// 	dst.Close()
// }

// //PipeConn process connection by dial
// func (p *ProcConnDialer) PipeConn(raw io.ReadWriteCloser, target string) (err error) {
// 	conn, err := p.Dial(target)
// 	if err != nil {
// 		return
// 	}
// 	var dstA, srcA, dstB, srcB io.ReadWriteCloser
// 	if through, ok := conn.(ThroughReadeCloser); p.Through && ok && through.Throughable() {
// 		InfoLog("%v is do throughable by %v,%v", conn, ok, ok && through.Throughable())
// 		through.OnReceived(func(r io.ReadWriteCloser, p []byte) (err error) {
// 			_, err = raw.Write(p)
// 			if err != nil {
// 				InfoLog("%v will close by send data to %v fail with %v", conn, raw, err)
// 				conn.Close()
// 			}
// 			return
// 		})
// 		through.OnClosed(func(r io.ReadWriteCloser) (err error) {
// 			err = raw.Close()
// 			return
// 		})
// 	} else {
// 		// WarnLog("%v is not do throughable by %v,%v", reflect.TypeOf(conn), ok, ok && through.Throughable())
// 		dstA, srcA = raw, conn
// 	}
// 	if through, ok := raw.(ThroughReadeCloser); p.Through && ok && through.Throughable() {
// 		InfoLog("%v is do throughable by %v,%v", raw, ok, ok && through.Throughable())
// 		through.OnReceived(func(r io.ReadWriteCloser, p []byte) (err error) {
// 			_, err = conn.Write(p)
// 			if err != nil {
// 				InfoLog("%v will close by send data to %v fail with %v", raw, conn, err)
// 				raw.Close()
// 			}
// 			return
// 		})
// 		through.OnClosed(func(r io.ReadWriteCloser) (err error) {
// 			err = conn.Close()
// 			return
// 		})
// 	} else {
// 		// WarnLog("%v is not do throughable by %v,%v", reflect.TypeOf(raw), ok, ok && through.Throughable())
// 		if dstA != nil {
// 			dstB, srcB = conn, raw
// 		} else {
// 			dstA, srcA = conn, raw
// 		}
// 	}
// 	if dstB != nil {
// 		go copyClose(dstB, srcB, 32*1024)
// 	}
// 	if dstA != nil {
// 		copyClose(dstA, srcA, 32*1024)
// 	}
// 	return
// }

// func (p *ProcConnDialer) String() string {
// 	return "ProcConnDialer"
// }

// //EchoPiper is Processor impl for echo connection
// type EchoPiper struct {
// 	Through bool
// }

// //NewEchoPiper will return new EchoProceessor
// func NewEchoPiper(through bool) (echo *EchoPiper) {
// 	return &EchoPiper{Through: through}
// }

// //PipeConn will process connection
// func (e *EchoPiper) PipeConn(raw io.ReadWriteCloser, target string) (err error) {
// 	if through, ok := raw.(ThroughReadeCloser); e.Through && ok && through.Throughable() {
// 		through.OnReceived(func(r io.ReadWriteCloser, p []byte) (err error) {
// 			_, err = raw.Write(p)
// 			return
// 		})
// 		return
// 	}
// 	InfoLog("EchoProceessor process connection %v", target)
// 	copyClose(raw, raw, 32*1024)
// 	return
// }

// func (e *EchoPiper) String() string {
// 	return "EchoPiper"
// }

// //PrintPiper is Processor impl for echo connection
// type PrintPiper struct {
// 	Next xio.Piper
// }

// //NewPrintPiper will return new EchoProceessor
// func NewPrintPiper(next xio.Piper) (print *PrintPiper) {
// 	return &PrintPiper{Next: next}
// }

// //PipeConn will process connection
// func (p *PrintPiper) PipeConn(r io.ReadWriteCloser, target string) (err error) {
// 	conn := xio.NewPrintConn("Print", r)
// 	err = p.Next.PipeConn(conn, target)
// 	return
// }

// //RemoteByAddr will generate remote address by net.Addr
// func RemoteByAddr(addr net.Addr) (remote string) {
// 	if udp, ok := addr.(*net.UDPAddr); ok {
// 		remote = fmt.Sprintf("udp://%v:%v", udp.IP, udp.Port)
// 	} else if tcp, ok := addr.(*net.TCPAddr); ok {
// 		remote = fmt.Sprintf("tcp://%v:%v", tcp.IP, tcp.Port)
// 	} else {
// 		remote = fmt.Sprintf("%v://%v", addr.Network(), addr)
// 	}
// 	return
// }
