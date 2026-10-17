package tools

import (
	"context"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/fp"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils/spacengine/base"
)

func TestFingerprintTargetStreamCancelWhileInputIdle(t *testing.T) {
	for _, kind := range []string{"syn", "space-engine"} {
		t.Run(kind, func(t *testing.T) {
			var input interface{}
			if kind == "syn" {
				ch := make(chan *synscan.SynScanResult)
				input = ch
				defer close(ch)
			} else {
				ch := make(chan *base.NetSpaceEngineResult)
				input = ch
				defer close(ch)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			out, err := _scanFromTargetStream(input, fp.WithCtx(ctx), fp.WithPoolSize(1))
			if err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case result, ok := <-out:
				if ok {
					t.Fatalf("idle stream produced an unexpected result: %+v", result)
				}
			case <-time.After(time.Second):
				t.Fatal("canceled scan still waits for upstream to produce or close its input")
			}
		})
	}
}
