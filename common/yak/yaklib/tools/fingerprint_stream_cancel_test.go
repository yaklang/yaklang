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
	for _, name := range []string{"ScanFromSynResult", "ScanFromSpaceEngine"} {
		t.Run(name, func(t *testing.T) {
			scan, ok := FingerprintScanExports[name].(func(interface{}, ...fp.ConfigOption) (chan *fp.MatchResult, error))
			if !ok {
				t.Fatalf("%s changed its exported signature", name)
			}
			var input interface{}
			if name == "ScanFromSynResult" {
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
			out, err := scan(input, fp.WithCtx(ctx), fp.WithPoolSize(1))
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
