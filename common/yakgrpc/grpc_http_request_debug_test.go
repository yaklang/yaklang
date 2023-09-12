package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestServer_DebugPlugin_TestFlow(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(1000))
	defer cancel()

	var count = 0
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		var raw, _ = utils.HttpDumpWithBody(request, true)
		spew.Dump(raw)
		writer.Write(raw)
		count++
	})

	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `mirrorHTTPFlow = (tls, url, req, rsp, body) => {
	// 3
	fuzz.HTTPRequest(req)~.ExecFirst()
	fuzz.HTTPRequest(req)~.ExecFirst()
	fuzz.HTTPRequest(req)~.ExecFirst()
	for result in fuzz.HTTPRequest(req)~.Repeat(5).Exec()~ {}
}`,
		PluginType:          "mitm",
		Input:               "http://" + utils.HostPort(host, port) + "/",
		HTTPRequestTemplate: nil,
	})
	if err != nil {
		t.Errorf("DebugPlugin error: %v", err)
		t.FailNow()
	}

	var runtimeId string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if runtimeId == "" {
			runtimeId = rsp.GetRuntimeID()
		}
		spew.Dump(rsp)
	}

	rsp, err := client.QueryHTTPFlows(ctx, &ypb.QueryHTTPFlowRequest{RuntimeId: runtimeId})
	if err != nil {
		t.Fatal(err)
	}
	total := rsp.GetTotal()
	t.Log("total: ", total)
	if total != int64(count) && total >= 8 {
		t.Errorf("total: %d != count: %d", total, count)
	}
}

func TestServer_DebugPlugin_MITM_WithRawPacketAndPaths(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	aPass := false
	bPass := false
	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
		var raw, _ = utils.HttpDumpWithBody(request, true)
		spew.Dump(raw)
		if strings.Contains(string(raw), "GET /a?a=1") {
			aPass = true
		}
		if strings.Contains(string(raw), "POST /b?b=1") && strings.Contains(string(raw), `Cookie: d=1`) {
			bPass = true
		}

		if aPass && bPass {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	})
	var targetUrl = "http://" + utils.HostPort(host, port) + "/a?a=1"

	var token = utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
	count++
	db.SetKey(token, count)
	dump(req)
}`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			IsRawHTTPRequest: false,
			Method:           "POST",
			Path:             []string{"/b"},
			GetParams:        []*ypb.KVPair{{Key: "b", Value: "1"}},
			Cookie:           []*ypb.KVPair{{Key: "d", Value: "1"}},
		},
		Input: targetUrl,
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
	count := codec.Atoi(yakit.Get(token))
	t.Logf("count: %d", count)
	if count != 2 {
		panic("count should be 2")
	}
	if !aPass {
		panic("a should pass")
	}

	if !bPass {
		panic("b should pass")
	}
}

func TestServer_DebugPlugin_MITM_WithRawPacket(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	aPass := false
	bPass := false
	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
		var raw, _ = utils.HttpDumpWithBody(request, true)
		spew.Dump(raw)
		if strings.Contains(string(raw), "GET /a?a=1") {
			aPass = true
		}
		if strings.Contains(string(raw), "POST /b?b=1") {
			bPass = true
		}

		if aPass && bPass {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	})
	var targetUrl = "http://" + utils.HostPort(host, port) + "/a?a=1"

	var token = utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
	count++
	db.SetKey(token, count)
	dump(req)
}`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			IsRawHTTPRequest: true,
			RawHTTPRequest: []byte(`POST /b?b=1 HTTP/1.1
Host: www.example.com
User-Agent: xxx

`),
		},
		Input: targetUrl,
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
	count := codec.Atoi(yakit.Get(token))
	t.Logf("count: %d", count)
	if count != 2 {
		panic("count should be 2")
	}
	if !aPass {
		panic("a should pass")
	}

	if !bPass {
		panic("b should pass")
	}
}

func TestServer_DebugPlugin_MITM(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	aPass := false
	bPass := false
	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
		var raw, _ = utils.HttpDumpWithBody(request, true)
		spew.Dump(raw)
		if strings.Contains(string(raw), "a?a=1") {
			aPass = true
		}
		if strings.Contains(string(raw), "b?b=1") {
			bPass = true
		}

		if aPass && bPass {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	})
	var targetUrl = "http://" + utils.HostPort(host, port)

	var token = utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
	count++
	db.SetKey(token, count)
	dump(url)
}`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path: []string{"a?a=1", "b?b=1"},
		},
		Input: targetUrl,
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
	count := codec.Atoi(yakit.Get(token))
	t.Logf("count: %d", count)
	if count != 2 {
		panic("count should be 2")
	}
	if !aPass {
		panic("a should pass")
	}

	if !bPass {
		panic("b should pass")
	}
}

func TestServer_DebugPlugin_MITM_WithURLTARGET(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	aPass := false
	bPass := false
	cPass := false
	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
		var raw, _ = utils.HttpDumpWithBody(request, true)
		spew.Dump(raw)
		if strings.Contains(string(raw), "a?a=1") {
			aPass = true
		}
		if strings.Contains(string(raw), "b?b=1") {
			bPass = true
		}
		if strings.Contains(string(raw), "c?c=1") {
			cPass = true
		}

		if aPass && bPass && cPass {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	})
	var targetUrl = "http://" + utils.HostPort(host, port) + "/c?c=1"

	var token = utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
	count++
	db.SetKey(token, count)
	dump(url)
}`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path: []string{"a?a=1", "b?b=1"},
		},
		Input: targetUrl,
	})
	if err != nil {
		panic(err)
	}
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
	count := codec.Atoi(yakit.Get(token))
	t.Logf("count: %d", count)
	if count != 3 {
		panic("count should be 3")
	}
	if !aPass {
		panic("a should pass")
	}

	if !bPass {
		panic("b should pass")
	}
	if !cPass {
		panic("c should pass")
	}
}
