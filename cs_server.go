package gocs

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	proxyhttp "github.com/codingeasygo/util/proxy/http"
	proxysocks "github.com/codingeasygo/util/proxy/socks"
	"github.com/codingeasygo/util/runner"
	"github.com/codingeasygo/util/xcrypto"
	"github.com/codingeasygo/util/xio"
	"github.com/coversocks/gocs/core"
	"github.com/coversocks/gocs/udpgw"
	"golang.org/x/net/websocket"
)

//ServerConf is pojo for server configure
type ServerConf struct {
	HTTPAddr  string            `json:"http_addr"`
	HTTPSAddr string            `json:"https_addr"`
	HTTPSGen  int               `json:"https_gen"`
	HTTPSLen  int               `json:"https_len"`
	Manager   map[string]string `json:"manager"`
	UserFile  string            `json:"user_file"`
	LogLevel  int               `json:"log"`
	DNSServer string            `json:"dns_server"`
	Alias     core.MapAliasURI  `json:"alias"`
}

type httpServer struct {
	Server    *http.Server
	startTime time.Time
}

//Server is coversocks Sever implement
type Server struct {
	ConfPath    string
	Conf        ServerConf
	Dialer      xio.PiperDialer
	Server      *core.Server
	servers     map[string]*httpServer
	serversLock sync.RWMutex
	waiter      sync.WaitGroup
	mux         *http.ServeMux
	auth        *core.JSONFileAuth
}

//NewServer will return new Server
func NewServer(confPath string, conf ServerConf, dialer xio.PiperDialer) (server *Server) {
	server = &Server{
		ConfPath:    confPath,
		Conf:        conf,
		Dialer:      dialer,
		servers:     map[string]*httpServer{},
		serversLock: sync.RWMutex{},
		waiter:      sync.WaitGroup{},
	}
	return
}

//ServeHTTP is http.Handler implement
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

//Dial will dial remote by Dialer
func (s *Server) DialPiper(uri string, bufferSize int) (raw xio.Piper, err error) {
	if uri == "tcp://udpgw" {
		udpgw := udpgw.NewUDPGW()
		udpgw.DNS, err = net.ResolveUDPAddr("udp", s.Conf.DNSServer)
		raw = udpgw
	} else {
		raw, err = s.Dialer.DialPiper(uri, bufferSize)
	}
	return
}

func (s *Server) wsHandler(ws *websocket.Conn) {
	ok, err := s.auth.BasicAuth(ws.Request())
	if ok && err == nil {
		s.Server.ProcConn(ws)
	} else {
		core.WarnLog("Server receive auth fail connection from %v", ws.RemoteAddr())
	}
	ws.Close()
}

func (s *Server) httpStart() (err error) {
	InfoLog("Server starting http server on %v", s.Conf.HTTPAddr)
	addrs, err := parseListenAddr(s.Conf.HTTPAddr)
	if err != nil {
		ErrorLog("Server start http server on %v fail with %v", s.Conf.HTTPAddr, err)
		return
	}
	s.waiter.Add(len(addrs))
	for _, a := range addrs {
		go s.runServer(a, nil)
	}
	InfoLog("Server http server on %v is started", s.Conf.HTTPAddr)
	return
}

func (s *Server) httpsStart() (err error) {
	InfoLog("Server starting https server on %v", s.Conf.HTTPSAddr)
	addrs, err := parseListenAddr(s.Conf.HTTPSAddr)
	if err != nil {
		ErrorLog("Server start https server on %v fail with %v", s.Conf.HTTPSAddr, err)
		return
	}
	s.waiter.Add(len(addrs))
	for _, addr := range addrs {
		var cert tls.Certificate
		cert, _ = xcrypto.GenerateRSA(s.Conf.HTTPSLen)
		go s.runServer(addr, &cert)
	}
	InfoLog("Server https server on %v is started", s.Conf.HTTPSAddr)
	return
}

func (s *Server) runServer(addr string, cert *tls.Certificate) (err error) {
	srv := &http.Server{Addr: addr, Handler: s}
	s.serversLock.Lock()
	s.servers[addr] = &httpServer{Server: srv, startTime: time.Now()}
	s.serversLock.Unlock()
	if cert != nil {
		srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{*cert},
		}
		DebugLog("Server start tls server on %v", addr)
		err = srv.ListenAndServeTLS("", "")
	} else {
		DebugLog("Server start server on %v", addr)
		err = srv.ListenAndServe()
	}
	if err != nil {
		ErrorLog("Server http server on %v is stopped fail with %v", addr, err)
	}
	s.serversLock.Lock()
	delete(s.servers, addr)
	s.serversLock.Unlock()
	s.waiter.Done()
	return
}

//ProcRestart will process restart https server by timeout
func (s *Server) ProcRestart() (err error) {
	timeout := time.Duration(s.Conf.HTTPSGen) * time.Millisecond
	if timeout < 1 {
		timeout = time.Minute
	}
	var addr string
	var server *httpServer
	s.serversLock.Lock()
	for a, srv := range s.servers {
		if time.Now().Sub(srv.startTime) > timeout {
			addr, server = a, srv
			break
		}
	}
	s.serversLock.Unlock()
	if server == nil {
		err = runner.ErrNotTask
		return
	}
	InfoLog("Server https server on %v is restarting", addr)
	cert, err := xcrypto.GenerateRSA(s.Conf.HTTPSLen)
	if err == nil {
		server.Server.Close()
		time.Sleep(100 * time.Millisecond)
		s.waiter.Add(1)
		go s.runServer(addr, &cert)
	}
	return
}

//Start by configure path and raw dialer
func (s *Server) Start() (err error) {
	serverConfDir := filepath.Dir(s.ConfPath)
	InfoLog("Server using config from %v, work on %v, log level %v", s.ConfPath, serverConfDir, s.Conf.LogLevel)
	core.SetLogLevel(s.Conf.LogLevel)
	proxysocks.SetLogLevel(s.Conf.LogLevel)
	proxyhttp.SetLogLevel(s.Conf.LogLevel)
	userFile := s.Conf.UserFile
	if len(userFile) > 0 && !filepath.IsAbs(userFile) {
		userFile, _ = filepath.Abs(filepath.Join(serverConfDir, userFile))
	}
	s.auth = core.NewJSONFileAuth(s.Conf.Manager, userFile)
	s.Server = core.NewServer(core.DefaultBufferSize, s)
	s.mux = http.NewServeMux()
	s.mux.Handle("/cover", websocket.Handler(s.wsHandler))
	s.mux.HandleFunc("/manager/", s.auth.ListUser)
	s.mux.HandleFunc("/manager/addUser", s.auth.AddUser)
	s.mux.HandleFunc("/manager/removeUser", s.auth.RemoveUser)
	if len(s.Conf.HTTPAddr) < 1 && len(s.Conf.HTTPSAddr) < 1 {
		err = fmt.Errorf("http_addr and https_addr is empty")
		return
	}
	if len(s.Conf.HTTPAddr) > 0 {
		s.httpStart()
	}
	if len(s.Conf.HTTPSAddr) > 0 {
		s.httpsStart()
	}
	return
}

//Wait will wait all runner is stopped
func (s *Server) Wait() {
	s.waiter.Wait()
}

//Stop will stop running server
func (s *Server) Stop() {
	InfoLog("Server stopping client listener")
	s.serversLock.Lock()
	for addr, srv := range s.servers {
		srv.Server.Close()
		delete(s.servers, addr)
	}
	s.serversLock.Unlock()
}

func parseListenAddr(addr string) (addrs []string, err error) {
	addrParts := strings.SplitN(addr, ":", 2)
	if len(addrParts) < 2 {
		err = fmt.Errorf("invalid uri")
		return
	}
	return parsePortAddr(addrParts[0]+":", addrParts[1], "")
}

var server *Server

//StartServer by configure path
func StartServer(c string) (err error) {
	conf := ServerConf{}
	err = core.ReadJSON(c, &conf)
	if err != nil {
		ErrorLog("Server read configure from %v fail with %v", c, err)
		return
	}
	dialer := core.NewNetDialer("", conf.DNSServer)
	dialer.Alias = conf.Alias
	server = NewServer(c, conf, dialer)
	err = server.Start()
	return
}

//WaitServer will wait server
func WaitServer() {
	if server != nil {
		server.Wait()
	}
}

//StopServer will stop server
func StopServer() {
	if server != nil {
		server.Stop()
		server = nil
	}
}
