package tools

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

func TestScanxFromPingUtilsCancelBeforeFirstResult(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pingResults := make(chan *pingutil.PingResult)
	done := make(chan error, 1)
	go func() {
		resultCh, err := _scanxFromPingUtils(pingResults, "80", synscanx.WithCtx(ctx))
		if resultCh != nil {
			for range resultCh {
			}
		}
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("_scanxFromPingUtils did not return after its context was already canceled")
	}
}

func TestChooseScanxRouteSamplePrefersNonLoopback(t *testing.T) {
	tests := []struct {
		name    string
		targets string
		want    string
	}{
		{
			name:    "public first",
			targets: "175.178.223.47,127.0.0.1",
			want:    "175.178.223.47",
		},
		{
			name:    "loopback first",
			targets: "127.0.0.1,175.178.223.47",
			want:    "175.178.223.47",
		},
		{
			name:    "only loopback",
			targets: "127.0.0.1",
			want:    "127.0.0.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chooseScanxRouteSample(tt.targets); got != tt.want {
				t.Fatalf("chooseScanxRouteSample(%q) = %q, want %q", tt.targets, got, tt.want)
			}
		})
	}
}
