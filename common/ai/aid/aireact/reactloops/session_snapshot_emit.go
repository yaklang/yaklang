package reactloops

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func BuildSessionSnapshot(cfg *aicommon.Config, loop *ReActLoop, task aicommon.AIStatefulTask) *aicommon.SessionSnapshot {
	if cfg == nil {
		return nil
	}
	snapshot := &aicommon.SessionSnapshot{
		Revision:            cfg.NextSessionSnapshotRevision(),
		UpdatedAt:           time.Now().Unix(),
		Capabilities:        aicommon.BuildCapabilityInventoryItems(cfg, loopCapabilityContext(loop)),
		Perception:          buildSessionSnapshotPerception(loop, cfg),
		Execution:           cfg.BuildSessionSnapshotExecution(task),
		BackgroundProcesses: cfg.BuildSessionSnapshotBackgroundProcesses(),
	}
	aicommon.NormalizeSessionSnapshot(snapshot)
	return snapshot
}

func loopCapabilityContext(loop *ReActLoop) aicommon.CapabilityInventoryLoopContext {
	if loop == nil {
		return nil
	}
	return loop.capabilityInventoryContext()
}

func buildSessionSnapshotPerception(loop *ReActLoop, cfg *aicommon.Config) *aicommon.SessionSnapshotPerception {
	perception := &aicommon.SessionSnapshotPerception{}
	if cfg == nil {
		return perception
	}
	var state *PerceptionState
	if loop != nil {
		state = loop.GetPerceptionState()
	}
	capabilityMatches, knowledge := cfg.GetSessionSnapshotPerceptionExtras()
	perception.CapabilityMatches = capabilityMatches
	perception.Knowledge = knowledge
	if state == nil {
		return perception
	}
	perception.Summary = state.OneLinerSummary
	perception.Topics = append([]string(nil), state.Topics...)
	perception.Keywords = append([]string(nil), state.Keywords...)
	perception.Confidence = state.ConfidenceLevel
	perception.Changed = state.Changed
	perception.Epoch = state.Epoch
	perception.LastTrigger = state.LastTrigger
	perception.IntentShift = state.IntentShift
	if !state.LastUpdateAt.IsZero() {
		perception.LastUpdateAt = state.LastUpdateAt.Unix()
	}
	return perception
}

func EmitSessionSnapshot(cfg *aicommon.Config, loop *ReActLoop, task aicommon.AIStatefulTask) {
	if cfg == nil {
		return
	}
	emitter := cfg.GetEmitter()
	if loop != nil && loop.GetEmitter() != nil {
		emitter = loop.GetEmitter()
	}
	if emitter == nil {
		return
	}

	snapshot := BuildSessionSnapshot(cfg, loop, task)
	if snapshot == nil {
		return
	}
	_, _ = emitter.EmitSystemStructured(aicommon.SessionSnapshotNodeID, snapshot)

	if !cfg.EmitLegacySessionSnapshotEvents() {
		return
	}
	emitCapabilityInventorySnapshot(cfg, emitter, snapshot)
	emitLegacyPerceptionEvents(cfg, emitter, snapshot)
}

func emitCapabilityInventorySnapshot(cfg *aicommon.Config, emitter *aicommon.Emitter, snapshot *aicommon.SessionSnapshot) {
	if cfg == nil || emitter == nil || snapshot == nil {
		return
	}
	payload := aicommon.CapabilityInventoryPayloadFromItems(snapshot.Capabilities, cfg)
	_, _ = emitter.EmitSystemStructured(aicommon.CapabilityInventoryNodeID, payload)
}

func emitLegacyPerceptionEvents(cfg *aicommon.Config, emitter *aicommon.Emitter, snapshot *aicommon.SessionSnapshot) {
	if cfg == nil || snapshot == nil || snapshot.Perception == nil || emitter == nil {
		return
	}

	perception := snapshot.Perception
	if strings.TrimSpace(perception.Summary) != "" || len(perception.Topics) > 0 || len(perception.Keywords) > 0 {
		_, _ = emitter.EmitPerception(
			"perception",
			perception.Summary,
			perception.Topics,
			perception.Keywords,
			perception.Changed,
			perception.Confidence,
			perception.LastTrigger,
			perception.Epoch,
			perception.IntentShift,
		)
	}
	if matches := perception.CapabilityMatches; matches != nil {
		_, _ = emitter.EmitPerceptionCapabilities(
			"perception-capabilities",
			matches.Query,
			matches.MatchedToolNames,
			matches.MatchedForgeNames,
			matches.MatchedSkillNames,
			matches.MatchedFocusModeNames,
			matches.RecommendedCapabilities,
		)
	}
	if knowledge := perception.Knowledge; knowledge != nil && strings.TrimSpace(knowledge.Content) != "" {
		_, _ = emitter.EmitPerceptionKnowledge(
			"perception-knowledge",
			knowledge.Query,
			knowledge.KnowledgeBases,
			knowledge.Content,
		)
	}
}
