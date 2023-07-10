package lowhttp

import (
	"github.com/davecgh/go-spew/spew"
	"strings"
	"testing"
	"time"
)

func TestDebugEchoServer(t *testing.T) {
	host, port := DebugEchoServer()
	req, _ := ParseBytesToHttpRequest([]byte(`GET /cccaaabbb HTTP/1.1
Host: asdfasdfasdf

`))
	rsp, err := SendHTTPRequestRaw(false, host, port, req, 5*time.Second)
	if err != nil {
		panic(err)
	}
	rspStr := string(rsp)
	spew.Dump(rsp)
	if strings.Contains(rspStr, "cccaaabbb") && strings.Contains(rspStr, "Host: asdfasdfasdf") {
		t.Log("OK")
	} else {
		t.Error("BUG")
	}
}
