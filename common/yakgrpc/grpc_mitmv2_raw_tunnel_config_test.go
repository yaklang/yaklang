//go:build !yakit_exclude

package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"golang.org/x/net/proxy"
)

func openMITMV2SOCKS5Tunnel(t *testing.T, proxyAddr, host string, port int) net.Conn {
	t.Helper()
	dialer, err := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
	require.NoError(t, err)
	conn, err := dialer.Dial("tcp", utils.HostPort(host, port))
	require.NoError(t, err)
	require.NoError(t, conn.SetDeadline(time.Now().Add(10*time.Second)))
	return conn
}

func startMITMV2RawAndHTTPUpstream(t *testing.T) (string, int) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				buffer := make([]byte, 8192)
				n, readErr := conn.Read(buffer)
				if readErr != nil || n == 0 {
					return
				}
				packet := buffer[:n]
				if bytes.HasPrefix(packet, []byte("GET ")) {
					path := lowhttp.GetHTTPRequestPath(packet)
					_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(path), path)
					return
				}
				_, _ = conn.Write(packet)
			}(conn)
		}
	}()
	t.Cleanup(func() { _ = listener.Close() })

	host, port, err := utils.ParseStringToHostPort(listener.Addr().String())
	require.NoError(t, err)
	return host, port
}

// This is the gRPC-to-wire contract for MITMv2 upstream transport settings:
// the same DownstreamProxy, HostsMapping, mapping-before-proxy and system-proxy
// policy must apply to both an opaque SOCKS5 tunnel and an ordinary HTTP flow.
// Unknown payloads remain byte-transparent; normal HTTP remains interceptable.
func TestGRPCMUSTPASS_MITMV2_RawTunnelSharesUpstreamConfigWithHTTP(t *testing.T) {
	const targetDomain = "mitmv2-raw-config.invalid"

	upstreamHost, upstreamPort := startMITMV2RawAndHTTPUpstream(t)
	downstreamProxy, getTargets, closeDownstreamProxy := startRecordingConnectProxyToUpstream(
		t,
		utils.HostPort(upstreamHost, upstreamPort),
	)
	defer closeDownstreamProxy()

	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	stream, err := client.MITMV2(ctx)
	require.NoError(t, err)

	mitmPort := utils.GetRandomAvailableTCPPort()
	err = stream.Send(&ypb.MITMV2Request{
		Host:               "127.0.0.1",
		Port:               uint32(mitmPort),
		DownstreamProxy:    downstreamProxy,
		DisableSystemProxy: true,
		HostsMapping: []*ypb.KVPair{{
			Key:   targetDomain,
			Value: upstreamHost,
		}},
		EnableHostsMappingBeforeDownstreamProxy: true,
		SetAutoForward:                          true,
		AutoForwardValue:                        true,
	})
	require.NoError(t, err)
	require.True(t, waitMITMV2Started(stream, 30*time.Second), "MITMV2 server did not start in time")

	proxyAddr := utils.HostPort("127.0.0.1", mitmPort)
	rawConn := openMITMV2SOCKS5Tunnel(t, proxyAddr, targetDomain, upstreamPort)
	rawPayload := []byte{0x00, 0x00, 0x00, 0x15, 0x08, 0x02, 0x01, 0x00, 0xff, 0x51, 0x51}
	_, err = rawConn.Write(rawPayload)
	require.NoError(t, err)
	rawEcho := make([]byte, len(rawPayload))
	_, err = io.ReadFull(rawConn, rawEcho)
	require.NoError(t, err)
	require.Equal(t, rawPayload, rawEcho)
	require.NoError(t, rawConn.Close())

	token := utils.RandStringBytes(12)
	require.Equal(t, "/"+token, sendHTTPViaProxyPort(t, mitmPort, targetDomain, upstreamPort, token))

	mappedTarget := utils.HostPort(upstreamHost, upstreamPort)
	require.Eventually(t, func() bool {
		targets := getTargets()
		var matches int
		for _, target := range targets {
			if target == mappedTarget {
				matches++
			}
		}
		return matches >= 2
	}, 10*time.Second, 50*time.Millisecond,
		"raw and HTTP dials should both apply the gRPC host mapping before the downstream proxy")
}
