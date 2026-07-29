package utils

import (
	"bytes"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"unicode"
)

func parseHTTPHeaderLineLegacy(line []byte) (key, lowerKey, value string) {
	before, after, _ := bytes.Cut(line, []byte{':'})
	key = string(before)
	value = strings.TrimLeftFunc(string(after), unicode.IsSpace)
	if _, ok := commonHeader[key]; ok {
		key = http.CanonicalHeaderKey(key)
	}
	lowerKey = strings.ToLower(key)
	return
}

func TestParseOwnedHTTPHeaderLineMatchesLegacy(t *testing.T) {
	for _, line := range [][]byte{
		[]byte("Content-Type: application/json"),
		[]byte("content-type:\ttext/plain"),
		[]byte("X-Custom: value"),
		[]byte("X-Empty:"),
		[]byte("No-Colon"),
		[]byte{'X', '-', 0xff, ':', ' ', 0xfe},
	} {
		wantKey, wantLower, wantValue := parseHTTPHeaderLineLegacy(line)
		gotKey, gotLower, gotValue := parseOwnedHTTPHeaderLine(line)
		if gotKey != wantKey || gotLower != wantLower || gotValue != wantValue {
			t.Fatalf(
				"line %q differs: got (%q, %q, %q), want (%q, %q, %q)",
				line,
				gotKey,
				gotLower,
				gotValue,
				wantKey,
				wantLower,
				wantValue,
			)
		}
	}
}

func TestBorrowedHTTPHeaderStringsRemainStable(t *testing.T) {
	requestPacket := []byte(
		"GET /owned HTTP/1.1\r\n" +
			"Host: owned.example.test\r\n" +
			"Content-Type: application/json\r\n" +
			"X-Custom-Header: retained-value\r\n\r\n",
	)
	req, err := ReadHTTPRequestFromBytes(requestPacket)
	if err != nil {
		t.Fatal(err)
	}
	for i := range requestPacket {
		requestPacket[i] = 'x'
	}
	runtime.GC()

	if req.Host != "owned.example.test" {
		t.Fatalf("unexpected host after source mutation: %q", req.Host)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected content type after source mutation: %q", got)
	}
	if got := req.Header.Get("X-Custom-Header"); got != "retained-value" {
		t.Fatalf("unexpected custom header after source mutation: %q", got)
	}

	responsePacket := []byte(
		"HTTP/1.1 200 OK\r\n" +
			"Content-Type: application/json\r\n" +
			"X-Custom-Header: response-retained-value\r\n" +
			"Content-Length: 2\r\n\r\n{}",
	)
	rsp, err := ReadHTTPResponseFromBytes(responsePacket, req)
	if err != nil {
		t.Fatal(err)
	}
	for i := range responsePacket {
		responsePacket[i] = 'x'
	}
	runtime.GC()

	if got := rsp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("unexpected response content type after source mutation: %q", got)
	}
	if got := rsp.Header.Get("X-Custom-Header"); got != "response-retained-value" {
		t.Fatalf("unexpected response custom header after source mutation: %q", got)
	}
}

func FuzzParseOwnedHTTPHeaderLineMatchesLegacy(f *testing.F) {
	for _, seed := range [][]byte{
		[]byte("Content-Length: 42"),
		[]byte("mixed-CASE:\t value "),
		[]byte("No-Colon"),
		[]byte{0xff, ':', ' ', 0xfe},
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, line []byte) {
		if len(line) > 4096 {
			t.Skip()
		}
		wantKey, wantLower, wantValue := parseHTTPHeaderLineLegacy(line)
		gotKey, gotLower, gotValue := parseOwnedHTTPHeaderLine(line)
		if gotKey != wantKey || gotLower != wantLower || gotValue != wantValue {
			t.Fatalf(
				"line %q differs: got (%q, %q, %q), want (%q, %q, %q)",
				line,
				gotKey,
				gotLower,
				gotValue,
				wantKey,
				wantLower,
				wantValue,
			)
		}
	})
}

var benchmarkHTTPHeaderKey string
var benchmarkHTTPHeaderLowerKey string
var benchmarkHTTPHeaderValue string

func BenchmarkParseOwnedHTTPHeaderLine(b *testing.B) {
	lines := [][]byte{
		[]byte("Host: performance.example.test"),
		[]byte("User-Agent: yakit-performance"),
		[]byte("Accept: */*"),
		[]byte("X-Custom-Header: retained-value"),
	}
	for _, benchmark := range []struct {
		name string
		fn   func([]byte) (string, string, string)
	}{
		{name: "copy_strings", fn: parseHTTPHeaderLineLegacy},
		{name: "borrow_owned_line", fn: parseOwnedHTTPHeaderLine},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				key, lowerKey, value := benchmark.fn(lines[i%len(lines)])
				benchmarkHTTPHeaderKey = key
				benchmarkHTTPHeaderLowerKey = lowerKey
				benchmarkHTTPHeaderValue = value
			}
		})
	}
}
