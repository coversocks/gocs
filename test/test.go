package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

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
			go runRemote(wg, os.Args[3], os.Args[4], 0)
			time.Sleep(100 * time.Millisecond)
		}
	} else {
		wg.Add(int(cc) + 1)
		for i := int64(0); i < cc; i++ {
			go runRemote(wg, os.Args[3], os.Args[4], time.Second)
			time.Sleep(100 * time.Millisecond)
		}
		for {
			<-done
			wg.Add(1)
			go runRemote(wg, os.Args[3], os.Args[4], time.Second)
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

func runRemote(wg *sync.WaitGroup, task, remote string, timeout time.Duration) {
	switch task {
	case "tcp":
		runConn(wg, task, remote, timeout)
	case "udp":
		runConn(wg, task, remote, timeout)
	case "verify":
		http.DefaultClient.Timeout = timeout
		runVerify(wg, remote, http.DefaultClient)
	}
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
		if true {
			return
		}
		i := 0
		var writed int
		var err error
		// data := make([]byte, 10240)
		for running {
			// writed, err = conn.Write(data)
			writed, err = fmt.Fprintf(conn, "send--->%v\n", i)
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

const letterBytes = ("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func randBytes(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return b
}

func runVerify(wg *sync.WaitGroup, remote string, client *http.Client) {
	var err error
	var reqData, resData []byte
	defer func() {
		wg.Done()
		done <- 1
		atomic.AddInt32(&conns, -1)
		if err == nil {
			atomic.AddUint64(&recved, uint64(len(resData)))
		}
	}()
	reqData = randBytes(rand.Intn(1024 * 16))
	atomic.AddInt32(&conns, 1)
	atomic.AddUint64(&sended, uint64(len(reqData)))
	req, err := http.NewRequest("POST", remote, bytes.NewBuffer(reqData))
	if err != nil {
		fmt.Printf("run verify fail with %v", err)
		return
	}
	defer func() {

	}()
	res, err := client.Do(req)
	if err != nil {
		fmt.Printf("run verify fail with %v", err)
		return
	}
	resData, err = ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("run verify fail with %v", err)
		return
	}
	// fmt.Printf("req:%v\nres:%v\n\n", len(reqData), len(resData))
	if !bytes.Equal(reqData, resData) {
		panic("verify fail")
	}
}

func runVerify2(wg *sync.WaitGroup, remote string, client *http.Client) {
	var err error
	var reqData, resData []byte
	defer func() {
		wg.Done()
		done <- 1
		atomic.AddInt32(&conns, -1)
		if err == nil {
			atomic.AddUint64(&recved, uint64(len(resData)))
		}
	}()
	reqData = randBytes(rand.Intn(1024 * 16))
	atomic.AddInt32(&conns, 1)
	atomic.AddUint64(&sended, uint64(len(reqData)))
	req, err := http.NewRequest("POST", remote, bytes.NewBuffer(reqData))
	if err != nil {
		fmt.Printf("run verify fail with %v", err)
		return
	}
	defer func() {

	}()
	res, err := client.Do(req)
	if err != nil {
		fmt.Printf("run verify fail with %v", err)
		return
	}
	resData, err = ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Printf("run verify fail with %v", err)
		return
	}
	// fmt.Printf("req:%v\nres:%v\n\n", len(reqData), len(resData))
	if !bytes.Equal(reqData, resData) {
		panic("verify fail")
	}
}
