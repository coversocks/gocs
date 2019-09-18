package main

import (
	"flag"
	"log"
	"os"
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
		startServer(argConf)
	} else if argRunClient {
		startClient(argConf)
	} else {
		flag.Usage()
	}
}

var exitf = os.Exit
