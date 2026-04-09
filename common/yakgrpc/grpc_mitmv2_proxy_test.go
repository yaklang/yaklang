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

func TestGRPCMUSTPASS_MITMV2_DownstreamProxy_SpecialCharsCredentials(t *testing.T) {
	// 下游代理地址中凭据含 @ 等特殊字符，验证能正确解析并启动
	var downstreamPassed bool
	token := utils.RandNumberStringBytes(10)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, mockPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("u") == token {
			downstreamPassed = true
		}
		writer.Write([]byte("ok"))
	})
	mockUrl := "http://" + utils.HostPort("127.0.0.1", mockPort)

	port := utils.GetRandomAvailableTCPPort()
	server, err := crep.NewMITMServer(crep.MITM_SetHTTPRequestHijack(func(https bool, req *http.Request) *http.Request {
		if req.URL.Query().Get("u") == token {
			downstreamPassed = true
		}
		return req
	}))
	require.NoError(t, err)
	addr := utils.HostPort("127.0.0.1", port)
	go func() {
		server.Serve(ctx, addr)
	}()
	require.NoError(t, utils.WaitConnect(addr, 10))

	mitmPort := utils.GetRandomAvailableTCPPort()
	client, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)
	// 密码含 @，未手动编码
	downstreamWithSpecialChars := "http://user:pass@word@" + utils.HostPort("127.0.0.1", port)
	stream.Send(&ypb.MITMV2Request{
		Host:            "127.0.0.1",
		Port:            uint32(mitmPort),
		DownstreamProxy: downstreamWithSpecialChars,
	})
	started := false
	for {
		data, err := stream.Recv()
		if err != nil {
			break
		}
		if data.GetMessage().GetIsMessage() {
			msg := string(data.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				started = true
				_, _ = yak.Execute(
					`poc.Get(mockUrl, poc.proxy(mitmProxy), poc.replaceQueryParam("u", token))~`,
					map[string]any{
						"mockUrl":   mockUrl,
						"mitmProxy": "http://" + utils.HostPort("127.0.0.1", mitmPort),
						"token":     token,
					})
			}
			if strings.Contains(msg, "ERROR") && strings.Contains(msg, "downstream") {
				t.Fatalf("MITM should not fail with downstream parse error when credentials have @: %s", msg)
			}
		}
	}
	require.True(t, started, "MITM should start")
	require.True(t, downstreamPassed, "request should pass through downstream proxy")
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

func TestGRPCMUSTPASS_MITMV2_DownstreamProxy_EnableHostsMappingBeforeDownstreamProxy(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	fakeHost := "mitmv2-hosts-first.invalid"
	token := utils.RandStringBytes(16)

	targetHost, targetPort := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(token))
	})

	runCase := func(t *testing.T, enable bool) ([]string, *lowhttp.LowhttpResponse, error) {
		t.Helper()

		var (
			rsp    *lowhttp.LowhttpResponse
			reqErr error
		)

		targetURL := fmt.Sprintf("http://%s", utils.HostPort(fakeHost, targetPort))
		downstreamProxy, getTargets, closeProxy := startRecordingConnectProxy(t)
		defer closeProxy()

		mitmPort := utils.GetRandomAvailableTCPPort()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		RunMITMV2TestServerEx(client, ctx, func(stream ypb.Yak_MITMV2Client) {
			stream.Send(&ypb.MITMV2Request{
				Host:                                    "127.0.0.1",
				Port:                                    uint32(mitmPort),
				DownstreamProxy:                         downstreamProxy,
				Hosts:                                   []*ypb.KVPair{{Key: fakeHost, Value: targetHost}},
				EnableHostsMappingBeforeDownstreamProxy: enable,
			})
		}, func(stream ypb.Yak_MITMV2Client) {
			mitmProxy := "http://" + utils.HostPort("127.0.0.1", mitmPort)
			rsp, _, reqErr = poc.DoGET(targetURL, poc.WithProxy(mitmProxy))
			cancel()
		}, nil)

		require.Eventually(t, func() bool {
			return len(getTargets()) > 0
		}, 3*time.Second, 100*time.Millisecond)

		return getTargets(), rsp, reqErr
	}

	t.Run("disabled", func(t *testing.T) {
		targets, rsp, err := runCase(t, false)
		require.Contains(t, targets, utils.HostPort(fakeHost, targetPort))
		if err == nil && rsp != nil {
			require.NotContains(t, string(rsp.RawPacket), token)
		}
	})

	t.Run("enabled", func(t *testing.T) {
		targets, rsp, err := runCase(t, true)
		require.Contains(t, targets, utils.HostPort(targetHost, targetPort))
		require.NoError(t, err)
		require.NotNil(t, rsp)
		require.Contains(t, string(rsp.RawPacket), token)
	})
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
	name, clearFunc, err := yakit.CreateAndClearTemporaryYakScript("mitm", `
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
		pluginNameFound       bool
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
			if data.GetMessage().GetPluginName() == name {
				pluginNameFound = true
			}
			pluginStatusCardFound = true
		}
	}

	time.Sleep(1 * time.Second)

	require.True(t, pluginStatusCardFound, "plugin status card not found")
	require.True(t, hotStatusCardFound, "hot status card not found")
	require.True(t, pluginNameFound, "plugin name not found")
}
