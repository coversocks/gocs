package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coversocks/golang/cs"
)

var clientConf string
var clientConfDir string
var client *cs.Client
var proxyServer *cs.SocksProxy
var managerServer *http.Server
var managerListener net.Listener
var abpPath = filepath.Join(execDir(), "abp.js")
var gfwListPath = filepath.Join(execDir(), "gfwlist.txt")
var userRulesPath = filepath.Join(execDir(), "user_rules.txt")
var gfwListURL = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"

//ClientServerConf is pojo for dark socks server configure
type ClientServerConf struct {
	Enable   bool     `json:"enable"`
	Name     string   `json:"name"`
	Address  []string `json:"address"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	LastUsed int      `json:"-"`
}

//ClientConf is pojo for dark socks client configure
type ClientConf struct {
	Servers     []*ClientServerConf `json:"servers"`
	SocksAddr   string              `json:"socks_addr"`
	HTTPAddr    string              `json:"http_addr"`
	ManagerAddr string              `json:"manager_addr"`
	Mode        string              `json:"mode"`
	LogLevel    int                 `json:"log"`
	WorkDir     string              `json:"work_dir"`
}

//Dial connection by remote
func (c *ClientConf) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	for _, conf := range c.Servers {
		if conf.Enable && len(conf.Address) > 0 {
			address := conf.Address[conf.LastUsed]
			conf.LastUsed = (conf.LastUsed + 1) % len(conf.Address)
			if len(conf.Username) > 0 && len(conf.Password) > 0 {
				if strings.Contains(address, "?") {
					address += fmt.Sprintf("&username=%v&password=%v", conf.Username, conf.Password)
				} else {
					address += fmt.Sprintf("?username=%v&password=%v", conf.Username, conf.Password)
				}
			}
			cs.InfoLog("Client start connect one channel to %v", conf.Name)
			raw, err = cs.WebsocketDialer("").Dial(address)
			if err == nil {
				cs.InfoLog("Client connect one channel to %v success", conf.Name)
				conn := cs.NewStringConn(raw)
				conn.Name = conf.Name
				raw = conn
				break
			} else {
				cs.WarnLog("Client connect one channel fail with %v", err)
			}
		}
	}
	if raw == nil {
		err = fmt.Errorf("server not found")
	}
	return
}

//PAC is http handler to get pac js
func (c *ClientConf) PAC(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/x-javascript")
	//
	abpRaw, err := ioutil.ReadFile(abpPath)
	if err != nil {
		cs.ErrorLog("PAC read apb.js fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	abpStr := string(abpRaw)
	//
	//rules
	gfwRules, err := readGfwlist()
	if err != nil {
		cs.ErrorLog("PAC read gfwlist.txt fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	userRules, _ := readUserRules()
	gfwRules = append(gfwRules, userRules...)
	gfwRulesJS, _ := json.Marshal(gfwRules)
	abpStr = strings.Replace(abpStr, "__RULES__", string(gfwRulesJS), 1)
	//
	//proxy address
	if proxyServer == nil || proxyServer.Listener == nil {
		cs.ErrorLog("PAC load fail with socks proxy server is not started")
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", "socks proxy server is not started")
		return
	}
	//
	// socksProxy.
	parts := strings.SplitN(proxyServer.Addr().String(), ":", -1)
	abpStr = strings.Replace(abpStr, "__SOCKS5ADDR__", "127.0.0.1", -1)
	abpStr = strings.Replace(abpStr, "__SOCKS5PORT__", parts[len(parts)-1], -1)
	res.Write([]byte(abpStr))
}

//ChangeProxyMode is http handler to change proxy mode
func (c *ClientConf) ChangeProxyMode(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	_, err := changeProxyMode(mode)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "%v", err)
		return
	}
	c.Mode = mode
	err = cs.WriteJSON(clientConf, c)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "%v", err)
		return
	}
	fmt.Fprintf(w, "%v", "ok")
}

//UpdateGfwlist is http handler to update gfwlist.txt
func (c *ClientConf) UpdateGfwlist(w http.ResponseWriter, r *http.Request) {
	err := updateGfwlist()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "%v", err)
		return
	}
	fmt.Fprintf(w, "%v", "ok")
}

func startClient(c string) (err error) {
	conf := &ClientConf{Mode: "auto"}
	err = cs.ReadJSON(c, &conf)
	if err != nil {
		cs.ErrorLog("Client read configure fail with %v", err)
		exitf(1)
		return
	}
	if len(conf.SocksAddr) < 1 {
		cs.ErrorLog("Client socks_addr is required")
		exitf(1)
		return
	}
	clientConf = c
	clientConfDir = filepath.Dir(clientConf)
	if len(conf.WorkDir) > 0 {
		os.MkdirAll(workDir, os.ModePerm)
		workDir = conf.WorkDir
	}
	cs.SetLogLevel(conf.LogLevel)
	cs.InfoLog("Client using config from %v", c)
	client = cs.NewClient(cs.DefaultBufferSize, conf)
	proxyServer = cs.NewSocksProxy()
	proxyServer.Dialer = func(target string, raw io.ReadWriteCloser) (sid uint64, err error) {
		err = client.ProcConn(raw, target)
		return
	}
	if len(conf.ManagerAddr) > 0 {
		mux := http.NewServeMux()
		mux.HandleFunc("/pac.js", conf.PAC)
		mux.HandleFunc("/changeProxyMode", conf.ChangeProxyMode)
		mux.HandleFunc("/updateGfwlist", conf.UpdateGfwlist)
		var listener net.Listener
		managerServer = &http.Server{Addr: conf.ManagerAddr, Handler: mux}
		listener, err = net.Listen("tcp", conf.ManagerAddr)
		if err != nil {
			cs.ErrorLog("Client start web server fail with %v", err)
			exitf(1)
			return
		}
		managerServer.Addr = listener.Addr().String()
		managerListener = &cs.TCPKeepAliveListener{TCPListener: listener.(*net.TCPListener)}
	}
	err = proxyServer.Listen(conf.SocksAddr)
	if err != nil {
		cs.ErrorLog("Client start proxy server fail with %v", err)
		exitf(1)
		return
	}
	changeProxyMode(conf.Mode)
	// writeRuntimeVar()
	wait := sync.WaitGroup{}
	if managerServer != nil {
		wait.Add(1)
		cs.InfoLog("Client start web server on %v", managerListener.Addr())
		go func() {
			xerr := managerServer.Serve(managerListener)
			cs.WarnLog("Client the web server on %v is stopped by %v", managerListener.Addr(), xerr)
			wait.Done()
		}()
	}
	if len(conf.HTTPAddr) > 0 {
		wait.Add(1)
		proxyServer.HTTPUpstream = conf.HTTPAddr
		go func() {
			xerr := runPrivoxy(conf.HTTPAddr)
			cs.WarnLog("Client the privoxy on %v is stopped by %v", conf.HTTPAddr, xerr)
			wait.Done()
		}()
	}
	go handlerClientKill()
	proxyServer.Run()
	cs.InfoLog("Client all listener is stopped")
	changeProxyMode("manual")
	wait.Wait()
	return
}

func stopClient() {
	cs.InfoLog("Client stopping client listener")
	if proxyServer != nil {
		proxyServer.Close()
	}
	if managerServer != nil {
		managerServer.Close()
	}
	if privoxyRunner != nil && privoxyRunner.Process != nil {
		privoxyRunner.Process.Kill()
	}
	if client != nil {
		client.Close()
	}
}

var clientKillSignal chan os.Signal

func handlerClientKill() {
	clientKillSignal = make(chan os.Signal, 1000)
	signal.Notify(clientKillSignal, os.Kill, os.Interrupt)
	v := <-clientKillSignal
	cs.WarnLog("Clien receive kill signal:%v", v)
	stopClient()
}

func changeProxyMode(mode string) (message string, err error) {
	if proxyServer == nil || proxyServer.Listener == nil || managerServer == nil {
		err = fmt.Errorf("proxy server is not started")
		return
	}
	proxyServerParts := strings.Split(proxyServer.Addr().String(), ":")
	managerServerParts := strings.Split(managerServer.Addr, ":")
	switch mode {
	case "auto":
		pacURL := fmt.Sprintf("http://127.0.0.1:%v/pac.js?timestamp=%v", managerServerParts[len(managerServerParts)-1], time.Now().Local().UnixNano()/1e6)
		cs.InfoLog("start change proxy mode to %v by %v", mode, pacURL)
		message, err = changeProxyModeNative("auto", pacURL)
	case "global":
		cs.InfoLog("start change proxy mode to %v by 127.0.0.1:%v", mode, proxyServerParts[len(proxyServerParts)-1])
		message, err = changeProxyModeNative("global", "127.0.0.1", proxyServerParts[len(proxyServerParts)-1])
	default:
		message, err = changeProxyModeNative("manual")
	}
	if err != nil {
		cs.WarnLog("change proxy mode to %v fail with %v, the log is\n%v\n", mode, err, message)
	} else {
		cs.InfoLog("change proxy mode to %v is success", mode)
	}
	return
}

func readGfwlist() (rules []string, err error) {
	gfwFile := filepath.Join(workDir, "gfwlist.txt")
	gfwRaw, err := ioutil.ReadFile(gfwFile)
	if err != nil {
		gfwFile = gfwListPath
		gfwRaw, err = ioutil.ReadFile(gfwFile)
		if err != nil {
			err = fmt.Errorf("read gfwlist.txt fail with %v", err)
			return
		}
	}
	gfwData, err := base64.StdEncoding.DecodeString(string(gfwRaw))
	if err != nil {
		err = fmt.Errorf("decode gfwlist.txt fail with %v", err)
		return
	}
	gfwRulesAll := strings.Split(string(gfwData), "\n")
	for _, rule := range gfwRulesAll {
		if strings.HasPrefix(rule, "[") || strings.HasPrefix(rule, "!") || len(strings.TrimSpace(rule)) < 1 {
			continue
		}
		rules = append(rules, rule)
	}
	return
}

func readUserRules() (rules []string, err error) {
	gfwFile := filepath.Join(workDir, "user_rules.txt")
	gfwData, err := ioutil.ReadFile(gfwFile)
	if err != nil {
		gfwFile = userRulesPath
		gfwData, err = ioutil.ReadFile(gfwFile)
		if err != nil {
			err = fmt.Errorf("read gfwlist.txt fail with %v", err)
			return
		}
	}
	gfwRulesAll := strings.Split(string(gfwData), "\n")
	for _, rule := range gfwRulesAll {
		rule = strings.TrimSpace(rule)
		if strings.HasPrefix(rule, "--") || strings.HasPrefix(rule, "!") || len(strings.TrimSpace(rule)) < 1 {
			continue
		}
		rules = append(rules, rule)
	}
	return
}

func updateGfwlist() (err error) {
	if client == nil {
		err = fmt.Errorf("proxy server is not started")
		return
	}
	gfwData, err := client.HTTPGet(gfwListURL)
	if err != nil {
		return
	}
	os.MkdirAll(workDir, os.ModePerm)
	gfwFile := filepath.Join(workDir, "gfwlist.txt")
	err = ioutil.WriteFile(gfwFile, gfwData, os.ModePerm)
	return
}

const (
	//PrivoxyTmpl is privoxy template
	PrivoxyTmpl = `
listen-address {http}
toggle  1
enable-remote-toggle 1
enable-remote-http-toggle 1
enable-edit-actions 0
enforce-blocks 0
buffer-limit 4096
forwarded-connect-retries  0
accept-intercepted-requests 0
allow-cgi-request-crunching 0
split-large-forms 0
keep-alive-timeout 5
socket-timeout 60

forward-socks5 / {socks5} .
forward         192.168.*.*/     .
forward         10.*.*.*/        .
forward         127.*.*.*/       .

	`
)

func writePrivoxyConf(confFile, httpAddr, socksAddr string) (err error) {
	data := PrivoxyTmpl
	data = strings.Replace(data, "{http}", httpAddr, 1)
	data = strings.Replace(data, "{socks5}", socksAddr, 1)
	err = ioutil.WriteFile(confFile, []byte(data), os.ModePerm)
	return
}

func runPrivoxy(httpAddr string) (err error) {
	proxyServerParts := strings.SplitN(proxyServer.Addr().String(), ":", -1)
	socksAddr := fmt.Sprintf("127.0.0.1:%v", proxyServerParts[len(proxyServerParts)-1])
	cs.InfoLog("Client start privoxy by listening http proxy on %v and forwarding to %v", httpAddr, socksAddr)
	confFile := filepath.Join(workDir, "privoxy.conf")
	err = writePrivoxyConf(confFile, httpAddr, socksAddr)
	if err != nil {
		cs.WarnLog("Client save privoxy config to %v fail with %v", confFile, err)
		return
	}
	err = runPrivoxyNative(confFile)
	return
}
