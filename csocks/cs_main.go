package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
)

var version = "v1.2.0"
var argConf string
var argRunServer bool
var argRunClient bool
var argRunVersion bool

func init() {
	flag.StringVar(&argConf, "f", "/etc/coversocks/coversocks.json", "the cover socket configure file")
	flag.BoolVar(&argRunServer, "s", false, "start cover socket server")
	flag.BoolVar(&argRunClient, "c", false, "start cover socket client")
	flag.BoolVar(&argRunVersion, "v", false, "show version, current is "+version)
}

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ldate | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
	if argRunVersion {
		fmt.Printf("%v\n", version)
	} else if argRunServer {
		err := gocs.StartServer(argConf)
		if err != nil {
			fmt.Printf("start server fail with %v\n", err)
			os.Exit(1)
		}
		go handlerServerKill()
		gocs.WaitServer()
		time.Sleep(300 * time.Millisecond)
	} else if argRunClient {
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
	signal.Notify(serverKillSignal, os.Kill, os.Interrupt)
	v := <-serverKillSignal
	core.WarnLog("Server receive kill signal:%v", v)
	gocs.StopServer()
}

var clientKillSignal chan os.Signal

func handlerClientKill() {
	clientKillSignal = make(chan os.Signal, 1000)
	signal.Notify(clientKillSignal, os.Kill, os.Interrupt)
	v := <-clientKillSignal
	core.WarnLog("Clien receive kill signal:%v", v)
	gocs.StopClient()
}
