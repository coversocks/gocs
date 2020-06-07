package gocs

import (
	"bufio"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/websocket"
)

func init() {
	privoxyPath = "./privoxy"
	abpPath = "./abp.js"
	gfwListPath = "./gfwlist.txt"
	userRulesPath = "./work/user_rules.txt"
	workDir = "./work/"
	switch runtime.GOOS {
	case "darwin":
		exec.Command("cp", "-f", "./privoxy-Darwin", "privoxy").CombinedOutput()
		networksetupPath = "echo"
	case "linux":
		exec.Command("cp", "-f", "./privoxy-Linux", "privoxy").CombinedOutput()
		networksetupPath = "echo"
	default:
		exec.Command("powershell", "cp", "privoxy-Win.exe", "privoxy.exe").CombinedOutput()
		networksetupPath = "cmd"
	}
}

var echoListner net.Listener

func runEcho(listen string) (err error) {
	echoListner, err = net.Listen("tcp", listen)
	if err != nil {
		return
	}
	var conn net.Conn
	for {
		conn, err = echoListner.Accept()
		if err != nil {
			break
		}
		fmt.Printf("echo accept from %v\n", conn.RemoteAddr())
		go func() {
			io.Copy(conn, conn)
		}()
	}
	return
}

func httpGet(format string, args ...interface{}) (data string, err error) {
	resp, err := http.Get(fmt.Sprintf(format, args...))
	if err != nil {
		return
	}
	var d []byte
	d, err = ioutil.ReadAll(resp.Body)
	if err == nil {
		data = string(d)
	}
	return
}

func TestCS(t *testing.T) {
	os.Args = []string{"xxx"}
	os.RemoveAll(workDir)
	// exitf = func(code int) {}
	gfwMux := http.NewServeMux()
	gfwMux.HandleFunc("/gfwlist.txt", func(w http.ResponseWriter, r *http.Request) {
		writer := base64.NewEncoder(base64.StdEncoding, w)
		fmt.Fprintf(writer, "%v", "!xx")
		writer.Close()
	})
	gfwServer := httptest.NewServer(gfwMux)
	wait := sync.WaitGroup{}
	wait.Add(1)
	go func() {
		err := runEcho(":10331")
		wait.Done()
		fmt.Println("echo done with", err)
	}()
	wait.Add(1)
	go func() {
		err := StartServer("coversocks-s.json")
		wait.Done()
		fmt.Println("server done with", err)
	}()
	wait.Add(1)
	go func() {
		err := StartClient("coversocks-c.json")
		wait.Done()
		fmt.Println("client done with", err)
	}()
	time.Sleep(100 * time.Millisecond)
	managerServerParts := strings.Split(managerServer.Addr, ":")
	managerServer := fmt.Sprintf("http://127.0.0.1:%v", managerServerParts[len(managerServerParts)-1])
	defer func() {
		// clientKillSignal <- os.Kill
		// time.Sleep(10 * time.Millisecond)
		// serverKillSignal <- os.Kill
		// time.Sleep(10 * time.Millisecond)
		echoListner.Close()
		wait.Wait()
	}()
	//
	conn, err := proxyDial(t, "127.0.0.1:11105", "127.0.0.1", 10331)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Fprintf(conn, "xxx->")
	buf := make([]byte, 1024)
	readed, err := conn.Read(buf)
	fmt.Println(string(buf[:readed]))
	time.Sleep(time.Millisecond)
	//
	{ //test pac
		pac, err := httpGet("%v/pac.js", managerServer)
		if err != nil || !strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		testUserRules := filepath.Join(workDir, "user_rules.txt")
		ioutil.WriteFile(testUserRules, []byte(`
		!Abc
		||xxx.com
		`), os.ModePerm)
		pac, err = httpGet("%v/pac.js", managerServer)
		if err != nil || !strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		os.Remove(testUserRules)
		//
		abpPath = "xxx"
		pac, err = httpGet("%v/pac.js", managerServer)
		if err != nil || strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		abpPath = "./abp.js"
		//
		gfwListPath = "xxxx"
		pac, err = httpGet("%v/pac.js", managerServer)
		if err != nil || strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		testGfwList := filepath.Join(workDir, "gfwlist.txt")
		ioutil.WriteFile(testGfwList, []byte("abc"), os.ModePerm)
		pac, err = httpGet("%v/pac.js", managerServer)
		if err != nil || strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		os.Remove(testGfwList)
		gfwListPath = "./gfwlist.txt"
		//
		userRulesPath = "xxxx"
		pac, err = httpGet("%v/pac.js", managerServer)
		if err != nil || !strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		userRulesPath = "./user_rules.txt"
		//
		var old = proxyServer
		proxyServer = nil
		pac, err = httpGet("%v/pac.js", managerServer)
		if err != nil || strings.Contains(pac, "SOCKS") {
			t.Errorf("err:%v,%v", err, pac)
			return
		}
		proxyServer = old
	}
	{ //update gfw
		gfwListURL = gfwServer.URL + "/gfwlist.txt"
		res, err := httpGet("%v/updateGfwlist", managerServer)
		if err != nil || res != "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		var old = client
		client = nil
		res, err = httpGet("%v/updateGfwlist", managerServer)
		if err != nil || res == "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		client = old
		//
		gfwListURL = "http://127.0.0.1:10/gfwlist.txt"
		res, err = httpGet("%v/updateGfwlist", managerServer)
		if err != nil || res == "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		//
		gfwListURL = gfwServer.URL + "/gfwlist.txt"
	}
	{ //change proxy mode
		res, err := httpGet("%v/changeProxyMode?mode=auto", managerServer)
		if err != nil || res != "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		res, err = httpGet("%v/changeProxyMode?mode=global", managerServer)
		if err != nil || res != "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		res, err = httpGet("%v/changeProxyMode?mode=manual", managerServer)
		if err != nil || res != "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		//
		var old = proxyServer
		proxyServer = nil
		res, err = httpGet("%v/changeProxyMode?mode=manual", managerServer)
		if err != nil || res == "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		proxyServer = old
		//
		networksetupPath = "exit"
		res, err = httpGet("%v/changeProxyMode?mode=auto", managerServer)
		if err != nil || res == "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		if runtime.GOOS == "windows" {
			networksetupPath = "cmd"
		} else {
			networksetupPath = "echo"
		}
		//
		clientConfOld := clientConf
		clientConf = "/xxx/ssss/"
		res, err = httpGet("%v/changeProxyMode?mode=manual", managerServer)
		if err != nil || res == "ok" {
			t.Errorf("err:%v,%v", err, res)
			return
		}
		clientConf = clientConfOld
	}
	{ //test websocket auth fail
		raw, err := websocket.Dial("ws://127.0.0.1:5100/ds", "", "http://127.0.0.1:5100")
		if err != nil {
			t.Error(err)
			return
		}
		buf := make([]byte, 1000)
		_, err = raw.Read(buf)
		if err != io.EOF {
			t.Error(err)
			return
		}
	}
	{ //test privoxy error
		workDir = "/xxxxxxxx"
		err := runPrivoxy("127.0.0.1:11100")
		if err == nil {
			t.Error(err)
			return
		}
		workDir = "./work"
	}
}

func proxyDial(t *testing.T, proxy, remote string, port uint16) (conn net.Conn, err error) {
	conn, err = net.Dial("tcp", proxy)
	if err != nil {
		t.Error(err)
		return
	}
	buf := make([]byte, 1024*64)
	proxyReader := bufio.NewReader(conn)
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		return
	}
	err = fullBuf(proxyReader, buf, 2, nil)
	if err != nil {
		return
	}
	if buf[0] != 0x05 || buf[1] != 0x00 {
		err = fmt.Errorf("only ver 0x05 / method 0x00 is supported, but %x/%x", buf[0], buf[1])
		return
	}
	buf[0], buf[1], buf[2], buf[3] = 0x05, 0x01, 0x00, 0x03
	buf[4] = byte(len(remote))
	copy(buf[5:], []byte(remote))
	binary.BigEndian.PutUint16(buf[5+len(remote):], port)
	_, err = conn.Write(buf[:buf[4]+7])
	if err != nil {
		return
	}
	_, err = proxyReader.Read(buf)
	return
}

func fullBuf(r io.Reader, p []byte, length uint32, last *int64) error {
	all := uint32(0)
	buf := p[:length]
	for {
		readed, err := r.Read(buf)
		if err != nil {
			return err
		}
		if last != nil {
			*last = time.Now().Local().UnixNano() / 1e6
		}
		all += uint32(readed)
		if all < length {
			buf = p[all:]
			continue
		} else {
			break
		}
	}
	return nil
}

// func TestStartConfError(t *testing.T) {
// 	os.Args = []string{"xxx"}
// 	exitf = func(code int) {}
// 	os.RemoveAll("work")
// 	os.Mkdir("work", os.ModePerm)
// 	//
// 	//
// 	//not found
// 	argConf = "work/none.json"
// 	argRunServer = true
// 	main()
// 	//http addr error
// 	core.WriteJSON("work/s-http-addr-err.json", &ServerConf{
// 		HTTPListenAddr: "xxx:1x",
// 	})
// 	argConf = "work/s-http-addr-err.json"
// 	argRunServer = true
// 	main()
// 	//https addr error
// 	core.WriteJSON("work/s-https-addr-err.json", &ServerConf{
// 		HTTPSListenAddr: "xxx:1x",
// 	})
// 	argConf = "work/s-https-addr-err.json"
// 	argRunServer = true
// 	main()
// 	//
// 	argRunServer = false
// 	//
// 	//
// 	//not found
// 	argConf = "work/none.json"
// 	argRunClient = true
// 	main()
// 	//socks addr error
// 	core.WriteJSON("work/c-socks-addr-err.json", &ClientConf{})
// 	argConf = "work/c-socks-addr-err.json"
// 	argRunClient = true
// 	main()
// 	core.WriteJSON("work/c-socks-addr-err.json", &ClientConf{
// 		SocksAddr: ":xx",
// 	})
// 	argConf = "work/c-socks-addr-err.json"
// 	argRunClient = true
// 	main()
// 	//manager addr error
// 	core.WriteJSON("work/c-manager-addr-err.json", &ClientConf{
// 		SocksAddr:   ":0",
// 		ManagerAddr: ":xx",
// 	})
// 	argConf = "work/c-manager-addr-err.json"
// 	argRunClient = true
// 	main()
// 	//
// 	argRunClient = false
// 	//
// 	//
// 	//argument error
// 	argRunClient = false
// 	argRunServer = false
// 	main()
// }
