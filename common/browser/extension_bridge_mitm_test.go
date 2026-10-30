package browser

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/crep"
	"github.com/yaklang/yaklang/common/minimartian"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func testMITMIdentity(badge string) httpctx.BrowserInstance {
	return httpctx.BrowserInstance{Manager: "ytray", InstallationID: "install-" + badge, InstanceID: "instance-" + badge, Badge: badge}
}

func TestBrowserMITMIngressLeasesAndScope(t *testing.T) {
	manager := &ExtensionBridgeManager{}
	t.Cleanup(manager.closeMITMIngresses)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	incoming := make(chan *minimartian.WrapperedConn, 4)
	stop := manager.RegisterMITMIngress(ctx, "127.0.0.1:8083", false, incoming)
	t.Cleanup(stop)
	a, bridgeErr := manager.acquireInstanceMITMIngress("localhost", 8083, testMITMIdentity("A"))
	require.Nil(t, bridgeErr)
	b, bridgeErr := manager.acquireInstanceMITMIngress("127.0.0.1", 8083, testMITMIdentity("B"))
	require.Nil(t, bridgeErr)
	require.True(t, a.Available)
	require.NotEqual(t, a.Port, b.Port)
	renewed, bridgeErr := manager.acquireInstanceMITMIngress("127.0.0.1", 8083, testMITMIdentity("A"))
	require.Nil(t, bridgeErr)
	require.Equal(t, a.Port, renewed.Port)
	require.GreaterOrEqual(t, renewed.ExpiresAt, a.ExpiresAt)
	missing, bridgeErr := manager.acquireInstanceMITMIngress("example.com", 8083, testMITMIdentity("A"))
	require.Nil(t, bridgeErr)
	require.False(t, missing.Available)

	server := &ExtensionBridgeServer{manager: manager, managedClients: map[string]*extensionBridgeConnectedClient{
		"device-a": {status: ExtensionBridgeConnection{InstallationID: "install-A", ManagedInstance: &ExtensionBridgeManagedInstance{
			Manager: "ytray", InstanceID: "instance-A", Badge: "A",
		}}},
	}}
	manager.identity.Devices = []ExtensionBridgeDevice{{ID: "device-a", InstallationID: "install-A"}}
	params := json.RawMessage(`{"proxyHost":"127.0.0.1","proxyPort":8083}`)
	result, requestErr := server.acquireMITMIngress("unpaired", params)
	require.Nil(t, requestErr)
	require.False(t, result.(extensionMITMIngress).Available)
	_, requestErr = server.acquireMITMIngress("device-a", json.RawMessage(`{"proxyHost":"127.0.0.1","proxyPort":8083,"instanceId":"instance-B"}`))
	require.Equal(t, "invalid_params", requestErr.Code)
	result, requestErr = server.acquireMITMIngress("device-a", params)
	require.Nil(t, requestErr)
	require.Equal(t, a.Port, result.(extensionMITMIngress).Port)

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", a.Port), time.Second)
	require.NoError(t, err)
	initialConn := conn
	t.Cleanup(func() { initialConn.Close() })
	var accepted *minimartian.WrapperedConn
	select {
	case accepted = <-incoming:
	case <-time.After(time.Second):
		t.Fatal("ingress did not accept connection")
	}
	require.Equal(t, testMITMIdentity("A"), accepted.GetMetaInfo()[httpctx.BrowserInstanceContextKey])
	manager.releaseMITMInstallation("install-A")
	conn.SetReadDeadline(time.Now().Add(time.Second))
	_, err = conn.Read(make([]byte, 1))
	require.Error(t, err, "revocation must close established sockets")
	conn, err = net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", a.Port), time.Second)
	require.Error(t, err, "revocation must close the listener")
	for _, runtime := range manager.mitmRuntimes {
		manager.expireMITMLeases(runtime, time.Now().Add(extensionMITMLeaseTTL+time.Second))
		require.Empty(t, runtime.leases)
	}
	rebound, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", b.Port))
	require.NoError(t, err, "inactive instances must release their port")
	_ = rebound.Close()
	stop()
	missing, bridgeErr = manager.acquireInstanceMITMIngress("localhost", 8083, testMITMIdentity("A"))
	require.Nil(t, bridgeErr)
	require.False(t, missing.Available)
}

func TestBrowserMITMIngressHTTPHTTPSAndWebsocket(t *testing.T) {
	for _, protocol := range []string{"http", "https-h1", "https-h2", "websocket"} {
		t.Run(protocol, func(t *testing.T) {
			type observation struct {
				identity httpctx.BrowserInstance
				tags     []string
			}
			observed := make(chan observation, 32)
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("X-Yakit-Instance") != "" {
					w.WriteHeader(500)
					return
				}
				if websocket.IsWebSocketUpgrade(r) {
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						return
					}
					defer conn.Close()
					kind, data, err := conn.ReadMessage()
					if err == nil {
						_ = conn.WriteMessage(kind, data)
					}
					return
				}
				fmt.Fprint(w, "same response")
			}))
			origin.EnableHTTP2 = protocol == "https-h2"
			if protocol == "https-h1" || protocol == "https-h2" {
				origin.StartTLS()
			} else {
				origin.Start()
			}
			t.Cleanup(origin.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			t.Cleanup(cancel)
			incoming := make(chan *minimartian.WrapperedConn, 16)
			mitm, err := crep.NewMITMServer(crep.MITM_SetDisableSystemProxy(true), crep.MITM_SetHTTP2(protocol == "https-h2"),
				crep.MITM_SetWebsocketHijackMode(true), crep.MITM_SetExtraIncomingConnectionChannel(incoming),
				crep.MITM_SetWebsocketRequestHijackRaw(func(data []byte, _ *http.Request, _ *http.Response, _ int64) []byte { return data }),
				crep.MITM_SetWebsocketResponseHijackRaw(func(data []byte, _ *http.Request, _ *http.Response, _ int64) []byte { return data }),
				crep.MITM_SetHTTPRequestHijackRaw(func(_ bool, req *http.Request, packet []byte) []byte {
					identity, _ := httpctx.GetBrowserInstance(req)
					observed <- observation{identity: identity, tags: httpctx.GetFlowTags(req)}
					return packet
				}))
			require.NoError(t, err)
			main, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			done := make(chan error, 1)
			go func() { done <- mitm.ServerListener(ctx, main) }()
			t.Cleanup(func() {
				cancel()
				_ = main.Close()
				select {
				case <-done:
				case <-time.After(3 * time.Second):
					t.Error("MITM did not stop")
				}
			})
			manager := &ExtensionBridgeManager{}
			stop := manager.RegisterMITMIngress(ctx, main.Addr().String(), false, incoming)
			t.Cleanup(stop)
			basePort := main.Addr().(*net.TCPAddr).Port
			errors := make(chan error, 2)
			var wg sync.WaitGroup
			for _, badge := range []string{"A", "B"} {
				lease, bridgeErr := manager.acquireInstanceMITMIngress("127.0.0.1", basePort, testMITMIdentity(badge))
				require.Nil(t, bridgeErr)
				proxyURL, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", lease.Port))
				require.NoError(t, err)
				wg.Add(1)
				go func() {
					defer wg.Done()
					if protocol == "websocket" {
						dialer := websocket.Dialer{Proxy: http.ProxyURL(proxyURL), HandshakeTimeout: 5 * time.Second}
						conn, _, err := dialer.Dial("ws"+origin.URL[4:]+"/same", nil)
						if err != nil {
							errors <- err
							return
						}
						defer conn.Close()
						conn.SetReadDeadline(time.Now().Add(5 * time.Second))
						if err = conn.WriteMessage(websocket.TextMessage, []byte("echo")); err == nil {
							_, data, readErr := conn.ReadMessage()
							err = readErr
							if err == nil && string(data) != "echo" {
								err = fmt.Errorf("websocket payload changed")
							}
						}
						errors <- err
						return
					}
					transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), ForceAttemptHTTP2: protocol == "https-h2",
						TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
					defer transport.CloseIdleConnections()
					client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
					for i := 0; i < 2; i++ {
						response, err := client.Get(origin.URL + "/same")
						if err != nil {
							errors <- err
							return
						}
						data, err := io.ReadAll(response.Body)
						response.Body.Close()
						if err != nil {
							errors <- err
							return
						}
						if string(data) != "same response" || protocol == "https-h2" && response.ProtoMajor != 2 {
							errors <- fmt.Errorf("unexpected response: %s %q", response.Proto, data)
							return
						}
					}
					errors <- nil
				}()
			}
			wg.Wait()
			require.NoError(t, <-errors)
			require.NoError(t, <-errors)
			counts := map[string]int{}
			wanted := 4
			if protocol == "websocket" {
				wanted = 2
			}
			for i := 0; i < wanted; i++ {
				select {
				case got := <-observed:
					require.NotEmpty(t, got.identity.InstanceID)
					require.Contains(t, got.tags, "[浏览器 "+got.identity.Badge+"]")
					counts[got.identity.Badge]++
				case <-time.After(time.Second):
					t.Fatal("missing tagged request")
				}
			}
			require.Equal(t, wanted/2, counts["A"])
			require.Equal(t, wanted/2, counts["B"])
			if protocol == "http" {
				baseURL, err := url.Parse("http://" + main.Addr().String())
				require.NoError(t, err)
				transport := &http.Transport{Proxy: http.ProxyURL(baseURL)}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
				response, err := client.Get(origin.URL + "/same")
				require.NoError(t, err)
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				got := <-observed
				require.Empty(t, got.identity.InstanceID, "the shared main port must not inherit another browser's identity")
				require.Empty(t, got.tags)
			}
		})
	}
}
