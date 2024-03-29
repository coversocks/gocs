package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/udpgw"
)

var version = gocs.Version
var argConf string = "default-client.json"
var argRunServer bool
var argRunClient bool = true
var argRunVersion bool

func init() {
	flag.StringVar(&argConf, "f", "default-client.json", "the cover socket configure file")
	flag.BoolVar(&argRunServer, "s", false, "start cover socket server")
	flag.BoolVar(&argRunClient, "c", true, "start cover socket client")
	flag.BoolVar(&argRunVersion, "v", false, "show version, current is "+version)
	http.HandleFunc("/debug/udpgw", udpgw.StateH)
}

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ldate | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
	if argRunVersion {
		fmt.Printf("%v\n", version)
	} else if argRunServer {
		// go http.ListenAndServe(":6061", nil)
		log.Printf("boostrap coversock %v as server", version)
		err := gocs.StartServer(argConf)
		if err != nil {
			fmt.Printf("start server fail with %v\n", err)
			os.Exit(1)
		}
		go handlerServerKill()
		gocs.WaitServer()
		time.Sleep(300 * time.Millisecond)
	} else if argRunClient {
		// go http.ListenAndServe(":6062", nil)
		log.Printf("boostrap coversock %v as client", version)
		err := gocs.StartClient(argConf)
		if err != nil {
			fmt.Printf("start client fail with %v\n", err)
			os.Exit(1)
		}
		go handlerClientKill()
		gocs.WaitClient()
		time.Sleep(300 * time.Millisecond)
	} else {
		flag.Usage()
	}
}

var serverKillSignal chan os.Signal

func handlerServerKill() {
	serverKillSignal = make(chan os.Signal, 1000)
	signal.Notify(serverKillSignal, syscall.SIGTERM, os.Interrupt)
	v := <-serverKillSignal
	core.WarnLog("Server receive kill signal:%v", v)
	gocs.StopServer()
}

var clientKillSignal chan os.Signal

func handlerClientKill() {
	clientKillSignal = make(chan os.Signal, 1000)
	signal.Notify(clientKillSignal, syscall.SIGTERM, os.Interrupt)
	v := <-clientKillSignal
	core.WarnLog("Client receive kill signal:%v", v)
	gocs.StopClient()
}
