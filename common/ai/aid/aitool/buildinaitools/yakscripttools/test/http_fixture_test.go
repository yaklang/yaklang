package test

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
)

// The listener is ready before returning; cleanup closes active connections and
// joins accept/handler workers. Callers need neither startup sleeps nor probes.
func startOwnedTCPFixture(t *testing.T, serve func(net.Conn)) (string, int, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	var mu sync.Mutex
	connections := map[net.Conn]struct{}{}
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			connections[conn] = struct{}{}
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { _ = conn.Close(); mu.Lock(); delete(connections, conn); mu.Unlock() }()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				serve(conn)
			}()
		}
	}()
	var once sync.Once
	closeFixture := func() {
		once.Do(func() {
			_ = listener.Close()
			<-acceptDone
			mu.Lock()
			for conn := range connections {
				_ = conn.Close()
			}
			mu.Unlock()
			workers.Wait()
		})
	}
	t.Cleanup(closeFixture)
	addr := listener.Addr().(*net.TCPAddr)
	return addr.IP.String(), addr.Port, closeFixture
}

// Reading the request before closing preserves EOF/FIN semantics without RST
// from unread request bytes. Raw responses remain raw, including malformed ones.
func startImmediateHTTPResponse(t *testing.T, response []byte) (string, int) {
	t.Helper()
	host, port, _ := startOwnedTCPFixture(t, func(conn net.Conn) {
		request, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(conn))
		if err != nil {
			return
		}
		if request.Body != nil {
			_, _ = io.Copy(io.Discard, request.Body)
			_ = request.Body.Close()
		}
		_, _ = conn.Write(response)
	})
	return host, port
}

func startRawHTTPResponse(t *testing.T, respond func([]byte) []byte) (string, int) {
	t.Helper()
	host, port, _ := startOwnedTCPFixture(t, func(conn net.Conn) {
		request, err := utils.ReadHTTPRequestFromBufioReader(bufio.NewReader(conn))
		if err != nil {
			return
		}
		raw, err := utils.DumpHTTPRequest(request, true)
		if err != nil {
			return
		}
		if request.Body != nil {
			defer request.Body.Close()
		}
		_, _ = conn.Write(respond(raw))
	})
	return host, port
}

func startHTTPHandler(t *testing.T, handler http.HandlerFunc) (string, int) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	host, port, err := utils.ParseStringToHostPort(server.URL)
	require.NoError(t, err)
	return host, port
}
