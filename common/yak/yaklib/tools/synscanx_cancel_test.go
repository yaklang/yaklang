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


