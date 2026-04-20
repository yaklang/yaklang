package scannode

import (
	"context"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

const capabilityStatusSyncInterval = 10 * time.Second

func (b *legionJobBridge) syncCapabilityStatuses(ctx context.Context) {
	if b == nil || b.agent == nil || b.agent.node == nil || b.agent.capabilityManager == nil {
		return
	}

	session, ok := b.agent.node.GetSessionState()
	if !ok {
		b.resetCapabilityStatusSync()
		return
	}
	shouldSync, sessionChanged := b.shouldSyncCapabilityStatuses(session.SessionID)
	if sessionChanged {
		b.flushSuppressedObservationDrops(session.SessionID)
		b.handleCapabilitySessionReady(ctx, session.SessionID)
	}
	if !shouldSync {
		return
	}

	statuses := b.agent.capabilityManager.RuntimeStatuses()
	for _, status := range statuses {
		result := CapabilityApplyResult{
			CapabilityKey:    status.CapabilityKey,
			SpecVersion:      status.SpecVersion,
			Status:           status.Status,
			Message:          status.Message,
			StatusDetailJSON: cloneBytes(status.DetailJSON),
			ObservedAt:       status.ObservedAt,
		}
		ref := capabilityCommandRef{
			NodeID:        b.agent.node.CurrentNodeID(),
			CapabilityKey: status.CapabilityKey,
			SpecVersion:   status.SpecVersion,
		}
		if err := b.capabilityPublisher.PublishStatus(ctx, ref, result); err != nil {
			log.Errorf(
				"publish capability status failed: node_id=%s capability=%s err=%v",
				b.agent.node.CurrentNodeID(),
				status.CapabilityKey,
				err,
			)
		}
	}
}

func (b *legionJobBridge) shouldSyncCapabilityStatuses(sessionID string) (bool, bool) {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()

	now := time.Now().UTC()
	if b.lastStatusSessionID != sessionID {
		b.lastStatusSessionID = sessionID
		b.lastStatusSync = now
		return true, true
	}
	if now.Sub(b.lastStatusSync) < capabilityStatusSyncInterval {
		return false, false
	}
	b.lastStatusSync = now
	return true, false
}

func (b *legionJobBridge) resetCapabilityStatusSync() {
	b.statusMu.Lock()
	defer b.statusMu.Unlock()

	b.lastStatusSessionID = ""
	b.lastStatusSync = time.Time{}
}

func (b *legionJobBridge) handleCapabilitySessionReady(ctx context.Context, sessionID string) {
	if b == nil || b.agent == nil || b.agent.capabilityManager == nil {
		return
	}
	if err := b.agent.capabilityManager.OnSessionReady(ctx); err != nil {
		log.Errorf(
			"handle capability session ready failed: node_id=%s session_id=%s err=%v",
			b.agent.node.CurrentNodeID(),
			sessionID,
			err,
		)
	}
}

func (b *legionJobBridge) noteSuppressedObservationDrop() {
	if b == nil || b.agent == nil || b.agent.node == nil {
		return
	}
	b.observationMu.Lock()
	defer b.observationMu.Unlock()

	b.suppressedObservationDrop++
	if b.suppressedObservationDrop == 1 {
		log.Warnf(
			"node session unavailable; suppressing hids observation publication until session recovers: node_id=%s",
			b.agent.node.CurrentNodeID(),
		)
	}
}

func (b *legionJobBridge) flushSuppressedObservationDrops(sessionID string) {
	if b == nil || b.agent == nil || b.agent.node == nil {
		return
	}
	b.observationMu.Lock()
	defer b.observationMu.Unlock()

	if b.suppressedObservationDrop == 0 {
		return
	}
	log.Infof(
		"node session restored; skipped %d hids observation(s) while session was unavailable: node_id=%s session_id=%s",
		b.suppressedObservationDrop,
		b.agent.node.CurrentNodeID(),
		sessionID,
	)
	b.suppressedObservationDrop = 0
}
