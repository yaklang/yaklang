package aimem

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// AsyncAIMemory initializes the memory backend in the background. Only memory
// operations wait for readiness; creating a runtime and reading the session ID
// never wait for embedding checks or index loading.
type AsyncAIMemory struct {
	ctx       context.Context
	cancel    context.CancelFunc
	sessionID string
	ready     chan struct{}
	closed    chan struct{}
	memory    *AIMemoryTriage // published by closing ready
	err       error
}

var _ aicommon.MemoryTriage = (*AsyncAIMemory)(nil)
var _ aicommon.TimelineArchiveStore = (*AsyncAIMemory)(nil)

func NewAsyncAIMemory(ctx context.Context, sessionID string, opts ...Option) *AsyncAIMemory {
	return newAsyncAIMemory(ctx, sessionID, func() (*AIMemoryTriage, error) {
		return NewAIMemory(sessionID, opts...)
	})
}

func NewAsyncAIMemoryForQuery(ctx context.Context, sessionID string, opts ...Option) *AsyncAIMemory {
	return newAsyncAIMemory(ctx, sessionID, func() (*AIMemoryTriage, error) {
		return NewAIMemoryForQuery(sessionID, opts...)
	})
}

func newAsyncAIMemory(ctx context.Context, sessionID string, initialize func() (*AIMemoryTriage, error)) *AsyncAIMemory {
	ctx, cancel := context.WithCancel(ctx)
	m := &AsyncAIMemory{ctx: ctx, cancel: cancel, sessionID: sessionID, ready: make(chan struct{}), closed: make(chan struct{})}
	go func() {
		defer close(m.closed)
		if ctx.Err() != nil {
			m.err = ctx.Err()
		} else {
			m.memory, m.err = initialize()
			if m.err == nil && m.memory == nil {
				m.err = utils.Error("memory initializer returned no backend")
			}
		}
		close(m.ready)
		if m.err != nil {
			log.Warnf("initialize asynchronous memory %s failed: %v", sessionID, m.err)
		}
		if m.memory != nil {
			// Initialization may finish after cancellation. Close that backend too.
			<-ctx.Done()
			m.memory.Close()
		}
	}()
	return m
}

// WaitReady waits only at an explicit memory operation, respecting both the
// runtime lifetime and the caller's cancellation. Errors remain visible to all
// callers instead of silently disabling memory.
func (m *AsyncAIMemory) WaitReady(ctx context.Context) (*AIMemoryTriage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.ctx.Done():
		return nil, m.ctx.Err()
	case <-m.ready:
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := m.ctx.Err(); err != nil {
		return nil, err
	}
	return m.memory, m.err
}

func (m *AsyncAIMemory) GetSessionID() string { return m.sessionID }

// Close cancels pending waits without waiting for an uninterruptible backend
// constructor. The initialization goroutine owns eventual backend cleanup.
func (m *AsyncAIMemory) Close() error { m.cancel(); return nil }

func (m *AsyncAIMemory) SetInvoker(invoker aicommon.AIInvokeRuntime) {
	if memory, err := m.WaitReady(m.ctx); err == nil {
		memory.SetInvoker(invoker)
	}
}

func (m *AsyncAIMemory) AddRawText(text string) ([]*aicommon.MemoryEntity, error) {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return nil, err
	}
	return memory.AddRawText(text)
}

func (m *AsyncAIMemory) SaveMemoryEntities(entities ...*aicommon.MemoryEntity) error {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return err
	}
	return memory.SaveMemoryEntities(entities...)
}

func (m *AsyncAIMemory) SearchBySemantics(query string, limit int) ([]*aicommon.SearchResult, error) {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return nil, err
	}
	return memory.SearchBySemantics(query, limit)
}

func (m *AsyncAIMemory) SearchByTags(tags []string, matchAll bool, limit int) ([]*aicommon.MemoryEntity, error) {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return nil, err
	}
	return memory.SearchByTags(tags, matchAll, limit)
}

func (m *AsyncAIMemory) HandleMemory(input any) error {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return err
	}
	return memory.HandleMemory(input)
}

func (m *AsyncAIMemory) SearchMemory(input any, tokenLimit int) (*aicommon.SearchMemoryResult, error) {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return nil, err
	}
	return memory.SearchMemory(input, tokenLimit)
}

func (m *AsyncAIMemory) SearchMemoryWithoutAI(input any, tokenLimit int) (*aicommon.SearchMemoryResult, error) {
	memory, err := m.WaitReady(m.ctx)
	if err != nil {
		return nil, err
	}
	return memory.SearchMemoryWithoutAI(input, tokenLimit)
}

func (m *AsyncAIMemory) ArchiveCompressedBatch(ctx context.Context, batch *aicommon.TimelineArchiveBatch) (*aicommon.TimelineArchiveRef, error) {
	memory, err := m.WaitReady(ctx)
	if err != nil {
		return nil, err
	}
	return memory.ArchiveCompressedBatch(ctx, batch)
}

func (m *AsyncAIMemory) SearchArchivedBatches(ctx context.Context, query *aicommon.TimelineArchiveSearchQuery) (*aicommon.TimelineArchiveSearchResult, error) {
	memory, err := m.WaitReady(ctx)
	if err != nil {
		return nil, err
	}
	return memory.SearchArchivedBatches(ctx, query)
}
