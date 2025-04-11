package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"

	"github.com/stretchr/testify/require"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_MITMV2_Proxy(t *testing.T) {
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
	mockUrl := "http://" + utils.HostPort(mockHost, mockPort)

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
	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMV2Request{
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
			msg := string(data.GetMessage().GetMessage())
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

func TestGRPCMUSTPASS_MITMV2_S5Proxy(t *testing.T) {
	var (
		networkIsPassed bool
		token           = utils.RandNumberStringBytes(10)
		rspToken        = utils.RandStringBytes(10)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("u") == token {
			networkIsPassed = true
		}
		writer.Write([]byte(rspToken))
	})
	mockUrl := "http://" + utils.HostPort(mockHost, mockPort)

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				if _, err := yak.Execute(
					`
 rsp,_ = poc.Get(mockUrl, poc.proxy(mitmProxy), poc.replaceQueryParam("u", token))~
assert str.Contains(rsp.RawPacket,rspToken)`,
					map[string]any{
						"mockUrl":   mockUrl,
						"mitmProxy": "socks5://" + utils.HostPort("127.0.0.1", mitmPort),
						"token":     token,
						"rspToken":  rspToken,
					}); err != nil {
					t.Fatalf("execute script failed: %v", err)
				}
			}
		}
	}

	if !networkIsPassed {
		t.Fatalf("Network not passed")
	}
}

func TestGRPCMUSTPASS_MITMV2_S5Proxy_https(t *testing.T) {
	var (
		networkIsPassed bool
		token           = utils.RandNumberStringBytes(10)
		rspToken        = utils.RandStringBytes(10)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPSEx(func(req []byte) []byte {
		if lowhttp.GetHTTPRequestQueryParam(req, "u") == token {
			networkIsPassed = true
		}
		return []byte("HTTP/1.1 200 OK\r\nContent-length: 10\r\n\r\n" + rspToken)
	})
	mockUrl := "https://" + utils.HostPort(mockHost, mockPort)

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stream.Send(&ypb.MITMV2Request{
		Host: "127.0.0.1",
		Port: uint32(mitmPort),
	})
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			fmt.Println(msg)
			if strings.Contains(msg, "starting mitm server") {
				if _, err := yak.Execute(
					`
 rsp,_ = poc.Get(mockUrl, poc.proxy(mitmProxy), poc.replaceQueryParam("u", token),poc.https(true))~
assert str.Contains(rsp.RawPacket,rspToken)`,
					map[string]any{
						"mockUrl":   mockUrl,
						"mitmProxy": "socks5://" + utils.HostPort("127.0.0.1", mitmPort),
						"token":     token,
						"rspToken":  rspToken,
					}); err != nil {
					t.Fatalf("execute script failed: %v", err)
				}
			}
		}
	}

	if !networkIsPassed {
		t.Fatalf("Network not passed")
	}
}

func TestGRPCMUSTPASS_MITMV2_Runtime_Proxy(t *testing.T) {
	var (
		networkIsPassed  bool
		downstreamPassed bool
		token            = utils.RandNumberStringBytes(16)
	)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("u") == token {
			networkIsPassed = true
		}
		writer.Write([]byte("Hello Token"))
	})
	mockUrl := "http://" + utils.HostPort(mockHost, mockPort)

	port := utils.GetRandomAvailableTCPPort()
	server, err := crep.NewMITMServer(crep.MITM_SetHTTPRequestHijack(func(https bool, req *http.Request) *http.Request {
		if req.URL.Query().Get("u") == token {
			downstreamPassed = true
		}
		return req
	}))
	require.NoError(t, err)

	addr := utils.HostPort("127.0.0.1", port)
	go server.Serve(ctx, addr)

	require.NoError(t, utils.WaitConnect(addr, 10))

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)

	RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
		stream.Send(&ypb.MITMV2Request{
			Host: "127.0.0.1",
			Port: uint32(mitmPort),
		})
	}, func(stream ypb.Yak_MITMV2Client) {
		// not set proxy, send
		mitmProxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
		_, _, err := poc.DoGET(mockUrl, poc.WithProxy(mitmProxy), poc.WithReplaceHttpPacketQueryParam("u", token))

		require.NoError(t, err)
		require.False(t, downstreamPassed, "Downstream proxy should not passed")
		require.True(t, networkIsPassed, "Network should passed")

		// set downstream proxy
		stream.Send(&ypb.MITMV2Request{
			SetDownstreamProxy: true,
			DownstreamProxy:    "http://" + utils.HostPort("127.0.0.1", port),
		})
		networkIsPassed = false
		time.Sleep(1 * time.Second)

		// send again
		_, _, err = poc.DoGET(mockUrl, poc.WithProxy(mitmProxy), poc.WithReplaceHttpPacketQueryParam("u", token))

		require.NoError(t, err)
		require.True(t, downstreamPassed, "Downstream proxy should passed")
		require.True(t, networkIsPassed, "Network should passed")
		cancel()
	}, nil)
}

func TestGRPCMUSTPASS_MITMV2_Proxy_MITMPluginInheritProxy(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	passed := false
	_, port := utils.DebugMockHTTPEx(func(req []byte) []byte {
		if bytes.Contains(req, []byte("CONNECT www3.example.com:80 HTTP")) {
			passed = true
			cancel()
		}
		return []byte("HTTP/1.1 200 OK\r\n\r\n")
	})
	downstreamAddr := utils.HostPort("127.0.0.1", port)
	downstreamUrl := `http://` + downstreamAddr

	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}

	mitmPort := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMV2Request{DownstreamProxy: downstreamUrl, Port: uint32(mitmPort)})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		if rsp.GetMessage().GetIsMessage() {
			msg := string(rsp.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				stream.Send(&ypb.MITMV2Request{SetYakScript: true, YakScriptContent: `
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

func TestGRPCMUSTPASS_MITMV2_Proxy_StatusCard(t *testing.T) {
	name, clearFunc, err := yakit.CreateTemporaryYakScriptEx("mitm", `
yakit.AutoInitYakit()
yakit.StatusCard("mitmId", "StatusCard")
`)
	require.NoError(t, err)
	defer clearFunc()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte("Hello Token"))
	})
	targetUrl := "http://" + utils.HostPort(targetHost, targetPort)
	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)
	client.ResetGlobalNetworkConfig(context.Background(), &ypb.ResetGlobalNetworkConfigRequest{})
	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)
	stream.Send(&ypb.MITMV2Request{
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
			msg := string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") && !started {
				stream.Send(&ypb.MITMV2Request{
					SetYakScript: true,
					YakScriptContent: `
mirrorNewWebsite = (tls, url, req, rsp, body) => {
	yakit.StatusCard("abc", 1)
}
`,
				})

				stream.Send(&ypb.MITMV2Request{
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
				require.NoError(t, err)
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

	require.True(t, pluginStatusCardFound, "plugin status card not found")
	require.True(t, hotStatusCardFound, "hot status card not found")
}
