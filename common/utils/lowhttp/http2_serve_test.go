package lowhttp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
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
