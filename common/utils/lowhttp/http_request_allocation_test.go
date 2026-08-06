package lowhttp

import (
	"bytes"
	"fmt"
	"testing"
)

func TestFixHTTPPacketCRLFBodyViewMatchesCopyPath(t *testing.T) {
	largeBody := bytes.Repeat([]byte("0123456789abcdef"), 16*1024)
	largePacket := append(
		[]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", len(largeBody))),
		largeBody...,
	)

	tests := []struct {
		name        string
		packet      []byte
		noFixLength bool
	}{
		{
			name:        "large-content-length-response",
			packet:      largePacket,
			noFixLength: false,
		},
		{
			name: "no-fix-length",
			packet: []byte("POST /upload HTTP/1.1\nHost: example.test\nContent-Length: 99\n\n" +
				"payload"),
			noFixLength: true,
		},
		{
			name: "chunked-with-rest",
			packet: []byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n" +
				"7\r\npayload\r\n0\r\n\r\nHTTP/1.1 204 No Content\r\n\r\n"),
			noFixLength: false,
		},
		{
			name:        "multipart",
			packet:      []byte(_multipartDemo),
			noFixLength: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := bytes.Clone(test.packet)
			want := fixHTTPPacketCRLF(input, test.noFixLength, true)
			if !bytes.Equal(input, test.packet) {
				t.Fatal("copy path mutated its input")
			}

			got := fixHTTPPacketCRLF(input, test.noFixLength, false)
			if !bytes.Equal(got, want) {
				t.Fatalf("view path differs from copy path\ngot:  %q\nwant: %q", got, want)
			}
			if !bytes.Equal(input, test.packet) {
				t.Fatal("view path mutated its input")
			}

			if len(got) > 0 && len(input) > 0 {
				originalInputByte := input[0]
				got[0] ^= 0xff
				if input[0] != originalInputByte {
					t.Fatal("result aliases the input packet")
				}
			}
		})
	}
}

func TestFixHTTPPacketCRLFBorrowedMatchesOwnedResult(t *testing.T) {
	tests := []struct {
		name        string
		packet      []byte
		noFixLength bool
		wantAlias   bool
	}{
		{
			name:        "canonical-post",
			packet:      []byte("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 7\r\n\r\npayload"),
			wantAlias:   true,
			noFixLength: false,
		},
		{
			name:        "canonical-get",
			packet:      []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"),
			wantAlias:   true,
			noFixLength: false,
		},
		{
			name:        "wrong-content-length",
			packet:      []byte("POST /upload HTTP/1.1\r\nHost: example.test\r\nContent-Length: 99\r\n\r\npayload"),
			wantAlias:   false,
			noFixLength: false,
		},
		{
			name:        "lf-only",
			packet:      []byte("POST /upload HTTP/1.1\nHost: example.test\nContent-Length: 7\n\npayload"),
			wantAlias:   false,
			noFixLength: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := FixHTTPPacketCRLF(test.packet, test.noFixLength)
			got := FixHTTPPacketCRLFBorrowed(test.packet, test.noFixLength)
			if !bytes.Equal(got, want) {
				t.Fatalf("borrowed result differs from owned result\ngot:  %q\nwant: %q", got, want)
			}
			aliases := len(got) > 0 && len(test.packet) > 0 && &got[0] == &test.packet[0]
			if aliases != test.wantAlias {
				t.Fatalf("aliases input = %v, want %v", aliases, test.wantAlias)
			}
		})
	}
}

func BenchmarkFixHTTPPacketCRLFLargeBody(b *testing.B) {
	body := bytes.Repeat([]byte("0123456789abcdef"), 16*1024)
	packet := append(
		[]byte(fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/octet-stream\r\nContent-Length: %d\r\n\r\n", len(body))),
		body...,
	)
	b.SetBytes(int64(len(packet)))
	b.ReportAllocs()

	b.Run("body-copy", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkSplitHTTPBody = fixHTTPPacketCRLF(packet, false, true)
		}
	})
	b.Run("body-view", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkSplitHTTPBody = fixHTTPPacketCRLF(packet, false, false)
		}
	})
	b.Run("borrowed-noop", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			benchmarkSplitHTTPBody = FixHTTPPacketCRLFBorrowed(packet, false)
		}
	})
}
