package aimem

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/log"
)

// EnsureTimelineMidtermArchiveStore lazily attaches the parent midterm archive store on config.
func EnsureTimelineMidtermArchiveStore(cfg *aicommon.Config) *AIMemoryTriage {
	if cfg == nil {
		return nil
	}
	if store, ok := cfg.TimelineArchiveStore.(*AIMemoryTriage); ok && store != nil {
		return store
	}
	persistentSessionID := strings.TrimSpace(cfg.PersistentSessionId)
	if persistentSessionID == "" {
		return nil
	}
	midtermSessionID := PersistentSessionToMidtermMemorySessionID(persistentSessionID)
	store, err := NewAIMemoryForQuery(midtermSessionID, WithDatabase(cfg.GetDB()))
	if err != nil {
		log.Warnf("create midterm archive store failed for session %s: %v", persistentSessionID, err)
		return nil
	}
	cfg.TimelineArchiveStore = store
	return store
}
