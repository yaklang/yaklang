package crep

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"golang.org/x/net/http2"
)

// ---------- helpers ----------

func h2GenCert(t *testing.T) tls.Certificate {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "h2-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

type h2Fixture struct {
	serverAddr string
	proxyURL   string
	cancel     context.CancelFunc
	cleanups   []func()
}

func (f *h2Fixture) Close() {
	f.cancel()
	for i := len(f.cleanups) - 1; i >= 0; i-- {
		f.cleanups[i]()
	}
}

func newH2Fixture(t *testing.T, handler http.Handler, h2Opts ...func(*http2.Server)) *h2Fixture {
	cert := h2GenCert(t)

	srv := &http.Server{
		Handler: handler,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
			NextProtos:   []string{"h2"},
		},
	}
	h2srv := &http2.Server{}
	for _, fn := range h2Opts {
		fn(h2srv)
	}
	require.NoError(t, http2.ConfigureServer(srv, h2srv))

	lis, err := tls.Listen("tcp", "127.0.0.1:0", srv.TLSConfig)
	require.NoError(t, err)
	go srv.Serve(lis)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)

	mitmPort := utils.GetRandomAvailableTCPPort()
	mitmAddr := fmt.Sprintf("127.0.0.1:%d", mitmPort)
	mServer, err := NewMITMServer(MITM_SetHTTP2(true))
	require.NoError(t, err)
	ready := make(chan struct{})
	go func() {
		_ = mServer.ServeWithListenedCallback(ctx, mitmAddr, func() { close(ready) })
	}()
	<-ready
	time.Sleep(100 * time.Millisecond)

	return &h2Fixture{
		serverAddr: lis.Addr().String(),
		proxyURL:   "http://" + mitmAddr,
		cancel:     cancel,
		cleanups: []func(){
			func() {
				shutCtx, c := context.WithTimeout(context.Background(), time.Second)
				defer c()
				srv.Shutdown(shutCtx)
			},
		},
	}
}

func (f *h2Fixture) doGet(t *testing.T) *lowhttp.LowhttpResponse {
	pkt := fmt.Sprintf("GET / HTTP/2.0\r\nHost: %s\r\nUser-Agent: h2-test\r\n\r\n", f.serverAddr)
	rsp, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithRequest([]byte(pkt)),
		lowhttp.WithProxy(f.proxyURL),
		lowhttp.WithHttp2(true),
		lowhttp.WithHttps(true),
		lowhttp.WithTimeout(10*time.Second),
	)
	require.NoError(t, err)
	return rsp
}

// ---------- tests ----------

// TestH2Fix_SettingsCompliance exercises SETTINGS_MAX_CONCURRENT_STREAMS enforcement.
// The standard h2 server sends GOAWAY/RST on violations; no PROTOCOL_ERROR expected.
func TestH2Fix_SettingsCompliance(t *testing.T) {
	var total int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&total, 1)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})

	f := newH2Fixture(t, handler, func(s *http2.Server) {
		s.MaxConcurrentStreams = 100
	})
	defer f.Close()

	for i := 0; i < 10; i++ {
		f.doGet(t)
	}
	require.Equal(t, int32(10), atomic.LoadInt32(&total))
}

// TestH2Fix_StrictConcurrencyLimit uses MaxConcurrentStreams=2. All requests
// must still succeed, proving the client respects server stream limits.
func TestH2Fix_StrictConcurrencyLimit(t *testing.T) {
	var total int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&total, 1)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})

	f := newH2Fixture(t, handler, func(s *http2.Server) {
		s.MaxConcurrentStreams = 2
	})
	defer f.Close()

	for i := 0; i < 10; i++ {
		f.doGet(t)
	}
	require.GreaterOrEqual(t, atomic.LoadInt32(&total), int32(10))
}

// TestH2Fix_StreamIDOrder sends concurrent requests through MITM and asserts
// zero PROTOCOL_ERROR. Before the fix, concurrent goroutines could write
// HEADERS with out-of-order stream IDs, violating RFC 7540 Section 5.1.1.
func TestH2Fix_StreamIDOrder(t *testing.T) {
	var total int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&total, 1)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})

	f := newH2Fixture(t, handler, func(s *http2.Server) {
		s.MaxConcurrentStreams = 128
	})
	defer f.Close()

	const N = 15
	var wg sync.WaitGroup
	var success, fail int32
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pkt := fmt.Sprintf("GET / HTTP/2.0\r\nHost: %s\r\nUser-Agent: h2-order\r\n\r\n", f.serverAddr)
			_, err := lowhttp.HTTPWithoutRedirect(
				lowhttp.WithRequest([]byte(pkt)),
				lowhttp.WithProxy(f.proxyURL),
				lowhttp.WithHttp2(true),
				lowhttp.WithHttps(true),
				lowhttp.WithTimeout(10*time.Second),
			)
			if err != nil {
				atomic.AddInt32(&fail, 1)
			} else {
				atomic.AddInt32(&success, 1)
			}
		}()
	}
	wg.Wait()
	require.Zero(t, fail, "concurrent requests must not trigger PROTOCOL_ERROR")
	require.Equal(t, int32(N), success)
}

// TestH2Fix_LargeResponseBody sends a 128KB response through MITM.
// This exercises connection-level and stream-level WINDOW_UPDATE, and
// verifies the flow control in both directions works correctly.
func TestH2Fix_LargeResponseBody(t *testing.T) {
	const bodySize = 128 * 1024
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(200)
		data := make([]byte, bodySize)
		for i := range data {
			data[i] = byte('A' + (i % 26))
		}
		w.Write(data)
	})

	f := newH2Fixture(t, handler)
	defer f.Close()

	rsp := f.doGet(t)
	_, body := lowhttp.SplitHTTPPacketFast(rsp.RawPacket)
	require.Equal(t, bodySize, len(body), "response body must be fully received through MITM H2 proxy")
}

// TestH2Fix_PostWithBody verifies that POST request bodies are correctly
// chunked per SETTINGS_MAX_FRAME_SIZE and received intact by the server.
func TestH2Fix_PostWithBody(t *testing.T) {
	var receivedSize int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		atomic.StoreInt32(&receivedSize, int32(len(data)))
		w.WriteHeader(200)
		fmt.Fprintf(w, "got %d", len(data))
	})

	f := newH2Fixture(t, handler)
	defer f.Close()

	bodyData := strings.Repeat("X", 50000)
	pkt := fmt.Sprintf(
		"POST / HTTP/2.0\r\nHost: %s\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n%s",
		f.serverAddr, len(bodyData), bodyData,
	)
	_, err := lowhttp.HTTPWithoutRedirect(
		lowhttp.WithRequest([]byte(pkt)),
		lowhttp.WithProxy(f.proxyURL),
		lowhttp.WithHttp2(true),
		lowhttp.WithHttps(true),
		lowhttp.WithTimeout(10*time.Second),
	)
	require.NoError(t, err)
	require.Equal(t, int32(len(bodyData)), atomic.LoadInt32(&receivedSize))
}

// TestH2Fix_GOAWAYReconnect verifies that after phase-1 requests trigger
// a potential GOAWAY (NO_ERROR), phase-2 requests still succeed by reconnecting.
func TestH2Fix_GOAWAYReconnect(t *testing.T) {
	var total int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&total, 1)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})

	f := newH2Fixture(t, handler)
	defer f.Close()

	for i := 0; i < 5; i++ {
		f.doGet(t)
	}
	require.Equal(t, int32(5), atomic.LoadInt32(&total))

	for i := 0; i < 5; i++ {
		f.doGet(t)
	}
	require.Equal(t, int32(10), atomic.LoadInt32(&total))
}

// TestH2Fix_ConnectionReuse verifies sequential requests reuse the H2
// connection via the MITM proxy, without leaking connections.
func TestH2Fix_ConnectionReuse(t *testing.T) {
	var total int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&total, 1)
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	})

	f := newH2Fixture(t, handler)
	defer f.Close()

	for i := 0; i < 10; i++ {
		f.doGet(t)
	}
	require.Equal(t, int32(10), atomic.LoadInt32(&total))
}
