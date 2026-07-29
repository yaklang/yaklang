package utils

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func TestReadHTTPRequestFromBytesConcurrentOwnership(t *testing.T) {
	const workers = 128
	var wait sync.WaitGroup
	errors := make(chan error, workers)
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			req, err := ReadHTTPRequestFromBytes(benchmarkHTTPRequestPacket)
			if err != nil {
				errors <- err
				return
			}
			body, err := io.ReadAll(req.Body)
			if err != nil {
				errors <- err
				return
			}
			if !bytes.Equal(body, []byte("0123456789abcdef")) ||
				!bytes.Equal(httpctx.GetBareRequestBytes(req), benchmarkHTTPRequestPacket) {
				errors <- io.ErrUnexpectedEOF
			}
		}()
	}
	wait.Wait()
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	if len(httpRequestBytesReaderPool) > httpRequestBytesReaderPoolCapacity {
		t.Fatalf("reader pool exceeded its bound: %d", len(httpRequestBytesReaderPool))
	}
}

func TestHTTPRequestBytesReaderPoolDropsPacket(t *testing.T) {
	state := acquireHTTPRequestBytesReader(bytes.Repeat([]byte("x"), 1<<20))
	releaseHTTPRequestBytesReader(state)
	reused := acquireHTTPRequestBytesReader(nil)
	defer releaseHTTPRequestBytesReader(reused)
	if reused.packet.Len() != 0 || reused.reader.Buffered() != 0 {
		t.Fatalf("pooled reader retained packet data: packet=%d buffered=%d", reused.packet.Len(), reused.reader.Buffered())
	}
}

var benchmarkHTTPRequestPacket = []byte(
	"POST /submit?source=mitm HTTP/1.1\r\n" +
		"Host: 127.0.0.1\r\n" +
		"User-Agent: yakit-e2e\r\n" +
		"Content-Type: application/octet-stream\r\n" +
		"Content-Length: 16\r\n\r\n" +
		"0123456789abcdef",
)

var benchmarkHTTPRequestLargePacket = func() []byte {
	header := []byte(
		"POST /submit?source=mitm HTTP/1.1\r\n" +
			"Host: 127.0.0.1\r\n" +
			"User-Agent: yakit-e2e\r\n" +
			"Content-Type: application/octet-stream\r\n" +
			"Content-Length: 65536\r\n\r\n",
	)
	return append(header, bytes.Repeat([]byte("r"), 64<<10)...)
}()

func TestReadHTTPRequestFromBytesLargeBodyOwnership(t *testing.T) {
	packet := bytes.Clone(benchmarkHTTPRequestLargePacket)
	wantPacket := bytes.Clone(packet)
	req, err := ReadHTTPRequestFromBytes(packet)
	if err != nil {
		t.Fatal(err)
	}

	packet[len(packet)-1] = 'x'
	bare := httpctx.GetBareRequestBytes(req)
	if !bytes.Equal(bare, wantPacket) {
		t.Fatal("bare request changed with caller input")
	}
	bare[len(bare)-2] = 'y'

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 64<<10 || !bytes.Equal(body, bytes.Repeat([]byte("r"), 64<<10)) {
		t.Fatal("request body changed with caller input or bare packet")
	}
}

func TestReadHTTPRequestFromBufioReaderLargeBodyOwnership(t *testing.T) {
	packet := bytes.Clone(benchmarkHTTPRequestLargePacket)
	wantPacket := bytes.Clone(packet)
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		t.Fatal(err)
	}

	packet[len(packet)-1] = 'x'
	bare := httpctx.GetBareRequestBytes(req)
	if !bytes.Equal(bare, wantPacket) {
		t.Fatal("bare request changed with bufio input")
	}
	bare[len(bare)-2] = 'y'

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != 64<<10 || !bytes.Equal(body, bytes.Repeat([]byte("r"), 64<<10)) {
		t.Fatal("request body changed with bufio input or bare packet")
	}
}

func TestReadHTTPRequestFromBufioReaderPadsShortContentLength(t *testing.T) {
	packet := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\n\r\nabc")
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(httpctx.GetBareRequestBytes(req), packet) {
		t.Fatal("bare request does not preserve the short wire packet")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, []byte("abc\n\n")) {
		t.Fatalf("unexpected padded request body: %q", body)
	}
}

func TestReadHTTPRequestFromBufioReaderPadsShortSmuggledContentLength(t *testing.T) {
	packet := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: 5\r\nTransfer-Encoding: chunked\r\n\r\nabc")
	req, err := ReadHTTPRequestFromBufioReader(bufio.NewReader(bytes.NewReader(packet)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(httpctx.GetBareRequestBytes(req), packet) {
		t.Fatal("bare request does not preserve the short smuggled wire packet")
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, []byte("abc\n\n")) {
		t.Fatalf("unexpected padded smuggled request body: %q", body)
	}
}

func TestReadHTTPRequestBodyWithLimitUsesBoundedExactStorage(t *testing.T) {
	bodySize := 64 << 10
	want := bytes.Repeat([]byte("b"), bodySize)
	body, err := readHTTPRequestBodyWithLimit(bytes.NewReader(want), bodySize)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want) || cap(body) != bodySize {
		t.Fatalf("unexpected exact body storage: len=%d cap=%d", len(body), cap(body))
	}

	short, err := readHTTPRequestBodyWithLimit(bytes.NewReader([]byte("abc")), 5)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("unexpected short-read error: %v", err)
	}
	if padded := padHTTPRequestBody(short, 5); !bytes.Equal(padded, []byte("abc\n\n")) {
		t.Fatalf("unexpected short-read padding: %q", padded)
	}
}

func TestHTTPRequestBodyPreallocationIsBounded(t *testing.T) {
	bounded := new(bytes.Buffer)
	reserveHTTPRequestPacketBody(bounded, httpRequestBodyPreallocateLimit)
	if bounded.Cap() < httpRequestBodyPreallocateLimit {
		t.Fatalf("bounded request body was not reserved: cap=%d", bounded.Cap())
	}

	oversized := new(bytes.Buffer)
	reserveHTTPRequestPacketBody(oversized, httpRequestBodyPreallocateLimit+1)
	if oversized.Cap() != 0 {
		t.Fatalf("oversized request body was unexpectedly reserved: cap=%d", oversized.Cap())
	}

	want := bytes.Repeat([]byte("z"), httpRequestBodyPreallocateLimit+1)
	body, err := readHTTPRequestBodyWithLimit(bytes.NewReader(want), len(want))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, want) {
		t.Fatal("oversized request body changed on the legacy fallback path")
	}
}

func BenchmarkReadHTTPRequestFromBytes(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkHTTPRequestPacket)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := ReadHTTPRequestFromBytes(benchmarkHTTPRequestPacket)
		if err != nil {
			b.Fatal(err)
		}
		if req.ContentLength != 16 || len(httpctx.GetBareRequestBytes(req)) != len(benchmarkHTTPRequestPacket) {
			b.Fatal("request packet was not preserved")
		}
		if _, err := io.Copy(io.Discard, req.Body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadHTTPRequestFromBytes64KBody(b *testing.B) {
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkHTTPRequestLargePacket)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, err := ReadHTTPRequestFromBytes(benchmarkHTTPRequestLargePacket)
		if err != nil {
			b.Fatal(err)
		}
		if req.ContentLength != 64<<10 || len(httpctx.GetBareRequestBytes(req)) != len(benchmarkHTTPRequestLargePacket) {
			b.Fatal("request packet was not preserved")
		}
		if _, err := io.Copy(io.Discard, req.Body); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadHTTPRequestFromBufioReader64KBody(b *testing.B) {
	packetReader := bytes.NewReader(nil)
	reader := bufio.NewReader(packetReader)
	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkHTTPRequestLargePacket)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		packetReader.Reset(benchmarkHTTPRequestLargePacket)
		reader.Reset(packetReader)
		req, err := ReadHTTPRequestFromBufioReader(reader)
		if err != nil {
			b.Fatal(err)
		}
		if req.ContentLength != 64<<10 || len(httpctx.GetBareRequestBytes(req)) != len(benchmarkHTTPRequestLargePacket) {
			b.Fatal("request packet was not preserved")
		}
		if _, err := io.Copy(io.Discard, req.Body); err != nil {
			b.Fatal(err)
		}
	}
}
