package synscanx

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestSendPacketCancelUnblocksFullLoopbackQueue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	scanner := &Scannerx{
		ctx: ctx,
		config: &SynxConfig{
			Iface: &net.Interface{
				Name:  "lo",
				Flags: net.FlagLoopback,
			},
			SourceIP: net.ParseIP("127.0.0.1"),
		},
		LoopPacket: make(chan []byte, 1),
		PacketChan: make(chan []byte, 1),
	}
	scanner.LoopPacket <- []byte("already queued")

	targetCh := make(chan *SynxTarget, 1)
	targetCh <- &SynxTarget{Host: "127.0.0.1", Port: 80, Mode: TCP}
	close(targetCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner.sendPacket(targetCh)
	}()

	select {
	case <-done:
		t.Fatal("sendPacket returned before cancellation while the loopback packet queue was full")
	case <-time.After(50 * time.Millisecond):
	}

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sendPacket did not return after cancellation while blocked on the loopback packet queue")
	}
}
