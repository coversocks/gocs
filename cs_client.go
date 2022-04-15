package gocs

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codingeasygo/util/proxy"
	proxyhttp "github.com/codingeasygo/util/proxy/http"
	proxysocks "github.com/codingeasygo/util/proxy/socks"

	"github.com/codingeasygo/util/xio"
	"github.com/coversocks/gocs/core"
)

// var clientConf string
// var clientConfDir string

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
	DebugLog("Client start connect one channel to %v-%v", c.Name, c.LastUsed)
	raw, err = c.Base.Dial(address)
	if err == nil {
		DebugLog("Client connect one channel to %v-%v success", c.Name, c.LastUsed)
		conn := xio.NewStringConn(raw)
		conn.Name = c.Name
		raw = conn
	} else {
		WarnLog("Client connect one channel to %v-%v fail with %v", c.Name, c.LastUsed, err)
	}
	c.LastUsed = (c.LastUsed + 1) % len(c.Address)
	return
}

func (c *ClientServerDialer) String() string {
	return c.Name
}

//ClientConf is pojo for dark socks client configure
type ClientConf struct {
	Servers       []*ClientServerConf `json:"servers"`
	Forwards      map[string]string   `json:"forwards"`
	ProxyAddr     string              `json:"proxy_addr"`
	ProxySkip     []string            `json:"proxy_skip"`
	AutoProxyAddr string              `json:"auto_proxy_addr"`
	ManagerAddr   string              `json:"manager_addr"`
	Mode          string              `json:"mode"`
	LogLevel      int                 `json:"log"`
	WorkDir       string              `json:"work_dir"`
	PPROF         int                 `json:"pprof"`
}

//Client is dialer by ClientConf
type Client struct {
	*core.Client
	Conf        ClientConf
	WorkDir     string //current working dir
	ConfPath    string
	Dialer      core.Dialer
	Server      *proxy.Server
	AutoServer  *proxy.Server
	AutoDialer  *core.AutoPACDialer
	Manager     *http.Server
	Listener    net.Listener
	ForwardAll  map[string]net.Listener
	ForwardLock sync.RWMutex
}

//NewClient will return new Client.
func NewClient(config string, dialer core.Dialer) (client *Client) {
	client = &Client{
		ConfPath:    config,
		Dialer:      dialer,
		ForwardAll:  map[string]net.Listener{},
		ForwardLock: sync.RWMutex{},
	}
	return
}

//Boostrap will initial setting
func (c *Client) Boostrap(base core.Dialer) (err error) {
	var dialers = []core.Dialer{}
	for _, conf := range c.Conf.Servers {
		if !(conf.Enable && len(conf.Address) > 0) {
			continue
		}
		var innerDialers []core.Dialer
		for _, addr := range conf.Address {
			addrs, xerr := parseConnAddr(addr)
			if xerr != nil {
				return xerr
			}
			for _, a := range addrs {
				innerDialers = append(innerDialers, &ClientServerDialer{
					Base: base,
					ClientServerConf: &ClientServerConf{
						Enable:   true,
						Name:     a,
						Address:  []string{a},
						Username: conf.Username,
						Password: conf.Password,
					},
				})
			}
		}
		innerDialer := core.NewSortedDialer(innerDialers...)
		innerDialer.Name = conf.Name
		dialers = append(dialers, innerDialer)
	}
	var dialer = core.NewSortedDialer(dialers...)
	c.Client = core.NewClient(core.DefaultBufferSize, dialer)
	InfoLog("Client boostrap with %v server dialer", len(dialers))
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
	gfwData, err := c.GetBytes(gfwListURL)
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
		ErrorLog("PAC read apb.js fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	abpStr := string(abpRaw)
	//
	//rules
	gfwRules, err := c.ReadGfwRules()
	if err != nil {
		ErrorLog("PAC read gfwlist.txt fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	gfwRulesJS, _ := json.Marshal(gfwRules)
	abpStr = strings.Replace(abpStr, "__RULES__", string(gfwRulesJS), 1)
	//
	//proxy address
	if c.Server == nil {
		ErrorLog("PAC load fail with proxy server is not started")
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", "proxy server is not started")
		return
	}
	//
	// socksProxy.
	proxyAddr := c.Conf.ProxyAddr
	if len(c.Conf.AutoProxyAddr) > 0 {
		proxyAddr = c.Conf.AutoProxyAddr
	}
	parts := strings.SplitN(proxyAddr, ":", -1)
	abpStr = strings.Replace(abpStr, "__SOCKS5ADDR__", "127.0.0.1", -1)
	abpStr = strings.Replace(abpStr, "__SOCKS5PORT__", parts[len(parts)-1], -1)
	res.Write([]byte(abpStr))
}

//ChangeProxyModeH is http handler to change proxy mode
func (c *Client) ChangeProxyModeH(w http.ResponseWriter, r *http.Request) {
	mode := r.URL.Query().Get("mode")
	_, err := c.ChangeProxyMode(mode)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "%v", err)
		return
	}
	c.Conf.Mode = mode
	err = core.WriteJSON(c.ConfPath, c.Conf)
	if err != nil {
		w.WriteHeader(500)
		WarnLog("Client change proxy mode on config %v fail with %v", c.ConfPath, err)
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

func (c *Client) startForward(local, remote string) (err error) {
	InfoLog("Client %v forward is starting", local)
	locURL, err := url.Parse(local)
	if err != nil {
		WarnLog("Client parse forward %v=>%v fail with %v", local, remote, err)
		return
	}
	listener, err := net.Listen(locURL.Scheme, locURL.Host)
	if err != nil {
		WarnLog("Client listen forward %v=>%v fail with %v", local, remote, err)
		return
	}
	c.ForwardLock.Lock()
	c.ForwardAll[local] = listener
	c.ForwardLock.Unlock()
	defer func() {
		listener.Close()
		c.ForwardLock.Lock()
		delete(c.ForwardAll, local)
		c.ForwardLock.Unlock()
	}()
	for {
		conn, xerr := listener.Accept()
		if xerr != nil {
			err = xerr
			break
		}
		go c.procForward(conn, remote)
	}
	InfoLog("Client %v forward is stopped by %v", local, err)
	return
}

func (c *Client) procForward(conn net.Conn, remote string) (err error) {
	defer conn.Close()
	piper, err := c.Client.DialPiper(remote, c.BufferSize)
	if err == nil {
		defer piper.Close()
		err = piper.PipeConn(conn, remote)
	}
	return
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

//Start by configure and dialer
func (c *Client) Start() (err error) {
	conf := ClientConf{Mode: "auto"}
	err = core.ReadJSON(c.ConfPath, &conf)
	if err != nil {
		ErrorLog("Client read configure fail with %v", err)
		return
	}
	if len(conf.ProxyAddr) < 1 {
		ErrorLog("Client proxy_addr is required")
		err = fmt.Errorf("Client proxy_addr is required")
		return
	}
	workDir := ""
	if len(conf.WorkDir) > 0 {
		if filepath.IsAbs(conf.WorkDir) {
			workDir = conf.WorkDir
		} else {
			workDir = filepath.Join(filepath.Dir(c.ConfPath), conf.WorkDir)
		}
		workDir, err = filepath.Abs(workDir)
	} else {
		workDir, err = filepath.Abs(".")
	}
	if err != nil {
		return
	}
	c.Conf, c.WorkDir = conf, workDir
	InfoLog("Client using config from %v, work on %v, log level %v", c.ConfPath, workDir, c.Conf.LogLevel)
	core.SetLogLevel(c.Conf.LogLevel)
	proxysocks.SetLogLevel(c.Conf.LogLevel)
	proxyhttp.SetLogLevel(c.Conf.LogLevel)
	err = c.Boostrap(c.Dialer)
	if err != nil {
		ErrorLog("Client bootstrap fail with %v", err)
		return
	}
	defer func() {
		if err != nil {
			c.Stop()
		}
	}()
	rules, err := c.ReadGfwRules()
	if err != nil {
		ErrorLog("Client read gfw rules fail with %v", err)
		return
	}
	skipDialer := core.NewSkipDialer(c)
	err = skipDialer.AddSkip(c.Conf.ProxySkip...)
	if err != nil {
		ErrorLog("Client parse proxy skip fail with %v", err)
		return
	}
	gfw := core.NewGFW()
	gfw.Set(strings.Join(rules, "\n"), core.GfwProxy)
	directProcessor := core.NewNetDialer("", "")
	autoProcessor := core.NewAutoPACDialer(c, directProcessor)
	pacProcessor := core.NewPACDialer(c, autoProcessor)
	pacProcessor.Check = gfw.IsProxy
	pacProcessor.Mode = "auto"
	c.Server = proxy.NewServer(skipDialer)
	c.AutoServer = proxy.NewServer(pacProcessor)
	c.AutoDialer = autoProcessor
	c.AutoDialer.LoadCache(filepath.Join(c.WorkDir, "pac.cache"))
	_, err = c.Server.Start(conf.ProxyAddr)
	if err != nil {
		ErrorLog("Client start proxy server fail with %v", err)
		return
	}
	InfoLog("Client start proxy server on %v", conf.ProxyAddr)
	if len(conf.AutoProxyAddr) > 0 {
		_, err = c.AutoServer.Start(conf.AutoProxyAddr)
		if err != nil {
			ErrorLog("Client start auto proxy server fail with %v", err)
			return
		}
		InfoLog("Client start auto socks server on %v with mode %v", conf.AutoProxyAddr, pacProcessor.Mode)
	}
	if len(conf.ManagerAddr) > 0 {
		mux := http.NewServeMux()
		mux.HandleFunc("/pac.js", c.PACH)
		mux.HandleFunc("/changeProxyMode", c.ChangeProxyModeH)
		mux.HandleFunc("/updateGfwlist", c.UpdateGfwlistH)
		mux.HandleFunc("/state", c.StateH)
		if conf.PPROF == 1 {
			mux.HandleFunc("/debug/pprof/", pprof.Index)
			mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
			mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
			mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
			mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
		}
		var listener net.Listener
		c.Manager = &http.Server{Addr: conf.ManagerAddr, Handler: mux}
		listener, err = net.Listen("tcp", conf.ManagerAddr)
		if err != nil {
			ErrorLog("Client start manager server fail with %v", err)
			return
		}
		c.Manager.Addr = listener.Addr().String()
		c.Listener = &xio.TCPKeepAliveListener{TCPListener: listener.(*net.TCPListener)}
		InfoLog("Client start web server on %v", conf.ManagerAddr)
		go func() {
			xerr := c.Manager.Serve(c.Listener)
			WarnLog("Client the web server on %v is stopped by %v", conf.ManagerAddr, xerr)
		}()
	}
	c.ChangeProxyMode(conf.Mode)
	for k, v := range conf.Forwards {
		go c.startForward(k, v)
	}
	return
}

//Wait will wait all runner
func (c *Client) Wait() {
	if c.Server != nil {
		c.Server.Wait()
	}
	if c.AutoServer != nil {
		c.AutoServer.Wait()
	}
}

//Stop will stop running client
func (c *Client) Stop() {
	InfoLog("Client stopping client listener")
	if c.AutoDialer != nil {
		c.AutoDialer.SaveCache(filepath.Join(c.WorkDir, "pac.cache"))
	}
	c.Close()
	if c.Manager != nil {
		c.Listener.Close()
	}
	if c.Server != nil {
		c.Server.Close()
	}
	if c.AutoServer != nil {
		c.AutoServer.Close()
	}
	c.ForwardLock.Lock()
	for key, l := range c.ForwardAll {
		l.Close()
		delete(c.ForwardAll, key)
	}
	c.ForwardLock.Unlock()
}

var addrRegexp = regexp.MustCompile(`:[0-9\\,\\-]+/`)

func parseConnAddr(addr string) (addrs []string, err error) {
	addrParts := addrRegexp.Split(addr, 2)
	addrPorts := strings.Trim(addrRegexp.FindString(addr), ":/")
	if len(addrParts) < 2 {
		err = fmt.Errorf("invalid uri")
		return
	}
	return parsePortAddr(addrParts[0]+":", addrPorts, "/"+addrParts[1])
}

//ChangeProxyMode will change system proxy mode
func (c *Client) ChangeProxyMode(mode string) (message string, err error) {
	if c.Server == nil || c.Manager == nil {
		err = fmt.Errorf("proxy server is not started")
		WarnLog("change proxy mode to %v fail with %v", mode, err)
		return
	}
	proxyServerParts := strings.Split(c.Conf.ProxyAddr, ":")
	managerServerParts := strings.Split(c.Conf.ManagerAddr, ":")
	switch mode {
	case "auto":
		pacURL := fmt.Sprintf("http://127.0.0.1:%v/pac.js?timestamp=%v", managerServerParts[len(managerServerParts)-1], time.Now().Local().UnixNano()/1e6)
		InfoLog("start change proxy mode to %v by %v", mode, pacURL)
		message, err = changeProxyModeNative("auto", pacURL)
	case "global":
		InfoLog("start change proxy mode to %v by 127.0.0.1:%v", mode, proxyServerParts[len(proxyServerParts)-1])
		message, err = changeProxyModeNative("global", "127.0.0.1", proxyServerParts[len(proxyServerParts)-1])
	default:
		message, err = changeProxyModeNative("manual")
	}
	if err != nil {
		WarnLog("change proxy mode to %v fail with %v, the log is\n%v\n", mode, err, message)
	} else {
		InfoLog("change proxy mode to %v is success", mode)
	}
	return
}

var clientInstance *Client

//StartClient will start client by configure
func StartClient(c string) (err error) {
	clientInstance = NewClient(c, core.NewWebsocketDialer())
	return clientInstance.Start()
}

//WaitClient will wait all runner
func WaitClient() {
	if clientInstance != nil {
		clientInstance.Wait()
	}
}

//StopClient will stop running client
func StopClient() {
	if clientInstance != nil {
		clientInstance.Stop()
		clientInstance = nil
	}
}
