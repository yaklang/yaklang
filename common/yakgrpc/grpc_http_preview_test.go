package yakgrpc

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestHTTPPreviewResources(t *testing.T) {
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4z8DwHwAFgAI/ScLbtAAAAABJRU5ErkJggg==")
	var credentials atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" || r.Method != "GET" {
			credentials.Store(true)
		}
		switch r.URL.Path {
		case "/image":
			w.Header().Set("Content-Type", "image/png")
			w.Write(png)
		case "/css":
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte("body{color:red}"))
		case "/font":
			w.Write([]byte("wOF2font-test"))
		case "/redirect":
			http.Redirect(w, r, "/image", 302)
		case "/external":
			http.Redirect(w, r, "http://127.0.0.1:1/internal", 302)
		case "/big":
			w.Write(bytes.Repeat([]byte("a"), previewResourceLimit+100))
		case "/gzip":
			w.Header().Set("Content-Encoding", "gzip")
			zipped := gzip.NewWriter(w)
			zipped.Write(png)
			zipped.Close()
		case "/bomb":
			w.Header().Set("Content-Encoding", "gzip")
			zipped := gzip.NewWriter(w)
			zipped.Write(bytes.Repeat([]byte("a"), previewResourceLimit+100))
			zipped.Close()
		default:
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<script>alert(1)</script>"))
		}
	}))
	defer server.Close()
	s := &Server{}
	fetch := func(path, kind string) (*ypb.HTTPPreviewResourceResponse, error) {
		return s.FetchHTTPPreviewResource(context.Background(), &ypb.HTTPPreviewResourceRequest{URL: server.URL + path, PageURL: server.URL + "/page", Kind: kind, Network: &ypb.FuzzerRequest{NoSystemProxy: true}})
	}
	for _, tc := range []struct{ path, kind, mime string }{{"/image", "image", "image/png"}, {"/css", "css", "text/css"}, {"/font", "font", "font/woff2"}, {"/redirect", "image", "image/png"}, {"/gzip", "image", "image/png"}} {
		t.Run(tc.path, func(t *testing.T) {
			result, err := fetch(tc.path, tc.kind)
			require.NoError(t, err)
			require.Equal(t, tc.mime, result.MimeType)
			require.NotEmpty(t, result.Data)
		})
	}
	for _, tc := range []struct{ path, kind string }{{"/external", "image"}, {"/big", "image"}, {"/bomb", "image"}, {"/html", "image"}, {"/image", "script"}} {
		t.Run(tc.path, func(t *testing.T) { _, err := fetch(tc.path, tc.kind); require.Error(t, err) })
	}
	require.False(t, credentials.Load())
	// Same-origin intranet testing is supported with the original transport mapping.
	target := strings.Replace(server.URL, "127.0.0.1", "preview.invalid", 1)
	result, err := s.FetchHTTPPreviewResource(context.Background(), &ypb.HTTPPreviewResourceRequest{URL: target + "/image", PageURL: target, Kind: "image", Network: &ypb.FuzzerRequest{NoSystemProxy: true, EtcHosts: []*ypb.KVPair{{Key: "preview.invalid", Value: "127.0.0.1"}}}})
	require.NoError(t, err)
	require.Equal(t, png, result.Data)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err = s.FetchHTTPPreviewResource(ctx, &ypb.HTTPPreviewResourceRequest{URL: server.URL + "/image", PageURL: server.URL, Kind: "image"})
	require.Error(t, err)
	require.Less(t, time.Since(start), time.Second)
}

func TestHTTPPreviewScopeAndTypes(t *testing.T) {
	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "http://user:password@host/a", "data:image/png;base64,aaa"} {
		_, err := previewURL(raw)
		require.Error(t, err)
	}
	for _, ip := range []string{"127.0.0.1", "::1", "::ffff:127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "fc00::1", "fe80::1"} {
		require.False(t, previewPublicIP(ip), ip)
	}
	require.True(t, previewPublicIP("8.8.8.8"))
	for _, kind := range []string{"image", "font", "css"} {
		_, err := previewMime(kind, "text/html", []byte("<script>alert(1)</script>"))
		require.Error(t, err)
	}
	_, err := previewMime("image", "image/svg+xml", []byte("<svg/>"))
	require.Error(t, err)
}

func TestHTTPPreviewCancelAndProxy(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			w.WriteHeader(200)
			w.(http.Flusher).Flush()
			close(started)
			<-release
			return
		}
		w.Header().Set("Content-Type", "text/css")
		w.Write([]byte("body{color:red}"))
	}))
	defer origin.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := (&Server{}).FetchHTTPPreviewResource(ctx, &ypb.HTTPPreviewResourceRequest{URL: origin.URL + "/slow", PageURL: origin.URL, Kind: "css", Network: &ypb.FuzzerRequest{NoSystemProxy: true}})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Error("resource request did not cancel promptly")
	}
	var proxied atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "CONNECT", r.Method)
		proxied.Add(1)
		target, err := net.Dial("tcp", r.Host)
		if err != nil {
			http.Error(w, "bad gateway", 502)
			return
		}
		defer target.Close()
		conn, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		buffer.Flush()
		go io.Copy(target, buffer)
		io.Copy(conn, target)
	}))
	defer proxy.Close()
	result, err := (&Server{}).FetchHTTPPreviewResource(context.Background(), &ypb.HTTPPreviewResourceRequest{URL: origin.URL + "/css", PageURL: origin.URL, Kind: "css", Network: &ypb.FuzzerRequest{Proxy: proxy.URL, NoSystemProxy: true}})
	require.NoError(t, err)
	require.Equal(t, "text/css", result.MimeType)
	require.EqualValues(t, 1, proxied.Load())
	// The new descriptor and generated client/server registration agree over gRPC.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	rpc := grpc.NewServer()
	ypb.RegisterYakServer(rpc, &Server{})
	go rpc.Serve(listener)
	defer rpc.Stop()
	connection, err := grpc.Dial(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer connection.Close()
	response, err := ypb.NewYakClient(connection).FetchHTTPPreviewResource(context.Background(), &ypb.HTTPPreviewResourceRequest{URL: origin.URL + "/css", PageURL: origin.URL, Kind: "css", Network: &ypb.FuzzerRequest{NoSystemProxy: true}})
	require.NoError(t, err)
	require.Equal(t, []byte("body{color:red}"), response.Data)
}
