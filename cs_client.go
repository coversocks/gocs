package gocs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/coversocks/gocs/core"
)

// var clientConf string
// var clientConfDir string
var client *Client
var proxyServer *core.SocksProxy
var managerServer *http.Server
var managerListener net.Listener

// var abpPath = filepath.Join(execDir(), "abp.js")
// var gfwListPath = filepath.Join(execDir(), "gfwlist.txt")
// var userRulesPath = filepath.Join(execDir(), "user_rules.txt")
var gfwListURL = "https://raw.githubusercontent.com/gfwlist/gfwlist/master/gfwlist.txt"

//ClientServerConf is pojo for dark socks server configure
type ClientServerConf struct {
	Enable   bool     `json:"enable"`
	Name     string   `json:"name"`
	Address  []string `json:"address"`
	Username string   `json:"username"`
	Password string   `json:"password"`
}

//ClientServerDialer is dialer by ClientServerConf
type ClientServerDialer struct {
	*ClientServerConf
	LastUsed int
	Base     core.Dialer
}

//Dial imp core.Dialer
func (c *ClientServerDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	address := c.Address[c.LastUsed]
	if len(c.Username) > 0 && len(c.Password) > 0 {
		if strings.Contains(address, "?") {
			address += fmt.Sprintf("&username=%v&password=%v", c.Username, c.Password)
		} else {
			address += fmt.Sprintf("?username=%v&password=%v", c.Username, c.Password)
		}
	}
	core.InfoLog("Client start connect one channel to %v-%v", c.Name, c.LastUsed)
	raw, err = c.Base.Dial(address)
	if err == nil {
		core.InfoLog("Client connect one channel to %v-%v success", c.Name, c.LastUsed)
		conn := core.NewStringConn(raw)
		conn.Name = c.Name
		raw = conn
	} else {
		core.WarnLog("Client connect one channel to %v-%v fail with %v", c.Name, c.LastUsed, err)
	}
	c.LastUsed = (c.LastUsed + 1) % len(c.Address)
	return
}

func (c *ClientServerDialer) String() string {
	return c.Name
}

//ClientConf is pojo for dark socks client configure
type ClientConf struct {
	Servers     []*ClientServerConf `json:"servers"`
	SocksAddr   string              `json:"socks_addr"`
	SocksPAC    string              `json:"socks_pac"`
	HTTPAddr    string              `json:"http_addr"`
	ManagerAddr string              `json:"manager_addr"`
	Mode        string              `json:"mode"`
	LogLevel    int                 `json:"log"`
	WorkDir     string              `json:"work_dir"`
}

//Client is dialer by ClientConf
type Client struct {
	*core.Client
	Conf     ClientConf
	WorkDir  string //current working dir
	ConfPath string
}

//Boostrap will initial setting
func (c *Client) Boostrap(base core.Dialer) (err error) {
	var dialers = []core.Dialer{}
	for _, conf := range c.Conf.Servers {
		if conf.Enable && len(conf.Address) > 0 {
			dialers = append(dialers, &ClientServerDialer{Base: base, ClientServerConf: conf})
		}
	}
	var dialer = core.NewSortedDialer(dialers...)
	c.Client = core.NewClient(core.DefaultBufferSize, dialer)
	core.InfoLog("Client boostrap with %v server dialer", len(dialers))
	return
}

//ReadGfwRules will read the gfwlist.txt and append user_rules
func (c *Client) ReadGfwRules() (rules []string, err error) {
	gfwFile := filepath.Join(c.WorkDir, "gfwlist.txt")
	userFile := filepath.Join(c.WorkDir, "user_rules.txt")
	// core.DebugLog("Client read gfw rule from %v", gfwFile)
	rules, err = core.ReadGfwlist(gfwFile)
	if err == nil {
		// core.DebugLog("Client read user rule from %v", userFile)
		userRules, _ := core.ReadUserRules(userFile)
		rules = append(rules, userRules...)
	}
	return
}

//UpdateGfwlist will update the gfwlist.txt
func (c *Client) UpdateGfwlist() (err error) {
	if c.Client == nil {
		err = fmt.Errorf("proxy server is not started")
		return
	}
	gfwData, err := c.Client.HTTPGet(gfwListURL)
	if err != nil {
		return
	}
	os.MkdirAll(c.WorkDir, os.ModePerm)
	gfwFile := filepath.Join(c.WorkDir, "gfwlist.txt")
	err = ioutil.WriteFile(gfwFile, gfwData, os.ModePerm)
	return
}

//PACH is http handler to get pac js
func (c *Client) PACH(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Content-Type", "application/x-javascript")
	//
	abpPath := filepath.Join(c.WorkDir, "abp.js")
	abpRaw, err := ioutil.ReadFile(abpPath)
	if err != nil {
		core.ErrorLog("PAC read apb.js fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	abpStr := string(abpRaw)
	//
	//rules
	gfwRules, err := c.ReadGfwRules()
	if err != nil {
		core.ErrorLog("PAC read gfwlist.txt fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	gfwRulesJS, _ := json.Marshal(gfwRules)
	abpStr = strings.Replace(abpStr, "__RULES__", string(gfwRulesJS), 1)
	//
	//proxy address
	if proxyServer == nil || proxyServer.Listener == nil {
		core.ErrorLog("PAC load fail with socks proxy server is not started")
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

//ChangeProxyModeH is http handler to change proxy mode
func (c *Client) ChangeProxyModeH(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	_, err := changeProxyMode(mode)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "%v", err)
		return
	}
	c.Conf.Mode = mode
	err = core.WriteJSON(c.ConfPath, c.Conf)
	if err != nil {
		w.WriteHeader(500)
		core.WarnLog("Client change proxy mode on config %v fail with %v", c.ConfPath, err)
		fmt.Fprintf(w, "%v", err)
		return
	}
	fmt.Fprintf(w, "%v", "ok")
}

//UpdateGfwlistH is http handler to update gfwlist.txt
func (c *Client) UpdateGfwlistH(w http.ResponseWriter, r *http.Request) {
	err := c.UpdateGfwlist()
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "%v", err)
		return
	}
	fmt.Fprintf(w, "%v", "ok")
}

//StateH is http handler to show client state
func (c *Client) StateH(w http.ResponseWriter, r *http.Request) {
	res := map[string]interface{}{}
	if d, ok := c.Client.Dialer.(core.Statable); ok {
		res["dialers"] = d.State()
	}
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	encoder := json.NewEncoder(w)
	encoder.Encode(res)
}

func (c *Client) String() string {
	return "CoverSocksClient"
}

//StartClient by configure
func StartClient(c string) (err error) {
	return StartDialerClient(c, core.NewWebsocketDialer())
}

//StartDialerClient by configure and dialer
func StartDialerClient(c string, base core.Dialer) (err error) {
	conf := ClientConf{Mode: "auto"}
	err = core.ReadJSON(c, &conf)
	if err != nil {
		core.ErrorLog("Client read configure fail with %v", err)
		return
	}
	if len(conf.SocksAddr) < 1 {
		core.ErrorLog("Client socks_addr is required")
		err = fmt.Errorf("Client socks_addr is required")
		return
	}
	// clientConf = c
	// clientConfDir = filepath.Dir(clientConf)
	var workDir = filepath.Dir(c)
	if len(conf.WorkDir) > 0 {
		if filepath.IsAbs(conf.WorkDir) {
			workDir = conf.WorkDir
		} else {
			workDir = filepath.Join(workDir, conf.WorkDir)
		}
		workDir, _ = filepath.Abs(workDir)
	}
	core.SetLogLevel(conf.LogLevel)
	core.InfoLog("Client using config from %v, work on %v", c, workDir)
	client = &Client{ConfPath: c, Conf: conf, WorkDir: workDir}
	client.Boostrap(base)
	rules, err := client.ReadGfwRules()
	if err != nil {
		return
	}
	gfw := core.NewGFW()
	gfw.Set(strings.Join(rules, "\n"), core.GfwProxy)
	directProcessor := core.NewProcConnDialer(false, core.NewNetDialer("", ""))
	pacProcessor := core.NewPACProcessor(client, directProcessor)
	pacProcessor.Check = gfw.IsProxy
	pacProcessor.Mode = conf.SocksPAC
	if len(pacProcessor.Mode) < 1 {
		pacProcessor.Mode = "global"
	}
	proxyServer = core.NewSocksProxy()
	proxyServer.Processor = core.ProcessorF(func(raw io.ReadWriteCloser, target string) (err error) {
		err = pacProcessor.ProcConn(raw, "tcp://"+target)
		return
	})
	if len(conf.ManagerAddr) > 0 {
		mux := http.NewServeMux()
		mux.HandleFunc("/pac.js", client.PACH)
		mux.HandleFunc("/changeProxyMode", client.ChangeProxyModeH)
		mux.HandleFunc("/updateGfwlist", client.UpdateGfwlistH)
		mux.HandleFunc("/state", client.StateH)
		var listener net.Listener
		managerServer = &http.Server{Addr: conf.ManagerAddr, Handler: mux}
		listener, err = net.Listen("tcp", conf.ManagerAddr)
		if err != nil {
			core.ErrorLog("Client start web server fail with %v", err)
			return
		}
		managerServer.Addr = listener.Addr().String()
		managerListener = &core.TCPKeepAliveListener{TCPListener: listener.(*net.TCPListener)}
	}
	err = proxyServer.Listen(conf.SocksAddr)
	if err != nil {
		core.ErrorLog("Client start proxy server fail with %v", err)
		return
	}
	core.InfoLog("Client start socks server on %v with mode %v", conf.SocksAddr, pacProcessor.Mode)
	changeProxyMode(conf.Mode)
	// writeRuntimeVar()
	wait := sync.WaitGroup{}
	if managerServer != nil {
		wait.Add(1)
		core.InfoLog("Client start web server on %v", managerListener.Addr())
		go func() {
			xerr := managerServer.Serve(managerListener)
			core.WarnLog("Client the web server on %v is stopped by %v", managerListener.Addr(), xerr)
			wait.Done()
		}()
	}
	if len(conf.HTTPAddr) > 0 {
		wait.Add(1)
		proxyServer.HTTPUpstream = conf.HTTPAddr
		go func() {
			xerr := runPrivoxy(workDir, conf.HTTPAddr)
			core.WarnLog("Client the privoxy on %v is stopped by %v", conf.HTTPAddr, xerr)
			wait.Done()
		}()
	}
	proxyServer.Run()
	core.InfoLog("Client all listener is stopped")
	changeProxyMode("manual")
	wait.Wait()
	return
}

//StopClient will stop running client
func StopClient() {
	core.InfoLog("Client stopping client listener")
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
		core.InfoLog("start change proxy mode to %v by %v", mode, pacURL)
		message, err = changeProxyModeNative("auto", pacURL)
	case "global":
		core.InfoLog("start change proxy mode to %v by 127.0.0.1:%v", mode, proxyServerParts[len(proxyServerParts)-1])
		message, err = changeProxyModeNative("global", "127.0.0.1", proxyServerParts[len(proxyServerParts)-1])
	default:
		message, err = changeProxyModeNative("manual")
	}
	if err != nil {
		core.WarnLog("change proxy mode to %v fail with %v, the log is\n%v\n", mode, err, message)
	} else {
		core.InfoLog("change proxy mode to %v is success", mode)
	}
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

func runPrivoxy(workDir, httpAddr string) (err error) {
	proxyServerParts := strings.SplitN(proxyServer.Addr().String(), ":", -1)
	socksAddr := fmt.Sprintf("127.0.0.1:%v", proxyServerParts[len(proxyServerParts)-1])
	core.InfoLog("Client start privoxy by listening http proxy on %v and forwarding to %v", httpAddr, socksAddr)
	confFile := filepath.Join(workDir, "privoxy.conf")
	err = writePrivoxyConf(confFile, httpAddr, socksAddr)
	if err != nil {
		core.WarnLog("Client save privoxy config to %v fail with %v", confFile, err)
		return
	}
	err = runPrivoxyNative(confFile)
	return
}
