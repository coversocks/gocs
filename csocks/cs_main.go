package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"

	"net/http"
	_ "net/http/pprof"
)

func init() {
	go func() {
		log.Println(http.ListenAndServe(":5060", nil))
	}()
}

var version = "v1.1.0"
var argConf string
var argRunServer bool
var argRunClient bool
var argRunVersion bool

func init() {
	flag.StringVar(&argConf, "f", "/etc/coversocks/coversocks.json", "the dark socket configure file")
	flag.BoolVar(&argRunServer, "s", false, "start dark socket server")
	flag.BoolVar(&argRunClient, "c", false, "start dark socket client")
	flag.BoolVar(&argRunVersion, "v", false, "show version, current is "+version)
}

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ldate | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
	if argRunVersion {
		fmt.Printf("%v\n", version)
	} else if argRunServer {
		go handlerServerKill()
		gocs.StartServer(argConf)
	} else if argRunClient {
		go handlerClientKill()
		gocs.StartClient(argConf)
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
