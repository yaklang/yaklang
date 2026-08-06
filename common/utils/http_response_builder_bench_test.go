package utils

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
)

func BenchmarkReadHTTPResponseContentLength4K(b *testing.B) {
	packet := benchmarkHTTPResponsePacket(4 * 1024)
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()

	b.Run("bytes-reader", func(b *testing.B) {
		reader := bytes.NewReader(packet)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader.Reset(packet)
			rsp, err := ReadHTTPResponseFromBufioReader(reader, &http.Request{Method: http.MethodGet})
			if err != nil {
				b.Fatal(err)
			}
			if rsp.ContentLength != 4*1024 {
				b.Fatal("response body was not preserved")
			}
		}
	})

	b.Run("buffered-reader", func(b *testing.B) {
		packetReader := bytes.NewReader(packet)
		reader := bufio.NewReaderSize(packetReader, 4*1024)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			packetReader.Reset(packet)
			reader.Reset(packetReader)
			rsp, err := ReadHTTPResponseFromBufioReader(reader, &http.Request{Method: http.MethodGet})
			if err != nil {
				b.Fatal(err)
			}
			if rsp.ContentLength != 4*1024 {
				b.Fatal("response body was not preserved")
			}
		}
	})
}

func benchmarkHTTPResponsePacket(bodySize int) []byte {
	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: " +
		strconv.Itoa(bodySize) + "\r\n\r\n")
	packet := make([]byte, len(header)+bodySize)
	copy(packet, header)
	for i := len(header); i < len(packet); i++ {
		packet[i] = byte(i)
	}
	return packet
}

func BenchmarkReadHTTPResponseContentLength256K(b *testing.B) {
	packet := benchmarkHTTPResponsePacket(256 * 1024)
	b.ReportAllocs()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := &http.Request{Method: http.MethodGet}
		rsp, err := ReadHTTPResponseFromBufioReader(bytes.NewReader(packet), req)
		if err != nil {
			b.Fatal(err)
		}
		if rsp.ContentLength != 256*1024 || len(httpctx.GetBareResponseBytes(req)) != len(packet) {
			b.Fatal("response packet was not preserved")
		}
		if err := rsp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadHTTPResponseFromBytesContentLength256K(b *testing.B) {
	packet := benchmarkHTTPResponsePacket(256 * 1024)
	b.ReportAllocs()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rsp, err := ReadHTTPResponseFromBytes(packet, nil)
		if err != nil {
			b.Fatal(err)
		}
		if rsp.ContentLength != 256*1024 {
			b.Fatal("response body was not preserved")
		}
		if err := rsp.Body.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadHTTPResponseMetadataContentLength256K(b *testing.B) {
	packet := benchmarkHTTPResponsePacket(256 * 1024)

	for _, benchmark := range []struct {
		name     string
		metadata bool
		borrowed bool
		fallback bool
		wantBody bool
	}{
		{
			name:     "retained-intermediate-body",
			wantBody: true,
		},
		{
			name:     "discarded-intermediate-body",
			metadata: true,
		},
		{
			name:     "borrowed-transport-packet",
			metadata: true,
			borrowed: true,
		},
		{
			name:     "borrowed-transport-packet-fallback",
			metadata: true,
			borrowed: true,
			fallback: true,
		},
	} {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(packet)))
			for i := 0; i < b.N; i++ {
				req := &http.Request{Method: http.MethodGet}
				var wire bytes.Buffer
				var rsp *http.Response
				var err error
				if benchmark.borrowed {
					borrowFinalPacket := func(finalPacketSize int) []byte {
						captured := wire.Bytes()
						return captured[len(captured)-finalPacketSize:]
					}
					if benchmark.fallback {
						rsp, err = ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacketFallback(
							io.TeeReader(bytes.NewReader(packet), &wire),
							nil,
							req,
							wire.Grow,
							borrowFinalPacket,
							func(finalBodySize int) []byte {
								captured := wire.Bytes()
								return captured[len(captured)-finalBodySize:]
							},
						)
					} else {
						rsp, err = ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket(
							io.TeeReader(bytes.NewReader(packet), &wire),
							nil,
							req,
							wire.Grow,
							borrowFinalPacket,
						)
					}
				} else if benchmark.metadata {
					rsp, err = ReadHTTPResponseMetadataFromBufioReader(io.TeeReader(bytes.NewReader(packet), &wire), req, wire.Grow)
				} else {
					rsp, err = ReadHTTPResponseFromBufioReader(io.TeeReader(bytes.NewReader(packet), &wire), req)
				}
				if err != nil {
					b.Fatal(err)
				}
				if benchmark.metadata {
					if !HTTPResponseHasDiscardedIntermediateBody(rsp) {
						b.Fatal("metadata body was not discarded")
					}
				}
				if len(httpctx.GetBareResponseBytes(req)) != len(packet) {
					b.Fatal("response packet was not preserved")
				}
				hasRetainedBody := rsp.Body != http.NoBody && !HTTPResponseHasDiscardedIntermediateBody(rsp)
				if hasRetainedBody != benchmark.wantBody {
					b.Fatal("unexpected intermediate response body state")
				}
			}
		})
	}
}

func TestReadHTTPResponseMetadataDiscardsBoundedBody(t *testing.T) {
	body := bytes.Repeat([]byte("metadata-body"), 32)
	packet := benchmarkHTTPResponsePacket(len(body))
	bodyOffset := len(packet) - len(body)
	copy(packet[bodyOffset:], body)
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	preparedCapacity := 0
	rsp, err := ReadHTTPResponseMetadataFromBufioReader(io.TeeReader(bytes.NewReader(packet), &wire), req, func(size int) {
		preparedCapacity = size
		wire.Grow(size)
	})
	require.NoError(t, err)
	require.Equal(t, len(body), preparedCapacity)
	require.True(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Equal(t, int64(len(body)), httpctx.GetResponseBodySize(req))
	require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	require.Equal(t, packet, wire.Bytes())

	bare := httpctx.GetBareResponseBytes(req)
	require.Equal(t, packet, bare)
	wire.Bytes()[bodyOffset] = 'X'
	require.Equal(t, body[0], bare[bodyOffset], "stored bare packet aliases the transport capture")
}

func TestReadHTTPResponseMetadataBorrowsTransportPacket(t *testing.T) {
	finalBody := []byte("yak-response")
	finalPacket := []byte("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(finalBody)) + "\r\n\r\n" + string(finalBody))
	interimPacket := []byte("HTTP/1.1 103 Early Hints\r\nLink: </style.css>; rel=preload\r\n\r\n")
	packet := append(bytes.Clone(interimPacket), finalPacket...)
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	rsp, err := ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket(
		io.TeeReader(bytes.NewReader(packet), &wire),
		nil,
		req,
		wire.Grow,
		func(finalPacketSize int) []byte {
			captured := wire.Bytes()
			if finalPacketSize > len(captured) {
				return nil
			}
			return captured[len(captured)-finalPacketSize:]
		},
	)
	require.NoError(t, err)
	require.True(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Equal(t, packet, wire.Bytes(), "transport capture must retain informational responses")
	require.Equal(t, finalPacket, httpctx.GetBareResponseBytes(req))
	require.Same(t, &wire.Bytes()[len(interimPacket)], &httpctx.GetBareResponseBytes(req)[0])
}

func TestReadHTTPResponseMetadataBorrowedPacketFallsBackForNormalizedHeader(t *testing.T) {
	finalBody := []byte("yak-response")
	finalWirePacket := []byte("HTTP/1.1 200 OK\nContent-Length: " + strconv.Itoa(len(finalBody)) + "\n\n" + string(finalBody))
	finalNormalizedPacket := []byte("HTTP/1.1 200 OK\r\nContent-Length: " + strconv.Itoa(len(finalBody)) + "\r\n\r\n" + string(finalBody))
	packet := bytes.Clone(finalWirePacket)
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	rsp, err := ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacketFallback(
		io.TeeReader(bytes.NewReader(packet), &wire),
		nil,
		req,
		wire.Grow,
		func(finalPacketSize int) []byte {
			captured := wire.Bytes()
			if finalPacketSize != len(captured) {
				return nil
			}
			return captured
		},
		func(finalBodySize int) []byte {
			captured := wire.Bytes()
			if finalBodySize > len(captured) {
				return nil
			}
			return captured[len(captured)-finalBodySize:]
		},
	)
	require.NoError(t, err)
	require.True(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Equal(t, packet, wire.Bytes(), "transport capture must preserve the original LF-only response")

	bare := httpctx.GetBareResponseBytes(req)
	require.Equal(t, finalNormalizedPacket, bare)
	require.NotSame(t, &wire.Bytes()[0], &bare[0], "normalized packet must own its storage")
	wire.Bytes()[len(packet)-1] = 'X'
	require.Equal(t, finalBody[len(finalBody)-1], bare[len(bare)-1], "owned fallback aliases the transport capture")
}

func TestReadHTTPResponseMetadataBorrowedPacketPreservesShortBody(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nabc")
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	rsp, err := ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket(
		io.TeeReader(bytes.NewReader(packet), &wire),
		nil,
		req,
		wire.Grow,
		func(finalPacketSize int) []byte {
			captured := wire.Bytes()
			return captured[len(captured)-finalPacketSize:]
		},
	)
	require.NoError(t, err)
	require.True(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Equal(t, int64(5), httpctx.GetResponseBodySize(req))
	require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	require.Same(t, &wire.Bytes()[0], &httpctx.GetBareResponseBytes(req)[0])
}

func TestReadHTTPResponseMetadataRejectsInvalidBorrowedPacket(t *testing.T) {
	packet := benchmarkHTTPResponsePacket(128)
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	_, err := ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket(
		io.TeeReader(bytes.NewReader(packet), &wire),
		nil,
		req,
		wire.Grow,
		func(int) []byte { return nil },
	)
	require.ErrorContains(t, err, "invalid borrowed HTTP response packet")
	require.Empty(t, httpctx.GetBareResponseBytes(req))
}

func TestReadHTTPResponseMetadataDoesNotBorrowNonBoundedBody(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nyak\r\n0\r\n\r\n")
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer
	borrowCalls := 0

	rsp, err := ReadHTTPResponseMetadataFromBufioReaderConnWithBorrowedPacket(
		io.TeeReader(bytes.NewReader(packet), &wire),
		nil,
		req,
		wire.Grow,
		func(int) []byte {
			borrowCalls++
			return nil
		},
	)
	require.NoError(t, err)
	require.False(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Zero(t, borrowCalls)
	require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	wire.Bytes()[0] = 'X'
	require.Equal(t, byte('H'), httpctx.GetBareResponseBytes(req)[0])
}

func TestReadHTTPResponseMetadataKeepsNonBoundedBodies(t *testing.T) {
	t.Run("chunked", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nyak\r\n0\r\n\r\n")
		req := &http.Request{Method: http.MethodGet}

		rsp, err := ReadHTTPResponseMetadataFromBufioReader(bytes.NewReader(packet), req, nil)
		require.NoError(t, err)
		require.False(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
		require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	})

	t.Run("response callback", func(t *testing.T) {
		packet := benchmarkHTTPResponsePacket(128)
		req := &http.Request{Method: http.MethodGet}
		httpctx.SetResponseHeaderCallback(req, func(_ *http.Response, _ []byte, body io.Reader) (io.Reader, error) {
			return body, nil
		})

		rsp, err := ReadHTTPResponseMetadataFromBufioReader(bytes.NewReader(packet), req, nil)
		require.NoError(t, err)
		require.False(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
		require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	})

	t.Run("over preallocation limit", func(t *testing.T) {
		packet := benchmarkHTTPResponsePacket(httpResponseBodyPreallocateLimit + 1)
		req := &http.Request{Method: http.MethodGet}

		rsp, err := ReadHTTPResponseMetadataFromBufioReader(bytes.NewReader(packet), req, nil)
		require.NoError(t, err)
		require.False(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
		require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	})
}

func TestReadHTTPResponseMetadataPreservesFinalBarePacket(t *testing.T) {
	finalPacket := []byte("HTTP/1.1 200 OK\r\nContent-Length: 3\r\n\r\nyak")
	packet := append([]byte("HTTP/1.1 100 Continue\r\nX-Interim: true\r\n\r\n"), finalPacket...)
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	rsp, err := ReadHTTPResponseMetadataFromBufioReader(io.TeeReader(bytes.NewReader(packet), &wire), req, wire.Grow)
	require.NoError(t, err)
	require.True(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Equal(t, packet, wire.Bytes(), "wire capture must retain informational responses")
	require.Equal(t, finalPacket, httpctx.GetBareResponseBytes(req), "bare response must contain only the final response")
}

func TestReadHTTPResponseMetadataPreservesShortContentLengthBarePacket(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nabc")
	req := &http.Request{Method: http.MethodGet}
	var wire bytes.Buffer

	rsp, err := ReadHTTPResponseMetadataFromBufioReader(io.TeeReader(bytes.NewReader(packet), &wire), req, wire.Grow)
	require.NoError(t, err)
	require.True(t, HTTPResponseHasDiscardedIntermediateBody(rsp))
	require.Equal(t, int64(5), httpctx.GetResponseBodySize(req))
	require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
	require.Equal(t, packet, wire.Bytes())
}

func TestReadHTTPResponseBodyAndBarePacketHaveIndependentOwnership(t *testing.T) {
	body := bytes.Repeat([]byte("yak"), 64)
	packet := benchmarkHTTPResponsePacket(len(body))
	copy(packet[len(packet)-len(body):], body)
	req := &http.Request{Method: http.MethodGet}

	rsp, err := ReadHTTPResponseFromBufioReader(bytes.NewReader(packet), req)
	require.NoError(t, err)
	bare := httpctx.GetBareResponseBytes(req)
	require.Len(t, bare, len(packet))
	bodyOffset := len(bare) - len(body)
	packet[len(packet)-len(body)] = 'Y'
	require.Equal(t, body[0], bare[bodyOffset], "bare packet changed with caller input")
	bare[bodyOffset] = 'X'
	require.Equal(t, byte('Y'), packet[len(packet)-len(body)], "caller input changed with bare packet")

	gotBody, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, body, gotBody)
}

func TestReadHTTPResponseFromBytesOwnsBody(t *testing.T) {
	body := bytes.Repeat([]byte("response"), 32)
	packet := benchmarkHTTPResponsePacket(len(body))
	bodyOffset := len(packet) - len(body)
	copy(packet[bodyOffset:], body)

	rsp, err := ReadHTTPResponseFromBytes(packet, nil)
	require.NoError(t, err)
	packet[bodyOffset] = 'X'

	gotBody, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, body, gotBody)
}

func TestReadHTTPResponseFromBytesWithRequestPreservesBarePacket(t *testing.T) {
	body := bytes.Repeat([]byte("owned-response"), 32)
	packet := benchmarkHTTPResponsePacket(len(body))
	bodyOffset := len(packet) - len(body)
	copy(packet[bodyOffset:], body)
	req := &http.Request{Method: http.MethodGet}

	rsp, err := ReadHTTPResponseFromBytes(packet, req)
	require.NoError(t, err)
	bare := httpctx.GetBareResponseBytes(req)
	require.Equal(t, packet, bare)

	packet[bodyOffset] = 'X'
	require.Equal(t, body[0], bare[bodyOffset], "bare packet aliases caller input")
	bare[bodyOffset] = 'Y'
	gotBody, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, body, gotBody, "response body aliases captured bare packet")
}

func TestReadHTTPResponsePreservesShortContentLengthPadding(t *testing.T) {
	packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nabc")
	req := &http.Request{Method: http.MethodGet}

	rsp, err := ReadHTTPResponseFromBufioReader(bytes.NewReader(packet), req)
	require.NoError(t, err)
	gotBody, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, []byte("abc\n\n"), gotBody)
	require.Equal(t, packet, httpctx.GetBareResponseBytes(req))
}
