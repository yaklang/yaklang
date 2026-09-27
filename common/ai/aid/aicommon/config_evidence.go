package aicommon

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// Serialize saves from shared configs through the session lock. Timeline never
// calls back into SessionPromptState. Persist the journal and its business mirror
// together, so a restart cannot restore an older journal over a successful save.
func (c *Config) applyEvidenceToTimeline(ops []EvidenceOperation) {
	s := c.GetSessionPromptState()
	s.m.Lock()
	defer s.m.Unlock()
	timeline := c.GetTimeline()
	if _, err := timeline.applyEvidenceOperations(ops, s.evidenceJSON, c.AcquireId); err != nil {
		log.Warnf("journal session evidence: %v", err)
		return
	}
	timeline.mu.RLock()
	store, _ := timeline.evidenceStoreLocked()
	s.evidenceJSON = store.Marshal()
	if c.PersistentSessionId == "" || c.GetDB() == nil || timeline.branchTimeline {
		timeline.mu.RUnlock()
		return
	}
	raw, err := marshalTimelineUnlocked(timeline)
	timeline.mu.RUnlock()
	if err != nil {
		log.Warnf("marshal evidence timeline: %v", err)
		return
	}
	if err := c.GetDB().Model(&schema.AIAgentRuntime{}).Where("persistent_session = ?", c.PersistentSessionId).Updates(map[string]any{
		"quoted_evidence": codec.StrConvQuote(s.evidenceJSON), "quoted_timeline": codec.StrConvQuote(raw),
	}).Error; err != nil {
		log.Warnf("persist session evidence and timeline failed: %v", err)
	}
}

// The journal is authoritative once migrated, including an empty state after
// rollback/deletion. Only legacy DB evidence without a journal is imported.
// Initialization, never prompt rendering, performs this one-time migration.
func (c *Config) restoreEvidenceTimeline() {
	s := c.GetSessionPromptState()
	s.m.Lock()
	defer s.m.Unlock()
	if store, found := c.GetTimeline().evidenceStore(); found {
		s.evidenceJSON = store.Marshal()
		return
	}
	if s.evidenceJSON == "" {
		return
	}
	store := UnmarshalEvidenceStore(s.evidenceJSON)
	store.ShrinkToTokenBudget(sessionEvidenceTokenBudget)
	if err := c.GetTimeline().replaceEvidence(store, c.AcquireId); err != nil {
		log.Warnf("restore evidence timeline: %v", err)
		return
	}
	s.evidenceJSON = store.Marshal()
}
