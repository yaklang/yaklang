package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/yaklang/yaklang/common/consts"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTP_Server_DebugPlugin_TestFlow(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(utils.TimeoutContextSeconds(1000))
	defer cancel()

	count := 0
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		raw, _ := utils.HttpDumpWithBody(request, true)
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
	// 5
    for result in fuzz.HTTPRequest(req)~.Repeat(5).Exec()~ {}
	// 4
	http.Get(url)~
    http.Post(url)~
    http.Request("DELETE", url)~
    http.Do(http.NewRequest("PUT", url)~)
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
	if total != int64(count) && total >= 3+5+4 {
		t.Errorf("total: %d != count: %d", total, count)
	}
}

func TestGRPCMUSTPASS_HTTP_Server_DebugPlugin_MITM_WithRawPacketAndPaths(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	aPass := false
	bPass := false
	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
		raw, _ := utils.HttpDumpWithBody(request, true)
		spew.Dump(raw)
		if strings.Contains(string(raw), "POST /a?b=1&a=1") {
			aPass = true
		}
		if strings.Contains(string(raw), "POST /b?b=1&a=1") && strings.Contains(string(raw), `Cookie: d=1`) {
			bPass = true
		}

		if aPass && bPass {
			go func() {
				time.Sleep(2 * time.Second)
				cancel()
			}()
		}
	})
	targetUrl := "http://" + utils.HostPort(host, port) + "/a?a=1"

	token := utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
lock = sync.NewMutex()
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
    lock.Lock()
    defer lock.Unlock()
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
		panic("count should be 4")
	}
	if !aPass {
		panic("a should pass")
	}

	if !bPass {
		panic("b should pass")
	}
}

//func TestGRPCMUSTPASS_HTTP_Server_DebugPlugin_MITM_WithRawPacket(t *testing.T) {
//    client, err := NewLocalClient()
//    if err != nil {
//        panic(err)
//    }
//
//    aPass := false
//    bPass := false
//    ctx, cancel := context.WithCancel(context.Background())
//    host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
//        writer.Write([]byte("hello"))
//        var raw, _ = utils.HttpDumpWithBody(request, true)
//        spew.Dump(raw)
//        if strings.Contains(string(raw), "GET /a?a=1") {
//            aPass = true
//        }
//        if strings.Contains(string(raw), "POST /b?b=1") {
//            bPass = true
//        }
//
//        if aPass && bPass {
//            go func() {
//                time.Sleep(2 * time.Second)
//                cancel()
//            }()
//        }
//    })
//    var targetUrl = "http://" + utils.HostPort(host, port) + "/a?a=1"
//
//    var token = utils.RandStringBytes(20)
//    stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
//        Code: `token = ` + strconv.Quote(token) + `;
//var count = 0;
//lock = sync.NewMutex()
//mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
//    lock.Lock()
//    defer lock.Unlock()
//    count++
//    db.SetKey(token, count)
//    dump(req)
//}`,
//        PluginType: "mitm",
//        HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
//            IsRawHTTPRequest: true,
//            RawHTTPRequest: []byte(`POST /b?b=1 HTTP/1.1
//Host: www.example.com
//User-Agent: xxx
//
//`),
//        },
//        Input: targetUrl,
//    })
//    if err != nil {
//        panic(err)
//    }
//    for {
//        rsp, err := stream.Recv()
//        if err != nil {
//            break
//        }
//        spew.Dump(rsp)
//    }
//    count := codec.Atoi(yakit.Get(token))
//    t.Logf("count: %d", count)
//    if count != 1 {
//        panic("count should be 1")
//    }
//    if aPass {
//        panic("a should not pass")
//    }
//
//    if !bPass {
//        panic("b should pass")
//    }
//}

func TestGRPCMUSTPASS_HTTP_Server_DebugPlugin_MITM(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	aPass := false
	bPass := false
	ctx, cancel := context.WithCancel(context.Background())
	host, port := utils.DebugMockHTTPHandlerFuncContext(ctx, func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("hello"))
		raw, _ := utils.HttpDumpWithBody(request, true)
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
	targetUrl := "http://" + utils.HostPort(host, port)

	token := utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
lock = sync.NewMutex()
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
    lock.Lock()
    defer lock.Unlock()
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
}

func TestGRPCMUSTPASS_HTTP_Server_DebugPlugin_MITM_WithURLTARGET(t *testing.T) {
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
		raw, _ := utils.HttpDumpWithBody(request, true)
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
	targetUrl := "http://" + utils.HostPort(host, port) + "/c?c=1"

	token := utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
var count = 0;
lock = sync.NewMutex()
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
    lock.Lock()
    defer lock.Unlock()
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

func TestGRPCMUSTPASS_HTTP_FuzzPacket(t *testing.T) {
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
			if !rsp.IsMessage {
				continue
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
	targetUrl := "http://" + utils.HostPort(host, port) + "/aa"
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

func TestGRPCMUSTPASS_HTTP_CodecDebug(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	codecString := utils.RandStringBytes(10)
	expected := codec.EncodeBase64(codecString)
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code: `handle = func(a){
return codec.EncodeBase64(a)
}
`,
		PluginType: "codec",
		Input:      codecString,
	})
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	for {
		exec, err := stream.Recv()
		if err != nil {
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), expected) {
				checked = true
			}
		}
	}
	if !checked {
		t.Fatal("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_HTTP_YakDebug(t *testing.T) {
	client, err := NewLocalClient()
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 0

`))
	if err != nil {
		t.Fatal(err)
	}
	codecString := utils.RandStringBytes(10)
	expected := codec.EncodeBase64(codecString)
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code: fmt.Sprintf(`s = cli.String("s")
b = cli.Bool("b")
cli.check()
yakit.EnableWebsiteTrees("sssss")
poc.Get("%s",poc.save(true))
if b {
yakit.Output(codec.EncodeBase64(s))
}
`, utils.HostPort(host, port)),
		PluginType: "yak",
		ExecParams: []*ypb.KVPair{{Key: "s", Value: codecString}, {Key: "b", Value: "true"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	for {
		exec, err := stream.Recv()
		if err != nil {
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), expected) {
				checked = true
			}
		}
	}
	if !checked {
		t.Fatal("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_Yak_Debug_Context(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	serverPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code:       fmt.Sprintf(`httpserver.Serve("127.0.0.1",%d)`, serverPort),
		PluginType: "yak",
	})
	if err != nil {
		t.Fatal(err)
	}
	flag := false
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !utils.IsTCPPortAvailable(serverPort) { // 不可用即开
			flag = true
			break
		}
	}
	if flag {
		cancel()
		time.Sleep(2 * time.Second)
		if !utils.IsTCPPortAvailable(serverPort) {
			t.Fatal("context close server port failed")
		}
	} else {
		cancel()
		t.Fatal("start server port failed")
	}
}

func TestGRPCMUSTPASS_Codec_Debug_Context(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	serverPort := utils.GetRandomAvailableTCPPort()
	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: fmt.Sprintf(`handle = func(i){
httpserver.Serve("127.0.0.1",%d)}`, serverPort),
		PluginType: "codec",
		Input:      "aaa",
	})
	if err != nil {
		t.Fatal(err)
	}
	flag := false
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !utils.IsTCPPortAvailable(serverPort) { // 不可用即开
			flag = true
			break
		}
	}
	if flag {
		cancel()
		time.Sleep(2 * time.Second)
		if !utils.IsTCPPortAvailable(serverPort) {
			t.Fatal("context close server port failed")
		}
	} else {
		cancel()
		t.Fatal("start server port failed")
	}
}

func TestGRPCMUSTPASS_MITM_Debug_Context(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	serverPort := utils.GetRandomAvailableTCPPort()
	host, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK
Content-Length: 0

`))
	ctx, cancel := context.WithCancel(context.Background())
	_, err = client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: fmt.Sprintf(`mirrorHTTPFlow = func(isHttps /*bool*/, url /*string*/, req /*[]byte*/, rsp /*[]byte*/, body /*[]byte*/) {
httpserver.Serve("127.0.0.1",%d)}`, serverPort),
		PluginType: "mitm",
		Input:      utils.HostPort(host, port),
	})
	if err != nil {
		t.Fatal(err)
	}
	flag := false
	for i := 0; i < 10; i++ {
		time.Sleep(500 * time.Millisecond)
		if !utils.IsTCPPortAvailable(serverPort) { // 不可用即开
			flag = true
			break
		}
	}
	if flag {
		cancel()
		time.Sleep(2 * time.Second)
		if !utils.IsTCPPortAvailable(serverPort) {
			t.Fatal("context close server port failed")
		}
	} else {
		cancel()
		t.Fatal("start server port failed")
	}
}

func TestGRPCMUSTPASS_MITM_Debug_BoolParams(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	randStr := utils.RandStringBytes(10)
	ctx, _ := context.WithCancel(context.Background())
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: fmt.Sprintf(`a = cli.Bool("a")
cli.check()
if !a{
yakit.Output("%s")
}`, randStr),
		PluginType: "yak",
		ExecParams: []*ypb.KVPair{},
	})
	if err != nil {
		t.Fatal(err)
	}

	ok := false
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.IsMessage && bytes.Contains(rsp.Message, []byte(randStr)) {
			ok = true
		}
		spew.Dump(rsp)
	}
	if !ok {
		t.Fatal("bool param check err")
	}
}

func TestGRPCMUSTPASS_DebugPlugin_Nuclei(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	ok := false
	host, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		ok = true
		return []byte("HTTP/1.1 200 OK\nContent-Length: 5\n\nHello")
	})
	// randStr := utils.RandStringBytes(10)
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code: fmt.Sprintf(`id: basic-example
info:
    name: Test HTTP Template
http:
  - method: GET
    path:
        - "{{BaseURL}}"
    matchers:
        - type: status
          status:
            - 200`),
		PluginType: "nuclei",
		ExecParams: []*ypb.KVPair{},
		Input:      utils.HostPort(host, port),
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		rsp, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			t.Errorf("recv Error: %v", err)
		}
		spew.Dump(rsp)
	}
	if !ok {
		t.Fatal("nuclei check error")
	}
}

func TestDebug_Plugin_Cancel_Check_ForChan(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	code := `
	time.AfterFunc(1, func(){
		println("Exit ok")
		os.Exit(0)
	})
	ch = make(chan var)
	for i in ch{
}
`

	okChan := make(chan struct{})
	go func() {
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			Code:       code,
			PluginType: "yak",
			ExecParams: []*ypb.KVPair{},
		})
		if err != nil {
			t.Fatal(err)
		}

		for {
			rsp, err := stream.Recv()
			if err != nil {
				break
			}
			spew.Dump(rsp)
		}
		okChan <- struct{}{}
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("cancel fail")
	case <-okChan:
	}
}

func TestDebug_Plugin_Cancel_Check_Chan(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	code := `
	time.AfterFunc(1, func(){
		println("Exit ok")
		os.Exit(0)
	})
	ch = make(chan var)
	<- ch
`

	var okChan = make(chan struct{})
	go func() {
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			Code:       code,
			PluginType: "yak",
			ExecParams: []*ypb.KVPair{},
		})
		if err != nil {
			t.Fatal(err)
		}

		for {
			rsp, err := stream.Recv()
			if err != nil {
				break
			}
			spew.Dump(rsp)
		}
		okChan <- struct{}{}
	}()

	select {
	case <-time.After(5 * time.Second):
		t.Fatal("cancel fail")
	case <-okChan:
	}
}

func TestDebug_Plugin_cli_Yakitplugin(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	code := `
scriptNames = cli.YakitPlugin()
cli.check()
`

	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       code,
		PluginType: "yak",
		ExecParams: []*ypb.KVPair{},
	})
	if err != nil {
		t.Fatal(err)
	}

	for {
		rsp, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("check YakitPlugin param error:%s", err)
			}
			break
		}
		spew.Dump(rsp)
	}

}

func TestDebugPluginRiskCount(t *testing.T) {
	t.Run("create risk count", func(t *testing.T) {
		client, err := NewLocalClient()
		if err != nil {
			t.Fatal(err)
		}
		code := `
for i in 10 {
a = risk.NewRisk("127.0.0.1")
}
`
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			Code:       code,
			PluginType: "yak",
			ExecParams: []*ypb.KVPair{},
		})
		if err != nil {
			t.Fatal(err)
		}

		var cardCount = 0
		var riskMessageCount = 0
		var runtimeID string
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("check YakitPlugin param error:%s", err)
				}
				break
			}

			if rsp.RuntimeID != "" {
				runtimeID = rsp.RuntimeID
			}

			if rsp.GetIsMessage() {
				level := gjson.Get(string(rsp.GetMessage()), "content.level").String()
				if level == "feature-status-card-data" {
					data := gjson.Get(string(rsp.GetMessage()), "content.data").String()
					if gjson.Get(data, "id").String() == "漏洞/风险/指纹" {
						cardCount = int(gjson.Get(data, "data").Int())
					}
				} else if level == "json-risk" {
					riskMessageCount++
				}
			}
		}
		riskCount, err := yakit.CountRiskByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
		require.NoError(t, err)
		require.Equal(t, cardCount, riskCount, "risk count not match")
		require.Equal(t, cardCount, riskMessageCount, "risk message count not match")
	})

	t.Run("poc risk count", func(t *testing.T) {
		server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

aaa`))
		target := utils.HostPort(server, port)
		client, err := NewLocalClient()
		if err != nil {
			t.Fatal(err)
		}
		code := `
id: WebFuzzer-Template-dwgZTlBz

info:
  name: WebFuzzer Template dwgZTlBz
  author: god
  severity: low
  description: write your description here
  reference:
  - https://github.com/
  - https://cve.mitre.org/
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: bf1b38820525a92e811300bf655c8fe7

http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: '{"key": "value"}'

  max-redirects: 3
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: status
    words:
    - "200"
    condition: and


# Generated From WebFuzzer on 2024-06-13 16:14:16
`
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			Code:       code,
			PluginType: "nuclei",
			ExecParams: []*ypb.KVPair{},
			Input:      target,
		})
		if err != nil {
			t.Fatal(err)
		}

		var cardCount = 0
		var runtimeID string
		var riskMessageCount = 0
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("check YakitPlugin param error:%s", err)
				}
				break
			}
			if rsp.RuntimeID != "" {
				runtimeID = rsp.RuntimeID
			}
			if rsp.GetIsMessage() {
				level := gjson.Get(string(rsp.GetMessage()), "content.level").String()
				if level == "feature-status-card-data" {
					data := gjson.Get(string(rsp.GetMessage()), "content.data").String()
					if gjson.Get(data, "id").String() == "漏洞/风险/指纹" {
						cardCount = int(gjson.Get(data, "data").Int())
					}
				} else if level == "json-risk" {
					riskMessageCount++
				}
			}
		}

		riskCount, err := yakit.CountRiskByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
		require.NoError(t, err)
		require.Equal(t, cardCount, riskCount, "risk count not match")
		require.Equal(t, cardCount, riskMessageCount, "risk message count not match")
	})
}

var nucleiCode = `id: WebFuzzer-Template-dwgZTlBz

info:
  name: WebFuzzer Template dwgZTlBz
  author: god
  severity: low
  description: write your description here
  reference:
  - https://github.com/
  - https://cve.mitre.org/
  metadata:
    max-request: 1
    shodan-query: ""
    verified: true
  yakit-info:
    sign: bf1b38820525a92e811300bf655c8fe7

http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: '{"key": "value"}'

  max-redirects: 3
  matchers-condition: and
  matchers:
  - id: 1
    type: word
    part: status
    words:
    - "200"
    condition: and


# Generated From WebFuzzer on 2024-06-13 16:14:16
`
var yakCode = `
 risk.NewRisk("127.0.0.1")
`

func TestGRPCMUSTPASS_DebugPlugin_Risk_PluginMetaInfo(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	yakScript, err := yakit.NewTemporaryYakScript("yak", yakCode)
	require.NoError(t, err)
	yakScript.Uuid = uuid.New().String()
	err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), yakScript.ScriptName, yakScript)
	require.NoError(t, err)

	nucleiScript, err := yakit.NewTemporaryYakScript("nuclei", nucleiCode)
	require.NoError(t, err)
	nucleiScript.Uuid = uuid.New().String()
	err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), nucleiScript.ScriptName, nucleiScript)
	require.NoError(t, err)

	server, port := utils.DebugMockHTTP([]byte(`HTTP/1.1 200 OK

aaa`))
	target := utils.HostPort(server, port)
	t.Run("nuclei risk info", func(t *testing.T) {
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			PluginName: nucleiScript.ScriptName,
			PluginType: "nuclei",
			ExecParams: []*ypb.KVPair{},
			Input:      target,
		})
		if err != nil {
			t.Fatal(err)
		}

		var runtimeID string
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("check YakitPlugin param error:%s", err)
				}
				break
			}

			if rsp.RuntimeID != "" {
				runtimeID = rsp.RuntimeID
			}
		}

		risks, err := yakit.GetRisksByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
		require.NoError(t, err)
		for _, riskIns := range risks {
			require.Equal(t, riskIns.YakScriptUUID, nucleiScript.Uuid, "nuclei uuid not match")
			require.Equal(t, riskIns.FromYakScript, nucleiScript.ScriptName, "nuclei name not match")
		}
	})

	t.Run("yak risk info", func(t *testing.T) {
		stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
			PluginName: yakScript.ScriptName,
			PluginType: "yak",
			ExecParams: []*ypb.KVPair{},
			Input:      target,
		})
		if err != nil {
			t.Fatal(err)
		}

		var runtimeID string
		for {
			rsp, err := stream.Recv()
			if err != nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("check YakitPlugin param error:%s", err)
				}
				break
			}

			if rsp.RuntimeID != "" {
				runtimeID = rsp.RuntimeID
			}
		}

		risks, err := yakit.GetRisksByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
		require.NoError(t, err)
		for _, riskIns := range risks {
			require.Equal(t, riskIns.YakScriptUUID, yakScript.Uuid, "nuclei uuid not match")
			require.Equal(t, riskIns.FromYakScript, yakScript.ScriptName, "nuclei name not match")
		}
	})

}

func TestGRPCMUSTPASS_DebugPlugin_Nuclei_Risk_URL(t *testing.T) {
	randPath := utils.RandStringBytes(5)
	token := utils.RandStringBytes(5)
	nucleiScript, err := yakit.NewTemporaryYakScript("nuclei", fmt.Sprintf(`id: test 

info:
  name: test 
  author: god
  severity: critical
  description: test

http:
- raw:
  - |-
    @timeout: 30s
    GET /%s HTTP/1.1
    Host: {{Hostname}}

  max-redirects: 3
  matchers-condition: and
  matchers:
      - type: word
        words:
          - "%s"
        part: body`, randPath, token))
	require.NoError(t, err)
	err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), nucleiScript.ScriptName, nucleiScript)
	require.NoError(t, err)
	defer yakit.DeleteYakScriptByName(consts.GetGormProfileDatabase(), nucleiScript.ScriptName)
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(token))
	})
	targetUrl := fmt.Sprintf("http://%s:%d/%s", host, port, randPath)
	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		PluginName: nucleiScript.ScriptName,
		PluginType: "nuclei",
		ExecParams: []*ypb.KVPair{},
		Input:      targetUrl,
	})
	require.NoError(t, err)

	var runtimeID string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				t.Fatalf("check YakitPlugin param error:%s", err)
			}
			break
		}
		if rsp.RuntimeID != "" {
			runtimeID = rsp.RuntimeID
		}
	}

	risks, err := yakit.GetRisksByRuntimeId(consts.GetGormProjectDatabase(), runtimeID)
	require.NoError(t, err)
	require.Len(t, risks, 1)
	for _, risk := range risks {
		require.Equal(t, risk.Url, targetUrl)
	}
}

func TestGRPCMUSTPASS_DebugPlugin_MITM_Cli(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n\r\n"))
	targetUrl := "http://" + utils.HostPort(host, port)

	token := utils.RandStringBytes(20)
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code: `token = ` + strconv.Quote(token) + `;
test = cli.String("test", cli.setRequired(true))
cli.check()
mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
    yakit.Output(test)
}`,
		PluginType: "mitm",
		Input:      targetUrl,
		ExecParams: []*ypb.KVPair{{Key: "test", Value: token}},
	})
	require.NoError(t, err)
	var tokenCheck bool
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.IsMessage {
			if bytes.Contains(rsp.Message, []byte(token)) {
				tokenCheck = true
			}
		}
	}
	require.True(t, tokenCheck, "debug plugin mitm cli fail")
}
