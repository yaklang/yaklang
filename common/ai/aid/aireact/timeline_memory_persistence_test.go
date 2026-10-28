package aireact

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aimem"
)

type timelinePersistenceMemory struct {
	aicommon.MemoryTriage
	store       *aimem.MemoryStore
	db          *gorm.DB
	calls       atomic.Int32
	recoverOnce atomic.Bool
}

func (m *timelinePersistenceMemory) SaveMemoryEntities(entities ...*aicommon.MemoryEntity) error {
	return fmt.Errorf("unexpected legacy best-effort memory save")
}

func (m *timelinePersistenceMemory) PersistTimelineMemories(ctx context.Context, candidates []any) error {
	if len(candidates) > 0 {
		m.calls.Add(1)
	}
	err := m.store.PersistTimelineMemories(ctx, candidates)
	if err != nil && m.recoverOnce.Swap(false) {
		if dropErr := m.db.Exec(`DROP TRIGGER fail_memory`).Error; dropErr != nil {
			return dropErr
		}
	}
	return err
}

func timelinePersistenceCandidate() map[string]any {
	return map[string]any{"content": "用户明确要求报告附实际验证范围。",
		"tags": []string{"报告"}, "potential_questions": []string{"报告需附哪些验证信息？"},
		"scores": map[string]any{"connectivity": 0.2, "origin": 1.0, "relevance": 0.8,
			"emotion": 0.0, "perference": 1.0, "actionability": 0.8, "temporality": 0.9}}
}

func (m *timelinePersistenceMemory) AddRawText(string) ([]*aicommon.MemoryEntity, error) {
	return nil, fmt.Errorf("unexpected second memory extraction")
}

// Real compression parsing, runtime registration, SQLite/TTL and both indexes.
// Only the model response and embedding vector are controlled by this fixture.
