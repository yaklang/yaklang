package lowhttp

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

var (
	benchmarkSplitHTTPHeaders string
	benchmarkSplitHTTPBody    []byte
	benchmarkSplitHTTPMethod  string
	benchmarkSplitHTTPURI     string
	benchmarkSplitHTTPProto   string
)

func TestSplitHTTPPacketBodyIsIndependentFromInput(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload")
	_, body := SplitHTTPHeadersAndBodyFromPacket(packet)
	if string(body) != "payload" {
		t.Fatalf("unexpected body: %q", body)
	}
	body[0] = 'P'
	if bytes.HasSuffix(packet, []byte("Payload")) {
		t.Fatal("returned body aliases the input packet")
	}
}

func TestSplitHTTPPacketBodyAcrossReaderBufferBoundaries(t *testing.T) {
	for _, size := range []int{0, 1, 4095, 4096, 4097, 256 * 1024} {
		t.Run(fmt.Sprintf("body-%d", size), func(t *testing.T) {
			body := make([]byte, size)
			for index := range body {
				body[index] = byte(index % 251)
			}
			header := []byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n", size))
			packet := append(append([]byte(nil), header...), body...)

			_, got := SplitHTTPHeadersAndBodyFromPacket(packet)
			if !bytes.Equal(got, body) {
				t.Fatalf("body mismatch at size %d", size)
			}
			if len(got) > 0 {
				original := packet[len(header)]
				got[0] ^= 0xff
				if packet[len(header)] != original {
					t.Fatalf("body aliases input at size %d", size)
				}
			}
		})
	}
}

func TestSplitHTTPPacketBodyViewAliasesInput(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload")
	_, body := SplitHTTPHeadersAndBodyFromPacketView(packet)
	if string(body) != "payload" {
		t.Fatalf("unexpected body: %q", body)
	}
	body[0] = 'P'
	if !bytes.HasSuffix(packet, []byte("Payload")) {
		t.Fatal("view body does not alias the input packet")
	}
}

func TestSplitHTTPPacketBodyViewAcrossReaderBufferBoundaries(t *testing.T) {
	for _, size := range []int{0, 1, 4095, 4096, 4097, 256 * 1024} {
		t.Run(fmt.Sprintf("body-%d", size), func(t *testing.T) {
			body := make([]byte, size)
			for index := range body {
				body[index] = byte(index % 251)
			}
			header := []byte(fmt.Sprintf("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: %d\r\n\r\n", size))
			packet := append(append([]byte(nil), header...), body...)

			_, got := SplitHTTPHeadersAndBodyFromPacketView(packet)
			if !bytes.Equal(got, body) {
				t.Fatalf("body mismatch at size %d", size)
			}
			if len(got) > 0 && &got[0] != &packet[len(header)] {
				t.Fatalf("body view does not alias input at size %d", size)
			}
		})
	}
}

func TestSplitHTTPPacketCanonicalViewMatchesLegacyParser(t *testing.T) {
	tests := []struct {
		name   string
		packet string
	}{
		{name: "request", packet: "POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 7\r\n\r\npayload"},
		{name: "response", packet: "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload"},
		{name: "rtsp-response", packet: "RTSP/1.0 200 OK\r\nCSeq: 1\r\nContent-Length: 0\r\n\r\n"},
		{name: "folded-response", packet: "HTTP/1.1 200 OK\r\nX-Folded: first\r\n second\r\nContent-Length: 7\r\n\r\npayload"},
		{name: "mixed-case-content-length", packet: "HTTP/1.1 200 OK\r\ncOnTeNt-LeNgTh : 3\r\n\r\n \t "},
		{name: "whitespace-body-without-content-length", packet: "HTTP/1.1 204 No Content\r\nDate: today\r\n\r\n \t "},
		{name: "binary-body", packet: "HTTP/1.1 200 OK\r\nContent-Length: 4\r\n\r\n\x00\r\n\xff"},
		{name: "lf-only-fallback", packet: "GET / HTTP/1.1\nHost: example.test\n\npayload"},
		{name: "prefixed-fallback", packet: "  HTTP/1.1 200 OK\r\n  Content-Length: 7\r\n  \r\npayload"},
		{name: "trimmed-first-line-fallback", packet: "HTTP/1.1 200 OK \r\nContent-Length: 7\r\n\r\npayload"},
		{name: "extra-cr-fallback", packet: "HTTP/1.1 200 OK\r\nX-Test: value\r\r\nContent-Length: 7\r\n\r\npayload"},
		{name: "request-whitespace-terminator-fallback", packet: "GET / HTTP/1.1\r\nHost: example.test\r\n \t\r\nbody"},
		{name: "empty", packet: ""},
		{name: "missing-header-terminator", packet: "GET / HTTP/1.1\r\nHost: example.test"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			packet := []byte(test.packet)
			wantHeaders, wantBody := splitHTTPPacketEx(packet, nil, nil, nil, false)
			gotHeaders, gotBody := SplitHTTPHeadersAndBodyFromPacketView(packet)
			if gotHeaders != wantHeaders {
				t.Fatalf("header mismatch:\n got: %q\nwant: %q", gotHeaders, wantHeaders)
			}
			if !bytes.Equal(gotBody, wantBody) {
				t.Fatalf("body mismatch: got %q want %q", gotBody, wantBody)
			}
			if len(gotBody) > 0 && len(wantBody) > 0 && &gotBody[0] != &wantBody[0] {
				t.Fatal("fast and legacy views do not reference the same body")
			}
		})
	}
}

func TestSplitHTTPPacketCanonicalViewHeaderIsIndependentFromInput(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload")
	headers, _ := SplitHTTPHeadersAndBodyFromPacketView(packet)
	packet[0] = 'R'
	if headers != "HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\n" {
		t.Fatalf("returned headers alias the input packet: %q", headers)
	}
}

func TestSplitHTTPPacketCanonicalViewRequestCallbackMatchesLegacyParser(t *testing.T) {
	for _, packet := range [][]byte{
		[]byte("POST /upload?q=1 HTTP/1.1\r\nHost: example.test\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("POST /upload?q=1 HTTP/1.1\r\nX-Test: value\r\r\nContent-Length: 7\r\n\r\npayload"),
	} {
		var wantCalls, gotCalls int
		var wantMethod, wantURI, wantProto string
		wantHeaders, wantBody := splitHTTPPacketEx(packet, func(method, requestURI, proto string) error {
			wantCalls++
			wantMethod, wantURI, wantProto = method, requestURI, proto
			return nil
		}, nil, nil, false)
		gotHeaders, gotBody := SplitHTTPHeadersAndBodyFromPacketViewEx(packet, func(method, requestURI, proto string) error {
			gotCalls++
			if method != wantMethod || requestURI != wantURI || proto != wantProto {
				t.Fatalf("callback mismatch: got (%q, %q, %q) want (%q, %q, %q)", method, requestURI, proto, wantMethod, wantURI, wantProto)
			}
			return nil
		})
		if gotCalls != wantCalls || gotHeaders != wantHeaders || !bytes.Equal(gotBody, wantBody) {
			t.Fatalf("parser mismatch: calls got %d want %d, headers got %q want %q, body got %q want %q", gotCalls, wantCalls, gotHeaders, wantHeaders, gotBody, wantBody)
		}
	}
}

func TestSplitHTTPPacketCanonicalViewHeaderHooksMatchLegacyParser(t *testing.T) {
	for _, packet := range [][]byte{
		[]byte("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("HTTP/1.1 200 OK\r\nX-Folded: first\r\n second\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("GET / HTTP/1.1\nHost: example.test\n\npayload"),
	} {
		var wantLines, gotLines []string
		wantHeaders, wantBody := splitHTTPPacketEx(packet, nil, nil, nil, false, func(line string) string {
			wantLines = append(wantLines, line)
			return line
		})
		gotHeaders, gotBody := SplitHTTPHeadersAndBodyFromPacketView(packet, func(line string) {
			gotLines = append(gotLines, line)
		})
		if gotHeaders != wantHeaders || !bytes.Equal(gotBody, wantBody) {
			t.Fatalf("parser mismatch for %q: headers got %q want %q, body got %q want %q", packet, gotHeaders, wantHeaders, gotBody, wantBody)
		}
		if !equalStrings(gotLines, wantLines) {
			t.Fatalf("hook lines mismatch for %q: got %#v want %#v", packet, gotLines, wantLines)
		}
	}
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func TestSplitHTTPPacketCanonicalViewHeaderHookStringsAreIndependentFromInput(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload")
	var lines []string
	SplitHTTPHeadersAndBodyFromPacketView(packet, func(line string) {
		lines = append(lines, line)
	})
	copy(packet, bytes.Repeat([]byte{'x'}, len(packet)))
	if fmt.Sprint(lines) != "[Content-Type: text/plain Content-Length: 7]" {
		t.Fatalf("returned hook lines alias the input packet: %#v", lines)
	}
}

func TestSplitHTTPPacketCanonicalViewRequestCallbackAbortMatchesLegacyParser(t *testing.T) {
	packet := []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n")
	callback := func(string, string, string) error { return fmt.Errorf("abort test") }
	wantHeaders, wantBody := splitHTTPPacketEx(packet, callback, nil, nil, false)
	gotHeaders, gotBody := SplitHTTPHeadersAndBodyFromPacketViewEx(packet, callback)
	if gotHeaders != wantHeaders || !bytes.Equal(gotBody, wantBody) {
		t.Fatalf("abort mismatch: headers got %q want %q, body got %q want %q", gotHeaders, wantHeaders, gotBody, wantBody)
	}
}

func FuzzSplitHTTPPacketCanonicalViewMatchesLegacyParser(f *testing.F) {
	for _, packet := range [][]byte{
		[]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		[]byte("POST /upload HTTP/1.1\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: 7\r\n\r\npayload"),
		[]byte("HTTP/1.1 204 No Content\r\nDate: today\r\n\r\n \t "),
		[]byte("GET / HTTP/1.1\nHost: example.test\n\npayload"),
	} {
		f.Add(packet)
	}

	f.Fuzz(func(t *testing.T, packet []byte) {
		if len(packet) > 64*1024 {
			t.Skip()
		}

		wantHeaders, wantBody := splitHTTPPacketEx(packet, nil, nil, nil, false)
		gotHeaders, gotBody := SplitHTTPHeadersAndBodyFromPacketView(packet)
		if gotHeaders != wantHeaders || !bytes.Equal(gotBody, wantBody) {
			t.Fatalf("parser mismatch for %q: headers got %q want %q, body got %q want %q", packet, gotHeaders, wantHeaders, gotBody, wantBody)
		}

		var wantLines, gotLines []string
		wantHeaders, wantBody = splitHTTPPacketEx(packet, nil, nil, nil, false, func(line string) string {
			wantLines = append(wantLines, line)
			return line
		})
		gotHeaders, gotBody = SplitHTTPHeadersAndBodyFromPacketView(packet, func(line string) {
			gotLines = append(gotLines, line)
		})
		if gotHeaders != wantHeaders || !bytes.Equal(gotBody, wantBody) || !equalStrings(gotLines, wantLines) {
			t.Fatalf("hook parser mismatch for %q: headers got %q want %q, body got %q want %q, lines got %#v want %#v", packet, gotHeaders, wantHeaders, gotBody, wantBody, gotLines, wantLines)
		}

		var wantCalls, gotCalls int
		var wantMethod, wantURI, wantProto string
		wantHeaders, wantBody = splitHTTPPacketEx(packet, func(method, requestURI, proto string) error {
			wantCalls++
			wantMethod, wantURI, wantProto = method, requestURI, proto
			return nil
		}, nil, nil, false)
		gotHeaders, gotBody = SplitHTTPHeadersAndBodyFromPacketViewEx(packet, func(method, requestURI, proto string) error {
			gotCalls++
			if method != wantMethod || requestURI != wantURI || proto != wantProto {
				t.Fatalf("request callback mismatch for %q: got (%q, %q, %q) want (%q, %q, %q)", packet, method, requestURI, proto, wantMethod, wantURI, wantProto)
			}
			return nil
		})
		if gotCalls != wantCalls || gotHeaders != wantHeaders || !bytes.Equal(gotBody, wantBody) {
			t.Fatalf("callback parser mismatch for %q: calls got %d want %d, headers got %q want %q, body got %q want %q", packet, gotCalls, wantCalls, gotHeaders, wantHeaders, gotBody, wantBody)
		}
	})
}

func TestSplitHTTPPacketReaderPoolConcurrent(t *testing.T) {
	const (
		workers    = 8
		iterations = 250
	)

	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		worker := worker
		wait.Add(1)
		go func() {
			defer wait.Done()
			headers := fmt.Sprintf("HTTP/1.1 200 OK\r\nX-Worker: %d\r\nContent-Length: 7\r\n\r\n", worker)
			packet := []byte(headers + "payload")
			for iteration := 0; iteration < iterations; iteration++ {
				var gotHeaders string
				var gotBody []byte
				if (worker+iteration)%2 == 0 {
					gotHeaders, gotBody = SplitHTTPHeadersAndBodyFromPacketView(packet)
				} else {
					gotHeaders, gotBody = SplitHTTPHeadersAndBodyFromPacket(packet)
				}
				if gotHeaders != headers || string(gotBody) != "payload" {
					errors <- fmt.Errorf("worker %d iteration %d received headers %q body %q", worker, iteration, gotHeaders, gotBody)
					return
				}
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
}

func BenchmarkSplitHTTPPacket256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkSplitHTTPHeaders, benchmarkSplitHTTPBody = SplitHTTPHeadersAndBodyFromPacket(packet)
		if len(benchmarkSplitHTTPBody) != len(body) {
			b.Fatalf("unexpected body length: %d", len(benchmarkSplitHTTPBody))
		}
	}
}

func BenchmarkSplitHTTPPacketView256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkSplitHTTPHeaders, benchmarkSplitHTTPBody = SplitHTTPHeadersAndBodyFromPacketView(packet)
		if len(benchmarkSplitHTTPBody) != len(body) {
			b.Fatalf("unexpected body length: %d", len(benchmarkSplitHTTPBody))
		}
	}
}

func BenchmarkSplitHTTPPacketLegacyView256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkSplitHTTPHeaders, benchmarkSplitHTTPBody = splitHTTPPacketEx(packet, nil, nil, nil, false)
		if len(benchmarkSplitHTTPBody) != len(body) {
			b.Fatalf("unexpected body length: %d", len(benchmarkSplitHTTPBody))
		}
	}
}

func BenchmarkSplitHTTPPacketRequestCallbackView256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 262144\r\n\r\n"), body...)
	callback := func(method, requestURI, proto string) error {
		benchmarkSplitHTTPMethod, benchmarkSplitHTTPURI, benchmarkSplitHTTPProto = method, requestURI, proto
		return nil
	}
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkSplitHTTPHeaders, benchmarkSplitHTTPBody = SplitHTTPHeadersAndBodyFromPacketViewEx(packet, callback)
	}
}

func BenchmarkSplitHTTPPacketLegacyRequestCallbackView256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 262144\r\n\r\n"), body...)
	callback := func(method, requestURI, proto string) error {
		benchmarkSplitHTTPMethod, benchmarkSplitHTTPURI, benchmarkSplitHTTPProto = method, requestURI, proto
		return nil
	}
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		benchmarkSplitHTTPHeaders, benchmarkSplitHTTPBody = splitHTTPPacketEx(packet, callback, nil, nil, false)
	}
}

func BenchmarkSplitHTTPPacketHeaderHookView256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	hook := func(line string) { benchmarkSplitHTTPHeaders = line }
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, benchmarkSplitHTTPBody = SplitHTTPHeadersAndBodyFromPacketView(packet, hook)
	}
}

func BenchmarkSplitHTTPPacketLegacyHeaderHookView256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	hook := func(line string) string {
		benchmarkSplitHTTPHeaders = line
		return line
	}
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, benchmarkSplitHTTPBody = splitHTTPPacketEx(packet, nil, nil, nil, false, hook)
	}
}
