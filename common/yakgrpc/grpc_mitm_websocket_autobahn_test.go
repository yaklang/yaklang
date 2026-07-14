package yakgrpc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const (
	autobahnGorillaDirectAgent = "gorilla-direct"
	autobahnGorillaMITMAgent   = "gorilla-via-yak-mitm"
	autobahnLowHTTPMITMAgent   = "yak-lowhttp-via-yak-mitm"
)

func autobahnDifferentialServerHostPort() string {
	if addr := os.Getenv("AUTOBAHN_SERVER_HOSTPORT"); addr != "" {
		return addr
	}
	return os.Getenv("Autobahn_Server_HostPort")
}

func autobahnGorillaDialer(proxyURL *url.URL) websocket.Dialer {
	dialer := *websocket.DefaultDialer
	dialer.EnableCompression = true
	dialer.HandshakeTimeout = 8 * time.Second
	if proxyURL != nil {
		dialer.Proxy = http.ProxyURL(proxyURL)
	}
	return dialer
}

func autobahnGorillaConnect(
	parent context.Context,
	hostPort string,
	resource string,
	proxyURL *url.URL,
) (*websocket.Conn, context.CancelFunc, error) {
	caseTimeout := 15 * time.Second
	if rawTimeout := os.Getenv("AUTOBAHN_CASE_TIMEOUT"); rawTimeout != "" {
		if parsed, err := time.ParseDuration(rawTimeout); err == nil {
			caseTimeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(parent, caseTimeout)
	dialer := autobahnGorillaDialer(proxyURL)
	conn, _, err := dialer.DialContext(ctx, "ws://"+hostPort+resource, nil)
	if err != nil {
		cancel()
		return nil, func() {}, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(caseTimeout))
	return conn, cancel, nil
}

func autobahnGorillaCaseCount(ctx context.Context, hostPort string) (int, error) {
	conn, cancel, err := autobahnGorillaConnect(ctx, hostPort, "/getCaseCount", nil)
	if err != nil {
		return 0, err
	}
	defer cancel()
	defer conn.Close()
	_, payload, err := conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(payload)))
}

func runAutobahnGorillaCase(
	ctx context.Context,
	hostPort string,
	caseID int,
	agent string,
	proxyURL *url.URL,
) error {
	resource := fmt.Sprintf("/runCase?case=%d&agent=%s", caseID, url.QueryEscape(agent))
	conn, cancel, err := autobahnGorillaConnect(ctx, hostPort, resource, proxyURL)
	if err != nil {
		return err
	}
	defer cancel()
	defer conn.Close()

	for {
		messageType, payload, readErr := conn.ReadMessage()
		if readErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				return fmt.Errorf("Autobahn case %d timed out: %w", caseID, readErr)
			}
			return nil
		}
		if conn.WriteMessage(messageType, payload) != nil {
			return nil
		}
	}
}

func updateAutobahnGorillaReport(
	ctx context.Context,
	hostPort string,
	agent string,
	proxyURL *url.URL,
) error {
	resource := "/updateReports?agent=" + url.QueryEscape(agent)
	conn, cancel, err := autobahnGorillaConnect(ctx, hostPort, resource, proxyURL)
	if err != nil {
		return err
	}
	defer cancel()
	defer conn.Close()
	for {
		if _, _, readErr := conn.ReadMessage(); readErr != nil {
			var netErr net.Error
			if errors.As(readErr, &netErr) && netErr.Timeout() {
				return fmt.Errorf("Autobahn report update for %s timed out: %w", agent, readErr)
			}
			return nil
		}
	}
}

func runAutobahnGorillaAgent(
	t *testing.T,
	ctx context.Context,
	hostPort string,
	caseCount int,
	agent string,
	proxyURL *url.URL,
) {
	t.Helper()
	for caseID := 1; caseID <= caseCount; caseID++ {
		caseID := caseID
		t.Run(fmt.Sprintf("case_%d", caseID), func(t *testing.T) {
			require.NoError(t, runAutobahnGorillaCase(ctx, hostPort, caseID, agent, proxyURL))
		})
	}
	require.NoError(t, updateAutobahnGorillaReport(ctx, hostPort, agent, proxyURL))
}

func autobahnLowHTTPUpgradePacket(hostPort, resource string) []byte {
	return []byte(fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==\r\n"+
		"Connection: Upgrade\r\n"+
		"Upgrade: websocket\r\n\r\n", resource, hostPort))
}

func runAutobahnLowHTTPViaMITMConnection(
	parent context.Context,
	hostPort string,
	resource string,
	proxyURL *url.URL,
	onMessage func(*lowhttp.WebsocketClient, []byte, []*lowhttp.Frame) error,
) error {
	host, port, err := utils.ParseStringToHostPort(hostPort)
	if err != nil {
		return err
	}
	caseTimeout := 15 * time.Second
	if rawTimeout := os.Getenv("AUTOBAHN_CASE_TIMEOUT"); rawTimeout != "" {
		if parsed, parseErr := time.ParseDuration(rawTimeout); parseErr == nil {
			caseTimeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(parent, caseTimeout)
	defer cancel()

	callbackErr := make(chan error, 1)
	client, err := lowhttp.NewWebsocketClient(
		autobahnLowHTTPUpgradePacket(hostPort, resource),
		lowhttp.WithWebsocketHost(host),
		lowhttp.WithWebsocketPort(port),
		lowhttp.WithWebsocketProxy(proxyURL.String()),
		lowhttp.WithWebsocketWithContext(ctx),
		lowhttp.WithWebsocketStrictMode(true),
		lowhttp.WithWebsocketRFC7692FullCompression(),
		lowhttp.WithWebsocketFromServerHandlerEx(func(client *lowhttp.WebsocketClient, data []byte, frames []*lowhttp.Frame) {
			if onMessage == nil {
				return
			}
			if callbackErrValue := onMessage(client, data, frames); callbackErrValue != nil {
				select {
				case callbackErr <- callbackErrValue:
				default:
				}
				_ = client.Close()
			}
		}),
	)
	if err != nil {
		return err
	}
	client.Start()
	select {
	case <-client.WaitChannel():
	case callbackErrValue := <-callbackErr:
		return callbackErrValue
	case <-ctx.Done():
		_ = client.Close()
		return fmt.Errorf("Autobahn resource %s timed out through MITM: %w", resource, ctx.Err())
	}
	select {
	case callbackErrValue := <-callbackErr:
		return callbackErrValue
	default:
		return nil
	}
}

func runAutobahnLowHTTPViaMITMAgent(
	t *testing.T,
	ctx context.Context,
	hostPort string,
	caseCount int,
	proxyURL *url.URL,
) {
	t.Helper()
	agent := url.QueryEscape(autobahnLowHTTPMITMAgent)
	for caseID := 1; caseID <= caseCount; caseID++ {
		caseID := caseID
		t.Run(fmt.Sprintf("case_%d", caseID), func(t *testing.T) {
			resource := fmt.Sprintf("/runCase?case=%d&agent=%s", caseID, agent)
			err := runAutobahnLowHTTPViaMITMConnection(ctx, hostPort, resource, proxyURL, func(client *lowhttp.WebsocketClient, data []byte, frames []*lowhttp.Frame) error {
				if len(frames) == 0 {
					return errors.New("Autobahn delivered a message without frames")
				}
				return client.WriteEx(data, frames[0].Type())
			})
			require.NoError(t, err)
		})
	}
	require.NoError(t, runAutobahnLowHTTPViaMITMConnection(
		ctx,
		hostPort,
		"/updateReports?agent="+agent,
		proxyURL,
		nil,
	))
}

func TestGRPCMUSTPASS_MITM_WebSocketAutobahnDifferential(t *testing.T) {
	hostPort := autobahnDifferentialServerHostPort()
	if hostPort == "" {
		t.Skip("set AUTOBAHN_SERVER_HOSTPORT to run the Autobahn MITM differential suite")
	}
	t.Setenv("YAKIT_HOME", t.TempDir())
	suiteTimeout := 25 * time.Minute
	if rawTimeout := os.Getenv("AUTOBAHN_SUITE_TIMEOUT"); rawTimeout != "" {
		parsed, parseErr := time.ParseDuration(rawTimeout)
		require.NoError(t, parseErr, "invalid AUTOBAHN_SUITE_TIMEOUT")
		suiteTimeout = parsed
	}
	ctx, cancel := context.WithTimeout(context.Background(), suiteTimeout)
	defer cancel()

	caseCount, err := autobahnGorillaCaseCount(ctx, hostPort)
	require.NoError(t, err)
	require.Positive(t, caseCount)
	t.Logf("Autobahn server selected %d cases", caseCount)
	disableFlowStorage := false
	if raw := os.Getenv("AUTOBAHN_MITM_DISABLE_FLOW_STORAGE"); raw != "" {
		disableFlowStorage, err = strconv.ParseBool(raw)
		require.NoError(t, err, "invalid AUTOBAHN_MITM_DISABLE_FLOW_STORAGE")
	}
	if os.Getenv("AUTOBAHN_MITM_TEST_CLIENT") == "lowhttp" {
		proxyURL := startWebsocketMITMProxy(t, ctx, cancel, disableFlowStorage)
		t.Run("lowhttp_via_yak_mitm", func(t *testing.T) {
			runAutobahnLowHTTPViaMITMAgent(t, ctx, hostPort, caseCount, proxyURL)
		})
		return
	}

	t.Run("direct", func(t *testing.T) {
		runAutobahnGorillaAgent(t, ctx, hostPort, caseCount, autobahnGorillaDirectAgent, nil)
	})

	proxyURL := startWebsocketMITMProxy(t, ctx, cancel, disableFlowStorage)
	t.Run("via_yak_mitm", func(t *testing.T) {
		runAutobahnGorillaAgent(t, ctx, hostPort, caseCount, autobahnGorillaMITMAgent, proxyURL)
	})
}
