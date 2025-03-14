package utils

import (
	"context"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

func TestReadConnWithContextTimeout(t *testing.T) {
	host, port := DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(200)
		for i := 0; i < 10; i++ {
			time.Sleep(100 * time.Millisecond)
			writer.Write([]byte("hello world " + fmt.Sprint(i)))
			writer.(http.Flusher).Flush()
		}
		return
	})
	conn, err := net.Dial("tcp", HostPort(host, port))
	if err != nil {
		t.Fatal(err)
	}
	conn.Write([]byte("GET / HTTP/1.1\r\nHost: " + HostPort(host, port) + "\r\n\r\n"))
	time.Sleep(300 * time.Millisecond)
	bytes, err := ReadConnUntil(conn, 300*time.Millisecond)
	spew.Dump(bytes)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bytes), `hello world 8`) {
		t.Fatal("should not have read all")
	}
	conn.Close()
}

func TestReadConnWithTimeout(t *testing.T) {
	var listener net.Listener
	host, port := DebugMockTCPEx(func(ctx context.Context, lis net.Listener, conn net.Conn) {
		listener = lis
		time.Sleep(500 * time.Millisecond)
		_, err := conn.Write([]byte("hello"))
		if err != nil {
			log.Errorf("write tcp failed: %v", err)
		}
	})
	if listener != nil {
		defer func() {
			_ = listener.Close()
		}()
	}
	addr := HostPort(host, port)
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Logf("failed dail %v: %s", addr, err)
		t.FailNow()
	}

	data, err := ReadConnWithTimeout(c, 200*time.Millisecond)
	if err == nil {
		t.Logf("BUG: should not read data: %s", string(data))
		t.FailNow()
	}

	data, err = ReadConnWithTimeout(c, 500*time.Millisecond)
	if err != nil {
		t.Logf("BUG: should have read data: %s", err)
		t.FailNow()
	}

	if string(data) != "hello" {
		t.Logf("read data is not hello: %s", data)
		t.FailNow()
	}
}

func TestTrigger(t *testing.T) {
	var check = false
	NewTriggerWriter(10, func(buffer io.ReadCloser, _ string) {
		check = true
	}).Write([]byte("àf.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)"))
	if !check {
		t.Fatal("should have triggered")
	}

	check = true
	NewTriggerWriter(100000, func(buffer io.ReadCloser, _ string) {
		check = false
	}).Write([]byte("àf.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)f.h(f.w)"))
	if !check {
		t.Fatal("should have non-triggered")
	}
}
