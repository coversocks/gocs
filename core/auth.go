package core

import (
	"fmt"
	"html/template"
	"net/http"
	"sync"
)

//JSONFileAuth is the basic auth impl
type JSONFileAuth struct {
	FileName   string
	adminUser  map[string]string
	normalUser map[string]string
	userLck    sync.RWMutex
}

//NewJSONFileAuth return new JSONFileAuth, it will read the users from json file
func NewJSONFileAuth(adimUser map[string]string, filename string) (auth *JSONFileAuth) {
	auth = &JSONFileAuth{
		FileName:   filename,
		adminUser:  adimUser,
		normalUser: map[string]string{},
		userLck:    sync.RWMutex{},
	}
	if len(filename) > 0 {
		auth.readAuthFile()
	}
	return
}

func (j *JSONFileAuth) readAuthFile() (err error) {
	j.userLck.Lock()
	j.normalUser = map[string]string{}
	err = ReadJSON(j.FileName, &j.normalUser)
	j.userLck.Unlock()
	if err != nil {
		WarnLog("JSONFileAuth read normal user from %v fail with %v", j.FileName, err)
	}
	return
}

func (j *JSONFileAuth) addUser(username, password string) (err error) {
	j.userLck.Lock()
	j.normalUser[username] = SHA1([]byte(password))
	err = WriteJSON(j.FileName, j.normalUser)
	j.userLck.Unlock()
	return
}

func (j *JSONFileAuth) removeUser(username string) (err error) {
	j.userLck.Lock()
	delete(j.normalUser, username)
	err = WriteJSON(j.FileName, j.normalUser)
	j.userLck.Unlock()
	return
}

//BasicAuth will auth by http basic auth
func (j *JSONFileAuth) BasicAuth(req *http.Request) (ok bool, err error) {
	username, userPass, ok := req.BasicAuth()
	if ok {
		j.userLck.RLock()
		havingPass := j.normalUser[username]
		j.userLck.RUnlock()
		if SHA1([]byte(userPass)) != havingPass {
			err = fmt.Errorf("username/password is not correct")
		}
	}
	return
}

func (j *JSONFileAuth) adminBasicAuth(res http.ResponseWriter, req *http.Request) (ok bool) {
	username, userPass, ok := req.BasicAuth()
	if !ok {
		res.Header().Set("WWW-Authenticate", `Basic realm="Dark Socket"`)
		res.WriteHeader(401)
		res.Write([]byte("Login Required.\n"))
		return
	}
	j.userLck.RLock()
	havingPass := j.adminUser[username]
	j.userLck.RUnlock()
	// fmt.Println(username, userPass, havingPass)
	if SHA1([]byte(userPass)) != havingPass {
		res.WriteHeader(401)
		fmt.Fprintf(res, "auth fail")
		ok = false
	}
	return
}

//AddUser will add user to json file
func (j *JSONFileAuth) AddUser(res http.ResponseWriter, req *http.Request) {
	if !j.adminBasicAuth(res, req) {
		return
	}
	username, password := req.URL.Query().Get("username"), req.URL.Query().Get("password")
	if len(username) < 1 || len(password) < 1 {
		res.WriteHeader(400)
		fmt.Fprintf(res, "username/password is required")
		return
	}
	err := j.addUser(username, password)
	if err != nil {
		ErrorLog("JSONFileAuth add user fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	fmt.Fprintf(res, "%v", "ok")
}

//RemoveUser will remove user to json file
func (j *JSONFileAuth) RemoveUser(res http.ResponseWriter, req *http.Request) {
	if !j.adminBasicAuth(res, req) {
		return
	}
	username := req.URL.Query().Get("username")
	if len(username) < 1 {
		res.WriteHeader(400)
		fmt.Fprintf(res, "username is required")
		return
	}
	err := j.removeUser(username)
	if err != nil {
		ErrorLog("JSONFileAuth remove user fail with %v", err)
		res.WriteHeader(500)
		fmt.Fprintf(res, "%v", err)
		return
	}
	fmt.Fprintf(res, "%v", "ok")
}

//ListUser will list user as html page
func (j *JSONFileAuth) ListUser(res http.ResponseWriter, req *http.Request) {
	if !j.adminBasicAuth(res, req) {
		return
	}
	res.Header().Add("Content-Type", "text/html;charset=utf-8")
	tmpl := `
	<html>

	<head>
		<meta http-equiv="X-UA-Compatible" content="IE=edge,chrome=1">
		<meta http-equiv="content-type" content="text/html;charset=utf-8">
		<script src="https://apps.bdimg.com/libs/jquery/2.1.4/jquery.min.js"></script>
		<script type="text/javascript">
			function addUser() {
				var username = document.getElementById("username").value;
				var password = document.getElementById("password").value;
				if (username.length < 1 || password.length < 1) {
					alert("username/password is required")
					return;
				}
				$.get("addUser?username=" + username + "&password=" + password, function (data, status) {
					if(data!="ok"){
						alert(data)
						return;
					}
					window.location.reload();
				});
			}
	
			function removeUser(username) {
				$.get("removeUser?username=" + username , function (data, status) {
					if(data!="ok"){
						alert(data)
						return;
					}
					window.location.reload();
				});
			}
		</script>
	</head>
	
	<body>
		<table>
			{{range .users}}
			<tr>
				<td>{{.}}</td>
				<td>
					<a href="#" {{.}} onclick="removeUser('{{.}}');">Remove</a>
				</td>
			</tr>
			{{end}}
			<tr>
				<td>
					<input id="username" type="text" />
					<input id="password" type="password" />
				</td>
				<td>
					<a href="#" onclick="addUser();">Add</a>
				</td>
			</tr>
		</table>
	</body>
	
	</html>
	`
	t := template.Must(template.New("user").Parse(tmpl))
	var users []template.HTML
	j.userLck.RLock()
	for u := range j.normalUser {
		users = append(users, template.HTML(u))
	}
	j.userLck.RUnlock()
	t.Execute(res, map[string]interface{}{
		"users": users,
	})
}
