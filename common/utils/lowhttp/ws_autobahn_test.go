package lowhttp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

const autobahnYakClientAgent = "yak-lowhttp-client"

func autobahnServerHostPort() string {
	if addr := os.Getenv("AUTOBAHN_SERVER_HOSTPORT"); addr != "" {
		return addr
	}
	return os.Getenv("Autobahn_Server_HostPort")
}

func autobahnUpgradePacket(hostPort, resource string) []byte {
	return []byte(fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"Sec-WebSocket-Key: wDqumtseNBJdhkihL6PW7w==\r\n"+
		"Connection: Upgrade\r\n"+
		"Upgrade: websocket\r\n\r\n", resource, hostPort))
}

func runAutobahnLowHTTPConnection(
	parent context.Context,
	host string,
	port int,
	resource string,
	onMessage func(*WebsocketClient, []byte, []*Frame) error,
) error {
	caseTimeout := 15 * time.Second
	if rawTimeout := os.Getenv("AUTOBAHN_CASE_TIMEOUT"); rawTimeout != "" {
		if parsed, err := time.ParseDuration(rawTimeout); err == nil {
			caseTimeout = parsed
		}
	}
	ctx, cancel := context.WithTimeout(parent, caseTimeout)
	defer cancel()

	callbackErr := make(chan error, 1)
	options := []WebsocketClientOpt{
		WithWebsocketTLS(false),
		WithWebsocketHost(host),
		WithWebsocketPort(port),
		WithWebsocketWithContext(ctx),
		WithWebsocketStrictMode(true),
		WithWebsocketRFC7692FullCompression(),
		WithWebsocketFromServerHandlerEx(func(client *WebsocketClient, data []byte, frames []*Frame) {
			if onMessage == nil {
				return
			}
			if err := onMessage(client, data, frames); err != nil {
				select {
				case callbackErr <- err:
				default:
				}
				_ = client.Close()
			}
		}),
	}
	if proxy := os.Getenv("AUTOBAHN_PROXY"); proxy != "" {
		options = append(options, WithWebsocketProxy(proxy))
	}

	client, err := NewWebsocketClient(autobahnUpgradePacket(utils.HostPort(host, port), resource), options...)
	if err != nil {
		return err
	}
	client.Start()
	select {
	case <-client.WaitChannel():
	case err := <-callbackErr:
		return err
	case <-ctx.Done():
		_ = client.Close()
		return utils.Errorf("autobahn resource %s timed out: %v", resource, ctx.Err())
	}
	select {
	case err := <-callbackErr:
		return err
	default:
		return nil
	}
}

func autobahnLowHTTPCaseCount(ctx context.Context, host string, port int) (int, error) {
	count := make(chan int, 1)
	err := runAutobahnLowHTTPConnection(ctx, host, port, "/getCaseCount", func(client *WebsocketClient, data []byte, _ []*Frame) error {
		parsed, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil {
			return utils.Wrap(err, "parse Autobahn case count")
		}
		select {
		case count <- parsed:
		default:
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	select {
	case result := <-count:
		return result, nil
	default:
		return 0, utils.Error("Autobahn server returned no case count")
	}
}

func TestWebsocket_AutobahnClient(t *testing.T) {
	hostPort := autobahnServerHostPort()
	if hostPort == "" {
		t.Skip("set AUTOBAHN_SERVER_HOSTPORT to run the Autobahn client suite")
	}
	host, port, err := utils.ParseStringToHostPort(hostPort)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	count, err := autobahnLowHTTPCaseCount(ctx, host, port)
	require.NoError(t, err)
	require.Positive(t, count)
	t.Logf("Autobahn server selected %d cases", count)

	agent := url.QueryEscape(autobahnYakClientAgent)
	for caseID := 1; caseID <= count; caseID++ {
		caseID := caseID
		t.Run(fmt.Sprintf("case_%d", caseID), func(t *testing.T) {
			resource := fmt.Sprintf("/runCase?case=%d&agent=%s", caseID, agent)
			err := runAutobahnLowHTTPConnection(ctx, host, port, resource, func(client *WebsocketClient, data []byte, frames []*Frame) error {
				if len(frames) == 0 {
					return utils.Error("Autobahn delivered a message without frames")
				}
				return client.WriteEx(data, frames[0].Type())
			})
			require.NoError(t, err)
		})
	}

	err = runAutobahnLowHTTPConnection(
		ctx,
		host,
		port,
		"/updateReports?agent="+agent,
		nil,
	)
	require.NoError(t, err)
}
