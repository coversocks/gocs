Cover Socks
===

### Download Installer
* download installer from [https://github.com/coversocks/golang/releases]

### Install Server(Linux)
* install service
```.sh
unzip ~/coversocks-xxxx-Linux.zip
cd ~/coversocks
./coversocks-install.sh -i
```
* edit configure on `/home/cs/conf/coversocks.json`, see configure section for detail
* add port `5200`(default) to firewalld
* restart service
```.sh
systemctl restart coversocks
systemctl status coversocks
```

### Install Server(Windows)
* uncompress the `coversocks-xxxx-Windows-xx.zip`
* double click `cert.bat` to create `cscert.crt/cscert.key`, `openssl` command is required
* edit configure on `default-server.json`, see configure section for detail
* right click `coversocks-install.bat` and run as administrator
* add dartksocks.exe to firewall


### Add User
* open browser and goto `https://x.x.x.x:5200/manager/`
* input username/password, then click add

### Start Client
* edit configure on `default-client.json`, change adderss/username/password
* start proxy by `./run.sh` on linux/osx, `run.bat` on windows
* test proxy by
```.sh
export http_proxy=http://127.0.0.1:1105
export https_proxy=http://127.0.0.1:1105
export socks_proxy=socks5://127.0.0.1:1105
curl https://www.google.com
```

### Configure(Server)
```.json
{
    "http_addr": "",
    "https_addr": ":5200-5210",
    "https_gen": 300000,
    "https_len": 2048,
    "manager": {
        "test": "40bd001563085fc35165329ea1ff5c5ecbdbbeef"
    },
    "user_file": "dsuser.json"
}
```
* `http_addr` the http listen address, it alway is used when run coversocks behind reverse proxy server like nginx/httpd
* `https_addr` the https listen address, it can be connected direct by coversocks client
* `https_gen` the https cert gen delay
* `https_len` the https cert key length
* `manager` the manager user list, the key is username, the value is password which is encrypted by sha1, create it by `echo -n xxxx | sha1sum`
* `user_file` the access user list by json key/value, the key is username, the value is password which is encrypted by sha1, create it by `echo -n xxxx | sha1sum`

### Configure(Client)
```.json
{
    "servers": [
        {
            "enable": true,
            "name": "test1",
            "address": [
                "wss://127.0.0.1:5200-5210/cover?skip_verify=1"
            ],
            "username": "test",
            "password": "123"
        }
    ],
    "proxy_addr": "0.0.0.0:1105",
    "auto_proxy_addr": "0.0.0.0:1104",
    "manager_addr": "0.0.0.0:1101",
    "mode": "auto"
}
```
* `servers.enable` whether server is enabled
* `servers.name` the server name, it can be any what you want
* `servers.address` the server websocks address,
* `servers.username` the username to server,
* `servers.password` the password to login server,
* `proxy_addr` the socks5/http/https proxy server listen address
* `auto_proxy_addr` the auto mode socks5/http/https proxy server listen address
* `manager_addr` the auto proxy url listen address (pac)
* `mode` the proxy mode which can be `manual/auto/global`

### Compile Binary
* for osx and linux
```.sh
#get source code
go get github.com/coversocks/gocs/coversocks
#package binary
cd $GOPATH/src/github.com/coversocks/gocs
./pkg.sh
```
* for windows
```.bat
go get github.com/coversocks/gocs/coversocks
cd $GOPATH\src\github.com\coversocks\gocs
pkg 386
pkg amd64
```

