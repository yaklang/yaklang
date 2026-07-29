package schema

import (
	"bytes"
	"strings"
	"testing"
)

func TestExtractHTTPFlowHTMLTitleBytesMatchesString(t *testing.T) {
	invalidUTF8 := append([]byte("HTTP/1.1 200 OK\r\n\r\n<title>"), 0xff, 0xfe)
	invalidUTF8 = append(invalidUTF8, []byte(" title</title>")...)

	beforeLimit := bytes.Repeat([]byte("x"), maxHTTPFlowHTMLTitleScanBytes-64)
	beforeLimit = append(beforeLimit, []byte("<title>before limit</title>")...)
	afterLimit := bytes.Repeat([]byte("x"), maxHTTPFlowHTMLTitleScanBytes)
	afterLimit = append(afterLimit, []byte("<title>after limit</title>")...)
	crossesLimit := bytes.Repeat([]byte("x"), maxHTTPFlowHTMLTitleScanBytes-16)
	crossesLimit = append(crossesLimit, []byte("<title>crosses limit</title>")...)

	testCases := [][]byte{
		nil,
		[]byte("HTTP/1.1 200 OK\r\n\r\n<html><title> Example title </title></html>"),
		[]byte("<TITLE>Upper Case</TITLE>"),
		[]byte("<title></title>"),
		[]byte("<title data-kind=\"ignored\">attribute is not a historical match</title>"),
		[]byte("<title>" + strings.Repeat("界", 160) + "</title>"),
		invalidUTF8,
		beforeLimit,
		afterLimit,
		crossesLimit,
	}

	for index, response := range testCases {
		original := bytes.Clone(response)
		want := ExtractHTTPFlowHTMLTitle(string(response))
		got := ExtractHTTPFlowHTMLTitleBytes(response)
		if got != want {
			t.Fatalf("case %d: bytes title %q does not match string title %q", index, got, want)
		}
		if !bytes.Equal(response, original) {
			t.Fatalf("case %d: response bytes were modified", index)
		}
	}
}

var benchmarkHTTPFlowHTMLTitle string

func BenchmarkExtractHTTPFlowHTMLTitle256K(b *testing.B) {
	prefix := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/html\r\n\r\n<html><head><title>benchmark title</title></head><body>")
	response := append(prefix, bytes.Repeat([]byte("r"), 256*1024-len(prefix))...)
	b.SetBytes(int64(len(response)))

	b.Run("legacy-full-string", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkHTTPFlowHTMLTitle = ExtractHTTPFlowHTMLTitle(string(response))
		}
	})

	b.Run("bytes", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkHTTPFlowHTMLTitle = ExtractHTTPFlowHTMLTitleBytes(response)
		}
	})
}
