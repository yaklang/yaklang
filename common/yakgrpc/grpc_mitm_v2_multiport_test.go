//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// waitMITMV2Started reads from stream until "starting mitm server" is received
// or the timeout elapses. Returns true on success.
func waitMITMV2Started(stream ypb.Yak_MITMV2Client, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		type recvResult struct {
			rsp *ypb.MITMV2Response
			err error
		}
		ch := make(chan recvResult, 1)
		go func() {
			rsp, err := stream.Recv()
			ch <- recvResult{rsp, err}
		}()

		select {
		case <-deadline.C:
			return false
		case res := <-ch:
			if res.err != nil {
				return false
			}
			if res.rsp.GetHaveMessage() {
				msg := string(res.rsp.GetMessage().GetMessage())
				if strings.Contains(msg, "starting mitm server") {
					return true
				}
			}
		}
	}
}

// sendHTTPViaProxy sends a plain HTTP GET through the given MITM proxy port,
// targeting targetHost:targetPort/token, and returns the raw response body.
func sendHTTPViaProxyPort(t *testing.T, proxyPort int, targetHost string, targetPort int, token string) string {
	t.Helper()
	packet := fmt.Sprintf("GET /%s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n",
		token, utils.HostPort(targetHost, targetPort))
	rsp, err := lowhttp.HTTP(
		lowhttp.WithPacketBytes([]byte(packet)),
		lowhttp.WithProxy(fmt.Sprintf("http://127.0.0.1:%d", proxyPort)),
		lowhttp.WithTimeout(10*time.Second),
	)
	require.NoError(t, err)
	_, body := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rsp.RawPacket)
	return string(body)
}

// TestGRPCMUSTPASS_MITMV2_MultiPort verifies that when ExtraPorts is provided
// in the initial MITMV2 request, the MITM server listens on all specified ports
// and proxies traffic correctly through each of them.
func TestGRPCMUSTPASS_MITMV2_MultiPort(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// Backend server: echo the request path as the response body.
	mockHost, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		path := lowhttp.GetHTTPRequestPath(req)
		rspBody := []byte(path)
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
			len(rspBody), rspBody))
	})

	primaryPort := utils.GetRandomAvailableTCPPort()
	extraPort1 := utils.GetRandomAvailableTCPPort()
	extraPort2 := utils.GetRandomAvailableTCPPort()

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	err = stream.Send(&ypb.MITMV2Request{
		Host:             "127.0.0.1",
		Port:             uint32(primaryPort),
		ExtraPorts:       []uint32{uint32(extraPort1), uint32(extraPort2)},
		SetAutoForward:   true,
		AutoForwardValue: true,
	})
	require.NoError(t, err)

	require.True(t, waitMITMV2Started(stream, 30*time.Second),
		"MITMV2 server did not start in time")

	// Allow extra port listeners time to bind.
	time.Sleep(500 * time.Millisecond)

	token := utils.RandStringBytes(16)
	expected := "/" + token

	var (
		mu     sync.Mutex
		bodies []string
	)
	var wg sync.WaitGroup
	for _, port := range []int{primaryPort, extraPort1, extraPort2} {
		port := port
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := sendHTTPViaProxyPort(t, port, mockHost, mockPort, token)
			mu.Lock()
			bodies = append(bodies, body)
			mu.Unlock()
		}()
	}
	wg.Wait()

	require.Len(t, bodies, 3, "expected responses from all three ports")
	for i, body := range bodies {
		require.Equalf(t, expected, body,
			"port index %d: response body mismatch", i)
	}
}

// TestGRPCMUSTPASS_MITMV2_MultiPort_BindFailRollback verifies that when one of
// the extra ports fails to bind (because it is already in use), all previously
// bound listeners—including the primary port—are released before the error is
// returned, leaving no lingering listeners.
func TestGRPCMUSTPASS_MITMV2_MultiPort_BindFailRollback(t *testing.T) {
	// Occupy a port so the MITM server cannot bind it.
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer occupied.Close()
	occupiedPort := occupied.Addr().(*net.TCPAddr).Port

	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	primaryPort := utils.GetRandomAvailableTCPPort()
	goodExtraPort := utils.GetRandomAvailableTCPPort()

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	err = stream.Send(&ypb.MITMV2Request{
		Host:       "127.0.0.1",
		Port:       uint32(primaryPort),
		ExtraPorts: []uint32{uint32(goodExtraPort), uint32(occupiedPort)},
	})
	require.NoError(t, err)

	// The server should report a failure message (port bind error) and close
	// the stream; it must NOT leave primary or goodExtraPort bound.
	gotError := false
	deadline := time.After(15 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for MITMV2 error response")
		default:
		}
		rsp, recvErr := stream.Recv()
		if recvErr != nil {
			// Stream closed due to server-side error — expected.
			gotError = true
			break
		}
		if rsp.GetHaveMessage() {
			msg := string(rsp.GetMessage().GetMessage())
			if strings.Contains(msg, "starting mitm server") {
				t.Fatal("MITM server started despite a port bind failure")
			}
		}
	}
	require.True(t, gotError, "expected stream to be closed with an error")

	// Give the OS a moment to reclaim the ports.
	time.Sleep(200 * time.Millisecond)

	// Verify primary and goodExtraPort are now free to bind again.
	for _, port := range []int{primaryPort, goodExtraPort} {
		l, lisErr := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		require.NoErrorf(t, lisErr, "port %d should be released after bind failure rollback", port)
		l.Close()
	}
}

// TestGRPCMUSTPASS_MITMV2_MultiPort_BackwardCompat verifies that omitting
// ExtraPorts leaves single-port behaviour unchanged.
func TestGRPCMUSTPASS_MITMV2_MultiPort_BackwardCompat(t *testing.T) {
	client, err := NewLocalClient()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	mockHost, mockPort := utils.DebugMockHTTPEx(func(req []byte) []byte {
		path := lowhttp.GetHTTPRequestPath(req)
		rspBody := []byte(path)
		return []byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s",
			len(rspBody), rspBody))
	})

	primaryPort := utils.GetRandomAvailableTCPPort()

	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	err = stream.Send(&ypb.MITMV2Request{
		Host:             "127.0.0.1",
		Port:             uint32(primaryPort),
		SetAutoForward:   true,
		AutoForwardValue: true,
	})
	require.NoError(t, err)

	require.True(t, waitMITMV2Started(stream, 30*time.Second),
		"MITMV2 server did not start in time")

	time.Sleep(200 * time.Millisecond)

	token := utils.RandStringBytes(16)
	body := sendHTTPViaProxyPort(t, primaryPort, mockHost, mockPort, token)
	require.Equal(t, "/"+token, body)
}
