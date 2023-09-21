package utils

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

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
