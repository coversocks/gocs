package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var done = make(chan int, 10000)

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: test mode paralles type remote\n")
		return
	}
	running = true
	cc, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		panic(err)
	}
	wg := &sync.WaitGroup{}
	go showSended(wg)
	if os.Args[1] == "n" {
		wg.Add(int(cc) + 1)
		for i := int64(0); i < cc; i++ {
			go runConn(wg, os.Args[3], os.Args[4], 0)
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		wg.Add(int(cc) + 1)
		for i := int64(0); i < cc; i++ {
			go runConn(wg, os.Args[3], os.Args[4], time.Second)
			time.Sleep(100 * time.Millisecond)
		}
		for {
			<-done
			wg.Add(1)
			go runConn(wg, os.Args[3], os.Args[4], time.Second)
		}
	}
	wg.Wait()
}

var conns int32
var sended uint64
var recved uint64
var running bool

func showSended(wg *sync.WaitGroup) {
	var begin = uint64(time.Now().Local().UnixNano() / 1e6)
	for running {
		time.Sleep(time.Second)
		var now = uint64(time.Now().Local().UnixNano() / 1e6)
		fmt.Printf("C(%v) S(%v,%v/%v),R(%v,%v/%v),S-R(%v)\n", conns, sended,
			sended/(now-begin), time.Millisecond,
			recved, recved/(now-begin), time.Millisecond,
			sended-recved)
	}
	wg.Done()
}

func runConn(wg *sync.WaitGroup, netowrk, remote string, timeout time.Duration) {
	conn, cerr := net.Dial(netowrk, remote)
	if cerr != nil {
		fmt.Printf("connec error:%v\n", cerr)
		return
	}
	// fmt.Printf("connecton to %v://%v success\n", netowrk, remote)
	defer conn.Close()
	atomic.AddInt32(&conns, 1)
	if timeout > 0 {
		go func() {
			time.Sleep(timeout)
			conn.Close()
		}()
	}
	// var writeDone bool
	var totReaded, totWrited int
	go func() {
		i := 0
		var writed int
		var err error
		for running {
			writed, err = fmt.Fprintf(conn, "send-send->%v", i)
			if err != nil {
				break
			}
			atomic.AddUint64(&sended, uint64(writed))
			// time.Sleep(1000 * time.Nanosecond)
			i++
			totWrited += writed
			// if totWrited > 100000 {
			// 	writeDone = true
			// 	break
			// }
		}
		// fmt.Printf("conn write done with %v\n", err)
	}()
	{
		var readed int
		var err error
		buf := make([]byte, 1024)
		for {
			readed, err = conn.Read(buf)
			if err != nil {
				break
			}
			// fmt.Printf("read:%v\n", string(buf[0:readed]))
			atomic.AddUint64(&recved, uint64(readed))
			totReaded += readed
			// if writeDone && totReaded >= totWrited {
			// 	break
			// }
		}
		// fmt.Printf("conn read done with %v\n", err)
	}
	atomic.AddInt32(&conns, -1)
	wg.Done()
	done <- 1
}
