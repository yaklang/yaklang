package minimartian

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

func TestProxyCancelsUpstreamWhenDownstreamCloses(t *testing.T) {
	upstreamStarted := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	releaseUpstream := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(upstreamStarted)
		select {
		case <-request.Context().Done():
			close(upstreamCanceled)
		case <-releaseUpstream:
			writer.WriteHeader(http.StatusOK)
		}
	}))
	defer upstream.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	proxy := NewProxy()
	proxy.SetLowhttpConfig([]lowhttp.LowhttpOpt{
		lowhttp.WithTimeout(5 * time.Second),
		lowhttp.WithConnectTimeout(time.Second),
	})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- proxy.Serve(listener, proxyCtx)
	}()
	defer func() {
		cancelProxy()
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
			t.Error("proxy did not stop")
		}
	}()

	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	parsedUpstream, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(
		client,
		"GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n",
		upstream.URL,
		parsedUpstream.Host,
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-upstreamStarted:
	case <-time.After(2 * time.Second):
		_ = client.Close()
		close(releaseUpstream)
		t.Fatal("upstream request did not start")
	}
	if tcpClient, ok := client.(*net.TCPConn); ok {
		if err := tcpClient.SetLinger(0); err != nil {
			t.Fatal(err)
		}
	}
	_ = client.Close()

	select {
	case <-upstreamCanceled:
		close(releaseUpstream)
	case <-time.After(2 * time.Second):
		close(releaseUpstream)
		t.Fatal("upstream request was not canceled after downstream close")
	}
}

func TestProxyDoesNotCancelUpstreamWhenDownstreamHalfClosesWrite(t *testing.T) {
	upstreamStarted := make(chan struct{})
	upstreamCanceled := make(chan struct{})
	releaseUpstream := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		close(upstreamStarted)
		select {
		case <-request.Context().Done():
			close(upstreamCanceled)
		case <-releaseUpstream:
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte("ok"))
		}
	}))
	defer upstream.Close()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	proxy := NewProxy()
	proxy.SetLowhttpConfig([]lowhttp.LowhttpOpt{
		lowhttp.WithTimeout(5 * time.Second),
		lowhttp.WithConnectTimeout(time.Second),
	})
	serveDone := make(chan error, 1)
	go func() {
		serveDone <- proxy.Serve(listener, proxyCtx)
	}()
	defer func() {
		cancelProxy()
		_ = listener.Close()
		select {
		case <-serveDone:
		case <-time.After(2 * time.Second):
			t.Error("proxy did not stop")
		}
	}()

	rawClient, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	client := rawClient.(*net.TCPConn)
	defer client.Close()
	parsedUpstream, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(
		client,
		"GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n",
		upstream.URL,
		parsedUpstream.Host,
	); err != nil {
		t.Fatal(err)
	}

	select {
	case <-upstreamStarted:
	case <-time.After(2 * time.Second):
		close(releaseUpstream)
		t.Fatal("upstream request did not start")
	}
	if err := client.CloseWrite(); err != nil {
		close(releaseUpstream)
		t.Fatal(err)
	}

	select {
	case <-upstreamCanceled:
		close(releaseUpstream)
		t.Fatal("half-closing the request direction canceled a valid upstream request")
	case <-time.After(3 * downstreamDisconnectPollInterval):
	}
	close(releaseUpstream)

	response, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatalf("read response after CloseWrite: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("response status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestBindHTTPRequestToDownstreamPreservesBufferedPipeline(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	reader := bufio.NewReader(server)
	pipelined := "GET /next HTTP/1.1\r\nHost: example.com\r\n\r\n"
	writeDone := make(chan struct{})
	go func() {
		_, _ = client.Write([]byte(pipelined))
		close(writeDone)
	}()
	buffered, err := reader.Peek(len(pipelined))
	if err != nil {
		t.Fatal(err)
	}
	<-writeDone

	request := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	stopWatching := bindHTTPRequestToDownstream(request, server, reader)

	select {
	case <-request.Context().Done():
		t.Fatal("buffered pipelined data was treated as downstream abandonment")
	case <-time.After(3 * downstreamDisconnectPollInterval):
	}
	if string(buffered) != pipelined || !strings.Contains(string(buffered), "/next") {
		t.Fatal("pipelined request bytes changed")
	}
	stopWatching()
	select {
	case <-request.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("stopping downstream watcher did not release request context")
	}
}
