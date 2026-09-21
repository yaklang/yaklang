package dockerhttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"
)

// newHTTPClient builds an *http.Client that dials the given Docker host.
func newHTTPClient(h *Host, tlsConfig *tls.Config, timeout time.Duration) (*http.Client, *http.Transport, error) {
	tr := &http.Transport{
		TLSClientConfig:     tlsConfig,
		MaxIdleConns:        6,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true, // Docker streams often need raw bodies
	}

	switch h.Proto {
	case "unix":
		addr := h.Addr
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 10 * time.Second}
			return d.DialContext(ctx, "unix", addr)
		}
	case "npipe":
		dial, err := npipeDialContext(h.Addr)
		if err != nil {
			return nil, nil, err
		}
		tr.DialContext = dial
	case "tcp", "http", "https":
		tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	default:
		proto, addr := h.Proto, h.Addr
		tr.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 10 * time.Second}
			return d.DialContext(ctx, proto, addr)
		}
	}

	client := &http.Client{
		Transport:     tr,
		CheckRedirect: checkRedirect,
		Timeout:       timeout, // 0 = no overall timeout; use context
	}
	return client, tr, nil
}

// checkRedirect mirrors the official Docker client's policy: follow GET
// redirects by using the last response; reject non-GET redirects.
func checkRedirect(_ *http.Request, via []*http.Request) error {
	if via[0].Method == http.MethodGet {
		return http.ErrUseLastResponse
	}
	return fmt.Errorf("unexpected redirect in response")
}

// httpScheme returns the URL scheme used in requests (http or https).
func httpScheme(h *Host, tlsConfig *tls.Config) string {
	if h.Proto == "https" || tlsConfig != nil {
		return "https"
	}
	return "http"
}

// requestHost returns the Host header / URL host for the connection.
func requestHost(h *Host) string {
	switch h.Proto {
	case "unix", "npipe":
		return DummyHost
	default:
		return h.Addr
	}
}
