package lowhttp

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestHTTP2WaitResponse_NoLeakOnTimeout(t *testing.T) {
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
		t.Fatal("listener is nil")
	}
	defer lis.Close()

	blockCh := make(chan struct{})
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
			<-blockCh
			resp := []byte("HTTP/2 200 OK\r\nContent-Length: 0\r\n\r\n")
			return resp, io.NopCloser(bytes.NewBuffer(nil)), nil
		}))
	}()

	reqBytes := []byte(fmt.Sprintf("GET / HTTP/2\r\nHost: %s\r\n\r\n", utils.HostPort("127.0.0.1", port)))

	assertNoGoroutineLeak(t, "http2 wait response timeout", func() {
		resultCh := make(chan error, 1)
		go func() {
			_, err := HTTPWithoutRedirect(
				WithHttps(false),
				WithHttp2(true),
				WithPacketBytes(reqBytes),
				WithHost("127.0.0.1"),
				WithPort(port),
				WithTimeout(200*time.Millisecond),
			)
			resultCh <- err
		}()

		var conn net.Conn
		select {
		case conn = <-connCh:
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for http2 connection")
		}
		defer func() {
			close(blockCh)
			if conn != nil {
				_ = conn.Close()
			}
			select {
			case <-serveDone:
			case <-time.After(3 * time.Second):
				t.Fatal("serveH2 did not exit after closing connection")
			}
		}()

		select {
		case err := <-resultCh:
			if err == nil {
				t.Fatal("expected timeout error")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for http2 request")
		}
	})
}
