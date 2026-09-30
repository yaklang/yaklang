package utils

import (
	"io"
	"net"
	"testing"
	"time"
)

func TestWaitConnectClosesSuccessfulProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	probeClosed := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			probeClosed <- err
			return
		}
		defer conn.Close()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		var buf [1]byte
		_, err = conn.Read(buf[:])
		probeClosed <- err
	}()
	if err := WaitConnect(listener.Addr().String(), 1); err != nil {
		t.Fatal(err)
	}
	if err := <-probeClosed; err != io.EOF {
		t.Fatalf("successful readiness probe must close its connection: %v", err)
	}
}
