package lowhttp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
