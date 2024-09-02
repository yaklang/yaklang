package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTP_DebugPlugin_NoMatcherNExtractors_YamlPOC(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	check := false
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("a"))
		check = true
	})
	target := utils.HostPort(host, port)

	stream, err := client.DebugPlugin(utils.TimeoutContextSeconds(4), &ypb.DebugPluginRequest{
		Code: `id: WebFuzzer-Template-gPdWZhvP

info:
  name: WebFuzzer Template gPdWZhvP
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
    sign: a948d87b1972d786c871bb68ef43b6b6

http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: '{"key": "value"}'

  max-redirects: 3
  matchers-condition: and

# Generated From WebFuzzer on 2024-03-23 09:36:56
`,
		PluginType: "nuclei",
		HTTPRequestTemplate: &ypb.HTTPRequestBuilderParams{
			IsRawHTTPRequest: true,
			RawHTTPRequest: []byte(`GET / HTTP/1.1
Host: ` + target + `
`),
		},
	})
	if err != nil {
		return
	}
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(data)
	}
	if !check {
		t.Fatal("failed to default matchers")
	}
}

func TestGRPCMUSTPASS_HTTP_DebugPlugin_SmockingWithEmptyInput(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
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
		t.Fatal(err)
	}

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		spew.Dump(rsp)
	}
}

func TestGRPCMUSTPASS_HTTP_BuildHTTPRequest_Results(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		IsRawHTTPRequest: true,
		RawHTTPRequest: []byte(`GET /ac?a=1 HTTP/1.1
Host: baidu.com
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	keepPathQuery := false
	for _, i := range rsp.Results {
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "/ac?a=1", `{{Hostname}}`) {
			keepPathQuery = true
		}
	}
	if !keepPathQuery {
		t.Fatal("path query not keep")
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
		t.Fatal(err)
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
		t.Fatal("count not match, expect 2, got: " + spew.Sprint(count))
	}
	if !ceq1 || !eeq2 {
		t.Fatal("no raw (using) path query not keep")
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
		t.Fatal(err)
	}
	count = 0
	ceq1 = false
	eeq2 = false
	for _, i := range rsp.Results {
		count++
		// fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?", "a=1", "b=2", "{{Hostname}}", "c=1", "User-Agent: yaklang") {
			ceq1 = true
		}
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "d?", "e=2", "{{Hostname}}", "a=1", "b=2", "User-Agent: yaklang") {
			eeq2 = true
		}
	}
	// t.Logf("count: %d", count)
	if count != 2 {
		t.Fatal("header count not match, expect 2, got: " + spew.Sprint(count))
	}
	if !ceq1 || !eeq2 {
		t.Fatal("header no raw (using) path query not keep")
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
		t.Fatal(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		// fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?", "c=1", "{{Hostname}}", "Cookie: ", "aaa=111", "bbb=222") && utils.MatchAnyOfSubString(
			string(i.HTTPRequest), "Cookie: aaa=111; bbb=222", "Cookie: bbb=222; aaa=111") {
			ceq1 = true
		}
	}
	t.Logf("count: %d", count)
	if count != 1 {
		t.Fatal("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		t.Fatal("cookie no raw (using) path query not keep")
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
		t.Fatal(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		// fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?c=1", "{{Hostname}}", "aabcdasdf") {
			ceq1 = true
		}
	}
	// t.Logf("count: %d", count)
	if count != 1 {
		t.Fatal("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		t.Fatal("body no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		PostParams:          []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		MultipartParams:     nil,
		MultipartFileParams: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		// fmt.Println(string(i.HTTPRequest))
		if utils.MatchAllOfSubString(string(i.HTTPRequest), "a?c=1", "{{Hostname}}") &&
			utils.MatchAnyOfSubString(string(i.HTTPRequest), "a=1&b=2", "b=2&a=1") {
			ceq1 = true
		}
	}
	// t.Logf("count: %d", count)
	if count != 1 {
		t.Fatal("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		t.Fatal("body no raw (using) path query not keep")
	}

	// multipart
	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		MultipartParams:     []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		MultipartFileParams: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		// fmt.Println(string(i.HTTPRequest))
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
	// t.Logf("count: %d", count)
	if count != 1 {
		t.Fatal("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		t.Fatal("body no raw (using) path query not keep")
	}

	rsp, err = client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		Method:              "GET",
		Path:                []string{"a?c=1"},
		MultipartParams:     []*ypb.KVPair{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}},
		MultipartFileParams: []*ypb.KVPair{{Key: "c", Value: "3"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	ceq1 = false
	for _, i := range rsp.Results {
		count++
		// fmt.Println(string(i.HTTPRequest))
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
	if count != 1 {
		t.Fatal("count not match, expect 1, got: " + spew.Sprint(count))
	}
	if !ceq1 {
		t.Fatal("body no raw (using) path query not keep")
	}
}

func TestGRPCMUSTPASS_HTTP_HTTPRequestBuilderWithDebug(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	rsp, err := client.HTTPRequestBuilder(context.Background(), &ypb.HTTPRequestBuilderParams{
		IsRawHTTPRequest: true,
		RawHTTPRequest: []byte(`GET / HTTP/1.1
Host: baidu.com

`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rsp.Templates, `Host: {{Hostname}}`) {
		t.Fatal("raw packet build failed")
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
		t.Fatal(err)
	}
	// println(rsp.Templates)
	if !strings.Contains(rsp.Templates, `{{BaseURL}}/admin-123?aaa=ccc`) {
		t.Fatal("raw packet build failed")
	}

	if len(rsp.GetResults()) <= 0 {
		t.Fatal("no http request is build")
	}
	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaabbbaaabbb`))

	host, port := utils.DebugMockHTTP(rspRaw)
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
		t.Fatal(err)
	}
	checked := false
	for {
		t.Logf("stream.Recv() start...")
		exec, err := stream.Recv()
		// println(spew.Sdump(exec))
		if err != nil {
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), "PLUGIN IS EXECUTED") {
				checked = true
			}
		}
	}
	if !checked {
		t.Fatal("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_HTTP_HTTPRequestBuilderWithDebug2(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
	host, port := utils.DebugMockHTTP(rspRaw)
	log.Infof("start to debug mock http on: %v", utils.HostPort(host, port))
	rsp, err := http.Get("http://" + utils.HostPort(host, port))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := utils.HttpDumpWithBody(rsp, true)
	_ = raw
	// println(string(raw))
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       "yakit.AutoInitYakit(); handle = result => {dump(`executed in plugin`); dump(result); yakit.Info(`PLUGIN IS EXECUTED`)}",
		PluginType: "port-scan",
		Input:      "http://" + utils.HostPort(host, port) + "/abc",
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
			if strings.Contains(string(exec.Message), "PLUGIN IS EXECUTED") {
				checked = true
			}
		}
	}
	if !checked {
		t.Fatal("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_HTTP_HTTPRequestBuilderWithDebug3(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
	host, port := utils.DebugMockHTTP(rspRaw)
	log.Infof("start to debug mock http on: %v", utils.HostPort(host, port))
	rsp, err := http.Get("http://" + utils.HostPort(host, port))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := utils.HttpDumpWithBody(rsp, true)
	_ = raw
	// println(string(raw))
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       "mirrorHTTPFlow = (https, url, req, rsp, body) => { yakit.Info(`MESSAGE:FETCH URL :` + url); }",
		PluginType: "mitm",
		Input:      "http://" + utils.HostPort(host, port) + "/abc?key=value",
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
			if strings.Contains(string(exec.Message), "MESSAGE:FETCH URL") ||
				strings.Contains(string(exec.Message), "/abc?key=value") {
				checked = true
			}
		}
	}
	if !checked {
		t.Fatal("plugin is not executed")
	}
}

func TestGRPCMUSTPASS_DebugPlugin(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
	host, port := utils.DebugMockHTTP(rspRaw)
	log.Infof("start to debug mock http on: %v", utils.HostPort(host, port))

	tempName1, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", "test")
	require.NoError(t, err)
	defer clearFunc()
	tempName2, clearFunc2, err := yakit.CreateTemporaryYakScriptEx("mitm", "test")
	require.NoError(t, err)
	defer clearFunc2()
	// println(string(raw))

	testCode := fmt.Sprintf(`yakit.AutoInitYakit()
pluginList = cli.YakitPlugin()
for p in pluginList{
    if p == "%s"{
        yakit.Info("load plugin by name ok")
    }
    if p == "%s"{
        yakit.Info("load plugin by filter ok")
    }
}
cli.check()
`, tempName1, tempName2)

	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       testCode,
		PluginType: "yak",
		LinkPluginConfig: &ypb.HybridScanPluginConfig{
			PluginNames: []string{tempName1},
			Filter: &ypb.QueryYakScriptRequest{
				Keyword:  tempName2,
				IsIgnore: true,
			},
		},
		Input: "http://" + utils.HostPort(host, port),
	})
	require.NoError(t, err)
	checkedName := false
	checkedFilter := false
	for {
		exec, err := stream.Recv()
		if err != nil {
			break
		}
		if string(exec.Message) != "" {
			if strings.Contains(string(exec.Message), "load plugin by name ok") {
				checkedName = true
			}
			if strings.Contains(string(exec.Message), "load plugin by filter ok") {
				checkedFilter = true
			}
		}
	}
	require.True(t, checkedName, "load plugin by name failed")

	require.True(t, checkedFilter, "load plugin by filter failed")
}

func TestBuild_Http_Request_Packet(t *testing.T) {
	targetInput := "baidu.com"
	p := &ypb.HTTPRequestBuilderParams{
		IsHttps:          false,
		IsRawHTTPRequest: false,
		Method:           "GET",
	}
	packets, err := BuildHttpRequestPacket(consts.GetGormProjectDatabase(), p, targetInput)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for packet := range packets {
		spew.Dump(packet)
		count++
	}
	if count != 1 {
		t.Fatal("build packet error")
	}

	p = &ypb.HTTPRequestBuilderParams{
		IsHttps:          false,
		IsRawHTTPRequest: false,
		Method:           "GET",
		Path:             []string{"/xyz", "/abc"},
	}
	packets, err = BuildHttpRequestPacket(consts.GetGormProjectDatabase(), p, targetInput)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	for packet := range packets {
		spew.Dump(packet)
		count++
	}
	if count != 3 {
		t.Fatal("build packet error")
	}
}

func TestBuild_Http_Request_Packet_Smoking(t *testing.T) {
	targetInput := "abc:accc"
	p := &ypb.HTTPRequestBuilderParams{
		IsHttps:          false,
		IsRawHTTPRequest: false,
		Method:           "GET",
	}
	packets, err := BuildHttpRequestPacket(consts.GetGormProjectDatabase(), p, targetInput)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for packet := range packets {
		spew.Dump(packet)
		count++
	}
	require.Equal(t, 0, count)
}

func TestGRPCMUSTPASS_DebugPlugin_ServiceScan_RuntimeId(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	rspRaw, _, _ := lowhttp.FixHTTPResponse([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
	host, port := utils.DebugMockHTTP(rspRaw)
	log.Infof("start to debug mock http on: %v", utils.HostPort(host, port))

	testCode := fmt.Sprintf(`yakit.AutoInitYakit()
res =  servicescan.Scan("%s", "%d")~
for i in res {
}
`, host, port)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stream, err := client.DebugPlugin(ctx, &ypb.DebugPluginRequest{
		Code:       testCode,
		PluginType: "yak",
	})
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.RuntimeID != "" {
			id = rsp.RuntimeID
		}
		spew.Dump(rsp)
	}

	if id == "" {
		t.Fatal("runtime id is empty")
	}
	_, flow, err := yakit.QueryHTTPFlow(consts.GetGormProjectDatabase(), &ypb.QueryHTTPFlowRequest{RuntimeId: id})
	if err != nil {
		t.Fatal(err)
	}

	require.Condition(t, func() (success bool) {
		return len(flow) > 0
	}, "flow set runtime error")
}

func TestGRPCMUSTPASS_HTTP_DebugPlugin_Global_SaveHTTPFlow(t *testing.T) {
	client, err := NewLocalClient(true)
	if err != nil {
		t.Fatal(err)
	}
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("a"))
	})

	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	config, err := client.GetGlobalNetworkConfig(context.Background(), &ypb.GetGlobalNetworkConfigRequest{})
	require.NoError(t, err)
	config.SkipSaveHTTPFlow = true
	client.SetGlobalNetworkConfig(context.Background(), config)

	target := "http://" + utils.HostPort(host, port)
	testTemplate := `id: WebFuzzer-Template-gPdWZhvP

info:
  name: WebFuzzer Template gPdWZhvP
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
    sign: a948d87b1972d786c871bb68ef43b6b6

http:
- method: POST
  path:
  - '{{RootURL}}/'
  headers:
    Content-Type: application/json
  body: '{"key": "value"}'

  max-redirects: 3
  matchers-condition: and

`

	stream, err := client.DebugPlugin(utils.TimeoutContextSeconds(4), &ypb.DebugPluginRequest{
		Code: fmt.Sprintf(`
target := "%s"
poc.Get(target)
http.Get(target)
nuclei.Scan(target, nuclei.rawTemplate(codec.DecodeBase64("%s")~))
`, target, codec.EncodeBase64([]byte(testTemplate))),
		PluginType: "yak",
	})
	if err != nil {
		return
	}
	var runtimeID string
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.RuntimeID != "" {
			runtimeID = data.RuntimeID
		}
	}

	time.Sleep(2 * time.Second)
	out, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		RuntimeId: runtimeID,
	})
	require.NoError(t, err)
	require.Len(t, out.Data, 0)

}

func TestGRPCMUSTPASS_HTTP_DebugPlugin_SaveHTTPFlow_HOOK(t *testing.T) {
	client, err := NewLocalClient(true)
	require.NoError(t, err)
	code := `db.SaveHTTPFlowFromRawWithOption("http://www.yak.com", "abc", "bca")`
	tempName, clearFunc, err := yakit.CreateTemporaryYakScriptEx("yak", code)
	require.NoError(t, err)
	defer clearFunc()

	stream, err := client.DebugPlugin(utils.TimeoutContextSeconds(4), &ypb.DebugPluginRequest{
		PluginName: tempName,
		PluginType: "yak",
	})
	if err != nil {
		return
	}
	var runtimeID string
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.RuntimeID != "" {
			runtimeID = data.RuntimeID
		}
	}

	time.Sleep(2 * time.Second)
	out, err := client.QueryHTTPFlows(context.Background(), &ypb.QueryHTTPFlowRequest{
		RuntimeId:  runtimeID,
		FromPlugin: tempName,
	})
	require.NoError(t, err)
	require.Len(t, out.Data, 1)
}
