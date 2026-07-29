package utils

import (
	"bytes"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"testing"
)

func responseBodyViewTestPacket(bodySize int) []byte {
	header := []byte("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: " + strconv.Itoa(bodySize) + "\r\n\r\n")
	packet := make([]byte, len(header)+bodySize)
	copy(packet, header)
	for i := len(header); i < len(packet); i++ {
		packet[i] = byte(i)
	}
	return packet
}

func readResponseBodyForTest(t testing.TB, rsp *http.Response) []byte {
	t.Helper()
	body, err := io.ReadAll(rsp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestReadHTTPResponseFromBytesWithBodyViewMatchesOwnedParser(t *testing.T) {
	tests := []struct {
		name   string
		packet []byte
	}{
		{name: "content-length", packet: responseBodyViewTestPacket(32 * 1024)},
		{name: "short-content-length", packet: []byte("HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\nabc")},
		{name: "informational-then-final", packet: []byte("HTTP/1.1 100 Continue\r\nX-Interim: yes\r\n\r\nHTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")},
		{name: "chunked", packet: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n3\r\nyak\r\n0\r\n\r\n")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			packetBefore := bytes.Clone(test.packet)
			ownedRsp, err := ReadHTTPResponseFromBytes(bytes.Clone(test.packet), nil)
			if err != nil {
				t.Fatal(err)
			}
			viewRsp, err := ReadHTTPResponseFromBytesWithBodyView(test.packet)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(test.packet, packetBefore) {
				t.Fatal("view parser mutated its input packet")
			}
			if ownedRsp.Status != viewRsp.Status || ownedRsp.StatusCode != viewRsp.StatusCode || ownedRsp.Proto != viewRsp.Proto || ownedRsp.ContentLength != viewRsp.ContentLength || ownedRsp.Close != viewRsp.Close {
				t.Fatalf("response metadata differs: owned=%#v view=%#v", ownedRsp, viewRsp)
			}
			if !reflect.DeepEqual(ownedRsp.Header, viewRsp.Header) || !reflect.DeepEqual(ownedRsp.TransferEncoding, viewRsp.TransferEncoding) {
				t.Fatalf("response headers differ: owned=%#v view=%#v", ownedRsp.Header, viewRsp.Header)
			}
			ownedBody := readResponseBodyForTest(t, ownedRsp)
			viewBody := readResponseBodyForTest(t, viewRsp)
			if !bytes.Equal(ownedBody, viewBody) {
				t.Fatalf("response body differs: owned=%q view=%q", ownedBody, viewBody)
			}
		})
	}
}

func TestReadHTTPResponseFromBytesWithBodyViewAliasesInput(t *testing.T) {
	packet := responseBodyViewTestPacket(128)
	bodyOffset := bytes.Index(packet, []byte("\r\n\r\n")) + 4
	rsp, err := ReadHTTPResponseFromBytesWithBodyView(packet)
	if err != nil {
		t.Fatal(err)
	}

	packet[bodyOffset] ^= 0xff
	body := readResponseBodyForTest(t, rsp)
	if body[0] != packet[bodyOffset] {
		t.Fatal("response Body does not retain the documented packet view")
	}
}

func BenchmarkReadHTTPResponseFromBytesBodyView256K(b *testing.B) {
	packet := responseBodyViewTestPacket(256 * 1024)

	b.Run("owned-body-copy", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(packet)))
		for i := 0; i < b.N; i++ {
			rsp, err := ReadHTTPResponseFromBytes(packet, nil)
			if err != nil {
				b.Fatal(err)
			}
			if rsp.ContentLength != 256*1024 {
				b.Fatal("unexpected content length")
			}
		}
	})

	b.Run("read-only-body-view", func(b *testing.B) {
		b.ReportAllocs()
		b.SetBytes(int64(len(packet)))
		for i := 0; i < b.N; i++ {
			rsp, err := ReadHTTPResponseFromBytesWithBodyView(packet)
			if err != nil {
				b.Fatal(err)
			}
			if rsp.ContentLength != 256*1024 {
				b.Fatal("unexpected content length")
			}
		}
	})
}
