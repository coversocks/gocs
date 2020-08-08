package core

import (
	"fmt"
	"strings"
	"testing"
)

func TestGFW(t *testing.T) {
	gfw := NewGFW()
	rules, err := ReadGfwlist("../gfwlist.txt")
	if err != nil {
		t.Error(err)
		return
	}
	users, _ := ReadUserRules("../user_rules.txt")
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
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("www.baidu.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("baidu.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("notexistxxx.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("qq.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("x.qq.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("x1.x2.qq.com") {
		t.Error("hav proxy")
		return
	}
	if gfw.IsProxy("192.168.1.1") {
		t.Error("hav proxy")
		return
	}
	if !gfw.IsProxy("192.168.1.18") {
		t.Error("not proxy")
		return
	}
	if gfw.IsProxy("sxbastudio.com") {
		t.Error("hav proxy")
		return
	}
	fmt.Printf("info:%v\n", gfw)
}
