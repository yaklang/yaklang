package tools

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/fp"
)

func TestFingerprintScanTimingSeparatesMatchingAndResultDelivery(t *testing.T) {
	const delay = 40 * time.Millisecond
	for _, slowMatch := range []bool{true, false} {
		name := "slow_result_delivery"
		if slowMatch {
			name = "slow_match"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			timing := &fingerprintScanTiming{started: time.Now()}
			out := make(chan *fp.MatchResult, 1)
			want := &fp.MatchResult{Port: 1}
			if !slowMatch {
				out <- &fp.MatchResult{}
			}
			match := func(context.Context, string, int, ...fp.ConfigOption) (*fp.MatchResult, error) {
				if slowMatch {
					time.Sleep(delay)
				}
				return want, nil
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				result, _ := timing.match(ctx, match, "fixture", 1)
				sendMatchResultOrDrop(ctx, out, result, timing)
			}()

			var matchBeforeDelivery, matchEndBeforeDelivery int64
			if !slowMatch {
				require.Eventually(t, func() bool { return timing.sendWaits.Load() == 1 }, time.Second, time.Millisecond)
				matchBeforeDelivery = timing.matchTime.Load()
				matchEndBeforeDelivery = timing.lastMatchEnd.UnixNano()
				require.Positive(t, matchEndBeforeDelivery)
				time.Sleep(delay)
				<-out
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("result delivery did not finish")
			}
			select {
			case got := <-out:
				require.Same(t, want, got)
			default:
				t.Fatal("completed result is missing")
			}
			require.EqualValues(t, 1, timing.matches.Load())
			if slowMatch {
				require.GreaterOrEqual(t, time.Duration(timing.matchTime.Load()), delay)
				require.Zero(t, timing.sendWaitTime.Load())
			} else {
				require.Equal(t, matchBeforeDelivery, timing.matchTime.Load())
				require.Equal(t, matchEndBeforeDelivery, timing.lastMatchEnd.UnixNano())
				require.GreaterOrEqual(t, time.Duration(timing.sendWaitTime.Load()), delay)
			}
			t.Logf("matching=%s last_probe_finished_at_ns=%d result_send_wait=%s", time.Duration(timing.matchTime.Load()), timing.lastMatchEnd.UnixNano(), time.Duration(timing.sendWaitTime.Load()))
		})
	}
}
