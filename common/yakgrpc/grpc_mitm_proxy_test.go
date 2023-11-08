package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TemplateTestGRPCMUSTPASS_MITM_WithoutProxy_StatusCard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello Token"))
	})
	var targetUrl = "http://" + utils.HostPort(targetHost, targetPort)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				stream.Send(&ypb.MITMRequest{SetYakScript: true, YakScriptContent: `
mirrorNewWebsite = (tls, url, req, rsp, body) => {
	yakit.StatusCard("abc", 1)
}
`})
			}
		}

		if data.GetMessage().GetIsMessage() && strings.Contains(string(data.GetMessage().GetMessage()), `HotPatched MITM HOOKS`) {
			// do sth
			_, err := yak.Execute(`rsp, req := poc.Get(targetUrl, poc.proxy(mitmProxy))~
assert string(rsp.RawPacket).Contains("Hello Token")
cancel()
`, map[string]any{"targetUrl": targetUrl, "mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort), "cancel": cancel})
			if err != nil {
				t.Fatal(err)
			}
		}

	}
}

func TemplateTestGRPCMUSTPASS_MITM_Proxy_Template(t *testing.T) {
	ctx := utils.TimeoutContextSeconds(10)
	port := utils.GetRandomAvailableTCPPort()
	server, err := crep.NewMITMServer()
	if err != nil {
		t.Fatal(err)
	}
	addr := utils.HostPort("127.0.0.1", port)
	go func() {
		server.Serve(ctx, addr)
	}()
	if utils.WaitConnect(addr, 10) != nil {
		panic("wait connect timeout")
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:            "127.0.0.1",
		Port:            uint32(mitmPort),
		DownstreamProxy: "http://" + utils.HostPort("127.0.0.1", port),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				// do sth
			}
		}
	}
}

func TestGRPCMUSTPASS_MITM_Proxy(t *testing.T) {
	var (
		networkIsPassed  bool
		downstreamPassed bool
		token            = utils.RandNumberStringBytes(10)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("u") == token {
			networkIsPassed = true
			cancel()
		}
		writer.Write([]byte("Hello Token"))
	})
	var mockUrl = "http://" + utils.HostPort(mockHost, mockPort)

	port := utils.GetRandomAvailableTCPPort()
	server, err := crep.NewMITMServer(crep.MITM_SetHTTPRequestHijack(func(https bool, req *http.Request) *http.Request {
		if req.URL.Query().Get("u") == token {
			downstreamPassed = true
		}
		return req
	}))
	if err != nil {
		t.Fatal(err)
	}
	addr := utils.HostPort("127.0.0.1", port)
	go func() {
		server.Serve(ctx, addr)
	}()
	if utils.WaitConnect(addr, 10) != nil {
		t.Fatal("wait connect timeout")
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host:            "127.0.0.1",
		Port:            uint32(mitmPort),
		DownstreamProxy: "http://" + utils.HostPort("127.0.0.1", port),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				if _, err := yak.Execute(
					`
poc.Get(mockUrl, poc.proxy(mitmProxy), poc.replaceQueryParam("u", token))~`,
					map[string]any{
						"mockUrl":   mockUrl,
						"mitmProxy": "http://" + utils.HostPort("127.0.0.1", mitmPort),
						"token":     token,
					}); err != nil {
					t.Fatalf("execute script failed: %v", err)
				}
			}
		}
	}

	if !downstreamPassed {
		t.Fatalf("Downstream proxy not passed")
	}

	if !networkIsPassed {
		t.Fatalf("Network not passed")
	}
}

func TestGRPCMUSTPASS_MITM_Proxy_MITMPluginInheritProxy(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	var passed = false
	_, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		if bytes.Contains(req, []byte("CONNECT www3.example.com:80 HTTP")) {
			passed = true
			cancel()
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\n")
	})
	downstreamAddr := utils.HostPort("127.0.0.1", port)
	downstreamUrl := `http://` + downstreamAddr

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{DownstreamProxy: downstreamUrl, Port: uint32(mitmPort)})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.GetMessage().GetIsMessage() {
			var msg = string(rsp.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				stream.Send(&ypb.MITMRequest{SetYakScript: true, YakScriptContent: `
mirrorNewWebsite = (tls, url, req, rsp, body) => {
	poc.Get("http://www3.example.com")
}
`})
				go func() {
					time.Sleep(time.Second)
					_, err := yak.Execute(`
poc.Get("http://www.example.com", poc.proxy(mitmProxy))
`, map[string]any{
						"mitmProxy": "http://127.0.0.1:" + fmt.Sprint(mitmPort),
					})
					if err != nil {
						t.Fatal(err)
					}
				}()
			}
		}
	}

	if !passed {
		t.Fatal("Downstream proxy not passed")
	}
}

func TestGRPCMUSTPASS_MITM_Proxy_StatusCard(t *testing.T) {
	name, err := yakit.CreateTemporaryYakScript("mitm", `
yakit.AutoInitYakit()
yakit.StatusCard("mitmId", "StatusCard")
`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello Token"))
	})
	var targetUrl = "http://" + utils.HostPort(targetHost, targetPort)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMRequest{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	var (
		started               bool
		pluginStartLoading    bool
		pluginStatusCardFound bool
		hotStatusCardFound    bool
	)
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			var msg = string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") && !started {
				stream.Send(&ypb.MITMRequest{
					SetYakScript: true,
					YakScriptContent: `
mirrorNewWebsite = (tls, url, req, rsp, body) => {
	yakit.StatusCard("abc", 1)
}
`,
				})

				stream.Send(&ypb.MITMRequest{
					SetPluginMode:   true,
					InitPluginNames: []string{name},
				})
				started = true
			}

			if data.GetMessage().GetIsMessage() && strings.Contains(string(data.GetMessage().GetMessage()), `HotPatched MITM HOOKS`) {
				// do sth
				_, err := yak.Execute(`rsp, req := poc.Get(targetUrl, poc.proxy(mitmProxy))~
assert string(rsp.RawPacket).Contains("Hello Token")
go func{
	sleep(2)
	cancel()
}
`, map[string]any{"targetUrl": targetUrl, "mitmProxy": `http://` + utils.HostPort("127.0.0.1", mitmPort), "cancel": cancel})
				if err != nil {
					t.Fatal(err)
				}
			}
		}

		if strings.Contains(spew.Sdump(data), "abc") && strings.Contains(spew.Sdump(data), "feature-status-card-data") {
			hotStatusCardFound = true
		}

		if !pluginStartLoading && started && strings.Contains(spew.Sdump(data), "Initializing MITM Plugin: "+name) {
			pluginStartLoading = true
		}

		if strings.Contains(spew.Sdump(data), "StatusCard") && strings.Contains(spew.Sdump(data), "mitmId") {
			pluginStatusCardFound = true
		}
	}

	time.Sleep(1 * time.Second)

	if !pluginStatusCardFound {
		t.Fatal("plugin status card not found")
	}

	if !hotStatusCardFound {
		t.Fatal("hot status card not found")
	}

	if !(pluginStatusCardFound && hotStatusCardFound) {
		panic("mitm plugin/hot patched status card not found")
	}
}
