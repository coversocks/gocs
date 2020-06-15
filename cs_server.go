package gocs

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/coversocks/gocs/core"
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
	DNSServer       string            `json:"dns_server"`
}

var serverConf string
var serverConfDir string
var httpServer = map[string]*http.Server{}
var httpServerLck = sync.RWMutex{}

//StartServer by configure path
func StartServer(c string) (err error) {
	conf := &ServerConf{}
	err = core.ReadJSON(c, &conf)
	if err != nil {
		core.ErrorLog("Server read configure from %v fail with %v", c, err)
		return
	}
	serverConf = c
	serverConfDir = filepath.Dir(serverConf)
	core.SetLogLevel(conf.LogLevel)
	userFile := conf.UserFile
	if len(userFile) > 0 && !filepath.IsAbs(userFile) {
		userFile, _ = filepath.Abs(filepath.Join(serverConfDir, userFile))
	}
	auth := core.NewJSONFileAuth(conf.Manager, userFile)
	dialer := core.NewNetDialer("", conf.DNSServer)
	server := core.NewServer(core.DefaultBufferSize, dialer)
	mux := http.NewServeMux()
	mux.Handle("/ds", websocket.Handler(func(ws *websocket.Conn) {
		ok, err := auth.BasicAuth(ws.Request())
		if ok && err == nil {
			server.ProcConn(core.NewBaseConn(ws, server.BufferSize))
		} else {
			core.WarnLog("Server receive auth fail connection from %v", ws.RemoteAddr())
		}
		ws.Close()
	}))
	mux.HandleFunc("/manager/", auth.ListUser)
	mux.HandleFunc("/manager/addUser", auth.AddUser)
	mux.HandleFunc("/manager/removeUser", auth.RemoveUser)
	wait := sync.WaitGroup{}
	if len(conf.HTTPListenAddr) > 0 {
		core.InfoLog("Server start http server on %v", conf.HTTPListenAddr)
		var addrs []string
		addrs, err = parseListenAddr(conf.HTTPListenAddr)
		if err != nil {
			core.ErrorLog("Server start http server on %v fail with %v", conf.HTTPListenAddr, err)
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
					core.ErrorLog("Server http server on %v is stopped fail with %v", addr, rerr)
				}
				httpServerLck.Lock()
				delete(httpServer, fmt.Sprintf("%p", s))
				httpServerLck.Unlock()
				wait.Done()
			}(a)
		}
	}
	if len(conf.HTTPSListenAddr) > 0 {
		core.InfoLog("Server start https server on %v", conf.HTTPSListenAddr)
		var addrs []string
		addrs, err = parseListenAddr(conf.HTTPSListenAddr)
		if err != nil {
			core.ErrorLog("Server start https server on %v fail with %v", conf.HTTPSListenAddr, err)
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
					core.ErrorLog("Server https server on %v is stopped fail with %v", addr, rerr)
				}
				httpServerLck.Lock()
				delete(httpServer, fmt.Sprintf("%p", s))
				httpServerLck.Unlock()
				wait.Done()
			}(a)
		}
	}
	wait.Wait()
	core.InfoLog("Server all listener is stopped")
	return
}

//StopServer will stop running server
func StopServer() {
	core.InfoLog("Server stopping client listener")
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
