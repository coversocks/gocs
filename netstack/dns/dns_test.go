package dns

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
	"github.com/miekg/dns"
)

func TestGFW(t *testing.T) {
	core.SetLogLevel(core.LogLevelDebug)
	gfw := NewGFW()
	rules, err := gocs.ReadGfwlist("../../gfwlist.txt")
	if err != nil {
		t.Error(err)
		return
	}
	users, _ := gocs.ReadUserRules("../../user_rules.txt")
	rules = append(rules, users...)
	// ioutil.WriteFile("gfwxx.txt", []byte(strings.Join(rules, "\n")), os.ModePerm)
	gfw.Set(strings.Join(rules, "\n"), GfwProxy)
	gfw.Set(`
testproxy
192.168.1.18
	`, GfwProxy)
	if !gfw.IsProxy("youtube-ui.l.google.com") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("google.com") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("google.com.") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy(".google.com. ") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("www.google.com.hk") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("www.google.cn") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("google.cn") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy(".youtube.com.") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("testproxy") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("d3c33hcgiwev3.cloudfront.net") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("xx.wwwhost.biz") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("www.ftchinese.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("xxddsf.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("www.baidu.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("baidu.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("notexistxxx.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("qq.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("x.qq.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("x1.x2.qq.com") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("192.168.1.1") {
		t.Error("not proxy")
		return
	}
	if !gfw.IsProxy("192.168.1.18") {
		t.Error("not proxy")
		return
	}
	fmt.Printf("info:%v\n", gfw)
}

func TestDNS(t *testing.T) {
	// data := []byte{81, 52, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 3, 119, 119, 119, 5, 98, 97, 105, 100, 117, 3, 99, 111, 109, 0, 0, 1, 0, 1, 9, 110}
	// msg := new(dns.Msg)
	// err := msg.Unpack(data)
	// if err != nil {
	// 	t.Error(err)
	// 	return
	// }
	// fmt.Println(msg)
	//
	data2 := []byte{81, 52, 129, 128, 0, 1, 0, 3, 0, 0, 0, 0, 3, 119, 119, 119, 5, 98, 97, 105, 100, 117, 3, 99, 111, 109, 0, 0, 1, 0, 1, 192, 12, 0, 5, 0, 1, 0, 0, 3, 114, 0, 15, 3, 119, 119, 119, 1, 97, 6, 115, 104, 105, 102, 101, 110, 192, 22, 192, 43, 0, 1, 0, 1, 0, 0, 0, 131, 0, 4, 14, 215, 177, 38, 192, 43, 0, 1, 0, 1, 0, 0, 0, 131, 0, 4, 14, 215, 177, 39}
	msg2 := new(dns.Msg)
	err2 := msg2.Unpack(data2)
	if err2 != nil {
		t.Error(err2)
		return
	}
	fmt.Println(msg2)
}
