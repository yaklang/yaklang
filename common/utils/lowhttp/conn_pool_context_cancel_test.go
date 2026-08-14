package lowhttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestConnPoolRequestContextCancellationStopsActiveRead(t *testing.T) {
	requestStarted := make(chan struct{})
	requestStopped := make(chan struct{})
	releaseRequest := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(requestStarted)
		select {
		case <-request.Context().Done():
			close(requestStopped)
		case <-releaseRequest:
			writer.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	host, port, err := utils.ParseStringToHostPort(parsed.Host)
	if err != nil {
		t.Fatal(err)
	}

	poolCtx, cancelPool := context.WithCancel(context.Background())
	defer cancelPool()
	pool := NewHttpConnPool(poolCtx, 4, 2)
	defer pool.Clear()

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, requestErr := HTTPWithoutRedirect(
			WithPacketBytes([]byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\nConnection: keep-alive\r\n\r\n", parsed.Host))),
			WithHost(host),
			WithPort(port),
			WithContext(requestCtx),
			WithTimeout(5*time.Second),
			WithConnPool(true),
			ConnPool(pool),
		)
		result <- requestErr
	}()

	select {
	case <-requestStarted:
	case <-time.After(2 * time.Second):
		close(releaseRequest)
		t.Fatal("request did not start")
	}
	cancelRequest()

	select {
	case requestErr := <-result:
		if requestErr == nil || requestErr != context.Canceled {
			close(releaseRequest)
			t.Fatalf("request error = %v, want context canceled", requestErr)
		}
	case <-time.After(2 * time.Second):
		close(releaseRequest)
		t.Fatal("request did not stop after context cancellation")
	}

	select {
	case <-requestStopped:
	case <-time.After(2 * time.Second):
		close(releaseRequest)
		t.Fatal("server connection remained active after cancellation")
	}
}

func TestConnPoolRequestContextCancellationStopsSlotWait(t *testing.T) {
	pool := NewHttpConnPool(context.Background(), 1, 1)
	defer pool.Clear()
	if err := pool.acquireConnSlot(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer pool.releaseConnSlot()

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- pool.acquireConnSlot(requestCtx)
	}()
	cancelRequest()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("slot wait error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request remained blocked waiting for an H1 pool slot")
	}
}

func TestHTTP2RequestContextCancellationResetsOnlyCurrentStream(t *testing.T) {
	slowStarted := make(chan struct{})
	releaseSlow := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseSlow) })
	}
	defer release()

	handler := func(header []byte, _ io.ReadCloser) ([]byte, io.ReadCloser, error) {
		if strings.Contains(string(header), " /slow ") {
			close(slowStarted)
			<-releaseSlow
		}
		return []byte("HTTP/2 200 OK\r\nContent-Length: 2\r\n\r\n"), io.NopCloser(strings.NewReader("ok")), nil
	}
	port, shutdown := startPlainH2Server(t, handler)
	defer shutdown()

	pool := h2PoolFor(context.Background(), time.Minute)
	defer pool.Clear()
	send := func(ctx context.Context, path string) error {
		_, err := HTTPWithoutRedirect(
			WithHttps(false),
			WithHttp2(true),
			WithPacketBytes([]byte(fmt.Sprintf("GET %s HTTP/2\r\nHost: 127.0.0.1:%d\r\n\r\n", path, port))),
			WithHost("127.0.0.1"),
			WithPort(port),
			WithConnPool(true),
			ConnPool(pool),
			WithContext(ctx),
			WithTimeout(5*time.Second),
		)
		return err
	}

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- send(requestCtx, "/slow") }()
	select {
	case <-slowStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("slow H2 stream did not start")
	}
	cancelRequest()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled H2 stream error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("H2 stream remained blocked after request cancellation")
	}

	pool.h2Mu.Lock()
	connectionCount := len(pool.h2ConnMap)
	connectionClosed := false
	for _, pc := range pool.h2ConnMap {
		connectionClosed = connectionClosed || pc.alt == nil || pc.alt.closed
	}
	pool.h2Mu.Unlock()
	if connectionCount != 1 || connectionClosed {
		t.Fatalf("canceling one H2 stream affected shared connection: count=%d closed=%t", connectionCount, connectionClosed)
	}

	if err := send(context.Background(), "/fast"); err != nil {
		t.Fatalf("second stream on shared H2 connection failed: %v", err)
	}
	release()
}

func TestHTTP2StreamSlotWaitObservesRequestContext(t *testing.T) {
	h2Conn := &http2ClientConn{
		mu:              new(sync.Mutex),
		activeStreams:   1,
		maxStreamsCount: 1,
	}
	h2Conn.streamsCond = sync.NewCond(h2Conn.mu)

	requestCtx, cancelRequest := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://example.invalid/", nil)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, streamErr := h2Conn.newStream(req, nil, nil)
		result <- streamErr
	}()
	cancelRequest()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("H2 stream slot wait error = %v, want context canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request remained blocked waiting for an H2 stream slot")
	}
}

func TestRequestContextCancellationStopsDial(t *testing.T) {
	for _, withPool := range []bool{false, true} {
		withPool := withPool
		t.Run(fmt.Sprintf("pool=%t", withPool), func(t *testing.T) {
			dialStarted := make(chan struct{})
			releaseDial := make(chan struct{})
			dialReturned := make(chan struct{})
			dialConn, peerConn := net.Pipe()
			defer peerConn.Close()
			var releaseOnce sync.Once
			release := func() {
				releaseOnce.Do(func() { close(releaseDial) })
			}
			defer release()

			requestCtx, cancelRequest := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				_, requestErr := HTTPWithoutRedirect(
					WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1\r\nConnection: close\r\n\r\n")),
					WithHost("127.0.0.1"),
					WithPort(1),
					WithContext(requestCtx),
					WithTimeout(5*time.Second),
					WithConnectTimeout(5*time.Second),
					WithConnPool(withPool),
					WithDialer(func(time.Duration, string) (net.Conn, error) {
						close(dialStarted)
						<-releaseDial
						close(dialReturned)
						return dialConn, nil
					}),
				)
				result <- requestErr
			}()

			select {
			case <-dialStarted:
			case <-time.After(2 * time.Second):
				t.Fatal("dial did not start")
			}
			cancelRequest()

			select {
			case requestErr := <-result:
				if !errors.Is(requestErr, context.Canceled) {
					t.Fatalf("request error = %v, want context canceled", requestErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("request remained blocked in dial after cancellation")
			}

			if err := peerConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatal(err)
			}
			release()
			select {
			case <-dialReturned:
			case <-time.After(2 * time.Second):
				t.Fatal("background dial did not finish after release")
			}
			if _, err := peerConn.Read(make([]byte, 1)); err == nil {
				t.Fatal("connection returned after cancellation was not closed")
			}
		})
	}
}
