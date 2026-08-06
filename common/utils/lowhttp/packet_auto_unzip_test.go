package lowhttp

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err := w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func zlibBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func deflateRawBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	require.NoError(t, err)
	_, err = w.Write(raw)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return buf.Bytes()
}

func unzipZlib(t *testing.T, raw []byte) []byte {
	t.Helper()
	r, err := zlib.NewReader(bytes.NewReader(raw))
	require.NoError(t, err)
	defer r.Close()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func unzipDeflateRaw(t *testing.T, raw []byte) []byte {
	t.Helper()
	r := flate.NewReader(bytes.NewReader(raw))
	defer r.Close()
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return out
}

func BenchmarkDeletePacketEncodingUnencoded256K(b *testing.B) {
	body := bytes.Repeat([]byte("x"), 256*1024)
	packet := append([]byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: 262144\r\n\r\n"), body...)
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		plain := DeletePacketEncoding(packet)
		if len(plain) != len(packet) || &plain[0] != &packet[0] {
			b.Fatal("unencoded packet did not preserve the original packet")
		}
	}
}

func TestDeletePacketEncodingWithOwnership(t *testing.T) {
	t.Run("unencoded packet remains borrowed", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload")
		plain, independentlyOwned := DeletePacketEncodingWithOwnership(packet)
		require.False(t, independentlyOwned)
		require.Same(t, &packet[0], &plain[0])
	})

	t.Run("decoded packet is independently owned", func(t *testing.T) {
		body := []byte("decoded-gzip-body")
		compressed := gzipBytes(t, body)
		packet := append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", len(compressed))), compressed...)

		plain, independentlyOwned := DeletePacketEncodingWithOwnership(packet)
		require.True(t, independentlyOwned)
		require.NotSame(t, &packet[0], &plain[0])
		_, plainBody := SplitHTTPHeadersAndBodyFromPacketView(plain)
		require.Equal(t, body, plainBody)
	})
}

func TestAutoUnzipPacketEncodingPreservesInputAndOutputOwnership(t *testing.T) {
	t.Run("unencoded returns original", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Length: 7\r\n\r\npayload")
		want := bytes.Clone(packet)

		plain, state, ok := AutoUnzipPacketEncoding(packet)
		require.False(t, ok)
		require.Nil(t, state)
		require.Equal(t, want, packet)
		require.Equal(t, &packet[0], &plain[0])
	})

	t.Run("gzip output is independent", func(t *testing.T) {
		body := []byte("decoded-gzip-body")
		compressed := gzipBytes(t, body)
		packet := append([]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n", len(compressed))), compressed...)
		wantPacket := bytes.Clone(packet)

		plain, state, ok := AutoUnzipPacketEncoding(packet)
		require.True(t, ok)
		require.NotNil(t, state)
		require.Equal(t, wantPacket, packet)
		_, plainBody := SplitHTTPHeadersAndBodyFromPacketView(plain)
		require.Equal(t, body, plainBody)

		packet[len(packet)-1] ^= 0xff
		_, plainBody = SplitHTTPHeadersAndBodyFromPacketView(plain)
		require.Equal(t, body, plainBody)
	})

	t.Run("invalid encoding returns original", func(t *testing.T) {
		packet := []byte("HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: 8\r\n\r\nnot-gzip")
		want := bytes.Clone(packet)

		plain, state, ok := AutoUnzipPacketEncoding(packet)
		require.False(t, ok)
		require.Nil(t, state)
		require.Equal(t, want, packet)
		require.Equal(t, &packet[0], &plain[0])
	})

	t.Run("invalid chunked body cannot mutate input", func(t *testing.T) {
		packet := append([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n"), bytes.Repeat([]byte("x"), 256)...)
		want := bytes.Clone(packet)

		plain, state, ok := AutoUnzipPacketEncoding(packet)
		require.False(t, ok)
		require.Nil(t, state)
		require.Equal(t, want, packet)
		require.Equal(t, &packet[0], &plain[0])
	})
}

func TestAutoUnzipAndZipPacketEncoding_Gzip(t *testing.T) {
	plainBody := []byte("hello yak")
	gz := gzipBytes(t, plainBody)

	orig := []byte(fmt.Sprintf(
		"POST / HTTP/1.1\r\nHost: example.com\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n%s",
		len(gz), string(gz),
	))

	view, st, ok := AutoUnzipPacketEncoding(orig)
	require.True(t, ok)
	require.NotNil(t, st)
	require.Contains(t, st.ContentEncoding, "gzip")
	require.NotContains(t, string(view), "Content-Encoding:")

	_, viewBody := SplitHTTPHeadersAndBodyFromPacket(view)
	require.Equal(t, plainBody, viewBody)

	rezip, ok := AutoZipPacketEncoding(view, st)
	require.True(t, ok)
	require.Contains(t, string(rezip), "Content-Encoding:")

	view2 := DeletePacketEncoding(rezip)
	_, view2Body := SplitHTTPHeadersAndBodyFromPacket(view2)
	require.Equal(t, plainBody, view2Body)
}

func TestAutoUnzipAndZipPacketEncoding_ChunkedGzip(t *testing.T) {
	plainBody := []byte("hello yak chunked gzip")
	gz := gzipBytes(t, plainBody)
	chunked := codec.HTTPChunkedEncode(gz)

	orig := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nContent-Encoding: gzip\r\n\r\n" + string(chunked))

	view, st, ok := AutoUnzipPacketEncoding(orig)
	require.True(t, ok)
	require.NotNil(t, st)
	require.True(t, st.WasChunked)
	require.Contains(t, st.TransferEncoding, "chunked")
	require.NotContains(t, string(view), "Transfer-Encoding:")
	require.NotContains(t, string(view), "Content-Encoding:")

	_, viewBody := SplitHTTPHeadersAndBodyFromPacket(view)
	require.Equal(t, plainBody, viewBody)

	rezip, ok := AutoZipPacketEncoding(view, st)
	require.True(t, ok)
	require.Contains(t, string(rezip), "Transfer-Encoding:")
	require.Contains(t, string(rezip), "Content-Encoding:")

	view2 := DeletePacketEncoding(rezip)
	_, view2Body := SplitHTTPHeadersAndBodyFromPacket(view2)
	require.Equal(t, plainBody, view2Body)
}

func TestAutoUnzipAndZipPacketEncoding_MagicZlibNoHeader(t *testing.T) {
	plainBody := []byte("hello yak zlib magic")
	zb := zlibBytes(t, plainBody)

	orig := []byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s",
		len(zb), string(zb),
	))

	view, st, ok := AutoUnzipPacketEncoding(orig)
	require.True(t, ok)
	require.NotNil(t, st)
	require.False(t, st.hadContentEncHdr)
	require.Equal(t, _contentAlgoZlib, st.detectedAlgo)

	_, viewBody := SplitHTTPHeadersAndBodyFromPacket(view)
	require.Equal(t, plainBody, viewBody)

	rezip, ok := AutoZipPacketEncoding(view, st)
	require.True(t, ok)
	require.NotContains(t, string(rezip), "Content-Encoding:")

	_, rezipBody := SplitHTTPHeadersAndBodyFromPacket(rezip)
	require.Equal(t, plainBody, unzipZlib(t, rezipBody))
}

func TestAutoUnzipAndZipPacketEncoding_DeflateRaw(t *testing.T) {
	plainBody := []byte("hello yak deflate raw")
	df := deflateRawBytes(t, plainBody)

	orig := []byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Encoding: deflate\r\nContent-Length: %d\r\n\r\n%s",
		len(df), string(df),
	))

	view, st, ok := AutoUnzipPacketEncoding(orig)
	require.True(t, ok)
	require.NotNil(t, st)
	require.True(t, st.hadContentEncHdr)
	require.Equal(t, _contentAlgoDeflateRaw, st.detectedAlgo)

	_, viewBody := SplitHTTPHeadersAndBodyFromPacket(view)
	require.Equal(t, plainBody, viewBody)

	rezip, ok := AutoZipPacketEncoding(view, st)
	require.True(t, ok)
	require.Contains(t, string(rezip), "Content-Encoding:")

	_, rezipBody := SplitHTTPHeadersAndBodyFromPacket(rezip)
	require.Equal(t, plainBody, unzipDeflateRaw(t, rezipBody))
}

func TestAutoUnzipAndZipPacketEncoding_ChunkedRaw(t *testing.T) {
	plainBody := []byte("hello yak chunked")
	chunked := codec.HTTPChunkedEncode(plainBody)

	orig := []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" + string(chunked))

	view, st, ok := AutoUnzipPacketEncoding(orig)
	require.True(t, ok)
	require.NotNil(t, st)
	require.True(t, st.WasChunked)
	require.Contains(t, st.TransferEncoding, "chunked")
	require.NotContains(t, string(view), "Transfer-Encoding:")

	_, viewBody := SplitHTTPHeadersAndBodyFromPacket(view)
	require.Equal(t, plainBody, viewBody)

	rezip, ok := AutoZipPacketEncoding(view, st)
	require.True(t, ok)
	require.Contains(t, string(rezip), "Transfer-Encoding:")
}

func TestAutoUnzipPacketEncodingWithLimit(t *testing.T) {
	plainBody := bytes.Repeat([]byte("yak"), 1024)
	gz := gzipBytes(t, plainBody)
	orig := []byte(fmt.Sprintf(
		"HTTP/1.1 200 OK\r\nContent-Encoding: gzip\r\nContent-Length: %d\r\n\r\n%s",
		len(gz), string(gz),
	))

	limited, state, ok := AutoUnzipPacketEncodingWithLimit(orig, 128)
	require.False(t, ok)
	require.Nil(t, state)
	require.Equal(t, orig, limited)

	decoded, state, ok := AutoUnzipPacketEncodingWithLimit(orig, len(plainBody))
	require.True(t, ok)
	require.NotNil(t, state)
	_, decodedBody := SplitHTTPHeadersAndBodyFromPacket(decoded)
	require.Equal(t, plainBody, decodedBody)
}
