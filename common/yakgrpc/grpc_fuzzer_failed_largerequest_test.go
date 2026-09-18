package yakgrpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGRPCMUSTPASS_HTTPFUZZER_LargeRequest_Failed(t *testing.T) {
	var crazyBody = "{{repeatstr(A|130000000)}}"

	// 构造一个很容易网络错误的东西
	port := utils.GetRandomAvailableTCPPort()
	lis, err := net.Listen("tcp", "127.0.0.1:"+fmt.Sprint(port))
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				break
			}
			conn.Close()
		}
	}()
	err = utils.WaitConnect(utils.HostPort("127.0.0.1", port), 4)
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewLocalClient()
	if err != nil {
		t.Fatal(err)
	}
	host := "127.0.0.1"
	token := utils.RandStringBytes(10)
	target := utils.HostPort(host, port)
	stream, err := client.HTTPFuzzer(context.Background(), &ypb.FuzzerRequest{
		Request:     "GET /?token=" + token + " HTTP/1.1\r\nHost: " + target + "\r\n\r\n" + crazyBody,
		ForceFuzz:   true,
		RepeatTimes: 20,
	})
	if err != nil {
		panic(err)
	}
	var count int
	for i := 0; i < 30; i++ {
		resp, err := stream.Recv()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "large requests must not exceed the gRPC response limit")
		if bytes.Contains(resp.RequestRaw, []byte("show chunked by yakit web fuzzer")) {
			count++
		}
	}
	assert.Equal(t, 20, count)
}

func TestGRPCMUSTPASS_FuzzerRequestPreview(t *testing.T) {
	for _, size := range []int{0, 2*1024*1024 - 1, 2 * 1024 * 1024, 2*1024*1024 + 100} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			packet := bytes.Repeat([]byte("A"), size)
			original := bytes.Clone(packet)
			preview := fuzzerRequestPreview(packet)
			require.Equal(t, original, packet, "preview must not overwrite the original request")
			if size <= 2*1024*1024 {
				require.Equal(t, packet, preview)
			} else {
				require.True(t, bytes.HasPrefix(preview, packet[:2*1024*1024]))
				require.Equal(t, "...(request > 2M) show chunked by yakit web fuzzer", string(preview[2*1024*1024:]))
			}
		})
	}
	require.Nil(t, fuzzerRequestPreview(nil))
}

func TestGRPCMUSTPASS_HTTPFUZZER_LargeRequest_SentIntact(t *testing.T) {
	body := bytes.Repeat([]byte("A"), 3*1024*1024)
	received := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		packet, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		received <- packet
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()
	client, err := NewLocalClient()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stream, err := client.HTTPFuzzer(ctx, &ypb.FuzzerRequest{
		Request: fmt.Sprintf("POST / HTTP/1.1\r\nHost: %s\r\nContent-Length: %d\r\n\r\n%s", server.Listener.Addr(), len(body), body),
	})
	require.NoError(t, err)
	response, err := stream.Recv()
	require.NoError(t, err)
	require.True(t, response.GetOk(), response.GetReason())
	require.Contains(t, string(response.GetRequestRaw()), "show chunked by yakit web fuzzer")
	require.Less(t, len(response.GetRequestRaw()), len(body))
	select {
	case packet := <-received:
		require.Equal(t, body, packet, "only the preview may be truncated, never the request sent on the wire")
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, err = stream.Recv()
	require.ErrorIs(t, err, io.EOF)
}
