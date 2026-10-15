package scannode

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
	"google.golang.org/protobuf/proto"
)

type durableJobMessage interface {
	proto.Message
	GetMetadata() *nodev1.EventMetadata
	GetJob() *jobv1.JobRef
}

func validateDurableJobEnvelope(e durableJobEvent) error {
	var msg durableJobMessage
	kind := strings.TrimPrefix(e.Subject, "legion.event.")
	switch kind {
	case legionEventClaimed:
		msg = &jobv1.JobClaimed{}
	case legionEventStarted:
		msg = &jobv1.JobStarted{}
	case legionEventProgress:
		msg = &jobv1.JobProgressed{}
	case legionEventRuleSnapshotPrepared:
		msg = &jobv1.JobRuleSnapshotPrepared{}
	case legionEventAsset:
		msg = &jobv1.JobAsset{}
	case legionEventRisk:
		msg = &jobv1.JobRisk{}
	case legionEventReport:
		msg = &jobv1.JobReport{}
	case legionEventArtifactReady:
		msg = &jobv1.JobArtifactReady{}
	case legionEventArtifactUploadFailed:
		msg = &jobv1.JobArtifactUploadFailed{}
	case legionEventSucceeded:
		msg = &jobv1.JobSucceeded{}
	case legionEventFailed:
		msg = &jobv1.JobFailed{}
	case legionEventCancelled:
		msg = &jobv1.JobCancelled{}
	default:
		return fmt.Errorf("unsupported durable job subject")
	}
	if e.Subject != "legion.event."+kind {
		return fmt.Errorf("invalid durable job subject")
	}
	if err := proto.Unmarshal(e.Payload, msg); err != nil {
		return err
	}
	m, j := msg.GetMetadata(), msg.GetJob()
	if m.GetEventId() != e.EventID || m.GetEventType() != kind || m.GetNode().GetNodeId() != e.NodeID || m.GetNode().GetNodeSessionId() != e.Ref.NodeSessionID || m.GetCausationId() != e.Ref.CommandID || j.GetAttemptId() != e.Ref.AttemptID || j.GetJobId() != e.Ref.JobID || j.GetSubtaskId() != e.Ref.SubtaskID {
		return fmt.Errorf("durable job binding mismatch")
	}
	terminal := kind == legionEventSucceeded || kind == legionEventFailed || kind == legionEventCancelled
	if terminal != e.Terminal {
		return fmt.Errorf("invalid durable terminal flag")
	}
	if terminal && e.EventID != e.Ref.AttemptID+":"+strings.TrimPrefix(kind, "job.") {
		return fmt.Errorf("invalid durable terminal event id")
	}
	return nil
}

func (s *ScanNode) PendingJobDeliveries() []node.PendingJobDelivery {
	if s.bridge == nil || s.bridge.publisher == nil || s.bridge.publisher.outbox == nil {
		return nil
	}
	return s.bridge.publisher.outbox.pendingDeliveries()
}

func (s *ScanNode) AcceptJobDeliveryRecovery(session node.SessionState) error {
	if s.bridge == nil || s.bridge.publisher == nil || s.bridge.publisher.outbox == nil {
		return nil
	}
	return s.bridge.publisher.outbox.acceptRecovery(session)
}

func (s *ScanNode) ApplyJobDeliveryReceipts(receipts []node.JobDeliveryReceipt) error {
	if s.bridge == nil || s.bridge.publisher == nil || s.bridge.publisher.outbox == nil {
		return nil
	}
	box := s.bridge.publisher.outbox
	before := box.snapshot()
	if err := box.applyReceipts(receipts); err != nil {
		return err
	}
	for _, r := range receipts {
		for _, e := range before {
			if e.Terminal && e.EventID == r.TerminalEventID && e.Ref.AttemptID == r.AttemptID {
				log.Infof("event=job_delivery_receipt attempt_id=%s disposition=%s recovery_duration_ms=%d", r.AttemptID, r.Disposition, time.Since(e.CreatedAt).Milliseconds())
			}
		}
	}
	return nil
}

func (p *jobEventPublisher) initializeOutbox() error {
	box, err := openJobEventOutbox(filepath.Join(p.node.BaseDir(), "legion", "job-outbox"))
	if err != nil {
		return fmt.Errorf("open durable job outbox: %w", err)
	}
	p.outbox = box
	return nil
}

func (p *jobEventPublisher) runDelivery(ctx context.Context) {
	if p.outbox == nil {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastLog := time.Time{}
	lastCount := -1
	for {
		if ctx.Err() != nil {
			return
		}
		pending := p.outbox.snapshot()
		if len(pending) != lastCount || time.Since(lastLog) >= time.Minute {
			bytes := 0
			oldest := int64(0)
			for _, e := range pending {
				bytes += len(e.Payload)
			}
			if len(pending) > 0 {
				oldest = time.Since(pending[0].CreatedAt).Milliseconds()
				if oldest < 0 {
					oldest = 0
				}
			}
			log.Infof("event=job_delivery_backlog pending_events=%d pending_bytes=%d oldest_age_ms=%d", len(pending), bytes, oldest)
			lastCount = len(pending)
			lastLog = time.Now()
		}
		if err := p.deliverPending(ctx, pending); err != nil && time.Since(lastLog) >= 5*time.Second {
			log.Warnf("event=job_delivery_retry error=%v", err)
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (p *jobEventPublisher) deliverPending(ctx context.Context, pending []durableJobEvent) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return nil
	}
	completed := make(map[string]bool)
	for _, e := range pending {
		if e.Terminal {
			completed[e.Ref.AttemptID] = true
		}
	}
	for _, e := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e.NodeID != p.node.CurrentNodeID() {
			return fmt.Errorf("durable job event belongs to another node")
		}
		if e.Ref.NodeSessionID != session.SessionID && !completed[e.Ref.AttemptID] {
			switch e.Subject {
			case "legion.event.job.claimed", "legion.event.job.started", "legion.event.job.progressed", "legion.event.job.rule_snapshot_prepared":
				// This process has no durable completion for that old execution.
				// Its delayed state must not reopen a dead/lost attempt. Results
				// can still replay with their original identity and platform fence.
				if err := p.outbox.retireInterruptedState(e.EventID); err != nil {
					return err
				}
				continue
			}
		}
		if e.Terminal && e.BrokerAck {
			continue
		} // Retained until the HTTP projection receipt.
		if e.Ref.NodeSessionID != session.SessionID && e.Terminal {
			p.outbox.mu.Lock()
			accepted := p.outbox.recovered[e.Ref.AttemptID]
			p.outbox.mu.Unlock()
			if !accepted {
				return fmt.Errorf("durable job recovery has not been admitted")
			}
		}
		if err := p.ensureJetStream(session.NATSURL); err != nil {
			return err
		}
		p.mu.Lock()
		js := p.js
		p.mu.Unlock()
		if js == nil {
			return fmt.Errorf("event jetstream is unavailable")
		}
		publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		msg := nats.NewMsg(e.Subject)
		msg.Data = e.Payload
		_, err := js.PublishMsg(msg, nats.MsgId(e.EventID), nats.Context(publishCtx))
		cancel()
		if err != nil {
			return err
		}
		if err := p.outbox.brokerAck(e.EventID); err != nil {
			return err
		}
	}
	return nil
}

func (p *jobEventPublisher) pendingHeartbeats() []node.ActiveAttemptHeartbeat {
	if p.outbox == nil {
		return nil
	}
	session, ok := p.node.GetSessionState()
	if !ok {
		return nil
	}
	result := []node.ActiveAttemptHeartbeat{}
	for _, e := range p.outbox.snapshot() {
		if e.Terminal && e.Ref.NodeSessionID == session.SessionID {
			result = append(result, node.ActiveAttemptHeartbeat{AttemptID: e.Ref.AttemptID, JobID: e.Ref.JobID, SubtaskID: e.Ref.SubtaskID, Status: "running", LastActivityAt: time.Now().UTC()})
		}
	}
	return result
}
