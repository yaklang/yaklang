package lowhttp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestConnPoolH1AcquiresConnectionAtCapacity(t *testing.T) {
	for _, tc := range []struct {
		name        string
		alreadyIdle bool
		sameOrigin  bool
	}{
		{name: "reclaim idle connection", alreadyIdle: true},
		{name: "reuse connection returned while waiting", sameOrigin: true},
		{name: "reclaim connection returned while waiting"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := make(chan string, 2)
			releaseFirst := make(chan struct{})
			release := sync.OnceFunc(func() { close(releaseFirst) })
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.RemoteAddr
				if r.URL.Path == "/first" {
					<-releaseFirst
				}
				_, _ = io.WriteString(w, "ok")
			})
			firstOrigin := httptest.NewServer(handler)
			defer firstOrigin.Close()
			secondOrigin := firstOrigin
			if !tc.sameOrigin {
				secondOrigin = httptest.NewServer(handler)
				defer secondOrigin.Close()
			}
			defer release()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			pool := NewHttpConnPool(ctx, 1, 1)
			defer pool.Clear()

			send := func(origin, path string) <-chan error {
				result := make(chan error, 1)
				go func() {
					host := strings.TrimPrefix(origin, "http://")
					_, err := HTTPWithoutRedirect(
						WithPacketBytes([]byte(fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\n\r\n", path, host))),
						WithContext(ctx), WithConnPool(true), ConnPool(pool),
						WithConnectTimeout(2*time.Second),
					)
					result <- err
				}()
				return result
			}

			first := send(firstOrigin.URL, "/first")
			var firstPeer string
			select {
			case firstPeer = <-requests:
			case <-ctx.Done():
				t.Fatal("the first request did not reach the origin")
			}
			if tc.alreadyIdle {
				release()
				require.NoError(t, <-first)
				require.Eventually(t, func() bool {
					pool.idleConnMux.Lock()
					defer pool.idleConnMux.Unlock()
					return len(pool.idleConnMap) == 1 && len(pool.connSem) == 1
				}, time.Second, time.Millisecond)
			}

			started := time.Now()
			second := send(secondOrigin.URL, "/second")
			if !tc.alreadyIdle {
				require.Eventually(t, func() bool {
					pool.idleConnMux.Lock()
					defer pool.idleConnMux.Unlock()
					return pool.h1IdleReady != nil
				}, time.Second, time.Millisecond, "the second request should wait while the slot is in use")
				release()
				require.NoError(t, <-first)
			}
			require.NoError(t, <-second)
			t.Logf("second request completed in %s", time.Since(started))
			if tc.sameOrigin {
				require.Equal(t, firstPeer, <-requests, "the waiter should reuse the returned connection")
			}
		})
	}
}

func TestConnPoolH1SlotWaitRespectsConnectTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool := NewHttpConnPool(ctx, 1, 1)
	defer pool.Clear()
	key := &connectKey{scheme: H1, addr: "127.0.0.1:1"}
	_, err := pool.acquireH1Conn(ctx, key, 0)
	require.NoError(t, err)
	defer pool.releaseConnSlot()

	_, err = HTTPWithoutRedirect(
		WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: 127.0.0.1:1\r\n\r\n")),
		WithContext(ctx), WithConnPool(true), ConnPool(pool),
		WithConnectTimeout(40*time.Millisecond),
	)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "waiting for HTTP/1.1 connection slot")
	require.Len(t, pool.connSem, 1)
}

func TestConnPoolH1EvictionKeepsBufferedResponse(t *testing.T) {
	// Exercise both select branches when the response and closure are ready.
	for range 32 {
		func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			pool := NewHttpConnPool(ctx, 1, 1)
			defer pool.Clear()
			key := &connectKey{scheme: H1, addr: "first.invalid:80"}
			_, err := pool.acquireH1Conn(ctx, key, 0)
			require.NoError(t, err)

			socket, peer := net.Pipe()
			defer socket.Close()
			defer peer.Close()
			connCtx, closeConn := context.WithCancel(context.Background())
			defer closeConn()
			pc := &persistConn{
				ctx:      connCtx,
				cancel:   closeConn,
				conn:     socket,
				p:        pool,
				cacheKey: key,
				reqCh:    make(chan requestAndResponseCh, 1),
				writeCh:  make(chan writeRequest),
			}
			tr := &transportRequest{option: &LowhttpExecConfig{}, traceInfo: &LowhttpTraceInfo{}}
			type outcome struct {
				response *transportResult
				err      error
			}
			done := make(chan outcome, 1)
			go func() {
				response, err := (&h1Transport{pool: pool}).roundTripPooled(ctx, tr, pc, nil)
				done <- outcome{response, err}
			}()

			request := <-pc.reqCh
			// Reproduce readLoop publishing the response and then returning
			// the connection to the pool. Keep the caller at its write handoff
			// until admission for another origin has evicted the connection.
			packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
			request.ch <- responseInfo{
				resp: &http.Response{StatusCode: http.StatusOK}, respBytes: packet,
				info: httpInfo{ServerTime: 10 * time.Millisecond},
			}
			err = pool.putIdleConn(pc)
			_, admissionErr := pool.acquireH1Conn(ctx, &connectKey{scheme: H1, addr: "second.invalid:80"}, 0)
			<-pc.writeCh
			result := <-done
			require.NoError(t, err)
			require.NoError(t, admissionErr)
			require.ErrorIs(t, connCtx.Err(), context.Canceled)
			require.NoError(t, result.err)
			require.Equal(t, http.StatusOK, result.response.firstResponse.StatusCode)
			require.Equal(t, packet, result.response.rawBytes)
			require.Equal(t, 10*time.Millisecond, tr.traceInfo.ServerTime)
		}()
	}
}

type gatedCloseConn struct {
	net.Conn
	closing   chan struct{}
	finish    <-chan struct{}
	closeOnce sync.Once
}

func (c *gatedCloseConn) Close() error {
	c.closeOnce.Do(func() {
		close(c.closing)
		<-c.finish
		_ = c.Conn.Close()
	})
	return nil
}

func TestConnPoolH1DoesNotPublishConnectionWhileCancellationClosesIt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool := NewHttpConnPool(ctx, 1, 1)
	defer pool.Clear()
	key := &connectKey{scheme: H1, addr: "origin.invalid:80"}
	_, err := pool.acquireH1Conn(ctx, key, 0)
	require.NoError(t, err)

	socket, origin := net.Pipe()
	defer socket.Close()
	defer origin.Close()
	finishClose := make(chan struct{})
	releaseClose := sync.OnceFunc(func() { close(finishClose) })
	defer releaseClose()
	conn := &gatedCloseConn{Conn: socket, closing: make(chan struct{}), finish: finishClose}
	connCtx, closeConn := context.WithCancel(context.Background())
	defer closeConn()
	pc := &persistConn{
		ctx: connCtx, cancel: closeConn, conn: conn, p: pool, cacheKey: key,
		reqCh: make(chan requestAndResponseCh, 1), br: bufio.NewReader(socket),
	}
	requestCtx, cancelRequest := context.WithCancel(ctx)
	defer cancelRequest()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "http://origin.invalid/", nil)
	require.NoError(t, err)
	httpctx.SetResponseFinishedCallback(req, func() {
		cancelRequest()
		select {
		case <-conn.closing:
		case <-ctx.Done():
		}
	})
	response := make(chan responseInfo, 1)
	pc.reqCh <- requestAndResponseCh{
		reqInstance: req,
		option:      &LowhttpExecConfig{Ctx: requestCtx},
		ch:          response,
	}
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		pc.readLoop()
	}()
	require.NoError(t, origin.SetWriteDeadline(time.Now().Add(time.Second)))
	_, err = io.WriteString(origin, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
	require.NoError(t, err)
	select {
	case <-conn.closing:
	case <-ctx.Done():
		t.Fatal("cancellation did not reach the connection")
	}
	waitCtx, stopWaiting := context.WithTimeout(ctx, 30*time.Millisecond)
	defer stopWaiting()
	idle, err := pool.acquireH1Conn(waitCtx, key, 0)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, idle)
	releaseClose()
	select {
	case result := <-response:
		require.NoError(t, result.err)
		require.Equal(t, http.StatusOK, result.resp.StatusCode)
	case <-ctx.Done():
		t.Fatal("the completed response was lost")
	}
	select {
	case <-readDone:
	case <-ctx.Done():
		t.Fatal("the reader remained active after closing")
	}
	require.Empty(t, pool.idleConnMap)
	require.Empty(t, pool.connSem)
}
