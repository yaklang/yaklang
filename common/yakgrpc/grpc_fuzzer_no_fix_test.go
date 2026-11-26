package yakgrpc

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// fuzzerWireServer 记录 WebFuzzer 实际发出的原始字节。
type fuzzerWireServer struct {
	addr string

	mu      sync.Mutex
	packets [][]byte
}

func newFuzzerWireServer(t *testing.T) *fuzzerWireServer {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = lis.Close() })

	s := &fuzzerWireServer{addr: lis.Addr().String()}
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()

				var buf []byte
				tmp := make([]byte, 1<<20)
				for {
					_ = conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
					n, rerr := conn.Read(tmp)
					if n > 0 {
						buf = append(buf, tmp[:n]...)
					}
					if rerr != nil {
						break
					}
				}
				if len(buf) > 0 {
					s.mu.Lock()
					s.packets = append(s.packets, append([]byte(nil), buf...))
					s.mu.Unlock()
				}
				_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok"))
			}(conn)
		}
	}()
	return s
}

func (s *fuzzerWireServer) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.packets = nil
}

func (s *fuzzerWireServer) only(t *testing.T) []byte {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	require.Len(t, s.packets, 1, "expect exactly one request on the wire")
	return s.packets[0]
}

func (s *fuzzerWireServer) recv(t *testing.T, req *ypb.FuzzerRequest) *ypb.FuzzerResponse {
	t.Helper()

	c, err := NewLocalClient()
	require.NoError(t, err)
	stream, err := c.HTTPFuzzer(context.Background(), req)
	require.NoError(t, err)

	var last *ypb.FuzzerResponse
	for {
		rsp, err := stream.Recv()
		if err != nil {
			break
		}
		last = rsp
	}
	require.NotNil(t, last)
	time.Sleep(200 * time.Millisecond)
	return last
}

// TestHTTPFuzzerNoFixContentLengthKeepsWirePacket 是前端「不修复数据包」开关的端到端契约：
// NoFixContentLength=true 时，服务端收到的请求字节必须与用户构造的一致；
// 关闭时保持原来的自动修复行为。
func TestHTTPFuzzerNoFixContentLengthKeepsWirePacket(t *testing.T) {
	srv := newFuzzerWireServer(t)

	packet := fmt.Sprintf("POST /cl HTTP/1.1\r\nHost: %s\r\nContent-Length: 100\r\n\r\nhello world", srv.addr)

	for _, noFix := range []bool{true, false} {
		srv.reset()
		rsp := srv.recv(t, &ypb.FuzzerRequest{
			Request:                  packet,
			NoFixContentLength:       noFix,
			DisableUseConnPool:       true,
			PerRequestTimeoutSeconds: 3,
		})
		require.True(t, rsp.Ok)

		wire := srv.only(t)
		header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(wire)
		gotCL := lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Length")

		// 无论哪种模式，响应元信息都必须可用（否则 WebFuzzer 结果表会没有状态码）
		require.Equal(t, int32(200), rsp.StatusCode)
		require.NotEmpty(t, rsp.Headers)

		if noFix {
			require.Equal(t, packet, string(wire), "「不修复数据包」必须原样发送")
			require.Equal(t, "100", gotCL)
			continue
		}
		require.Equal(t, "11", gotCL, "默认模式仍应修复 Content-Length")
	}
}

// TestHTTPFuzzerNoFixContentLengthReplaysExternalizedRequest 复现用户反馈的场景：
// multipart 上传的二进制 body 被外置成 {{file(...)}} 后，报文里的 Content-Length 必须
// 仍是原始 body 长度，否则「不修复数据包」发出的是错包。
func TestHTTPFuzzerNoFixContentLengthReplaysExternalizedRequest(t *testing.T) {
	const dumpLimit = 1 * 1024 * 1024
	prevLimit := consts.GetGlobalMaxContentLength()
	consts.SetGlobalMaxContentLength(dumpLimit)
	t.Setenv("YAKIT_HOME", t.TempDir())
	t.Cleanup(func() { consts.SetGlobalMaxContentLength(prevLimit) })

	srv := newFuzzerWireServer(t)

	bigPart := strings.Repeat("A", 2*dumpLimit)
	body := "--BOUND\r\n" +
		"Content-Disposition: form-data; name=\"file\"; filename=\"image.jpg\"\r\n" +
		"Content-Type: image/jpeg\r\n\r\n" + bigPart + "\r\n--BOUND--\r\n"
	packet := fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: %s\r\n"+
		"Content-Type: multipart/form-data; boundary=BOUND\r\nContent-Length: %d\r\n\r\n%s",
		srv.addr, len(body), body)

	// 模拟历史流量 → WebFuzzer：大 body 被外置成 {{file(...)}}
	stored, externalized, originalBodyLen, err := yakit.PrepareFuzzableHTTPRequestForStorage([]byte(packet))
	require.NoError(t, err)
	require.True(t, externalized)
	require.Equal(t, len(body), originalBodyLen)
	t.Cleanup(func() { yakit.CleanupFuzzableHTTPRequestResources(stored) })

	storedHeader, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(stored)
	require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(storedHeader), "Content-Length"),
		"外置后编辑/展示的报文必须保留原始 Content-Length")

	for _, noFix := range []bool{true, false} {
		srv.reset()
		rsp := srv.recv(t, &ypb.FuzzerRequest{
			RequestRaw:               stored,
			NoFixContentLength:       noFix,
			DisableUseConnPool:       true,
			PerRequestTimeoutSeconds: 10,
			FuzzTagMode:              "standard",
			ForceFuzz:                true,
		})
		require.True(t, rsp.Ok)

		wire := srv.only(t)
		wireHeader, wireBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(wire)
		require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(wireHeader), "Content-Length"),
			"noFix=%v: Content-Length 必须等于渲染后的 body 长度", noFix)
		require.Equal(t, len(body), len(wireBody), "noFix=%v: body 必须渲染回原始字节", noFix)
		require.Equal(t, int32(200), rsp.StatusCode)
	}
}
