package lowhttp

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/net/http2"
)

func TestH2_Serve(t *testing.T) {
	var port int
	var lis net.Listener
	var err error
	for i := 0; i < 10; i++ {
		port = utils.GetRandomAvailableTCPPort()
		lis, err = net.Listen("tcp", utils.HostPort("127.0.0.1", port))
		if err != nil {
			t.Error(err)
			continue
		}
		break
	}
	if lis == nil {
		t.Fatal("lis is nil")
	}
	defer lis.Close()

	token1, token2 := utils.RandStringBytes(20), utils.RandStringBytes(200)

	var checkPass atomic.Bool
	connCh := make(chan net.Conn, 1)
	serveDone := make(chan error, 1)

	go func() {
		conn, acceptErr := lis.Accept()
		if acceptErr != nil {
			serveDone <- acceptErr
			return
		}
		connCh <- conn
		serveDone <- serveH2(conn, conn, withH2Handler(func(header []byte, body io.ReadCloser) ([]byte, io.ReadCloser, error) {
			spew.Dump(header)

			if strings.Contains(string(header), "GET /"+token1+" HTTP/2") {
				checkPass.Store(true)
			}
			resp := []byte("HTTP/2 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 3\r\n\r\nabc")
			return resp, io.NopCloser(bytes.NewBufferString(token2)), nil
		}))
	}()

	reqBytes := []byte(fmt.Sprintf("GET /%s HTTP/2\r\nHost: 127.0.0.1\r\n\r\nabc", token1))
	rsp, err := HTTPWithoutRedirect(
		WithHttps(false),
		WithHttp2(true),
		WithPacketBytes(reqBytes),
		WithHost("127.0.0.1"),

		WithPort(port),
		WithRetryTimes(5),
	)
	if err != nil {
		t.Fatalf("http2 request failed: %v", err)
	}

	var conn net.Conn
	select {
	case conn = <-connCh:
	case acceptErr := <-serveDone:
		if acceptErr != nil {
			t.Fatalf("accept failed: %v", acceptErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for http2 connection")
	}
	t.Cleanup(func() {
		if conn != nil {
			_ = conn.Close()
		}
	})
	if !checkPass.Load() {
		t.Fatal("checkPass failed (h2 server cannot serve)")
	}
	if !bytes.Contains(rsp.RawPacket, []byte(token2)) {
		t.Fatal("token2 not found in response")
	}

	if conn != nil {
		_ = conn.Close()
	}

	select {
	case serveErr := <-serveDone:
		if serveErr != nil && !errors.Is(serveErr, net.ErrClosed) && !errors.Is(serveErr, io.EOF) && !strings.Contains(serveErr.Error(), "use of closed network connection") {
			t.Fatalf("serveH2 returned error: %v", serveErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serveH2 did not exit after closing connection")
	}
}

func TestH2ServeRSTStreamCancelsOnlyCurrentStream(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serveDone := make(chan error, 1)
	slowStarted := make(chan struct{})
	slowCanceled := make(chan struct{})

	go func() {
		serverConn, acceptErr := listener.Accept()
		if acceptErr != nil {
			serveDone <- acceptErr
			return
		}
		serveDone <- ServeHTTP2ConnectionWithContext(serverConn, func(ctx context.Context, header []byte, _ io.ReadCloser) ([]byte, io.ReadCloser, error) {
			if strings.Contains(string(header), " /slow ") {
				close(slowStarted)
				<-ctx.Done()
				close(slowCanceled)
				return nil, nil, ctx.Err()
			}
			return []byte("HTTP/2 200 OK\r\nContent-Length: 2\r\n\r\n"), io.NopCloser(strings.NewReader("ok")), nil
		})
	}()

	var dialCount atomic.Int32
	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLSContext: func(ctx context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
			if dialCount.Add(1) != 1 {
				return nil, errors.New("unexpected second HTTP/2 connection")
			}
			var dialer net.Dialer
			return dialer.DialContext(ctx, "tcp", listener.Addr().String())
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}

	slowCtx, cancelSlow := context.WithCancel(context.Background())
	slowReq, err := http.NewRequestWithContext(slowCtx, http.MethodGet, "http://example.test/slow", nil)
	if err != nil {
		t.Fatal(err)
	}
	slowResult := make(chan error, 1)
	go func() {
		_, requestErr := client.Do(slowReq)
		slowResult <- requestErr
	}()

	select {
	case <-slowStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("slow downstream H2 stream did not start")
	}
	cancelSlow()
	select {
	case err := <-slowResult:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled downstream H2 request error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("downstream H2 request did not return after cancellation")
	}
	select {
	case <-slowCanceled:
	case <-time.After(2 * time.Second):
		t.Fatal("RST_STREAM did not cancel the downstream stream context")
	}

	response, err := client.Get("http://example.test/fast")
	if err != nil {
		t.Fatalf("second stream on shared downstream H2 connection failed: %v", err)
	}
	responseBody, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(responseBody) != "ok" {
		t.Fatalf("second stream body = %q, want ok", responseBody)
	}
	if got := dialCount.Load(); got != 1 {
		t.Fatalf("HTTP/2 connection count = %d, want 1", got)
	}

	transport.CloseIdleConnections()
	select {
	case err := <-serveDone:
		if err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.EOF) && !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "closed pipe") {
			t.Fatalf("HTTP/2 server returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP/2 server did not stop after closing the connection")
	}
}
