package coreplugin_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/coreplugin"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/vulinbox"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_Smuggle_Negative_Pipeline(t *testing.T) {
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

func TestGRPCMUSTPASS_Pipeline_Negative_Chunked(t *testing.T) {
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

func TestGRPCMUSTPASS_Smuggle_Positive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := utils.GetRandomAvailableTCPPort()
	target := `POST / HTTP/1.1
Host: ` + `127.0.0.1:` + fmt.Sprint(port) + `
Connection: keep-alive
Content-Length: 48
Transfer-Encoding: chunked

0` + lowhttp.CRLF + lowhttp.CRLF + `GET /admin HTTP/1.1` + lowhttp.CRLF + `Host: 127.0.0.1:8080` + lowhttp.CRLF + lowhttp.CRLF

	spew.Dump(target)
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

func TestGRPCMUSTPASS_Smuggle_Positive_2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	port := utils.GetRandomAvailableTCPPort()
	target := `POST / HTTP/1.1
Host: ` + `127.0.0.1:` + fmt.Sprint(port) + `
Connection: keep-alive
Content-Length: 48
Transfer-Encoding: chunked, chunked

0` + lowhttp.CRLF + lowhttp.CRLF + `GET /admin HTTP/1.1` + lowhttp.CRLF + `Host: 127.0.0.1:8080` + lowhttp.CRLF + lowhttp.CRLF

	spew.Dump(target)
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

func TestGRPCMUSTPASS_Smuggle_Plugin_Positive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	port := utils.GetRandomAvailableTCPPort()
	go func() {
		vulinbox.Smuggle(ctx, port)
	}()
	err := utils.WaitConnect(utils.HostPort("127.0.0.1", port), 5)
	if err != nil {
		t.Fatal(err)
	}

	coreplugin.InitDBForTest()

	codeBytes := coreplugin.GetCorePluginData("HTTP请求走私")
	if codeBytes == nil {
		t.Errorf("无法从bindata获取%v", "HTTP请求走私.yak")
		return
	}
	client, err := yakgrpc.NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code:       string(codeBytes),
		PluginType: "mitm",
		Input:      utils.HostPort("127.0.0.1", port),
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			IsRawHTTPRequest: false,
			IsHttps:          false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var runtimeId string
	for {
		exec, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				break
			}
			log.Warn(err)
		}
		if runtimeId == "" {
			runtimeId = exec.RuntimeID
		}
	}

	checked := false
	for r := range yakit.YieldRisksByRuntimeId(consts.GetGormProjectDatabase(), ctx, runtimeId) {
		log.Infof("Risk: %v", r)
		if r.Port == port && strings.Contains(r.Title, "Smuggle") {
			checked = true
		}
	}
	if !checked {
		t.Fatal("risk not found")
	}
}
