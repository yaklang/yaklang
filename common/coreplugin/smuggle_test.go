package coreplugin

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_Pipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := utils.GetRandomAvailableTCPPort()
	target := `POST / HTTP/1.1
Host: ` + `127.0.0.1:` + fmt.Sprint(port) + `
Connection: keep-alive
Content-Length: 1

aGET /admin HTTP/1.1` + lowhttp.CRLF + `Host: 127.0.0.1:8080` + lowhttp.CRLF + lowhttp.CRLF
	var rspBytes []byte
	go func() {
		time.Sleep(2 * time.Second)
		engine, err := yak.Execute(`
rsp, req = poc.HTTP(target, poc.noFixContentLength(true))~
`, map[string]any{
			"target": target,
		})
		if err != nil {
			panic(err)
		}
		cancel()
		rspBytesRaw, _ := engine.GetVM().GetVar("rsp")
		rspBytes = rspBytesRaw.([]byte)
	}()
	vulinbox.Pipeline(ctx, port)
	var rsps []*http.Response
	reader := bufio.NewReader(bytes.NewReader(rspBytes))
	for {
		rsp, err := utils.ReadHTTPResponseFromBufioReader(reader, nil)
		if err != nil {
			break
		}
		io.ReadAll(rsp.Body)
		rsps = append(rsps, rsp)
	}
	if len(rsps) != 2 {
		t.Fatal("invalid rsp count")
	}
	t.Logf("Fetch RESPONSE COUNT: %v", len(rsps))
}

func TestGRPCMUSTPASS_Pipeline_Chunked(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := utils.GetRandomAvailableTCPPort()
	target := `POST / HTTP/1.1
Host: ` + `127.0.0.1:` + fmt.Sprint(port) + `
Connection: keep-alive
Transfer-Encoding: chunked

4
aa` + "\r\n\r\n" + `a
aaaaaaaaaa
0

GET /admin HTTP/1.1` + lowhttp.CRLF + `Host: 127.0.0.1:8080` + lowhttp.CRLF + lowhttp.CRLF
	var rspBytes []byte
	go func() {
		time.Sleep(2 * time.Second)
		engine, err := yak.Execute(`
rsp, req = poc.HTTP(target, poc.noFixContentLength(true))~
`, map[string]any{
			"target": target,
		})
		if err != nil {
			panic(err)
		}
		cancel()
		rspBytesRaw, _ := engine.GetVM().GetVar("rsp")
		rspBytes = rspBytesRaw.([]byte)
	}()
	vulinbox.Pipeline(ctx, port)
	var rsps []*http.Response
	reader := bufio.NewReader(bytes.NewReader(rspBytes))
	for {
		rsp, err := http.ReadResponse(reader, nil)
		if err != nil {
			break
		}
		io.ReadAll(rsp.Body)
		rsps = append(rsps, rsp)
	}
	if len(rsps) != 2 {
		t.Fatal("invalid rsp count")
	}
	t.Logf("Fetch RESPONSE COUNT: %v", len(rsps))
}

func TestGRPCMUSTPASS_Smuggle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := utils.GetRandomAvailableTCPPort()
	target := `POST / HTTP/1.1
Host: ` + `127.0.0.1:` + fmt.Sprint(port) + `
Connection: keep-alive
Content-Length: 48
Transfer-Encoding: chunked

0` + lowhttp.CRLF + lowhttp.CRLF + `GET /admin HTTP/1.1` + lowhttp.CRLF + `Host: 127.0.0.1:8080` + lowhttp.CRLF + lowhttp.CRLF
	var rspBytes []byte
	go func() {
		time.Sleep(2 * time.Second)
		engine, err := yak.Execute(`
rsp, req = poc.HTTP(target, poc.noFixContentLength(true))~
`, map[string]any{
			"target": target,
		})
		if err != nil {
			panic(err)
		}
		cancel()
		rspBytesRaw, _ := engine.GetVM().GetVar("rsp")
		rspBytes = rspBytesRaw.([]byte)
	}()
	vulinbox.Smuggle(ctx, port)
	var rsps []*http.Response
	reader := bufio.NewReader(bytes.NewReader(rspBytes))
	for {
		rsp, err := http.ReadResponse(reader, nil)
		if err != nil {
			break
		}
		io.ReadAll(rsp.Body)
		rsps = append(rsps, rsp)
	}
	if len(rsps) != 2 {
		t.Fatal("invalid rsp count")
	}
	t.Logf("Fetch RESPONSE COUNT: %v", len(rsps))
}
