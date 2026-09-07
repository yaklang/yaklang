package test

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func testAIRetryWaitOption() aicommon.ConfigOption {
	return aicommon.WithAIRetryWaitFunc(func(ctx context.Context, _ time.Duration) error {
		timer := time.NewTimer(100 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return nil
		}
	})
}
