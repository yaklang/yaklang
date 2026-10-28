package liteforgeapp

import (
	"context"
	"fmt"
	"time"
)

// trackProgress also covers preparation and upstream calls before a result
// channel exists, so both Yak/gRPC callers and native tools can observe waits.
func (c *AnalysisConfig) trackProgress(ctx context.Context, label string, details func() string) func(error) {
	started := time.Now()
	c.AnalyzeLog("%s started", label)
	done, stopped := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-done:
				return
			case <-ticker.C:
				extra := ""
				if details != nil {
					extra = details()
				}
				c.AnalyzeLog("%s waiting elapsed=%s %s", label, time.Since(started).Round(time.Millisecond), extra)
			}
		}
	}()
	return func(err error) {
		close(done)
		<-stopped
		status := "completed"
		if err != nil {
			status = fmt.Sprintf("failed: %v", err)
		}
		c.AnalyzeLog("%s %s elapsed=%s", label, status, time.Since(started).Round(time.Millisecond))
	}
}
