package coreplugin

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"testing"
	"time"
)

func TestConnectVulinboxAgent(t *testing.T) {
	vulAddr, err := vulinbox.NewVulinboxAgent(context.Background())
	if err != nil {
		panic(err)
	}
	var count int
	cancel, err := ConnectVulinboxAgent(vulAddr, func(request []byte) {
		count++
		spew.Dump(request)
	})
	host, port, _ := utils.ParseStringToHostPort(vulAddr)
	if host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	_, err = yak.Execute(
		`rsp, req = poc.HTTP("GET /_/ws HTTP/1.1\r\nHost: " + addr + "\r\n")~;log.info("code done! for " + addr)`,
		map[string]any{
			"addr": utils.HostPort(host, port),
		},
	)
	if err != nil {
		panic(err)
	}
	time.Sleep(time.Second * 1)
	cancel()
	if count <= 0 {
		panic("Connect to Vulinbox Agent FAILED!")
	}
}
