package pcaputil

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Opt in on a machine with capture permissions; no external traffic is sent.
func TestLiveCaptureCancellation(t *testing.T) {
	for _, workers := range []int{1, 4} {
		t.Run(fmt.Sprintf("workers=%d", workers), func(t *testing.T) { testLiveCaptureCancellation(t, workers) })
	}
}

func testLiveCaptureCancellation(t *testing.T, workers int) {
	iface := os.Getenv("PCAPX_LIVE_TEST_IFACE")
	if iface == "" {
		t.Skip("set PCAPX_LIVE_TEST_IFACE to a local capture interface")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	opened := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- Start(WithContext(ctx), WithTCPReassemblyWorkers(workers), WithDeviceAdapter(&DeviceAdapter{DeviceName: iface, Snaplen: 65535, Timeout: 20 * time.Millisecond, BPF: "udp and dst port 9"}), WithTCPReassemblyStream(64<<10), WithNetInterfaceCreated(func(*PcapHandleWrapper) {
			select {
			case opened <- struct{}{}:
			default:
			}
		}))
	}()
	select {
	case <-opened:
	case err := <-done:
		t.Fatalf("capture stopped before opening interface: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("capture did not start")
	}
	// Let the read enter its idle wait before cancelling.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("idle capture did not stop")
	}
}
