package lowhttp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type countedReadConn struct {
	net.Conn
	reads atomic.Int32
}

func (c *countedReadConn) Read(p []byte) (int, error) {
	c.reads.Add(1)
	return c.Conn.Read(p)
}

func TestTransportPreservesHTTPSResponseFlag(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(server.Close)

	addr := server.Listener.Addr().String()
	var checkerHTTPS bool
	rsp, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", addr))),
		WithHttps(true),
		WithVerifyCertificate(false),
		WithCustomFailureChecker(func(https bool, _ []byte, _ []byte, _ func(string)) {
			checkerHTTPS = https
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !rsp.Https || !checkerHTTPS {
		t.Fatalf("HTTPS request reported rsp.Https=%v, checkerHTTPS=%v", rsp.Https, checkerHTTPS)
	}
}

func TestDirectH1WaitsForBodyStreamReaderHandler(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "streamed body")
	}))
	t.Cleanup(server.Close)

	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	allowReturn := func() { releaseOnce.Do(func() { close(release) }) }
	defer allowReturn()
	returned := make(chan error, 1)
	addr := server.Listener.Addr().String()
	go func() {
		_, err := HTTPWithoutRedirect(
			WithPacketBytes([]byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", addr))),
			WithNoBodyBuffer(true),
			WithBodyStreamReaderHandler(func(_ []byte, body io.ReadCloser) {
				_, _ = io.Copy(io.Discard, body)
				close(started)
				<-release
			}),
		)
		returned <- err
	}()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("body stream handler did not receive the response")
	}
	select {
	case err := <-returned:
		t.Fatalf("request returned before body stream handler completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	allowReturn()
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request did not return after body stream handler completed")
	}
}

func TestDirectH1DoesNotDrainCompleteKeepAliveResponse(t *testing.T) {
	client, server := net.Pipe()
	conn := &countedReadConn{Conn: client}
	serverDone := make(chan struct{})
	defer close(serverDone)
	go func() {
		defer server.Close()
		reader := bufio.NewReader(server)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if line == "\r\n" {
				break
			}
		}
		_, _ = io.WriteString(server, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
		<-serverDone // Keep the connection alive after the complete response.
	}()

	rsp, err := HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n")),
		WithHost("127.0.0.1"),
		WithPort(80),
		WithTimeout(time.Second),
		WithDialer(func(time.Duration, string) (net.Conn, error) { return conn, nil }),
	)
	if err != nil {
		t.Fatal(err)
	}
	if string(GetHTTPPacketBody(rsp.RawPacket)) != "ok" {
		t.Fatalf("unexpected response: %q", rsp.RawPacket)
	}
	if got := conn.reads.Load(); got != 1 {
		t.Fatalf("complete keep-alive response caused %d socket reads, want 1", got)
	}
}
