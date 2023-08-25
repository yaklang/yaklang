package yakgrpc

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
)

func TestGRPCMUSTPASS_DebugPlugin_SmockingWithEmptyInput(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("a"))
	})
	target := utils.HostPort(host, port)

	stream, err := client.DebugPlugin(utils.TimeoutContextSeconds(3), &ypb.DebugPluginRequest{
		Code: `mirrorFilteredHTTPFlow = (https, url, req, rsp, body) => {
	dump(url)
}`,
		PluginType: "mitm",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			IsRawHTTPRequest: true,
			RawHTTPRequest: []byte(`GET / HTTP/1.1
Host: ` + target + `
`),
		},
	})
	if err != nil {
		spew.Dump(err)
		panic(err)
	}

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestGRPCMUSTPASS_BuildHTTPRequest_Results(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		IsRawHTTPRequest: true,
		RawHTTPRequest: []byte(`GET /ac?a=1 HTTP/1.1
Host: baidu.com
`),
	})
	if err != nil {
		panic(err)
	}
	keepPathQuery := false
	for _, i := range rsp.Results {
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "/ac?a=1", `{{Hostname}}`) {
			keepPathQuery = true
		}
	}
	if !keepPathQuery {
		panic("path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		IsHttps: true,
		RawHTTPRequest: []byte(`GET /ac?a=1 HTTP/1.1
Host: baidu.com
`),
		Method:    "GET",
		Path:      []string{"a?c=1", "d?e=2"},
		GetParams: []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
	})
	if err != nil {
		panic(err)
	}
	count := 0
	ceq1 := false
	eeq2 := false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?", "a=1", "b=2", "{{Hostname}}", "c=1") {
			ceq1 = true
		}
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "d?", "e=2", "{{Hostname}}", "a=1", "b=2") {
			eeq2 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 2 {
		panic("count not match, expect 2, got: " + spew.Sprint(count))
	}
	if !ceq1 || !eeq2 {
		panic("no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(
		context.Background(),
		&ypb.HTTPRequestBuilderParams{
			IsHttps: true,
			RawHTTPRequest: []byte(`GET /ac?a=1 HTTP/1.1
Host: baidu.com
`),
			Method:    "GET",
			Path:      []string{"a?c=1", "d?e=2"},
			GetParams: []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
			Headers: []*ypb.KVPair{
				{Key: "User-Agent", Value: "yaklangdebugger/1.1"},
			},
		},
	)
	if err != nil {
		panic(err)
	}
	count = 0
	ceq1 = false
	eeq2 = false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?", "a=1", "b=2", "{{Hostname}}", "c=1", "User-Agent: yaklang") {
			ceq1 = true
		}
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "d?", "e=2", "{{Hostname}}", "a=1", "b=2", "User-Agent: yaklang") {
			eeq2 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 2 {
		panic("header count not match, expect 2, got: " + spew.Sprint(count))
	}
	if !ceq1 || !eeq2 {
		panic("header no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		Cookie:              []*ypb.KVPair{{Key: "aaa", Value: "111"}, {Key: "bbb", Value: "222"}},
		Body:                nil,
		PostParams:          nil,
		MultipartParams:     nil,
		MultipartFileParams: nil,
	})
	if err != nil {
		panic(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?", "c=1", "{{Hostname}}", "Cookie: ", "aaa=111", "bbb=222") && utils.MatchAnyOfSubString(
			string(i.HTTPRequest), "Cookie: aaa=111; bbb=222", "Cookie: bbb=222; aaa=111") {
			ceq1 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 1 {
		panic("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		panic("cookie no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		Body:                []byte(`aabcdasdf`),
		PostParams:          nil,
		MultipartParams:     nil,
		MultipartFileParams: nil,
	})
	if err != nil {
		panic(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?c=1", "{{Hostname}}", "aabcdasdf") {
			ceq1 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 1 {
		panic("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		panic("body no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		PostParams:          []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		MultipartParams:     nil,
		MultipartFileParams: nil,
	})
	if err != nil {
		panic(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?c=1", "{{Hostname}}") &&
			utils.MatchAnyOfSubString(string(i.HTTPRequest), "a=1&b=2", "b=2&a=1") {
			ceq1 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 1 {
		panic("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		panic("body no raw (using) path query not keep")
	}

	// multipart
	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		MultipartParams:     []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		MultipartFileParams: nil,
	})
	if err != nil {
		panic(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(
			string(i.HTTPRequest),
			"a?c=1", "{{Hostname}}",
			"name=\"a\"\r\n\r\n1",
			"name=\"b\"\r\n\r\n2",
			"Content-Disposition: form-data;",
			"ype: multipart/form-data; boundary=",
		) {
			ceq1 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 1 {
		panic("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		panic("body no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		MultipartParams:     []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		MultipartFileParams: []*ypb.KVPair{{Key: "c", Value: "3"}},
	})
	if err != nil {
		panic(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(
			string(i.HTTPRequest),
			"a?c=1", "{{Hostname}}",
			"name=\"a\"\r\n\r\n1",
			"name=\"b\"\r\n\r\n2",
			"Content-Disposition: form-data;",
			"ype: multipart/form-data; boundary=",
			`filename="3"`,
		) {
			ceq1 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 1 {
		panic("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		panic("body no raw (using) path query not keep")
	}
}

func TestGRPCMUSTPASS_HTTPRequestBuilderWithDebug(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}
	rsp, err := client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		IsRawHTTPRequest: true,
		RawHTTPRequest: []byte(`GET / HTTP/1.1
Host: baidu.com

`),
	})
	if err != nil {
		panic(err)
	}
	if !strings.Contains(rsp.Templates, `Host: {{Hostname}}`) {
		panic("raw packet build failed")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Path: []string{"/admin-123", "/.wp?c=123"},
		GetParams: []*ypb.KVPair{
			{Key: "aaa", Value: "ccc"},
		},
		PostParams: []*ypb.KVPair{
			{Key: "cc", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
			{Key: "c1c", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
			{Key: "casdfa(*)(*()c", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
		},
	})
	if err != nil {
		panic(err)
	}
	println(rsp.Templates)
	if !strings.Contains(rsp.Templates, `{{BaseURL}}/admin-123?aaa=ccc`) {
		panic("raw packet build failed")
	}

	if len(rsp.GetResults()) <= 0 {
		panic("no http request is build")
	}
	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaabbbaaabbb`))

	var host, port = utils.DebugMockHTTP(rspRaw)
	log.Infof("start to decug mock http on: %v", utils.HostPort(host, port))
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       "yakit.AutoInitYakit(); handle = result => {dump(`executed in plugin`); dump(result); yakit.Info(`PLUGIN IS EXECUTED`);risk.NewRisk(`baidu.com`);}",
		PluginType: "port-scan",
		Input:      utils.HostPort(host, port),
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			Path: []string{"/admin-123", "/.wp?c=123"},
			GetParams: []*ypb.KVPair{
				{Key: "aaa", Value: "ccc"},
			},
			PostParams: []*ypb.KVPair{
				{Key: "cc", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
				{Key: "c1c", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
				{Key: "casdfa(*)(*()c", Value: "jklhadhio19u2439u1234*()HUOIY&T^*()^Y"},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	var checked = false
	for {
		t.Logf("stream.Recv() start...")
		exec, err := stream.Recv()
		println(spew.Sdump(exec))
		if err != nil {
			t.Logf("stream.Recv() error: %v", err)
			log.Warn(err)
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), "PLUGIN IS EXECUTED") {
				checked = true
			}
		}
	}
	if !checked {
		panic("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_HTTPRequestBuilderWithDebug2(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
	var host, port = utils.DebugMockHTTP(rspRaw)
	log.Infof("start to debug mock http on: %v", utils.HostPort(host, port))
	rsp, err := http.Get("http://" + utils.HostPort(host, port))
	if err != nil {
		panic(err)
	}
	raw, _ := utils.HttpDumpWithBody(rsp, true)
	println(string(raw))
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       "yakit.AutoInitYakit(); handle = result => {dump(`executed in plugin`); dump(result); yakit.Info(`PLUGIN IS EXECUTED`)}",
		PluginType: "port-scan",
		Input:      "http://" + utils.HostPort(host, port) + "/abc",
	})
	if err != nil {
		panic(err)
	}
	var checked = false
	for {
		exec, err := stream.Recv()
		if err != nil {
			log.Warn(err)
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), "PLUGIN IS EXECUTED") {
				checked = true
			}
		}
	}
	if !checked {
		panic("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_HTTPRequestBuilderWithDebug3(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		panic(err)
	}

	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
	var host, port = utils.DebugMockHTTP(rspRaw)
	log.Infof("start to debug mock http on: %v", utils.HostPort(host, port))
	rsp, err := http.Get("http://" + utils.HostPort(host, port))
	if err != nil {
		panic(err)
	}
	raw, _ := utils.HttpDumpWithBody(rsp, true)
	println(string(raw))
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       "mirrorHTTPFlow = (https, url, req, rsp, body) => { yakit.Info(`MESSAGE:FETCH URL :` + url); }",
		PluginType: "mitm",
		Input:      "http://" + utils.HostPort(host, port) + "/abc?key=value",
	})
	if err != nil {
		panic(err)
	}
	var checked = false
	for {
		exec, err := stream.Recv()
		if err != nil {
			log.Warn(err)
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), "MESSAGE:FETCH URL") ||
				strings.Contains(string(exec.Message), "/abc?key=value") {
				checked = true
			}
		}
	}
	if !checked {
		panic("plugin is not executed")
	}
}
