package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"sync"
)

func main() {
	switch os.Args[1] {
	case "udp":
		echoUDP()
	case "tcp":
		echoTCP()
	case "http":
		echoHTTP()
	case "all":
		wg := sync.WaitGroup{}
		wg.Add(3)
		go func() {
			echoUDP()
			wg.Done()
		}()
		go func() {
			echoTCP()
			wg.Done()
		}()
		go func() {
			echoHTTP()
			wg.Done()
		}()
		wg.Wait()
	default:
		fmt.Printf("not suppored %v\n", os.Args[1])
	}
}

func echoUDP() {
	fmt.Printf("listen udp on :%v\n", 8021)
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 8021})
	if err != nil {
		panic(err)
	}
	buf := make([]byte, 1024*8)
	for {
		n, from, err := conn.ReadFrom(buf)
		if err != nil {
			break
		}
		conn.WriteTo(buf[0:n], from)
	}
}

func echoTCP() {
	fmt.Printf("listen tcp on :%v\n", 8022)
	l, err := net.Listen("tcp", ":8022")
	if err != nil {
		panic(err)
	}
	for {
		conn, err := l.Accept()
		if err != nil {
			break
		}
		go io.Copy(conn, conn)
	}
}

func echoHTTP() {
	fmt.Printf("listen http on :%v\n", 8023)
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		all, err := ioutil.ReadAll(r.Body)
		if err != nil {
			fmt.Printf("echo fail by %v\n", err)
			return
		}
		writed, err := w.Write(all)
		if err != nil {
			fmt.Printf("echo fail by %v\n", err)
			return
		}
		fmt.Printf("echo success by %v\n", writed)
	})
	fmt.Println(http.ListenAndServe(":8023", nil))
}
