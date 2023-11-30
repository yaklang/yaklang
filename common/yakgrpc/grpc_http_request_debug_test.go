package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_Server_DebugPlugin_TestFlow(t *testing.T) {
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

func TestGRPCMUSTPASS_Server_DebugPlugin_MITM_WithRawPacketAndPaths(t *testing.T) {
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

func TestGRPCMUSTPASS_Server_DebugPlugin_MITM_WithRawPacket(t *testing.T) {
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
	if count != 1 {
		panic("count should be 1")
	}
	if aPass {
		panic("a should not pass")
	}

	if !bPass {
		panic("b should pass")
	}
}

func TestGRPCMUSTPASS_Server_DebugPlugin_MITM(t *testing.T) {
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

func TestGRPCMUSTPASS_Server_DebugPlugin_MITM_WithURLTARGET(t *testing.T) {
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

func TestGRPCMUSTPASS_FuzzPacket(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	ctx := context.Background()
	var getElement func(dict any, keys ...string) any
	getElement = func(dict any, keys ...string) any {
		if len(keys) <= 0 {
			return dict
		}
		if dict == nil {
			return nil
		}
		refV := reflect.ValueOf(dict)
		if refV.Kind() == reflect.Map {
			if refV.MapIndex(reflect.ValueOf(keys[0])).IsValid() {
				return getElement(refV.MapIndex(reflect.ValueOf(keys[0])).Interface(), keys[1:]...)
			}
		}
		return nil
	}
	fuzz := func(targetUrl string, cfg *ypb.DebugPluginRequest) []any {
		stream, err := client.DebugPlugin(ctx, cfg)
		if err != nil {
			panic(err)
		}
		res := []any{}
		for {
			rsp, err := stream.Recv()
			if err != nil {
				break
			}
			dict := make(map[string]any)
			err = json.Unmarshal(rsp.Message, &dict)
			if err != nil {
				panic(err)
			}
			data := getElement(dict, "content", "data")
			if data == nil {
				continue
			}
			dataJson := utils.InterfaceToString(data)
			err = json.Unmarshal([]byte(dataJson), &dict)
			if err != nil {
				continue
			}

			if getElement(dict, "ok") != nil {
				res = append(res, getElement(dict, "data"))
			}
		}
		return res
	}

	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
	})
	marshalResult := func(result []any) string {
		newRes := []string{}
		for _, r := range result {
			m := r.(map[string]any)
			newRes = append(newRes, spew.Sprintf("%v:%v", m["req"], m["https"]))
		}
		sort.Strings(newRes)
		raw, err := json.Marshal(newRes)
		if err != nil {
			panic(err)
		}
		return string(raw)
	}
	// path 合并
	var targetUrl = "http://" + utils.HostPort(host, port) + "/aa"
	res := fuzz(targetUrl, &ypb.DebugPluginRequest{
		Code: `
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	res = {}
	res["ok"] = 1
	res["data"] = {"req":string(req),"https":isHttps}
	yakit.Output(res)
}
`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path: []string{"bb"},
		},
		Input: targetUrl,
	})
	expect := []any{
		map[string]any{
			"https": false,
			"req":   "GET /aa HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\n\r\n",
		},
		map[string]any{
			"https": false,
			"req":   "GET /bb HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\n\r\n",
		},
	}
	assert.Equal(t, marshalResult(expect), marshalResult(res))

	// https 优先级
	targetUrl = "http://" + utils.HostPort(host, port) + "/aa"
	res = fuzz(targetUrl, &ypb.DebugPluginRequest{
		Code: `
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	res = {}
	res["ok"] = 1
	res["data"] = {"req":string(req),"https":isHttps}
	yakit.Output(res)
}
`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path:    []string{"bb"},
			IsHttps: true,
		},
		Input: targetUrl,
	})
	expect = []any{
		map[string]any{
			"https": false,
			"req":   "GET /aa HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\n\r\n",
		},
		map[string]any{
			"https": false,
			"req":   "GET /bb HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\n\r\n",
		},
	}
	assert.Equal(t, marshalResult(expect), marshalResult(res))
	// https 优先级
	targetUrl = "http://" + utils.HostPort(host, port) + "/aa"
	res = fuzz(targetUrl, &ypb.DebugPluginRequest{
		Code: `
mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
	res = {}
	res["ok"] = 1
	res["data"] = {"req":string(req),"https":isHttps}
	yakit.Output(res)
}
`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path:    []string{"bb"},
			Headers: []*ypb.KVPair{{Key: "Tag", Value: "123"}},
			IsHttps: true,
		},
		Input: targetUrl,
	})
	expect = []any{
		map[string]any{
			"https": false,
			"req":   "GET /aa HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\nTag: 123\r\n\r\n",
		},
		map[string]any{
			"https": false,
			"req":   "GET /bb HTTP/1.1\r\nHost: " + utils.HostPort(host, port) + "\r\nTag: 123\r\n\r\n",
		},
	}
	assert.Equal(t, marshalResult(expect), marshalResult(res))
}
