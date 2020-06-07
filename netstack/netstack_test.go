package netstack

import (
	"regexp"
	"strings"
	"testing"

	"github.com/coversocks/gocs"
)

func TestSocks(t *testing.T) {

}

func TestPattern(t *testing.T) {
	// domain := "www.google.com"
	// parts := strings.SplitAfterN(domain, ".", 3)
	// for i, p := range parts {
	// 	parts[i] = strings.Trim(p, ".")
	// }
	// fmt.Println(parts)
	// if len(parts) == 3 {
	// 	parts = parts[1:]
	// }
	// pattern := regexp.MustCompile(fmt.Sprintf("[\\|\\.]*%v\\.%v$", parts[0], parts[1]))
	// fmt.Println(pattern.MatchString("||google.com"))
	rules, err := gocs.ReadGfwlist("../gfwlist.txt")
	if err != nil {
		t.Error((err))
		return
	}
	all := strings.Join(rules, "\n")
	// (?m)^[\|\.]*l\.google.com$
	if !regexp.MustCompile(`(?m)^[\|\.]*\.youtube.com$`).MatchString(all) {
		t.Error("not matched")
	}
	if !regexp.MustCompile(`(?m)^[\|\.]*\.google.com$`).MatchString(all) {
		t.Error("not matched")
	}
}
