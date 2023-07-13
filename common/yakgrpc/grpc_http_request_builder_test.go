package yakgrpc

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
)

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

	var host, port = utils.DebugMockHTTP([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaabbbaaabbb`))
	log.Infof("start to decug mock http on: %v", utils.HostPort(host, port))
	stream, err := client.DebugPlugin(context.Background(), &ypb.DebugPluginRequest{
		Code:       "yakit.AutoInitYakit(); handle = result => {dump(`executed in plugin`); dump(result); yakit.Info(`PLUGIN IS EXECUTED`)}",
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

	var host, port = utils.DebugMockHTTP([]byte(`HTTP/1.1 200 Ok
Content-Length: 12

aaacccaaabbb`))
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
		println(spew.Sdump(exec))
		if err != nil {
			log.Warn(err)
			break
		}
		spew.Dump(exec)
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
		spew.Dump(exec)
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
