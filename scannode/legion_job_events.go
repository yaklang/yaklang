package scannode

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/node"
	jobv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/job/v1"
	nodev1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/node/v1"
)

type jobExecutionRef struct {
	CommandID string
	JobID     string
	SubtaskID string
	AttemptID string
}

type jobEventPublisher struct {
	node *node.NodeBase

	mu      sync.Mutex
	natsURL string
	conn    *nats.Conn
	js      nats.JetStreamContext
}

func newJobEventPublisher(base *node.NodeBase) *jobEventPublisher {
	return &jobEventPublisher{node: base}
}

func (p *jobEventPublisher) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeLocked()
}

func (p *jobEventPublisher) PublishClaimed(
	ctx context.Context,
	ref jobExecutionRef,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventClaimed, ref, ref.AttemptID+":claimed", &jobv1.JobClaimed{
		Job:       p.jobRef(ref),
		ClaimedAt: timestamppb.New(now),
	})
}

func (p *jobEventPublisher) PublishStarted(
	ctx context.Context,
	ref jobExecutionRef,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventStarted, ref, ref.AttemptID+":started", &jobv1.JobStarted{
		Job:       p.jobRef(ref),
		StartedAt: timestamppb.New(now),
	})
}

func (p *jobEventPublisher) PublishProgress(
	ctx context.Context,
	ref jobExecutionRef,
	stage string,
	message string,
	completedUnits uint32,
	totalUnits uint32,
) error {
	return p.publish(ctx, legionEventProgress, ref, uuid.NewString(), &jobv1.JobProgressed{
		Job:            p.jobRef(ref),
		Stage:          stage,
		Message:        message,
		CompletedUnits: completedUnits,
		TotalUnits:     totalUnits,
	})
}

func (p *jobEventPublisher) PublishAsset(
	ctx context.Context,
	ref jobExecutionRef,
	assetKind string,
	title string,
	target string,
	identityKey string,
	assetJSON []byte,
) error {
	return p.publish(ctx, legionEventAsset, ref, uuid.NewString(), &jobv1.JobAsset{
		Job:         p.jobRef(ref),
		AssetKind:   assetKind,
		Title:       title,
		Target:      target,
		IdentityKey: identityKey,
		AssetJson:   cloneBytes(assetJSON),
	})
}

func (p *jobEventPublisher) PublishRisk(
	ctx context.Context,
	ref jobExecutionRef,
	riskKind string,
	title string,
	target string,
	severity string,
	dedupeKey string,
	riskJSON []byte,
) error {
	return p.publish(ctx, legionEventRisk, ref, uuid.NewString(), &jobv1.JobRisk{
		Job:       p.jobRef(ref),
		RiskKind:  riskKind,
		Title:     title,
		Target:    target,
		Severity:  severity,
		DedupeKey: dedupeKey,
		RiskJson:  cloneBytes(riskJSON),
	})
}

func (p *jobEventPublisher) PublishReport(
	ctx context.Context,
	ref jobExecutionRef,
	reportKind string,
	reportJSON []byte,
) error {
	return p.publish(ctx, legionEventReport, ref, uuid.NewString(), &jobv1.JobReport{
		Job:        p.jobRef(ref),
		ReportKind: reportKind,
		ReportJson: cloneBytes(reportJSON),
	})
}

func (p *jobEventPublisher) PublishArtifactReady(
	ctx context.Context,
	ref jobExecutionRef,
	artifactKind string,
	artifactFormat string,
	objectKey string,
	codec string,
	sha256 string,
	rawSizeBytes uint64,
	storedSizeBytes uint64,
	metricsJSON []byte,
) error {
	return p.publish(ctx, legionEventArtifactReady, ref, uuid.NewString(), &jobv1.JobArtifactReady{
		Job:             p.jobRef(ref),
		ArtifactKind:    artifactKind,
		ArtifactFormat:  artifactFormat,
		ObjectKey:       objectKey,
		Codec:           codec,
		Sha256:          sha256,
		RawSizeBytes:    rawSizeBytes,
		StoredSizeBytes: storedSizeBytes,
		MetricsJson:     cloneBytes(metricsJSON),
	})
}

func (p *jobEventPublisher) PublishSucceeded(
	ctx context.Context,
	ref jobExecutionRef,
	result any,
) error {
	now := time.Now().UTC()
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal job result: %w", err)
	}
	return p.publish(ctx, legionEventSucceeded, ref, ref.AttemptID+":succeeded", &jobv1.JobSucceeded{
		Job:        p.jobRef(ref),
		FinishedAt: timestamppb.New(now),
		ResultJson: raw,
	})
}

func (p *jobEventPublisher) PublishFailed(
	ctx context.Context,
	ref jobExecutionRef,
	errorCode string,
	errorMessage string,
	detail map[string]string,
) error {
	now := time.Now().UTC()
	raw, err := json.Marshal(detail)
	if err != nil {
		return fmt.Errorf("marshal job failure detail: %w", err)
	}
	return p.publish(ctx, legionEventFailed, ref, ref.AttemptID+":failed", &jobv1.JobFailed{
		Job:             p.jobRef(ref),
		FinishedAt:      timestamppb.New(now),
		ErrorCode:       errorCode,
		ErrorMessage:    errorMessage,
		ErrorDetailJson: raw,
	})
}

func (p *jobEventPublisher) PublishCancelled(
	ctx context.Context,
	ref jobExecutionRef,
	reason string,
) error {
	now := time.Now().UTC()
	return p.publish(ctx, legionEventCancelled, ref, ref.AttemptID+":cancelled", &jobv1.JobCancelled{
		Job:        p.jobRef(ref),
		FinishedAt: timestamppb.New(now),
		Reason:     reason,
	})
}

func (p *jobEventPublisher) publish(
	ctx context.Context,
	eventType string,
	ref jobExecutionRef,
	eventID string,
	message proto.Message,
) error {
	session, ok := p.node.GetSessionState()
	if !ok {
		return fmt.Errorf("node session is not ready")
	}
	if err := p.ensureJetStream(session.NATSURL); err != nil {
		return err
	}

	subject := jobEventSubject(session.EventSubjectPrefix, eventType)
	metadata := &nodev1.EventMetadata{
		EventId:       eventID,
		EventType:     eventType,
		CausationId:   ref.CommandID,
		CorrelationId: ref.AttemptID,
		EmittedAt:     timestamppb.New(time.Now().UTC()),
		Node: &nodev1.NodeRef{
			NodeId:        p.node.CurrentNodeID(),
			NodeSessionId: session.SessionID,
		},
	}
	if err := attachEventMetadata(message, metadata); err != nil {
		return err
	}

	raw, err := proto.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal job event: %w", err)
	}
	msg := nats.NewMsg(subject)
	msg.Data = raw

	p.mu.Lock()
	js := p.js
	p.mu.Unlock()
	if js == nil {
		return fmt.Errorf("jetstream context is not ready")
	}
	if _, err := js.PublishMsg(msg, nats.MsgId(eventID)); err != nil {
		return fmt.Errorf("publish job event %s: %w", eventType, err)
	}
	log.Infof("published legion job event: type=%s attempt_id=%s", eventType, ref.AttemptID)
	return nil
}

func (p *jobEventPublisher) ensureJetStream(natsURL string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.js != nil && p.natsURL == natsURL {
		return nil
	}
	p.closeLocked()

	conn, err := nats.Connect(natsURL, nats.Name("yak-node-events-"+p.node.CurrentNodeID()))
	if err != nil {
		return fmt.Errorf("connect event nats: %w", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		return fmt.Errorf("build event jetstream context: %w", err)
	}
	p.conn = conn
	p.js = js
	p.natsURL = natsURL
	return nil
}

func (p *jobEventPublisher) closeLocked() {
	if p.conn != nil {
		p.conn.Close()
	}
	p.conn = nil
	p.js = nil
	p.natsURL = ""
}

func (p *jobEventPublisher) jobRef(ref jobExecutionRef) *jobv1.JobRef {
	return &jobv1.JobRef{
		JobId:     ref.JobID,
		SubtaskId: ref.SubtaskID,
		AttemptId: ref.AttemptID,
	}
}

func attachEventMetadata(message proto.Message, metadata *nodev1.EventMetadata) error {
	switch value := message.(type) {
	case *jobv1.JobClaimed:
		value.Metadata = metadata
	case *jobv1.JobStarted:
		value.Metadata = metadata
	case *jobv1.JobProgressed:
		value.Metadata = metadata
	case *jobv1.JobAsset:
		value.Metadata = metadata
	case *jobv1.JobRisk:
		value.Metadata = metadata
	case *jobv1.JobReport:
		value.Metadata = metadata
	case *jobv1.JobArtifactReady:
		value.Metadata = metadata
	case *jobv1.JobSucceeded:
		value.Metadata = metadata
	case *jobv1.JobFailed:
		value.Metadata = metadata
	case *jobv1.JobCancelled:
		value.Metadata = metadata
	default:
		return fmt.Errorf("unsupported job event message: %T", message)
	}
	return nil
}

func cloneBytes(input []byte) []byte {
	if len(input) == 0 {
		return nil
	}
	clone := make([]byte, len(input))
	copy(clone, input)
	return clone
}
