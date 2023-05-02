// main.go
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"yaklang.io/yaklang/common/utils/bruteutils/grdp/glog"
)

var (
	server bool
)

func init() {
	glog.SetLevel(glog.DEBUG)
	logger := log.New(os.Stdout, "", 0)
	glog.SetLogger(logger)
}

func main() {
	flag.BoolVar(&server, "s", false, "web server")
	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())
	//web example
	if server {
		socketIO()
	} else {
		//client example
		StartUI(1024, 768)
	}
}

type Screen struct {
	Height int `json:"height"`
	Width  int `json:"width"`
}

type Info struct {
	Domain   string `json:"domain"`
	Ip       string `json:"ip"`
	Port     string `json:"port"`
	Username string `json:"username"`
	Passwd   string `json:"password"`
	Screen   `json:"screen"`
}

func NewInfo(ip, user, passwd string) (error, *Info) {
	var i Info
	if ip == "" || user == "" || passwd == "" {
		return fmt.Errorf("Must ip/user/passwd"), nil
	}
	t := strings.Split(ip, ":")
	i.Ip = t[0]
	i.Port = "3389"
	if len(t) > 1 {
		i.Port = t[1]
	}
	if strings.Index(user, "\\") != -1 {
		t = strings.Split(user, "\\")
		i.Domain = t[0]
		i.Username = t[len(t)-1]
	} else if strings.Index(user, "/") != -1 {
		t = strings.Split(user, "/")
		i.Domain = t[0]
		i.Username = t[len(t)-1]
	} else {
		i.Username = user
	}

	i.Passwd = passwd

	return nil, &i
}
