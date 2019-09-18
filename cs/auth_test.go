package cs

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

var client = &http.Client{}

func httpGet(username, password, u string) (data string, err error) {
	req, err := http.NewRequest("GET", u, nil)
	if len(username) > 0 && len(password) > 0 {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	b, err := ioutil.ReadAll(resp.Body)
	data = string(b)
	resp.Body.Close()
	return
}

func TestJSONFileAuth(t *testing.T) {
	auth := NewJSONFileAuth(map[string]string{
		"admin": SHA1([]byte("123")),
	}, filepath.Join(os.TempDir(), "json-file-auth-test.json"))
	mux := http.NewServeMux()
	mux.HandleFunc("/addUser", auth.AddUser)
	mux.HandleFunc("/removeUser", auth.RemoveUser)
	mux.HandleFunc("/listUser", auth.ListUser)
	mux.HandleFunc("/testUser", func(res http.ResponseWriter, req *http.Request) {
		ok, err := auth.BasicAuth(req)
		if !ok || err != nil {
			fmt.Fprintf(res, "error")
		} else {
			fmt.Fprintf(res, "ok")
		}
	})
	ts := httptest.NewServer(mux)
	//
	fmt.Printf("listData--->\n")
	listData, err := httpGet("admin", "123", ts.URL+"/listUser")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("%v\n", listData)
	//
	fmt.Printf("addData--->\n")
	addData, err := httpGet("admin", "123", ts.URL+"/addUser?username=test&password=123")
	if err != nil || addData != "ok" {
		t.Error(err)
		return
	}
	fmt.Printf("%v\n", addData)
	//
	fmt.Printf("testData--->\n")
	testData, err := httpGet("test", "123", ts.URL+"/testUser")
	if err != nil || testData != "ok" {
		t.Error(err)
		return
	}
	fmt.Printf("%v\n", testData)
	//
	fmt.Printf("listData--->\n")
	listData, err = httpGet("admin", "123", ts.URL+"/listUser")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("%v\n", listData)
	//
	fmt.Printf("removeData--->\n")
	removeData, err := httpGet("admin", "123", ts.URL+"/removeUser?username=test")
	if err != nil || removeData != "ok" {
		t.Error(err)
		return
	}
	fmt.Printf("%v\n", removeData)
	//
	fmt.Printf("listData--->\n")
	listData, err = httpGet("admin", "123", ts.URL+"/listUser")
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("%v\n", listData)
	//
	//test error
	var res string
	res, err = httpGet("admin", "123", ts.URL+"/addUser")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("admin", "123x", ts.URL+"/addUser?username=test&password=123")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("admin", "123", ts.URL+"/removeUser")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("admin", "123x", ts.URL+"/removeUser?username=test")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("admin", "123x", ts.URL+"/listUser")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("test", "123", ts.URL+"/testUser")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("", "", ts.URL+"/listUser")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	//
	auth.FileName = "/s/xx.json"
	res, err = httpGet("admin", "123", ts.URL+"/addUser?username=test&password=123")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	res, err = httpGet("admin", "123", ts.URL+"/removeUser?username=test")
	if err != nil || res == "ok" {
		t.Error(err)
		return
	}
	err = auth.readAuthFile()
	if err == nil {
		t.Error(err)
		return
	}
}
