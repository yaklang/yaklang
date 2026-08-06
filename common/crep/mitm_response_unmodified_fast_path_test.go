package crep

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

var (
	benchmarkHijackedResponsePacket []byte
	benchmarkHijackedResponse       *http.Response
)

func TestHTTPResponseHijackModificationContract(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nyak")
	req := &http.Request{}
	rsp := &http.Response{Request: req}

	server := &MITMServer{}
	legacy := MITM_SetHTTPResponseHijackRaw(func(
		bool,
		*http.Request,
		*http.Response,
		[]byte,
		string,
	) []byte {
		return packet
	})
	if err := legacy(server); err != nil {
		t.Fatal(err)
	}
	if !server.hasHTTPResponseHijackHandler() {
		t.Fatal("legacy response hijacker was not registered")
	}
	if result, modified := server.callHTTPResponseHijackHandler(false, req, rsp, packet, ""); !samePacketView(result, packet) || !modified {
		t.Fatalf("legacy result = (%p, %v), want original packet and conservative modified=true", result, modified)
	}

	aware := MITM_SetHTTPResponseHijackRawWithModification(func(
		bool,
		*http.Request,
		*http.Response,
		[]byte,
		string,
	) ([]byte, bool) {
		return packet, false
	})
	if err := aware(server); err != nil {
		t.Fatal(err)
	}
	if server.responseHijackHandler != nil {
		t.Fatal("modification-aware option did not replace the legacy callback")
	}
	if result, modified := server.callHTTPResponseHijackHandler(false, req, rsp, packet, ""); !samePacketView(result, packet) || modified {
		t.Fatalf("aware result = (%p, %v), want original packet and modified=false", result, modified)
	}
}

func TestApplyHijackedResponseResultSkipsUnmodifiedOriginalPacket(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nyak")
	req := &http.Request{}
	originalBody := io.NopCloser(strings.NewReader("yak"))
	rsp := &http.Response{
		StatusCode: 200,
		Request:    req,
		Body:       originalBody,
	}

	result, err := applyHijackedResponseResult(req, rsp, packet, packet, false)
	if err != nil {
		t.Fatal(err)
	}
	if !samePacketView(result, packet) {
		t.Fatal("unmodified packet was copied")
	}
	if rsp.Body != originalBody || rsp.StatusCode != 200 {
		t.Fatal("unmodified response was reparsed")
	}
}

func TestApplyHijackedResponseResultPreservesLegacyAndMislabelledResults(t *testing.T) {
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
			packet := []byte("HTTP/1.1 201 Created\r\nContent-Length: 3\r\n\r\nyak")
			req := &http.Request{}
			rsp := &http.Response{StatusCode: 200, Request: req, Body: http.NoBody}
			hijacked := test.result(packet)

			owned, err := applyHijackedResponseResult(req, rsp, packet, hijacked, test.modified)
			if err != nil {
				t.Fatal(err)
			}
			if samePacketView(owned, hijacked) {
				t.Fatal("modified or independently owned result was not snapshotted")
			}
			hijacked[len(hijacked)-3] = 'X'
			body, err := io.ReadAll(rsp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if rsp.StatusCode != http.StatusCreated || string(body) != "yak" {
				t.Fatalf("parsed response = (%d, %q), want (201, yak)", rsp.StatusCode, body)
			}
		})
	}
}

func BenchmarkApplyHijackedResponseResult(b *testing.B) {
	const bodySize = 256 * 1024
	body := bytes.Repeat([]byte("y"), bodySize)
	header := []byte("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(bodySize) + "\r\n\r\n")
	packet := make([]byte, 0, len(header)+len(body))
	packet = append(packet, header...)
	packet = append(packet, body...)
	req := &http.Request{}

	b.Run("legacy_snapshot_and_parse", func(b *testing.B) {
		b.ReportAllocs()
		rsp := &http.Response{Request: req}
		for i := 0; i < b.N; i++ {
			var err error
			benchmarkHijackedResponsePacket, err = applyHijackedResponseResult(req, rsp, packet, packet, true)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkHijackedResponse = rsp
		}
	})

	b.Run("explicitly_unmodified", func(b *testing.B) {
		b.ReportAllocs()
		rsp := &http.Response{Request: req}
		for i := 0; i < b.N; i++ {
			var err error
			benchmarkHijackedResponsePacket, err = applyHijackedResponseResult(req, rsp, packet, packet, false)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkHijackedResponse = rsp
		}
	})
}
