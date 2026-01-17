package lowhttp

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/utils"
)

func TestBodyStreamReaderHandler_NoLeak_NonPool(t *testing.T) {
	body := bytes.Repeat([]byte("A"), 8*1024)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	assertNoGoroutineLeak(t, "nonpool stream handler", func() {
		for i := 0; i < 20; i++ {
			_, err := HTTP(
				WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)+"\r\n\r\n")),
				WithTimeout(2*time.Second),
				WithBodyStreamReaderHandler(func(_ []byte, body io.ReadCloser) {
					// Read a small part and return without draining.
					buf := make([]byte, 16)
					_, _ = body.Read(buf)
				}),
			)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
		}
	})
}

func TestBodyStreamReaderHandler_NoLeak_ConnPool(t *testing.T) {
	body := bytes.Repeat([]byte("B"), 4*1024)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	pool := NewHttpConnPool(context.Background(), 50, 2)

	assertNoGoroutineLeak(t, "connpool stream handler", func() {
		for i := 0; i < 20; i++ {
			_, err := HTTP(
				WithPacketBytes([]byte("GET / HTTP/1.1\r\nHost: "+utils.HostPort(host, port)+"\r\n\r\n")),
				WithConnPool(true),
				ConnPool(pool),
				WithTimeout(2*time.Second),
				WithBodyStreamReaderHandler(func(_ []byte, body io.ReadCloser) {
					buf := make([]byte, 32)
					_, _ = body.Read(buf)
				}),
			)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
		}
	})
}

func TestBodyStreamReaderHandler_Panic_NoLeak(t *testing.T) {
	body := bytes.Repeat([]byte("C"), 1024)
	host, port := utils.DebugMockHTTPHandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(body)
	})

	if utils.WaitConnect(utils.HostPort(host, port), 3) != nil {
		t.Fatal("debug server failed")
	}

	assertNoGoroutineLeak(t, "stream handler panic", func() {
		for i := 0; i < 10; i++ {
			_, err := HTTP(
				WithPacketBytes([]byte(fmt.Sprintf("GET / HTTP/1.1\r\nHost: %s\r\n\r\n", utils.HostPort(host, port)))),
				WithTimeout(2*time.Second),
				WithBodyStreamReaderHandler(func(_ []byte, _ io.ReadCloser) {
					panic("intentional panic for leak test")
				}),
			)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
		}
	})
}
