package main

import (
	"flag"
	"log"
	"os"
	"os/signal"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
)

var argConf string
var argRunServer bool
var argRunClient bool

func init() {
	flag.StringVar(&argConf, "f", "/etc/coversocks/coversocks.json", "the dark socket configure file")
	flag.BoolVar(&argRunServer, "s", false, "start dark socket server")
	flag.BoolVar(&argRunClient, "c", false, "start dark socket client")
}

func main() {
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.Ldate | log.Lmicroseconds)
	log.SetOutput(os.Stdout)
	if argRunServer {
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
