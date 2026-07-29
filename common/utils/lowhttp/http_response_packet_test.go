package lowhttp

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"strings"
	"testing"
)

var (
	benchmarkFixedHTTPResponse     []byte
	benchmarkFixedHTTPResponseBody []byte
	benchmarkFixedHTTPResponseErr  error
	benchmarkHTTPPacketBodyLength  int
	benchmarkFixedHTTPHeaderState  fixedHTTPResponseHeaderState
)

func legacyFixedHTTPResponseHeaderState(lines []string) fixedHTTPResponseHeaderState {
	state := fixedHTTPResponseHeaderState{noContentType: true}
	for _, line := range lines {
		if strings.HasPrefix(strings.ToLower(line), "content-type:") {
			_, state.contentType = SplitHTTPHeader(line)
			state.noContentType = false
		}
		line = strings.ToLower(line)
		if strings.HasPrefix(line, "transfer-encoding:") && strings.Contains(line, "chunked") {
			state.isChunked = true
		}
		if strings.HasPrefix(line, "content-encoding:") {
			state.contentEncoding = line
		}
	}
	return state
}

func TestFixedHTTPResponseHeaderStateMatchesLegacy(t *testing.T) {
	for _, lines := range [][]string{
		{"Content-Type: text/plain", "Content-Encoding: GZIP", "Transfer-Encoding: Chunked"},
		{"content-type: application/json", "content-encoding: br", "transfer-encoding: gzip, chunked"},
		{"cOnTeNt-TyPe: text/html; charset=GBK", "CoNtEnT-EnCoDiNg: ZSTD", "TrAnSfEr-EnCoDiNg: CHUNKED"},
		{"Content-Type : text/plain", "Content-Encoding : gzip", "Transfer-Encoding : chunked"},
		{"X-Content-Type: text/plain", "X-Transfer-Encoding: chunked", "Server: example"},
		{"", "Date: today", "X-Unicode: \u212a"},
	} {
		want := legacyFixedHTTPResponseHeaderState(lines)
		got := fixedHTTPResponseHeaderState{noContentType: true}
		for _, line := range lines {
			got.parse(line)
		}
		if got != want {
			t.Fatalf("header state mismatch for %#v: got %#v want %#v", lines, got, want)
		}
	}
}

func FuzzFixedHTTPResponseHeaderStateMatchesLegacy(f *testing.F) {
	for _, line := range []string{
		"Content-Type: text/plain",
		"Content-Encoding: GZIP",
		"Transfer-Encoding: Chunked",
		"Content-Type : text/plain",
		"X-Unicode: \u212a",
		"",
	} {
		f.Add(line)
	}

	f.Fuzz(func(t *testing.T, line string) {
		if len(line) > 64*1024 {
			t.Skip()
		}
		want := legacyFixedHTTPResponseHeaderState([]string{line})
		got := fixedHTTPResponseHeaderState{noContentType: true}
		got.parse(line)
		if got != want {
			t.Fatalf("header state mismatch for %q: got %#v want %#v", line, got, want)
		}
	})
}

func TestFixHTTPResponseMixedCaseEncodingHeaders(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte("mixed-case payload")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	packet := append([]byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\ncOnTeNt-TyPe: text/plain\r\nCoNtEnT-EnCoDiNg: GZIP\r\nContent-Length: %d\r\n\r\n",
		compressed.Len(),
	)), compressed.Bytes()...)

	fixed, err := FixHTTPResponsePacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	_, body := SplitHTTPHeadersAndBodyFromPacketView(fixed)
	if string(body) != "mixed-case payload" {
		t.Fatalf("mixed-case content encoding was not decoded: %q", body)
	}

	chunked := []byte("HTTP/1.1 200 OK\r\nTrAnSfEr-EnCoDiNg: ChUnKeD\r\n\r\n5\r\nhello\r\n0\r\n\r\n")
	fixed, err = FixHTTPResponsePacket(chunked)
	if err != nil {
		t.Fatal(err)
	}
	_, body = SplitHTTPHeadersAndBodyFromPacketView(fixed)
	if string(body) != "hello" {
		t.Fatalf("mixed-case transfer encoding was not decoded: %q", body)
	}
}

func TestFixHTTPResponsePacketMatchesLegacy(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write([]byte("compressed payload")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	gzipPacket := append([]byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n",
		compressed.Len(),
	)), compressed.Bytes()...)

	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "plain binary",
			raw:  []byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 7\r\n\r\npayload"),
		},
		{
			name: "gzip",
			raw:  gzipPacket,
		},
		{
			name: "chunked",
			raw:  []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nTransfer-Encoding: chunked\r\n\r\n5\r\nhello\r\n0\r\n\r\n"),
		},
		{
			name: "malformed chunked",
			raw: append(
				[]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nTransfer-Encoding: chunked\r\n\r\n"),
				bytes.Repeat([]byte("not-a-chunk"), 24)...,
			),
		},
		{
			name: "continue prefix",
			raw:  []byte("HTTP/1.1 100 Continue\r\n\r\nHTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 2\r\n\r\nok"),
		},
		{
			name: "empty",
			raw:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			legacyInput := bytes.Clone(tt.raw)
			want, _, wantErr := FixHTTPResponse(legacyInput)
			input := bytes.Clone(tt.raw)
			before := bytes.Clone(input)
			got, gotErr := FixHTTPResponsePacket(input)

			if (gotErr != nil) != (wantErr != nil) {
				t.Fatalf("error mismatch: packet-only=%v legacy=%v", gotErr, wantErr)
			}
			if gotErr != nil && gotErr.Error() != wantErr.Error() {
				t.Fatalf("error text mismatch: packet-only=%q legacy=%q", gotErr, wantErr)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("response mismatch:\npacket-only=%q\nlegacy=%q", got, want)
			}
			if !bytes.Equal(input, before) {
				t.Fatal("packet-only fixer modified its input")
			}

			gotSnapshot := bytes.Clone(got)
			if len(input) > 0 {
				input[len(input)-1] ^= 0xff
			}
			if !bytes.Equal(got, gotSnapshot) {
				t.Fatal("packet-only result aliases its input")
			}
		})
	}
}

func TestFixHTTPResponseLegacyBodyOwnership(t *testing.T) {
	input := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 7\r\n\r\npayload")
	fixed, body, err := FixHTTPResponse(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "payload" {
		t.Fatalf("unexpected body: %q", body)
	}

	input[len(input)-1] = 'X'
	if string(body) != "payload" {
		t.Fatal("legacy body aliases input")
	}
	_, fixedBody := SplitHTTPHeadersAndBodyFromPacketView(fixed)
	fixedBody[0] = 'P'
	if string(body) != "payload" {
		t.Fatal("legacy body aliases rebuilt packet")
	}
}

func TestParseBytesToHTTPResponseDoesNotRetainInput(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload")
	rsp, err := ParseBytesToHTTPResponse(packet)
	if err != nil {
		t.Fatal(err)
	}

	for i := range packet {
		packet[i] = 'x'
	}
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if rsp.StatusCode != 200 || rsp.Header.Get("Content-Type") != "text/plain" || string(body) != "payload" {
		t.Fatalf("parsed response retained input storage: status=%d content-type=%q body=%q", rsp.StatusCode, rsp.Header.Get("Content-Type"), body)
	}
}

func BenchmarkFixHTTPResponse256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	b.SetBytes(int64(len(packet)))

	b.Run("LegacyWithBody", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkFixedHTTPResponse, benchmarkFixedHTTPResponseBody, benchmarkFixedHTTPResponseErr = FixHTTPResponse(packet)
			if benchmarkFixedHTTPResponseErr != nil || len(benchmarkFixedHTTPResponseBody) != len(body) {
				b.Fatal("unexpected legacy result")
			}
		}
	})

	b.Run("PacketOnly", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkFixedHTTPResponse, benchmarkFixedHTTPResponseErr = FixHTTPResponsePacket(packet)
			if benchmarkFixedHTTPResponseErr != nil || len(benchmarkFixedHTTPResponse) == 0 {
				b.Fatal("unexpected packet-only result")
			}
		}
	})
}

func BenchmarkFixHTTPResponseHeaderParsing(b *testing.B) {
	lines := []string{
		"Date: today",
		"Content-Type: application/json; charset=UTF-8",
		"Server: example",
		"Content-Encoding: GZIP",
		"Content-Length: 262144",
		"Connection: keep-alive",
	}

	b.Run("LegacyLowercase", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkFixedHTTPHeaderState = legacyFixedHTTPResponseHeaderState(lines)
		}
	})

	b.Run("CaseFoldPrefix", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			state := fixedHTTPResponseHeaderState{noContentType: true}
			for _, line := range lines {
				state.parse(line)
			}
			benchmarkFixedHTTPHeaderState = state
		}
	})
}

func BenchmarkHTTPPacketBodyLength256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	b.SetBytes(int64(len(packet)))

	b.Run("LegacyClone", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkHTTPPacketBodyLength = len(GetHTTPPacketBody(packet))
			if benchmarkHTTPPacketBodyLength != len(body) {
				b.Fatal("unexpected body length")
			}
		}
	})

	b.Run("ReadOnlyView", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, bodyView := SplitHTTPHeadersAndBodyFromPacketView(packet)
			benchmarkHTTPPacketBodyLength = len(bodyView)
			if benchmarkHTTPPacketBodyLength != len(body) {
				b.Fatal("unexpected body length")
			}
		}
	})
}
