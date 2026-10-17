package synscanx

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/netstackvm"
)

func TestSendPacketCancelUnblocksUDPProbe(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	rec := &recordingProber{udp: func(ctx context.Context, _ string, _ []byte) (netstackvm.UDPSegment, error) {
		started <- struct{}{}
		<-ctx.Done()
		return netstackvm.UDPSegment{}, ctx.Err()
	}}
	scanner := &Scannerx{
		ctx: ctx,
		config: &SynxConfig{
			Iface:               &net.Interface{Name: "lo", Flags: net.FlagLoopback},
			SourceIP:            net.ParseIP("127.0.0.1"),
			tcpProbeConcurrency: 1,
			waiting:             time.Second,
		},
		halfOpen: rec,
	}
	targetCh := make(chan *SynxTarget, 1)
	targetCh <- &SynxTarget{Host: "127.0.0.1", Port: 53, Mode: UDP}
	close(targetCh)

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanner.sendPacket(targetCh)
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("udp probe did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sendPacket did not return after cancellation")
	}
}
