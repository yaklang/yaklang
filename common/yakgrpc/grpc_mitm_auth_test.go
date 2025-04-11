package yakgrpc

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestGRPCMUSTPASS_MITM_AUTH(t *testing.T) {
	test := assert.New(t)
	_ = test

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\na"))

	p := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host:            "127.0.0.1",
		Port:            uint32(p),
		ProxyUsername:   "admin" + "@/",
		ProxyPassword:   "@/:",
		EnableProxyAuth: true,
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		msg := string(rsp.GetMessage().GetMessage())
		if strings.Contains(msg, `starting mitm server`) {
			proxy := fmt.Sprintf("http://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("admin@/"),
				codec.QueryEscape("@/:"),
				p,
			)
			conn, err := netx.DialX(utils.HostPort(host, port), netx.DialX_WithProxy(proxy))
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			cancel()
		}
	}

}

func TestGRPCMUSTPASS_MITM_AUTH_Negative(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.MITM(ctx)
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\na"))

	p := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMRequest{
		Host:            "127.0.0.1",
		Port:            uint32(p),
		ProxyUsername:   "admin",
		ProxyPassword:   "12345",
		EnableProxyAuth: true,
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		target := utils.HostPort(host, port)
		msg := string(rsp.GetMessage().GetMessage())
		if strings.Contains(msg, `starting mitm server`) {
			proxy := fmt.Sprintf("http://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("test"),
				codec.QueryEscape("test"),
				p,
			)
			_, err := netx.DialX(target, netx.DialX_WithProxy(proxy))
			if err == nil {
				t.Fatal("mitm http1.1 proxy auth negative test failed: should not connect to server")
			}

			s5proxy := fmt.Sprintf("socks5://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("test"),
				codec.QueryEscape("test"),
				p,
			)
			_, err = netx.DialX(target, netx.DialX_WithProxy(s5proxy))
			if err == nil {
				t.Fatal("mitm s5 proxy auth negative test failed: should not connect to server")
			}

			lowhttpRsp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte(fmt.Sprintf("GET http://%s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target))), lowhttp.WithHost("127.0.0.1"), lowhttp.WithPort(p))
			require.NoError(t, err)
			require.NotNil(t, lowhttpRsp)
			require.Equal(t, lowhttpRsp.GetStatusCode(), http.StatusProxyAuthRequired, "mitm http1.1 proxy auth negative test failed: should not connect to server")

		}
	}

}

func TestGRPCMUSTPASS_MITMV2_AUTH(t *testing.T) {
	test := assert.New(t)
	_ = test

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\na"))

	p := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMV2Request{
		Host:            "127.0.0.1",
		Port:            uint32(p),
		ProxyUsername:   "admin" + "@/",
		ProxyPassword:   "@/:",
		EnableProxyAuth: true,
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		msg := string(rsp.GetMessage().GetMessage())
		if strings.Contains(msg, `starting mitm server`) {
			proxy := fmt.Sprintf("http://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("admin@/"),
				codec.QueryEscape("@/:"),
				p,
			)
			conn, err := netx.DialX(utils.HostPort(host, port), netx.DialX_WithProxy(proxy))
			if err != nil {
				t.Fatal(err)
			}
			conn.Close()
			cancel()
		}
	}

}

func TestGRPCMUSTPASS_MITMV2_AUTH_Negative(t *testing.T) {
	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.MITMV2(ctx)
	if err != nil {
		t.Fatal(err)
	}

	host, port := utils.DebugMockHTTP([]byte("HTTP/1.1 200 OK\r\n" +
		"Content-Length: 1\r\n\r\na"))

	p := utils.GetRandomAvailableTCPPort()
	stream.Send(&ypb.MITMV2Request{
		Host:            "127.0.0.1",
		Port:            uint32(p),
		ProxyUsername:   "admin",
		ProxyPassword:   "12345",
		EnableProxyAuth: true,
	})

	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		target := utils.HostPort(host, port)
		msg := string(rsp.GetMessage().GetMessage())
		if strings.Contains(msg, `starting mitm server`) {
			proxy := fmt.Sprintf("http://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("test"),
				codec.QueryEscape("test"),
				p,
			)
			_, err := netx.DialX(target, netx.DialX_WithProxy(proxy))
			if err == nil {
				t.Fatal("mitm http1.1 proxy auth negative test failed: should not connect to server")
			}

			s5proxy := fmt.Sprintf("socks5://%v:%v@127.0.0.1:%v",
				codec.QueryEscape("test"),
				codec.QueryEscape("test"),
				p,
			)
			_, err = netx.DialX(target, netx.DialX_WithProxy(s5proxy))
			if err == nil {
				t.Fatal("mitm s5 proxy auth negative test failed: should not connect to server")
			}

			lowhttpRsp, err := lowhttp.HTTPWithoutRedirect(lowhttp.WithPacketBytes([]byte(fmt.Sprintf("GET http://%s HTTP/1.1\r\nHost: %s\r\n\r\n", target, target))), lowhttp.WithHost("127.0.0.1"), lowhttp.WithPort(p))
			require.NoError(t, err)
			require.NotNil(t, lowhttpRsp)
			require.Equal(t, lowhttpRsp.GetStatusCode(), http.StatusProxyAuthRequired, "mitm http1.1 proxy auth negative test failed: should not connect to server")

		}
	}

}
