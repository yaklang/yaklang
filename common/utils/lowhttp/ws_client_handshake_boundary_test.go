package lowhttp

import (
	"bufio"
	"bytes"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func websocketBoundaryResponse(extraHeaders string) []byte {
	return []byte(fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: %s\r\n%s\r\n",
		ComputeWebsocketAcceptKey("dGhlIHNhbXBsZSBub25jZQ=="), extraHeaders))
}

func websocketBoundaryRequest(t *testing.T, address string) (*http.Request, string, int) {
	t.Helper()
	host, portText, err := net.SplitHostPort(address)
	require.NoError(t, err)
	port, err := strconv.Atoi(portText)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodGet, "http://"+address+"/ws", nil)
	require.NoError(t, err)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	return req, host, port
}

func TestWebsocketClientOpeningHandshakeBoundaryMatrix(t *testing.T) {
	frame := []byte{0x81, 0x02, 'o', 'k'}
	interim := []byte("HTTP/1.1 100 Continue\r\nX-Interim: one\r\n\r\n" +
		"HTTP/1.1 103 Early Hints\r\nLink: </style.css>; rel=preload\r\n\r\n" +
		"HTTP/1.1 102 Processing\r\n\r\n")

	tests := []struct {
		name         string
		prelude      []byte
		extraHeaders string
		segments     func(response, frame []byte) [][]byte
		wantBuffered int
	}{
		{
			name:         "chunked header and first frame in one write",
			extraHeaders: "Transfer-Encoding: chunked\r\n",
			segments: func(response, frame []byte) [][]byte {
				return [][]byte{append(bytes.Clone(response), frame...)}
			},
			wantBuffered: 4,
		},
		{
			name:         "content length and first frame in one write",
			extraHeaders: "Content-Length: 4096\r\n",
			segments: func(response, frame []byte) [][]byte {
				return [][]byte{append(bytes.Clone(response), frame...)}
			},
			wantBuffered: 4,
		},
		{
			name:         "proxy headers and first frame in one write",
			extraHeaders: "Via: 1.1 enterprise-proxy\r\nX-Accel-Buffering: no\r\nContent-Type: text/plain\r\n",
			segments: func(response, frame []byte) [][]byte {
				return [][]byte{append(bytes.Clone(response), frame...)}
			},
			wantBuffered: 4,
		},
		{
			name: "first frame arrives after response",
			segments: func(response, frame []byte) [][]byte {
				return [][]byte{bytes.Clone(response), bytes.Clone(frame)}
			},
			wantBuffered: 0,
		},
		{
			name: "partial first frame is buffered with response",
			segments: func(response, frame []byte) [][]byte {
				first := append(bytes.Clone(response), frame[:2]...)
				return [][]byte{first, bytes.Clone(frame[2:])}
			},
			wantBuffered: 2,
		},
		{
			name:         "informational chain then chunked upgrade and first frame",
			prelude:      interim,
			extraHeaders: "Transfer-Encoding: chunked\r\n",
			segments: func(response, frame []byte) [][]byte {
				return [][]byte{append(bytes.Clone(response), frame...)}
			},
			wantBuffered: 4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var listener net.Listener
			var err error
			for range 10 {
				listener, err = net.Listen("tcp4", "127.0.0.1:0")
				if err == nil {
					break
				}
				time.Sleep(time.Millisecond)
			}
			require.NoError(t, err)
			defer listener.Close()

			response := websocketBoundaryResponse(test.extraHeaders)
			serverDone := make(chan struct{})
			go func() {
				defer close(serverDone)
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				defer conn.Close()
				reader := bufio.NewReader(conn)
				for {
					line, readErr := reader.ReadString('\n')
					if readErr != nil || line == "\r\n" {
						break
					}
				}

				segments := test.segments(response, frame)
				segments[0] = append(bytes.Clone(test.prelude), segments[0]...)
				for index, segment := range segments {
					_, _ = conn.Write(segment)
					if index+1 < len(segments) {
						time.Sleep(100 * time.Millisecond)
					}
				}
				time.Sleep(100 * time.Millisecond)
			}()

			req, host, port := websocketBoundaryRequest(t, listener.Addr().String())
			var capturedResponse []byte
			frames := make(chan []byte, 2)
			startedAt := time.Now()
			client, err := NewWebsocketClientByUpgradeRequest(req,
				WithWebsocketHost(host),
				WithWebsocketPort(port),
				WithWebsocketStrictMode(true),
				WithWebsocketUpgradeResponseHandler(func(_ *http.Response, raw []byte, _ *WebsocketExtensions, _ error) []byte {
					capturedResponse = bytes.Clone(raw)
					return raw
				}),
				WithWebsocketAllFrameHandler(func(_ *WebsocketClient, got *Frame, data []byte, _ func()) {
					if got.Type() == TextMessage {
						frames <- bytes.Clone(data)
					}
				}),
			)
			require.NoError(t, err)
			require.Less(t, time.Since(startedAt), time.Second)
			require.Equal(t, response, capturedResponse)
			require.Equal(t, test.wantBuffered, client.BufferedAfterHandshake)
			require.Equal(t, http.NoBody, client.ResponseInstance.Body)

			client.Start()
			select {
			case got := <-frames:
				require.Equal(t, []byte("ok"), got)
			case <-time.After(time.Second):
				t.Fatal("server first frame was not delivered")
			}
			select {
			case duplicate := <-frames:
				t.Fatalf("server first frame was delivered twice: %q", duplicate)
			case <-time.After(30 * time.Millisecond):
			}
			require.NoError(t, client.Close())
			select {
			case <-serverDone:
			case <-time.After(time.Second):
				t.Fatal("test websocket server did not stop")
			}
		})
	}
}
