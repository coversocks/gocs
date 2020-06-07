package dns

import (
	"fmt"
	"strings"
	"testing"

	"github.com/coversocks/gocs"
	"github.com/coversocks/gocs/core"
)

func TestGFW(t *testing.T) {
	core.SetLogLevel(core.LogLevelDebug)
	gfw := NewGFW()
	rules, err := gocs.ReadGfwlist("../gfwlist.txt")
	if err != nil {
		t.Error(err)
		return
	}
	gfw.Add(strings.Join(rules, "\n"), GfwProxy)
	gfw.Add(`
testproxy
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
	fmt.Printf("info:%v\n", gfw)
}
