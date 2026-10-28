package netx

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestDialUdpXCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := DialUdpX("127.0.0.1:9", DialX_WithContext(ctx), DialX_WithTimeout(5*time.Second))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled udp dial err=%v", err)
	}
}

func TestDialUdpXReachesLocalListener(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer pc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, _, err := DialUdpX(pc.LocalAddr().String(), DialX_WithContext(ctx), DialX_WithTimeout(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("yak-udp")); err != nil {
		t.Fatal(err)
	}
	_ = pc.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 32)
	n, _, err := pc.ReadFrom(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != "yak-udp" {
		t.Fatalf("payload %q", buf[:n])
	}
}
