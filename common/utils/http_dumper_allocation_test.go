package utils

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

type writeOnlyHTTPResponseBuffer struct {
	buffer bytes.Buffer
}

func (w *writeOnlyHTTPResponseBuffer) Write(p []byte) (int, error) {
	return w.buffer.Write(p)
}

func BenchmarkDumpHTTPResponse256K(b *testing.B) {
	packet := benchmarkHTTPResponsePacket(256 * 1024)
	rsp, err := ReadHTTPResponseFromBytes(packet, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.SetBytes(int64(len(packet)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dumped, err := DumpHTTPResponse(rsp, true)
		if err != nil {
			b.Fatal(err)
		}
		if len(dumped) < 256*1024 {
			b.Fatal("response body was not dumped")
		}
	}
}

func BenchmarkWriteHTTPResponse256K(b *testing.B) {
	packet := benchmarkHTTPResponsePacket(256 * 1024)
	rsp, err := ReadHTTPResponseFromBytes(packet, nil)
	if err != nil {
		b.Fatal(err)
	}

	b.Run("dump-and-discard", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(packet)))
		for i := 0; i < b.N; i++ {
			if _, err := DumpHTTPResponse(rsp, true, io.Discard); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("writer-only", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(packet)))
		for i := 0; i < b.N; i++ {
			if err := WriteHTTPResponse(rsp, true, io.Discard); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkDumpHTTPRequestParsed64K(b *testing.B) {
	req, err := ReadHTTPRequestFromBytes(benchmarkHTTPRequestLargePacket)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(benchmarkHTTPRequestLargePacket)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dumped, err := DumpHTTPRequest(req, true)
		if err != nil {
			b.Fatal(err)
		}
		if len(dumped) < 64*1024 {
			b.Fatal("request body was not dumped")
		}
	}
}

func TestDumpHTTPRequestConsumesOwnedBodyAndRestoresIndependentRemainder(t *testing.T) {
	body := bytes.Repeat([]byte("request-body"), 32)
	header := []byte("POST / HTTP/1.1\r\nHost: example.com\r\nContent-Length: " + strconv.Itoa(len(body)) + "\r\n\r\n")
	packet := append(header, body...)
	req, err := ReadHTTPRequestFromBytes(packet)
	require.NoError(t, err)

	originalBody := req.Body
	prefix := make([]byte, 17)
	_, err = io.ReadFull(originalBody, prefix)
	require.NoError(t, err)

	dumped, err := DumpHTTPRequest(req, true)
	require.NoError(t, err)
	originalRemaining, err := io.ReadAll(originalBody)
	require.NoError(t, err)
	require.Empty(t, originalRemaining)

	wantRemaining := body[len(prefix):]
	bodyOffset := len(dumped) - len(wantRemaining)
	require.GreaterOrEqual(t, bodyOffset, 0)
	dumped[bodyOffset] ^= 0xff
	restored, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, wantRemaining, restored)
}

func TestDumpHTTPRequestPreservesExternalBodyFallback(t *testing.T) {
	body := []byte("external-request-body")
	originalBody := io.NopCloser(bytes.NewReader(body))
	req := &http.Request{
		Method:        http.MethodPost,
		RequestURI:    "/",
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Host:          "example.com",
		Header:        make(http.Header),
		Body:          originalBody,
		ContentLength: int64(len(body)),
	}
	prefix := make([]byte, 5)
	_, err := io.ReadFull(originalBody, prefix)
	require.NoError(t, err)

	_, err = DumpHTTPRequest(req, true)
	require.NoError(t, err)
	originalRemaining, err := io.ReadAll(originalBody)
	require.NoError(t, err)
	require.Empty(t, originalRemaining)
	restored, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, body[len(prefix):], restored)
}

func TestWriteHTTPResponseMatchesDumpAndRestoresBody(t *testing.T) {
	packet := benchmarkHTTPResponsePacket(256)
	rsp, err := ReadHTTPResponseFromBytes(packet, nil)
	require.NoError(t, err)

	dumped, err := DumpHTTPResponse(rsp, true)
	require.NoError(t, err)

	var written bytes.Buffer
	require.NoError(t, WriteHTTPResponse(rsp, true, &written))
	require.Equal(t, dumped, written.Bytes())

	restored, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, packet[len(packet)-256:], restored)
}

func TestWriteHTTPResponseWriteOnlyFallbackMatchesDump(t *testing.T) {
	packet := benchmarkHTTPResponsePacket(256)
	wantResponse, err := ReadHTTPResponseFromBytes(packet, nil)
	require.NoError(t, err)
	want, err := DumpHTTPResponse(wantResponse, true)
	require.NoError(t, err)

	response, err := ReadHTTPResponseFromBytes(packet, nil)
	require.NoError(t, err)
	var written writeOnlyHTTPResponseBuffer
	require.NoError(t, WriteHTTPResponse(response, true, &written))
	require.Equal(t, want, written.buffer.Bytes())
}

func TestHTTPResponseDumpWriterSelectsDirectAndFallbackPaths(t *testing.T) {
	var direct bytes.Buffer
	directWriter := newHTTPResponseDumpWriter(&direct)
	require.Nil(t, directWriter.buffered)
	_, err := directWriter.WriteString("direct")
	require.NoError(t, err)
	require.NoError(t, directWriter.Flush())
	require.Equal(t, "direct", direct.String())

	var fallback writeOnlyHTTPResponseBuffer
	fallbackWriter := newHTTPResponseDumpWriter(&fallback)
	require.NotNil(t, fallbackWriter.buffered)
	_, err = fallbackWriter.WriteString("buffered")
	require.NoError(t, err)
	require.Empty(t, fallback.buffer.Bytes())
	require.NoError(t, fallbackWriter.Flush())
	require.Equal(t, "buffered", fallback.buffer.String())
}

func TestWriteHTTPResponseRequiresWriter(t *testing.T) {
	rsp := &http.Response{}
	require.Error(t, WriteHTTPResponse(rsp, true, nil))
}

func TestDumpHTTPResponseConsumesOriginalAndRestoresRemainingBody(t *testing.T) {
	packet := benchmarkHTTPResponsePacket(256)
	rsp, err := ReadHTTPResponseFromBytes(packet, nil)
	require.NoError(t, err)
	originalBody := rsp.Body
	prefix := make([]byte, 17)
	_, err = io.ReadFull(originalBody, prefix)
	require.NoError(t, err)

	dumped, err := DumpHTTPResponse(rsp, true)
	require.NoError(t, err)
	require.NotEqual(t, originalBody, rsp.Body)
	originalRemaining, err := io.ReadAll(originalBody)
	require.NoError(t, err)
	require.Empty(t, originalRemaining)
	restored, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, packet[len(packet)-256+17:], restored)
	require.True(t, bytes.HasSuffix(dumped, restored))

	dumped[len(dumped)-len(restored)] ^= 0xff
	restoredAgain, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Empty(t, restoredAgain)
}

func TestDumpHTTPResponseOutputDoesNotAliasRestoredBody(t *testing.T) {
	packet := benchmarkHTTPResponsePacket(256)
	rsp, err := ReadHTTPResponseFromBytes(packet, nil)
	require.NoError(t, err)

	dumped, err := DumpHTTPResponse(rsp, true)
	require.NoError(t, err)
	bodyOffset := len(dumped) - 256
	dumped[bodyOffset] ^= 0xff
	restored, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, packet[len(packet)-256:], restored)
}

func TestDumpHTTPResponsePreservesExternalBodyFallback(t *testing.T) {
	body := []byte("external-response-body")
	originalBody := io.NopCloser(bytes.NewReader(body))
	rsp := &http.Response{
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Status:        "200 OK",
		StatusCode:    http.StatusOK,
		ContentLength: int64(len(body)),
		Header:        make(http.Header),
		Body:          originalBody,
	}
	prefix := make([]byte, 5)
	_, err := io.ReadFull(originalBody, prefix)
	require.NoError(t, err)

	_, err = DumpHTTPResponse(rsp, true)
	require.NoError(t, err)
	originalRemaining, err := io.ReadAll(originalBody)
	require.NoError(t, err)
	require.Empty(t, originalRemaining)
	restored, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, body[len(prefix):], restored)
}

func TestDumpHTTPResponseRestoresChunkedBody(t *testing.T) {
	body := []byte("3\r\nyak\r\n0\r\n\r\n")
	rsp := &http.Response{
		Proto:            "HTTP/1.1",
		ProtoMajor:       1,
		ProtoMinor:       1,
		Status:           "200 OK",
		StatusCode:       http.StatusOK,
		TransferEncoding: []string{"chunked"},
		Header:           make(http.Header),
		Body:             newOwnedHTTPResponseBody(body),
	}

	_, err := DumpHTTPResponse(rsp, true)
	require.NoError(t, err)
	restored, err := io.ReadAll(rsp.Body)
	require.NoError(t, err)
	require.Equal(t, body, restored)
}
