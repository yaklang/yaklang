package utils

import (
	"bytes"
	"io"
	"net/http"
	"runtime"
	"testing"
)

func TestReadOwnedHTTPRequestBodyViewRestoresUnreadBody(t *testing.T) {
	req := &http.Request{Body: newOwnedHTTPRequestBody([]byte("prefix-body"))}
	prefix := make([]byte, len("prefix-"))
	if _, err := io.ReadFull(req.Body, prefix); err != nil {
		t.Fatal(err)
	}

	body, ok := ReadOwnedHTTPRequestBodyView(req)
	if !ok {
		t.Fatal("parser-owned body was not recognized")
	}
	if string(body) != "body" {
		t.Fatalf("unexpected remaining body: %q", body)
	}

	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, body) {
		t.Fatalf("restored body mismatch: got %q, want %q", restored, body)
	}
}

func TestReadOwnedHTTPRequestBodyViewDoesNotConsumeForeignBody(t *testing.T) {
	req := &http.Request{Body: io.NopCloser(bytes.NewBufferString("foreign-body"))}
	if body, ok := ReadOwnedHTTPRequestBodyView(req); ok || body != nil {
		t.Fatalf("foreign body unexpectedly borrowed: ok=%v body=%q", ok, body)
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "foreign-body" {
		t.Fatalf("foreign body was consumed: %q", body)
	}
}

func TestReadOwnedHTTPRequestBodyViewSurvivesGCAndRepeatedReads(t *testing.T) {
	req := &http.Request{Body: newOwnedHTTPRequestBody(bytes.Repeat([]byte("r"), 64<<10))}
	first, ok := ReadOwnedHTTPRequestBodyView(req)
	if !ok {
		t.Fatal("parser-owned body was not recognized")
	}
	runtime.GC()

	second, ok := ReadOwnedHTTPRequestBodyView(req)
	if !ok {
		t.Fatal("reset parser-owned body was not recognized")
	}
	if len(first) != 64<<10 || !bytes.Equal(second, first) {
		t.Fatal("body view changed after reset or GC")
	}

	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, first) {
		t.Fatal("body was not readable after repeated view resets")
	}
}

func BenchmarkReadOwnedHTTPRequestBodyView64K(b *testing.B) {
	body := bytes.Repeat([]byte("r"), 64<<10)

	b.Run("copy-and-restore", func(b *testing.B) {
		req := &http.Request{Body: newOwnedHTTPRequestBody(body)}
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			var copied bytes.Buffer
			_, _ = io.Copy(&copied, req.Body)
			req.Body = io.NopCloser(&copied)
		}
	})

	b.Run("owned-view-and-reset", func(b *testing.B) {
		req := &http.Request{Body: newOwnedHTTPRequestBody(body)}
		b.SetBytes(int64(len(body)))
		b.ReportAllocs()
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			if _, ok := ReadOwnedHTTPRequestBodyView(req); !ok {
				b.Fatal("parser-owned body was not recognized")
			}
		}
	})
}
