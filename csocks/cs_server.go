package main

import (
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/coversocks/golang/cs"
	"golang.org/x/net/websocket"
)

//ServerConf is pojo for server configure
type ServerConf struct {
	HTTPListenAddr  string            `json:"http_listen_addr"`
	HTTPSListenAddr string            `json:"https_listen_addr"`
	HTTPSCert       string            `json:"https_cert"`
	HTTPSKey        string            `json:"https_key"`
	Manager         map[string]string `json:"manager"`
	UserFile        string            `json:"user_file"`
	LogLevel        int               `json:"log"`
}

var serverConf string
var serverConfDir string
var httpServer = map[string]*http.Server{}
var httpServerLck = sync.RWMutex{}

func startServer(c string) (err error) {
	conf := &ServerConf{}
	err = cs.ReadJSON(c, &conf)
	if err != nil {
		cs.ErrorLog("Server read configure from %v fail with %v", c, err)
		exitf(1)
		return
	}
	serverConf = c
	serverConfDir = filepath.Dir(serverConf)
	cs.SetLogLevel(conf.LogLevel)
	userFile := conf.UserFile
	if len(userFile) > 0 && !filepath.IsAbs(userFile) {
		userFile, _ = filepath.Abs(filepath.Join(serverConfDir, userFile))
	}
	auth := cs.NewJSONFileAuth(conf.Manager, userFile)
	server := cs.NewServer(cs.DefaultBufferSize, cs.NetDialer("tcp"))
	mux := http.NewServeMux()
	mux.Handle("/ds", websocket.Handler(func(ws *websocket.Conn) {
		ok, err := auth.BasicAuth(ws.Request())
		if ok && err == nil {
			server.ProcConn(cs.NewBaseConn(ws, server.BufferSize))
		} else {
			cs.WarnLog("Server receive auth fail connection from %v", ws.RemoteAddr())
		}
		ws.Close()
	}))
	mux.HandleFunc("/manager/", auth.ListUser)
	mux.HandleFunc("/manager/addUser", auth.AddUser)
	mux.HandleFunc("/manager/removeUser", auth.RemoveUser)
	wait := sync.WaitGroup{}
	if len(conf.HTTPListenAddr) > 0 {
		cs.InfoLog("Server start http server on %v", conf.HTTPListenAddr)
		var addrs []string
		addrs, err = parseListenAddr(conf.HTTPListenAddr)
		if err != nil {
			cs.ErrorLog("Server start http server on %v fail with %v", conf.HTTPListenAddr, err)
			exitf(1)
			return
		}
		wait.Add(len(addrs))
		for _, a := range addrs {
			go func(addr string) {
				s := &http.Server{Addr: addr, Handler: mux}
				httpServerLck.Lock()
				httpServer[fmt.Sprintf("%p", s)] = s
				httpServerLck.Unlock()
				rerr := s.ListenAndServe()
				if rerr != nil {
					cs.ErrorLog("Server http server on %v is stopped fail with %v", addr, rerr)
				}
				httpServerLck.Lock()
				delete(httpServer, fmt.Sprintf("%p", s))
				httpServerLck.Unlock()
				wait.Done()
			}(a)
		}
	}
	if len(conf.HTTPSListenAddr) > 0 {
		cs.InfoLog("Server start https server on %v", conf.HTTPSListenAddr)
		var addrs []string
		addrs, err = parseListenAddr(conf.HTTPSListenAddr)
		if err != nil {
			cs.ErrorLog("Server start https server on %v fail with %v", conf.HTTPSListenAddr, err)
			exitf(1)
			return
		}
		certFile, certKey := conf.HTTPSCert, conf.HTTPSKey
		if !filepath.IsAbs(certFile) {
			certFile, _ = filepath.Abs(filepath.Join(serverConfDir, certFile))
		}
		if !filepath.IsAbs(certKey) {
			certKey, _ = filepath.Abs(filepath.Join(serverConfDir, certKey))
		}
		wait.Add(len(addrs))
		for _, a := range addrs {
			go func(addr string) {
				s := &http.Server{Addr: addr, Handler: mux}
				httpServerLck.Lock()
				httpServer[fmt.Sprintf("%p", s)] = s
				httpServerLck.Unlock()
				rerr := s.ListenAndServeTLS(certFile, certKey)
				if rerr != nil {
					cs.ErrorLog("Server https server on %v is stopped fail with %v", addr, rerr)
				}
				httpServerLck.Lock()
				delete(httpServer, fmt.Sprintf("%p", s))
				httpServerLck.Unlock()
				wait.Done()
			}(a)
		}
	}
	go handlerServerKill()
	wait.Wait()
	cs.InfoLog("Server all listener is stopped")
	return
}

func stopServer() {
	cs.InfoLog("Server stopping client listener")
	httpServerLck.Lock()
	for _, s := range httpServer {
		s.Close()
	}
	httpServerLck.Unlock()
}

func parseListenAddr(addr string) (addrs []string, err error) {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) < 2 {
		err = fmt.Errorf("invalid uri")
		return
	}
	ports := strings.SplitN(parts[1], "-", 2)
	start, err := strconv.ParseInt(ports[0], 10, 32)
	if err != nil {
		return
	}
	end := start
	if len(ports) > 1 {
		end, err = strconv.ParseInt(ports[1], 10, 32)
		if err != nil {
			return
		}
	}
	for i := start; i <= end; i++ {
		addrs = append(addrs, fmt.Sprintf("%v:%v", parts[0], i))
	}
	return
}

var serverKillSignal chan os.Signal

func handlerServerKill() {
	serverKillSignal = make(chan os.Signal, 1000)
	signal.Notify(serverKillSignal, os.Kill, os.Interrupt)
	v := <-serverKillSignal
	cs.WarnLog("Server receive kill signal:%v", v)
	stopServer()
}
