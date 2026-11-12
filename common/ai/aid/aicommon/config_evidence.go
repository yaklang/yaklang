package aicommon

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// Serialize saves from shared configs through the session lock. Timeline never
// calls back into SessionPromptState. Persist the journal and its business mirror
// together, so a restart cannot restore an older journal over a successful save.
func (c *Config) applyEvidenceToTimeline(ops []EvidenceOperation) bool {
	s := c.GetSessionPromptState()
	s.m.Lock()
	timeline := c.GetTimeline()
	_, changed, err := timeline.applyEvidenceOperationsWithChange(ops, s.evidenceJSON, c.AcquireId)
	if err != nil {
		s.m.Unlock()
		log.Warnf("journal session evidence: %v", err)
		return false
	}
	c.persistEvidenceTimelineLocked(s, timeline)
	s.m.Unlock()
	return changed
}

// BroadcastEvidence 把一条 evidence 操作广播到同一 session 的所有 Timeline。
// 广播 item 不携带本地 ID，各 Timeline 接收后自行检查并分配 ID。
func (c *Config) BroadcastEvidence(op EvidenceOperation) error {
	item, err := newEvidenceTimelineItem(op)
	if err != nil {
		return err
	}
	if err := c.BroadcastTimelineItem(item); err != nil {
		return err
	}
	// Every Timeline assigns its own local ID. The session owner remains the
	// persistence authority for evidence restored on the next runtime.
	timeline := c.GetTimeline()
	if c.PersistentSessionId != "" && c.GetDB() != nil && timeline != nil && !timeline.IsBranchTimeline() {
		timeline.Save(c.GetDB(), c.PersistentSessionId)
	}
	return nil
}

// The current Timeline's journal is authoritative once migrated, including an
// empty state after rollback/deletion. The legacy mirror is only imported when
// this Timeline has no journal.
func (c *Config) restoreEvidenceTimeline() {
	s := c.GetSessionPromptState()
	s.m.Lock()
	defer s.m.Unlock()
	timeline := c.GetTimeline()
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
	c.persistEvidenceTimelineLocked(s, c.GetTimeline())
}

// Caller holds the session lock; the DB mirror and journal use one snapshot.
func (c *Config) persistEvidenceTimelineLocked(s *SessionPromptState, timeline *Timeline) {
	if timeline == nil {
		return
	}
	timeline.mu.RLock()
	store, _ := timeline.evidenceStoreLocked()
	s.evidenceJSON = store.Marshal()
	if c.PersistentSessionId == "" || c.GetDB() == nil || timeline.branchTimeline.Load() {
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
