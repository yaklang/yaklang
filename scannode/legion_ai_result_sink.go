package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	legionAIFocusResultSchemaV1  = "legion.focus-result.v1"
	maxInlineFocusRiskFieldBytes = 64 * 1024
)

type aiFocusResultEventPublisher interface {
	PublishRiskWithEventID(
		context.Context,
		jobExecutionRef,
		string,
		string,
		string,
		string,
		string,
		string,
		[]byte,
	) error
	PublishSucceeded(context.Context, jobExecutionRef, any) error
	PublishFailed(context.Context, jobExecutionRef, string, string, map[string]string) error
	PublishCancelled(context.Context, jobExecutionRef, string) error
}

type legionAIFocusResultSink struct {
	publisher  aiFocusResultEventPublisher
	ref        jobExecutionRef
	focusRunID string
	focusMode  string
}

func newLegionAIFocusResultSink(
	publisher aiFocusResultEventPublisher,
	bindCommandID string,
	resultContext *aiv1.AIFocusResultContext,
) (aicommon.ResultSink, error) {
	if resultContext == nil {
		return nil, nil
	}
	if publisher == nil {
		return nil, fmt.Errorf("ai focus result publisher is required")
	}
	ref, err := validateLegionAIFocusResultContext(bindCommandID, resultContext)
	if err != nil {
		return nil, err
	}
	return &legionAIFocusResultSink{
		publisher:  publisher,
		ref:        ref,
		focusRunID: strings.TrimSpace(resultContext.GetFocusRunId()),
		focusMode:  strings.TrimSpace(resultContext.GetFocusMode()),
	}, nil
}

type aiFocusResultLifecycle interface {
	Succeed(context.Context, []byte) error
	Fail(context.Context, string, string, []byte) error
	Cancel(context.Context, string) error
}

// aiSessionResultSinkProxy keeps an already-bound AI engine pointed at the
// current server-issued result identity when Legion rebinds the same session.
type aiSessionResultSinkProxy struct {
	mu   sync.RWMutex
	sink aicommon.ResultSink
}

func newAISessionResultSinkProxy(sink aicommon.ResultSink) *aiSessionResultSinkProxy {
	if sink == nil {
		return nil
	}
	return &aiSessionResultSinkProxy{sink: sink}
}

func (p *aiSessionResultSinkProxy) Set(sink aicommon.ResultSink) {
	if p == nil || sink == nil {
		return
	}
	p.mu.Lock()
	p.sink = sink
	p.mu.Unlock()
}

func (p *aiSessionResultSinkProxy) SubmitRisk(
	ctx context.Context,
	risk *schema.Risk,
) (aicommon.ResultReceipt, error) {
	sink := p.current()
	if sink == nil {
		return aicommon.ResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	return sink.SubmitRisk(ctx, risk)
}

func (p *aiSessionResultSinkProxy) Succeed(ctx context.Context, resultJSON []byte) error {
	if lifecycle, ok := p.current().(aiFocusResultLifecycle); ok {
		return lifecycle.Succeed(ctx, resultJSON)
	}
	return nil
}

func (p *aiSessionResultSinkProxy) Fail(
	ctx context.Context,
	code string,
	message string,
	detailJSON []byte,
) error {
	if lifecycle, ok := p.current().(aiFocusResultLifecycle); ok {
		return lifecycle.Fail(ctx, code, message, detailJSON)
	}
	return nil
}

func (p *aiSessionResultSinkProxy) Cancel(ctx context.Context, reason string) error {
	if lifecycle, ok := p.current().(aiFocusResultLifecycle); ok {
		return lifecycle.Cancel(ctx, reason)
	}
	return nil
}

func (p *aiSessionResultSinkProxy) current() aicommon.ResultSink {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sink
}

func validateLegionAIFocusResultContext(
	bindCommandID string,
	resultContext *aiv1.AIFocusResultContext,
) (jobExecutionRef, error) {
	if resultContext == nil {
		return jobExecutionRef{}, nil
	}
	job := resultContext.GetJob()
	ref := jobExecutionRef{CommandID: strings.TrimSpace(bindCommandID)}
	if job != nil {
		ref.JobID = strings.TrimSpace(job.GetJobId())
		ref.SubtaskID = strings.TrimSpace(job.GetSubtaskId())
		ref.AttemptID = strings.TrimSpace(job.GetAttemptId())
	}
	switch {
	case ref.CommandID == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result bind command_id is required")
	case strings.TrimSpace(resultContext.GetFocusRunId()) == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result focus_run_id is required")
	case strings.TrimSpace(resultContext.GetFocusMode()) == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result focus_mode is required")
	case strings.TrimSpace(resultContext.GetSchemaVersion()) != legionAIFocusResultSchemaV1:
		return jobExecutionRef{}, fmt.Errorf(
			"unsupported ai focus result schema_version %q",
			resultContext.GetSchemaVersion(),
		)
	case job == nil:
		return jobExecutionRef{}, fmt.Errorf("ai focus result job reference is required")
	case ref.JobID == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result job_id is required")
	case ref.SubtaskID == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result subtask_id is required")
	case ref.AttemptID == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result attempt_id is required")
	default:
		return ref, nil
	}
}

func (s *legionAIFocusResultSink) SubmitRisk(
	ctx context.Context,
	risk *schema.Risk,
) (aicommon.ResultReceipt, error) {
	if risk == nil {
		return aicommon.ResultReceipt{}, fmt.Errorf("ai focus risk is required")
	}

	normalized := *risk
	normalized.Hash = strings.TrimSpace(normalized.Hash)
	if normalized.Hash == "" {
		normalized.Hash = uuid.NewString()
	}
	normalized.Title = strings.TrimSpace(normalized.Title)
	normalized.RiskType = strings.TrimSpace(normalized.RiskType)
	normalized.Severity = normalizeFocusRiskSeverity(normalized.Severity)
	target := focusRiskTarget(&normalized)
	switch {
	case normalized.Title == "":
		return aicommon.ResultReceipt{}, fmt.Errorf("ai focus risk title is required")
	case normalized.RiskType == "":
		return aicommon.ResultReceipt{}, fmt.Errorf("ai focus risk type is required")
	case target == "":
		return aicommon.ResultReceipt{}, fmt.Errorf("ai focus risk target is required")
	}

	normalized.QuotedRequest = truncateFocusRiskField(normalized.QuotedRequest)
	normalized.QuotedResponse = truncateFocusRiskField(normalized.QuotedResponse)
	normalized.Details = truncateFocusRiskField(normalized.Details)
	normalized.Payload = truncateFocusRiskField(normalized.Payload)

	raw, err := json.Marshal(&normalized)
	if err != nil {
		return aicommon.ResultReceipt{}, fmt.Errorf("marshal ai focus risk: %w", err)
	}
	dedupeKey := focusRiskDedupeKey(&normalized, target)
	if err := s.publisher.PublishRiskWithEventID(
		ctx,
		s.ref,
		normalized.Hash,
		normalized.RiskType,
		normalized.Title,
		target,
		normalized.Severity,
		dedupeKey,
		raw,
	); err != nil {
		return aicommon.ResultReceipt{}, fmt.Errorf("publish ai focus risk: %w", err)
	}
	return aicommon.ResultReceipt{
		ResultID:  normalized.Hash,
		DedupeKey: dedupeKey,
		BackendID: s.ref.JobID,
	}, nil
}

func (s *legionAIFocusResultSink) Succeed(
	ctx context.Context,
	resultJSON []byte,
) error {
	result := any(map[string]string{
		"focus_run_id": s.focusRunID,
		"focus_mode":   s.focusMode,
	})
	if len(resultJSON) > 0 {
		var decoded any
		if err := json.Unmarshal(resultJSON, &decoded); err == nil {
			result = decoded
		} else {
			result = map[string]string{
				"focus_run_id":   s.focusRunID,
				"focus_mode":     s.focusMode,
				"result_payload": string(resultJSON),
			}
		}
	}
	return s.publisher.PublishSucceeded(ctx, s.ref, result)
}

func (s *legionAIFocusResultSink) Fail(
	ctx context.Context,
	code string,
	message string,
	detailJSON []byte,
) error {
	detail := map[string]string{
		"focus_run_id": s.focusRunID,
		"focus_mode":   s.focusMode,
	}
	if len(detailJSON) > 0 {
		detail["runtime_detail_json"] = string(detailJSON)
	}
	return s.publisher.PublishFailed(ctx, s.ref, code, message, detail)
}

func (s *legionAIFocusResultSink) Cancel(
	ctx context.Context,
	reason string,
) error {
	return s.publisher.PublishCancelled(ctx, s.ref, reason)
}

func normalizeFocusRiskSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return "critical"
	case "high":
		return "high"
	case "warning", "warn", "middle", "medium":
		return "medium"
	case "info", "informational":
		return "info"
	default:
		return "low"
	}
}

func focusRiskTarget(risk *schema.Risk) string {
	if risk == nil {
		return ""
	}
	if target := strings.TrimSpace(risk.Url); target != "" {
		return target
	}
	host := strings.TrimSpace(risk.Host)
	if host == "" {
		host = strings.TrimSpace(risk.IP)
	}
	if host == "" {
		return ""
	}
	if risk.Port > 0 {
		return net.JoinHostPort(host, fmt.Sprintf("%d", risk.Port))
	}
	return host
}

func focusRiskDedupeKey(risk *schema.Risk, target string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		strings.ToLower(strings.TrimSpace(risk.RiskType)),
		strings.ToLower(strings.TrimSpace(target)),
		strings.ToLower(strings.TrimSpace(risk.Parameter)),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func truncateFocusRiskField(value string) string {
	if len(value) <= maxInlineFocusRiskFieldBytes {
		return value
	}
	truncated := value[:maxInlineFocusRiskFieldBytes]
	for !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}
