package yakit

import (
	"bytes"
	"fmt"
	"runtime"
	"strconv"
	"testing"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var benchmarkCreatedHTTPFlow *schema.HTTPFlow
var benchmarkQuotedHTTPPacket string

func TestQuoteHTTPPacketMatchesStrconvQuoteAndOwnsResult(t *testing.T) {
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	allASCII := bytes.Clone(allBytes[:128])
	tests := [][]byte{
		nil,
		{},
		[]byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"),
		[]byte("中文\x00\t\r\n"),
		{0xff, 0xfe, 'a', 0xc3, 0x28},
		allASCII,
		allBytes,
	}
	for _, input := range tests {
		input := bytes.Clone(input)
		want := strconv.Quote(string(input))
		got := quoteHTTPPacket(input)
		if got != want {
			t.Fatalf("quoted packet mismatch:\nwant=%q\ngot =%q", want, got)
		}
		for i := range input {
			input[i] ^= 0xff
		}
		if got != want {
			t.Fatal("quoted result aliases input packet")
		}
	}
}

func quoteHTTPPacketStrconvAdaptiveForBenchmark(packet []byte) string {
	packetString := utils.UnsafeBytesToString(packet)
	quotedBytes := strconv.AppendQuote(make([]byte, 0, quoteHTTPPacketCapacity(packet)), packetString)
	quoted := utils.UnsafeBytesToString(quotedBytes)
	runtime.KeepAlive(packet)
	return quoted
}

func BenchmarkQuoteHTTPPacketASCIIFastPath256K(b *testing.B) {
	packet := benchmarkHTTPFlowResponsePacket(256 * 1024)

	b.Run("strconv-adaptive", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = quoteHTTPPacketStrconvAdaptiveForBenchmark(packet)
		}
	})
	b.Run("ascii-fast-path", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = quoteHTTPPacket(packet)
		}
	})
}

func BenchmarkQuoteHTTPPacketLateUnicodeFallback256K(b *testing.B) {
	packet := benchmarkHTTPFlowResponsePacket(256 * 1024)
	packet = append(packet, []byte("中文")...)

	b.Run("strconv-adaptive", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = quoteHTTPPacketStrconvAdaptiveForBenchmark(packet)
		}
	})
	b.Run("ascii-check-then-strconv", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = quoteHTTPPacket(packet)
		}
	})
}

func TestQuoteHTTPPacketCapacityAdaptsToEscapedInput(t *testing.T) {
	middleControls := bytes.Repeat([]byte("s"), 12*1024)
	copy(middleControls[4*1024:8*1024], bytes.Repeat([]byte{0}, 4*1024))
	tests := []struct {
		name        string
		packet      []byte
		wantDivisor int
	}{
		{name: "printable", packet: bytes.Repeat([]byte("s"), 8*1024), wantDivisor: 64},
		{name: "light-escapes", packet: bytes.Repeat(append(bytes.Repeat([]byte("s"), 127), '\n'), 64), wantDivisor: 64},
		{name: "unicode", packet: bytes.Repeat([]byte("中文"), 2*1024), wantDivisor: 64},
		{name: "medium-escapes", packet: bytes.Repeat([]byte("sssssssssssssss\""), 512), wantDivisor: 8},
		{name: "controls", packet: bytes.Repeat([]byte{0}, 8*1024), wantDivisor: 2},
		{name: "middle-controls", packet: middleControls, wantDivisor: 2},
		{name: "invalid-utf8", packet: bytes.Repeat([]byte{0xff}, 8*1024), wantDivisor: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := len(test.packet) + len(test.packet)/test.wantDivisor + 2
			if got := quoteHTTPPacketCapacity(test.packet); got != want {
				t.Fatalf("unexpected capacity: got %d, want %d", got, want)
			}
		})
	}
}

func TestQuoteHTTPPacketSampleDetectsMiddleNonASCII(t *testing.T) {
	packet := bytes.Repeat([]byte("s"), 12*1024)
	copy(packet[len(packet)/2:], []byte("中文"))
	if quoteHTTPPacketSampleIsASCII(packet) {
		t.Fatal("middle non-ASCII bytes were missed by packet sampling")
	}
}

func BenchmarkQuoteHTTPPacket256K(b *testing.B) {
	packet := benchmarkHTTPFlowResponsePacket(256 * 1024)

	b.Run("copied-input", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = strconv.Quote(string(packet))
		}
	})
	b.Run("read-only-view-copied-output", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = strconv.Quote(utils.UnsafeBytesToString(packet))
			runtime.KeepAlive(packet)
		}
	})
	b.Run("owned-output-handoff", func(b *testing.B) {
		b.SetBytes(int64(len(packet)))
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkQuotedHTTPPacket = quoteHTTPPacket(packet)
		}
	})
}

func BenchmarkQuoteHTTPPacketCapacityMatrix256K(b *testing.B) {
	allBytes := make([]byte, 256*1024)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}
	inputs := map[string][]byte{
		"printable":      benchmarkHTTPFlowResponsePacket(256 * 1024),
		"medium-escapes": bytes.Repeat([]byte("sssssssssssssss\""), 16*1024),
		"controls":       bytes.Repeat([]byte{0}, 256*1024),
		"all-bytes":      allBytes,
	}

	for name, packet := range inputs {
		packet := packet
		b.Run(name, func(b *testing.B) {
			capacities := map[string]int{
				"half-slack":         len(packet) + len(packet)/2,
				"eighth-slack":       len(packet) + len(packet)/8,
				"sixty-fourth-slack": len(packet) + len(packet)/64,
				"adaptive":           quoteHTTPPacketCapacity(packet),
			}
			for capacityName, capacity := range capacities {
				capacity := capacity
				b.Run(capacityName, func(b *testing.B) {
					packetString := utils.UnsafeBytesToString(packet)
					b.SetBytes(int64(len(packet)))
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						quotedBytes := strconv.AppendQuote(make([]byte, 0, capacity), packetString)
						benchmarkQuotedHTTPPacket = utils.UnsafeBytesToString(quotedBytes)
					}
					runtime.KeepAlive(packet)
				})
			}
		})
	}
}

func benchmarkHTTPFlowResponsePacket(bodySize int) []byte {
	body := bytes.Repeat([]byte("s"), bodySize)
	return append(
		[]byte(fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
			len(body),
		)),
		body...,
	)
}

func TestTruncateHTTPPacketBodyForStorageDoesNotMutateInput(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 128)
	packet := append(
		[]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(body))),
		body...,
	)
	original := bytes.Clone(packet)

	truncated := truncateHTTPPacketBodyForStorage(packet, 64)
	if !bytes.Equal(packet, original) {
		t.Fatal("storage truncation mutated its input packet")
	}
	_, truncatedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacketView(truncated)
	if len(truncatedBody) != 64 {
		t.Fatalf("unexpected truncated body length: %d", len(truncatedBody))
	}
}

func BenchmarkCreateHTTPFlowBodyMatrix64K256K(b *testing.B) {
	requestBody := bytes.Repeat([]byte("r"), 64*1024)
	responseBody := bytes.Repeat([]byte("s"), 256*1024)
	request := append(
		[]byte(fmt.Sprintf(
			"POST /body-matrix HTTP/1.1\r\nHost: example.test\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
			len(requestBody),
		)),
		requestBody...,
	)
	response := append(
		[]byte(fmt.Sprintf(
			"HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n",
			len(responseBody),
		)),
		responseBody...,
	)

	b.SetBytes(int64(len(request) + len(response)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		flow, err := CreateHTTPFlow(
			CreateHTTPFlowWithRequestRaw(request),
			CreateHTTPFlowWithResponseRaw(response),
			CreateHTTPFlowWithBareResponseRaw(response),
			CreateHTTPFlowWithSource(schema.HTTPFlow_SourceType_MITM),
			CreateHTTPFlowWithURL("http://example.test/body-matrix"),
			CreateHTTPFlowWithRemoteAddr("127.0.0.1:80"),
		)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkCreatedHTTPFlow = flow
	}
}

func BenchmarkCreateHTTPFlowResponseFixProvenance256K(b *testing.B) {
	request := []byte("GET /fix-provenance HTTP/1.1\r\nHost: example.test\r\n\r\n")
	response := benchmarkHTTPFlowResponsePacket(256 * 1024)
	fixed, err := lowhttp.FixHTTPResponsePacket(response)
	if err != nil {
		b.Fatal(err)
	}

	bench := func(b *testing.B, reuseFixed bool) {
		b.SetBytes(int64(len(request) + len(response)))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			opts := []CreateHTTPFlowOptions{
				CreateHTTPFlowWithRequestRaw(request),
				CreateHTTPFlowWithResponseRaw(response),
				CreateHTTPFlowWithBareResponseRaw(response),
				CreateHTTPFlowWithSource(schema.HTTPFlow_SourceType_MITM),
				CreateHTTPFlowWithURL("http://example.test/fix-provenance"),
				CreateHTTPFlowWithRemoteAddr("127.0.0.1:80"),
			}
			if reuseFixed {
				opts = append(opts, CreateHTTPFlowWithFixResponseRaw(fixed))
			}
			flow, err := CreateHTTPFlow(opts...)
			if err != nil {
				b.Fatal(err)
			}
			benchmarkCreatedHTTPFlow = flow
		}
	}

	b.Run("FixAgain", func(b *testing.B) { bench(b, false) })
	b.Run("ReuseFixed", func(b *testing.B) { bench(b, true) })
}
