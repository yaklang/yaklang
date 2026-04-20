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
