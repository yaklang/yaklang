package aotlib

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStrIsTLSServerProtocolProbe(t *testing.T) {
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	tlsServer := httptest.NewTLSServer(handler)
	defer tlsServer.Close()
	// This certificate is not trusted by the system pool. The helper must
	// recognize TLS without disabling verification or mistaking HTTP for TLS.
	if ok, err := StrIsTLSServer(strings.TrimPrefix(tlsServer.URL, "https://")); err != nil || !ok {
		t.Fatalf("untrusted TLS peer: tls=%v error=%v", ok, err)
	}
	plain := httptest.NewServer(handler)
	defer plain.Close()
	if ok, err := StrIsTLSServer(strings.TrimPrefix(plain.URL, "http://")); err != nil || ok {
		t.Fatalf("plain HTTP peer: tls=%v error=%v", ok, err)
	}
}
