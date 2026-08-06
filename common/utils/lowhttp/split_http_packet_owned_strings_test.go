package lowhttp

import (
	"runtime"
	"testing"

	"github.com/yaklang/yaklang/common/utils"
)

func TestSplitHTTPPacketOwnedStringsRemainStable(t *testing.T) {
	packet := []byte(
		"POST /owned?q=1 HTTP/1.1\r\n" +
			"Host: owned.example.test\r\n" +
			"X-Custom-Header: retained-value\r\n" +
			"Content-Length: 0\r\n\r\n",
	)
	var rawFirstLine string
	var method string
	var requestURI string
	var proto string
	var headerLines []string
	headers, _ := SplitHTTPPacketEx(
		packet,
		func(gotMethod, gotRequestURI, gotProto string) error {
			method, requestURI, proto = gotMethod, gotRequestURI, gotProto
			return nil
		},
		nil,
		func(line string) error {
			rawFirstLine = line
			return nil
		},
		func(line string) string {
			headerLines = append(headerLines, line)
			return line
		},
	)

	for index := range packet {
		packet[index] = 'x'
	}
	runtime.GC()

	if rawFirstLine != "POST /owned?q=1 HTTP/1.1" {
		t.Fatalf("unexpected retained first line: %q", rawFirstLine)
	}
	if method != "POST" || requestURI != "/owned?q=1" || proto != "HTTP/1.1" {
		t.Fatalf("unexpected retained request line parts: %q %q %q", method, requestURI, proto)
	}
	if len(headerLines) != 3 ||
		headerLines[0] != "Host: owned.example.test" ||
		headerLines[1] != "X-Custom-Header: retained-value" ||
		headerLines[2] != "Content-Length: 0" {
		t.Fatalf("unexpected retained header lines: %#v", headerLines)
	}
	if headers !=
		"POST /owned?q=1 HTTP/1.1\r\n"+
			"Host: owned.example.test\r\n"+
			"X-Custom-Header: retained-value\r\n"+
			"Content-Length: 0\r\n\r\n" {
		t.Fatalf("unexpected reconstructed headers: %q", headers)
	}
}

var benchmarkOwnedSplitHTTPLine string

func BenchmarkOwnedSplitHTTPLineString(b *testing.B) {
	line := []byte("X-Custom-Header: retained-value")
	b.Run("copy_string", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			benchmarkOwnedSplitHTTPLine = string(line)
		}
	})
	b.Run("borrow_owned_line", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			benchmarkOwnedSplitHTTPLine = utils.UnsafeBytesToString(line)
		}
	})
}
