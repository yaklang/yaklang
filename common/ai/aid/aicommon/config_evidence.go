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
	timeline := s.evidenceTimeline
	if timeline == nil {
		timeline = c.GetTimeline()
		s.evidenceTimeline = timeline
	}
	if _, err := timeline.applyEvidenceOperations(ops, s.evidenceJSON, c.AcquireId); err != nil {
		s.m.Unlock()
		log.Warnf("journal session evidence: %v", err)
		return
	}
	c.persistEvidenceTimelineLocked(s, timeline)
	s.m.Unlock()
	c.SyncSessionEvidenceTimeline()
}

// The journal is authoritative once migrated, including an empty state after
// rollback/deletion. Only legacy DB evidence without a journal is imported.
// Initialization, never prompt rendering, performs this one-time migration.
func (c *Config) restoreEvidenceTimeline() {
	s := c.GetSessionPromptState()
	s.m.Lock()
	defer s.m.Unlock()
	if s.evidenceTimeline == nil {
		s.evidenceTimeline = c.GetTimeline()
	}
	timeline := s.evidenceTimeline
	if store, found := timeline.evidenceStore(); found {
		s.evidenceJSON = store.Marshal()
		return
	}
	if s.evidenceJSON == "" {
		return
	}
	store := UnmarshalEvidenceStore(s.evidenceJSON)
	store.ShrinkToTokenBudget(sessionEvidenceTokenBudget)
	if err := timeline.replaceEvidence(store, c.AcquireId); err != nil {
		log.Warnf("restore evidence timeline: %v", err)
		return
	}
	s.evidenceJSON = store.Marshal()
}

// FlushRestoredSessionEvidence is called after the new runtime row exists.
// Preserve journal and freeze metadata even if no new evidence is saved this run.
func (c *Config) FlushRestoredSessionEvidence() {
	s := c.GetSessionPromptState()
	s.m.Lock()
	defer s.m.Unlock()
	c.persistEvidenceTimelineLocked(s, s.evidenceTimeline)
}

func (c *Config) sessionEvidenceTimeline() *Timeline {
	s := c.GetSessionPromptState()
	s.m.RLock()
	timeline := s.evidenceTimeline
	s.m.RUnlock()
	if timeline == nil {
		return c.GetTimeline()
	}
	return timeline
}

// SyncSessionEvidenceTimeline imports the session journal into a task's local
// Timeline before compression. Original IDs preserve ordering, deduplication
// and exact promotion; rendering itself remains read-only.
func (c *Config) SyncSessionEvidenceTimeline() {
	local, session := c.GetTimeline(), c.sessionEvidenceTimeline()
	if local != nil && session != nil && local != session {
		local.importSessionEvidence(session)
	}
}

// Caller holds the session lock; the DB mirror and journal use one snapshot.
func (c *Config) persistEvidenceTimelineLocked(s *SessionPromptState, timeline *Timeline) {
	if timeline == nil {
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
