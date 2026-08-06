package crep

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var benchmarkHijackedRequest *http.Request

func parseHijackedRequestTestPacket(tb testing.TB, packet []byte) *http.Request {
	tb.Helper()
	req, err := lowhttp.ParseBytesToHttpRequest(packet)
	if err != nil {
		tb.Fatal(err)
	}
	return req
}

func TestHTTPRequestHijackModificationContract(t *testing.T) {
	packet := []byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n")
	req := parseHijackedRequestTestPacket(t, packet)
	server := &MITMServer{}

	legacy := MITM_SetHTTPRequestHijackRaw(func(bool, *http.Request, []byte) []byte {
		return packet
	})
	if err := legacy(server); err != nil {
		t.Fatal(err)
	}
	if !server.hasHTTPRequestHijackHandler() {
		t.Fatal("legacy request hijacker was not registered")
	}
	if result, modified := server.callHTTPRequestHijackHandler(false, req, packet); !samePacketView(result, packet) || !modified {
		t.Fatalf("legacy result = (%p, %v), want original packet and conservative modified=true", result, modified)
	}

	aware := MITM_SetHTTPRequestHijackRawWithModification(func(bool, *http.Request, []byte) ([]byte, bool) {
		return packet, false
	})
	if err := aware(server); err != nil {
		t.Fatal(err)
	}
	if server.requestHijackHandler != nil {
		t.Fatal("modification-aware option did not replace the legacy callback")
	}
	if result, modified := server.callHTTPRequestHijackHandler(false, req, packet); !samePacketView(result, packet) || modified {
		t.Fatalf("aware result = (%p, %v), want original packet and modified=false", result, modified)
	}

	if err := legacy(server); err != nil {
		t.Fatal(err)
	}
	if server.requestHijackHandlerWithModification != nil {
		t.Fatal("legacy option did not replace the modification-aware callback")
	}
}

func TestApplyHijackedRequestResultSkipsUnmodifiedOriginalPacket(t *testing.T) {
	packet := []byte("POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: 3\r\n\r\nyak")
	req := parseHijackedRequestTestPacket(t, packet)
	originalURL := req.URL
	originalBody := req.Body

	err := applyHijackedRequestResult(req, false, 80, true, packet, packet, false)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != originalURL || req.Body != originalBody {
		t.Fatal("unmodified request was reparsed")
	}
}

func TestApplyHijackedRequestResultPreservesLegacyAndMislabelledResults(t *testing.T) {
	tests := []struct {
		name     string
		modified bool
		result   func([]byte) []byte
	}{
		{
			name:     "explicitly modified same packet",
			modified: true,
			result: func(packet []byte) []byte {
				return packet
			},
		},
		{
			name:     "independent result labelled unmodified",
			modified: false,
			result: func(packet []byte) []byte {
				return bytes.Clone(packet)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			original := []byte("GET /original HTTP/1.1\r\nHost: example.com\r\n\r\n")
			packet := []byte("GET /changed HTTP/1.1\r\nHost: example.com\r\n\r\n")
			req := parseHijackedRequestTestPacket(t, original)

			err := applyHijackedRequestResult(req, false, 80, true, packet, test.result(packet), test.modified)
			if err != nil {
				t.Fatal(err)
			}
			if req.URL.Path != "/changed" {
				t.Fatalf("parsed path = %q, want /changed", req.URL.Path)
			}
		})
	}
}

func BenchmarkApplyHijackedRequestResult(b *testing.B) {
	const bodySize = 256 * 1024
	body := bytes.Repeat([]byte("r"), bodySize)
	packet := append(
		[]byte(fmt.Sprintf(
			"POST /upload HTTP/1.1\r\nHost: example.com\r\nContent-Length: %d\r\n\r\n",
			len(body),
		)),
		body...,
	)

	b.Run("legacy_parse", func(b *testing.B) {
		req := parseHijackedRequestTestPacket(b, packet)
		b.ReportAllocs()
		b.SetBytes(bodySize)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := applyHijackedRequestResult(req, false, 80, true, packet, packet, true); err != nil {
				b.Fatal(err)
			}
			benchmarkHijackedRequest = req
		}
	})

	b.Run("explicitly_unmodified", func(b *testing.B) {
		req := parseHijackedRequestTestPacket(b, packet)
		b.ReportAllocs()
		b.SetBytes(bodySize)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := applyHijackedRequestResult(req, false, 80, true, packet, packet, false); err != nil {
				b.Fatal(err)
			}
			benchmarkHijackedRequest = req
		}
	})
}
