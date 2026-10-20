package aicommon

import (
	"context"
)

// A committed transaction and its detached observer receipt. The owner of the
// compression lifecycle dispatches receipts only after releasing execution.
type timelineCompressionCommit struct {
	Result *TimelineCompressionResult
	Notify func()
}

func (m *Timeline) prepareCompressionOptions(options TimelineCompressionOptions) TimelineCompressionOptions {
	if options.Context == nil {
		if config := m.compressionCallerConfig(); config != nil {
			options.Context = config.GetContext()
		}
		if options.Context == nil {
			options.Context = context.Background()
		}
	}
	notify := options.OnProgress
	var lastStage TimelineCompressionStage
	options.OnProgress = func(stage TimelineCompressionStage) {
		if notify != nil && stage != lastStage {
			lastStage = stage
			notify(stage)
		}
	}
	return options
}

// Both public compression entries use the same reservation. Ordinary writes
// still acquire Timeline.mu normally and can append while the model runs.
func (m *Timeline) tryBeginCompression() (<-chan struct{}, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.compressing {
		return m.compressionDone, false
	}
	m.compressing = true
	m.compressionDone = make(chan struct{})
	return m.compressionDone, true
}

func (m *Timeline) awaitCompression(options TimelineCompressionOptions) error {
	for {
		if err := options.Context.Err(); err != nil {
			return err
		}
		done, acquired := m.tryBeginCompression()
		if acquired {
			return nil
		}
		options.reportProgress(TimelineCompressionWaiting)
		select {
		case <-done:
		case <-options.Context.Done():
			return options.Context.Err()
		}
	}
}

func (m *Timeline) finishCompression(commits []*timelineCompressionCommit) {
	m.mu.Lock()
	m.compressionSnapshot = nil
	m.compressing = false
	close(m.compressionDone)
	m.compressionDone = nil
	m.mu.Unlock()
	// Release the reservation before invoking callbacks so they may reenter.
	for _, commit := range commits {
		if commit != nil && commit.Notify != nil {
			commit.Notify()
		}
	}
}
