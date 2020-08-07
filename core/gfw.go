package core

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"
)

const (
	//GfwProxy is GFW target for proxy
	GfwProxy = "proxy"
	//GfwLocal is GFW target for local
	GfwLocal = "local"
)

//GFW impl check if domain in gfw list
type GFW struct {
	list map[string]string
	lck  sync.RWMutex
}

//NewGFW will create new GFWList
func NewGFW() (gfw *GFW) {
	gfw = &GFW{
		list: map[string]string{
			"*": GfwLocal,
		},
		lck: sync.RWMutex{},
	}
	return
}

//Set list
func (g *GFW) Set(list, target string) {
	g.lck.Lock()
	defer g.lck.Unlock()
	g.list[list] = target
}

//Get list
func (g *GFW) Get(list string) (target string) {
	g.lck.Lock()
	defer g.lck.Unlock()
	target = g.list[list]
	return
}

//IsProxy return true, if domain target is dns://proxy
func (g *GFW) IsProxy(domain string) bool {
	return g.Find(domain) == GfwProxy
}

//Find domain target
func (g *GFW) Find(domain string) (target string) {
	g.lck.RLock()
	defer g.lck.RUnlock()
	domain = strings.Trim(domain, " \t.")
	if len(domain) < 1 {
		target = g.list["*"]
		return
	}
	parts := strings.Split(domain, ".")
	if len(parts) < 2 {
		target = g.check(parts...)
	} else {
		n := len(parts) - 1
		for i := 0; i < n; i++ {
			target = g.check(parts[i:]...)
			if len(target) > 0 {
				break
			}
		}
	}
	if len(target) < 1 {
		target = g.list["*"]
	}
	return
}

func (g *GFW) check(parts ...string) (target string) {
	ptxt := fmt.Sprintf("(?m)^[^\\@]*[\\|\\.]*(http://)?(https://)?%v$", strings.Join(parts, "\\."))
	pattern, err := regexp.Compile(ptxt)
	if err == nil {
		for key, val := range g.list {
			if len(pattern.FindString(key)) > 0 {
				target = val
				break
			}
		}
	}
	return
}

func (g *GFW) String() string {
	return "GFW"
}

//ReadGfwlist will read and decode gfwlist file
func ReadGfwlist(gfwFile string) (rules []string, err error) {
	gfwRaw, err := ioutil.ReadFile(gfwFile)
	if err != nil {
		return
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

//ReadUserRules will read and decode user rules
func ReadUserRules(gfwFile string) (rules []string, err error) {
	gfwData, err := ioutil.ReadFile(gfwFile)
	if err != nil {
		return
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
