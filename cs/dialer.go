package cs

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/url"

	"golang.org/x/net/websocket"
)

//Dialer is interface for dial raw connect by string
type Dialer interface {
	Dial(remote string) (raw io.ReadWriteCloser, err error)
}

//DialerF is an the implementation of Dialer by func
type DialerF func(remote string) (raw io.ReadWriteCloser, err error)

//Dial dial to remote by func
func (d DialerF) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	raw, err = d(remote)
	return
}

//NetDialer is an implementation of Dialer by net
type NetDialer string

//Dial dial to remote by net
func (t NetDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	DebugLog("NetDialer %v dial to %v", t, remote)
	raw, err = net.Dial(string(t), remote)
	if err == nil {
		raw = NewStringConn(raw)
	}
	return
}

//WebsocketDialer is an implementation of Dialer by websocket
type WebsocketDialer string

//Dial dial to remote by websocket
func (w WebsocketDialer) Dial(remote string) (raw io.ReadWriteCloser, err error) {
	targetURL, err := url.Parse(remote)
	if err != nil {
		return
	}
	username, password := targetURL.Query().Get("username"), targetURL.Query().Get("password")
	skipVerify := targetURL.Query().Get("skip_verify") == "1"
	var origin string
	if targetURL.Scheme == "wss" {
		origin = fmt.Sprintf("https://%v", targetURL.Host)
	} else {
		origin = fmt.Sprintf("http://%v", targetURL.Host)
	}
	config, err := websocket.NewConfig(targetURL.String(), origin)
	if err == nil {
		if len(username) > 0 && len(password) > 0 {
			config.Header.Set("Authorization", "Basic "+basicAuth(username, password))
		}
		config.TlsConfig = &tls.Config{}
		config.TlsConfig.InsecureSkipVerify = skipVerify
		raw, err = websocket.DialConfig(config)
	}
	return
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}
