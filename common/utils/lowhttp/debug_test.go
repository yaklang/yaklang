package lowhttp

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
	"time"
)

func TestDebugEchoServer(t *testing.T) {
	host, port := DebugEchoServer()
	reqIns, _ := ParseBytesToHttpRequest([]byte(`GET /cccaaabbb HTTP/1.1
Host: asdfasdfasdf

`))
	req, _ := utils.HttpDumpWithBody(reqIns, true)
	rsp, err := HTTPWithoutRedirect(WithHttps(false), WithHost(host), WithPort(port), WithPacketBytes(req), WithTimeout(5*time.Second))
	if err != nil {
		panic(err)
	}
	rspStr := string(rsp.RawRequest)
	spew.Dump(rsp)
	if strings.Contains(rspStr, "cccaaabbb") && strings.Contains(rspStr, "Host: asdfasdfasdf") {
		t.Log("OK")
	} else {
		t.Error("BUG")
	}
}

func TestDebugEchoServerContext(t *testing.T) {
	ip, port := DebugEchoServerContext(context.Background())
	log.Infof("ip: %s:%d", ip, port)
	select {}
}
