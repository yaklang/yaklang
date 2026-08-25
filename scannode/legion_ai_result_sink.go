package scannode

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/yaklang/yaklang/common/schema"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

const (
	legionAIFocusResultSchemaV1         = "legion.focus-result.v1"
	legionAIConversationAuditResultMode = "conversation_audit"
	legionAIConversationExecutionMode   = "multi_turn"
	maxInlineFocusRiskFieldBytes        = 64 * 1024
	maxInlineFocusAssetBytes            = 64 * 1024
	maxInlineCodeAuditReportBytes       = 512 * 1024
	maxInlineCodeAuditSummaryBytes      = 64 * 1024
)

var legionCodeFindingCWEPattern = regexp.MustCompile(`^CWE-[1-9][0-9]*$`)

type aiFocusResultReceipt struct {
	ResultID  string
	DedupeKey string
	BackendID string
}

type aiFocusAssetResult struct {
	Kind        string
	Title       string
	Target      string
	IdentityKey string
	Payload     []byte
}

type aiFocusCodeFinding struct {
	WorkspaceID        string  `json:"workspace_id"`
	LockedRevision     string  `json:"locked_revision"`
	SourceSHA256       string  `json:"source_sha256"`
	File               string  `json:"file"`
	StartLine          int     `json:"start_line"`
	EndLine            int     `json:"end_line"`
	StartColumn        int     `json:"start_column,omitempty"`
	EndColumn          int     `json:"end_column,omitempty"`
	CWE                string  `json:"cwe"`
	VulnerabilityType  string  `json:"vulnerability_type"`
	Category           string  `json:"category"`
	Module             string  `json:"module,omitempty"`
	Severity           string  `json:"severity"`
	Confidence         float64 `json:"confidence"`
	VerificationStatus string  `json:"verification_status"`
	Title              string  `json:"title"`
	Description        string  `json:"description,omitempty"`
	Evidence           string  `json:"evidence"`
	DataFlow           string  `json:"data_flow"`
	ExploitScenario    string  `json:"exploit_scenario"`
	Recommendation     string  `json:"recommendation,omitempty"`
	DedupeKey          string  `json:"dedupe_key"`
	Target             string  `json:"target"`
}

type aiFocusCodeAuditReport struct {
	WorkspaceID       string          `json:"workspace_id"`
	Markdown          string          `json:"markdown"`
	StructuredSummary json.RawMessage `json:"structured_summary"`
}

type aiFocusResultSink interface {
	SubmitRisk(context.Context, *schema.Risk) (aiFocusResultReceipt, error)
}

type aiFocusAssetResultSink interface {
	aiFocusResultSink
	SubmitAsset(context.Context, aiFocusAssetResult) (aiFocusResultReceipt, error)
}

type aiFocusCodeResultSink interface {
	aiFocusAssetResultSink
	SubmitCodeFinding(context.Context, string, aiFocusCodeFinding) (aiFocusResultReceipt, error)
	SubmitCodeAuditReport(context.Context, string, aiFocusCodeAuditReport) (aiFocusResultReceipt, error)
}

type aiFocusExecutionContractBinder interface {
	bindFocusExecutionContract(*legionFocusExecutionContract) error
}

type aiFocusCodeWorkspaceEvidenceBinder interface {
	bindCodeWorkspaceEvidence(lockedRevision string, sourceSHA256 string) error
}

type aiFocusResultEventPublisher interface {
	PublishAssetWithEventID(
		context.Context,
		jobExecutionRef,
		string,
		string,
		string,
		string,
		string,
		[]byte,
	) error
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
	PublishReportWithEventID(context.Context, jobExecutionRef, string, string, []byte) error
	PublishSucceeded(context.Context, jobExecutionRef, any) error
	PublishFailed(context.Context, jobExecutionRef, string, string, map[string]string) error
	PublishCancelled(context.Context, jobExecutionRef, string) error
}

type legionAIFocusResultSink struct {
	publisher              aiFocusResultEventPublisher
	ref                    jobExecutionRef
	focusRunID             string
	focusMode              string
	focusReleaseID         string
	targetURL              string
	mu                     sync.Mutex
	assetIDs               map[string]struct{}
	riskIDs                map[string]struct{}
	targets                map[string]struct{}
	requiredResultKinds    map[string]struct{}
	publishedResultKinds   map[string]struct{}
	codeWorkspaceLockedRev string
	codeWorkspaceSHA256    string
}

func newLegionAIFocusResultSink(
	publisher aiFocusResultEventPublisher,
	bindCommandID string,
	resultContext *aiv1.AIFocusResultContext,
) (aiFocusResultSink, error) {
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
		publisher:            publisher,
		ref:                  ref,
		focusRunID:           strings.TrimSpace(resultContext.GetFocusRunId()),
		focusMode:            strings.TrimSpace(resultContext.GetFocusMode()),
		focusReleaseID:       strings.TrimSpace(resultContext.GetFocusReleaseId()),
		targetURL:            strings.TrimSpace(resultContext.GetTargetUrl()),
		assetIDs:             make(map[string]struct{}),
		riskIDs:              make(map[string]struct{}),
		targets:              make(map[string]struct{}),
		requiredResultKinds:  make(map[string]struct{}),
		publishedResultKinds: make(map[string]struct{}),
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
	sink aiFocusResultSink
}

func newAISessionResultSinkProxy(sink aiFocusResultSink) *aiSessionResultSinkProxy {
	if sink == nil {
		return nil
	}
	return &aiSessionResultSinkProxy{sink: sink}
}

func (p *aiSessionResultSinkProxy) Set(sink aiFocusResultSink) {
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
) (aiFocusResultReceipt, error) {
	if p == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	sink := p.sink
	if sink == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	return sink.SubmitRisk(ctx, risk)
}

func (p *aiSessionResultSinkProxy) SubmitAsset(
	ctx context.Context,
	asset aiFocusAssetResult,
) (aiFocusResultReceipt, error) {
	if p == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	sink := p.sink
	if sink == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	assetSink, ok := sink.(aiFocusAssetResultSink)
	if !ok {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink does not accept assets")
	}
	return assetSink.SubmitAsset(ctx, asset)
}

func (p *aiSessionResultSinkProxy) SubmitCodeFinding(
	ctx context.Context,
	kind string,
	finding aiFocusCodeFinding,
) (aiFocusResultReceipt, error) {
	if p == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	sink, ok := p.sink.(aiFocusCodeResultSink)
	if !ok {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink does not accept code findings")
	}
	return sink.SubmitCodeFinding(ctx, kind, finding)
}

func (p *aiSessionResultSinkProxy) SubmitCodeAuditReport(
	ctx context.Context,
	kind string,
	report aiFocusCodeAuditReport,
) (aiFocusResultReceipt, error) {
	if p == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink is unavailable")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	sink, ok := p.sink.(aiFocusCodeResultSink)
	if !ok {
		return aiFocusResultReceipt{}, fmt.Errorf("ai session result sink does not accept code audit reports")
	}
	return sink.SubmitCodeAuditReport(ctx, kind, report)
}

func (p *aiSessionResultSinkProxy) bindFocusExecutionContract(contract *legionFocusExecutionContract) error {
	if p == nil {
		return fmt.Errorf("ai session result sink is unavailable")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	binder, ok := p.sink.(aiFocusExecutionContractBinder)
	if !ok {
		return fmt.Errorf("ai session result sink does not accept a Focus execution contract")
	}
	return binder.bindFocusExecutionContract(contract)
}

func (p *aiSessionResultSinkProxy) Succeed(ctx context.Context, resultJSON []byte) error {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if lifecycle, ok := p.sink.(aiFocusResultLifecycle); ok {
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
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if lifecycle, ok := p.sink.(aiFocusResultLifecycle); ok {
		return lifecycle.Fail(ctx, code, message, detailJSON)
	}
	return nil
}

func (p *aiSessionResultSinkProxy) Cancel(ctx context.Context, reason string) error {
	if p == nil {
		return nil
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if lifecycle, ok := p.sink.(aiFocusResultLifecycle); ok {
		return lifecycle.Cancel(ctx, reason)
	}
	return nil
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
	targetURL := strings.TrimSpace(resultContext.GetTargetUrl())
	parsedTarget, targetErr := normalizeServerFocusURL(targetURL)
	isWorkspaceSentinel := targetErr == nil && strings.EqualFold(parsedTarget.Hostname(), "workspace.invalid")
	switch {
	case ref.CommandID == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result bind command_id is required")
	case strings.TrimSpace(resultContext.GetFocusRunId()) == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result focus_run_id is required")
	case strings.TrimSpace(resultContext.GetFocusMode()) == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result focus_mode is required")
	case strings.TrimSpace(resultContext.GetFocusMode()) != legionAIConversationAuditResultMode &&
		strings.TrimSpace(resultContext.GetFocusReleaseId()) == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result focus_release_id is required")
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
	case strings.TrimSpace(resultContext.GetFocusMode()) == legionAIConversationAuditResultMode &&
		strings.TrimSpace(resultContext.GetExecutionMode()) != legionAIConversationExecutionMode:
		return jobExecutionRef{}, fmt.Errorf(
			"unsupported ai conversation result execution_mode %q",
			resultContext.GetExecutionMode(),
		)
	case strings.TrimSpace(resultContext.GetFocusMode()) != legionAIConversationAuditResultMode &&
		strings.TrimSpace(resultContext.GetExecutionMode()) != "single_run":
		return jobExecutionRef{}, fmt.Errorf(
			"unsupported ai focus result execution_mode %q",
			resultContext.GetExecutionMode(),
		)
	case strings.TrimSpace(resultContext.GetTargetUrl()) == "":
		return jobExecutionRef{}, fmt.Errorf("ai focus result target_url is required")
	case targetErr != nil:
		return jobExecutionRef{}, fmt.Errorf("ai focus result target_url: %w", targetErr)
	case strings.TrimSpace(resultContext.GetFocusMode()) == legionAIConversationAuditResultMode && isWorkspaceSentinel:
		return jobExecutionRef{}, fmt.Errorf("workspace.invalid sentinel is reserved for server-managed Professional Tasks")
	default:
		return ref, nil
	}
}

func (s *legionAIFocusResultSink) SubmitCodeFinding(
	ctx context.Context,
	kind string,
	finding aiFocusCodeFinding,
) (aiFocusResultReceipt, error) {
	kind = strings.TrimSpace(kind)
	finding.WorkspaceID = strings.TrimSpace(finding.WorkspaceID)
	finding.LockedRevision = ""
	finding.SourceSHA256 = ""
	finding.File = strings.TrimSpace(finding.File)
	finding.CWE = strings.ToUpper(strings.TrimSpace(finding.CWE))
	finding.VulnerabilityType = strings.TrimSpace(finding.VulnerabilityType)
	finding.Category = strings.TrimSpace(finding.Category)
	finding.Module = strings.TrimSpace(finding.Module)
	finding.Severity = strings.ToLower(strings.TrimSpace(finding.Severity))
	finding.VerificationStatus = strings.ToLower(strings.TrimSpace(finding.VerificationStatus))
	finding.Title = strings.TrimSpace(finding.Title)
	finding.Evidence = strings.TrimSpace(finding.Evidence)
	finding.Description = strings.TrimSpace(finding.Description)
	finding.DataFlow = strings.TrimSpace(finding.DataFlow)
	finding.ExploitScenario = strings.TrimSpace(finding.ExploitScenario)
	finding.Recommendation = strings.TrimSpace(finding.Recommendation)
	finding.DedupeKey = ""
	cleanedFile, err := cleanLegionCodeRelativePath(finding.File, false)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding file: %w", err)
	}
	finding.File = cleanedFile
	switch {
	case kind == "":
		return aiFocusResultReceipt{}, fmt.Errorf("finding result kind is required")
	case finding.WorkspaceID == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding workspace_id is required")
	case finding.StartLine <= 0 || finding.EndLine < finding.StartLine:
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding line range is invalid")
	case finding.StartColumn < 0 || finding.EndColumn < 0 ||
		(finding.StartLine == finding.EndLine && finding.EndColumn > 0 && finding.StartColumn > finding.EndColumn):
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding column range is invalid")
	case !legionCodeFindingCWEPattern.MatchString(finding.CWE):
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding cwe must use CWE-<number>")
	case finding.VulnerabilityType == "" || finding.Category == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding vulnerability_type and category are required")
	case len(finding.VulnerabilityType) > 256 || len(finding.Category) > 128 || len(finding.Module) > 256:
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding classification fields are too long")
	case !isLegionCodeFindingSeverity(finding.Severity):
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding severity %q is unsupported", finding.Severity)
	case math.IsNaN(finding.Confidence) || math.IsInf(finding.Confidence, 0) || finding.Confidence <= 0 || finding.Confidence > 1:
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding confidence must be greater than 0 and at most 1")
	case finding.VerificationStatus != "confirmed" && finding.VerificationStatus != "uncertain":
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding verification_status must be confirmed or uncertain")
	case finding.Title == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding title is required")
	case len(finding.Title) > 512:
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding title exceeds 512 bytes")
	case finding.Description == "" || finding.Evidence == "" || finding.DataFlow == "" || finding.ExploitScenario == "" || finding.Recommendation == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding description, evidence, data_flow, exploit_scenario, and recommendation are required")
	case len(finding.Description) > maxInlineFocusRiskFieldBytes ||
		len(finding.Evidence) > maxInlineFocusRiskFieldBytes ||
		len(finding.DataFlow) > maxInlineFocusRiskFieldBytes ||
		len(finding.ExploitScenario) > maxInlineFocusRiskFieldBytes ||
		len(finding.Recommendation) > maxInlineFocusRiskFieldBytes:
		return aiFocusResultReceipt{}, fmt.Errorf("ai code finding evidence fields exceed %d bytes", maxInlineFocusRiskFieldBytes)
	}
	lockedRevision, sourceSHA256, err := s.codeWorkspaceEvidence()
	if err != nil {
		return aiFocusResultReceipt{}, err
	}
	finding.LockedRevision = lockedRevision
	finding.SourceSHA256 = sourceSHA256
	finding.DedupeKey = legionCodeFindingDedupeKey(finding)
	target, err := legionCodeFindingTarget(s.targetURL, finding.WorkspaceID, finding.File)
	if err != nil {
		return aiFocusResultReceipt{}, err
	}
	finding.Target = target
	raw, err := json.Marshal(finding)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("marshal ai code finding: %w", err)
	}
	eventID := focusRiskEventID(s.ref.JobID, finding.DedupeKey)
	if err := s.publisher.PublishRiskWithEventID(
		ctx,
		s.ref,
		eventID,
		kind,
		finding.Title,
		target,
		finding.Severity,
		finding.DedupeKey,
		raw,
	); err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("publish ai code finding: %w", err)
	}
	s.recordRisk(eventID, target)
	s.mu.Lock()
	s.publishedResultKinds[kind] = struct{}{}
	s.mu.Unlock()
	return aiFocusResultReceipt{ResultID: eventID, DedupeKey: finding.DedupeKey, BackendID: s.ref.JobID}, nil
}

func (s *legionAIFocusResultSink) bindCodeWorkspaceEvidence(lockedRevision string, sourceSHA256 string) error {
	if s == nil {
		return fmt.Errorf("ai code finding result sink is unavailable")
	}
	lockedRevision = strings.TrimSpace(lockedRevision)
	sourceSHA256 = strings.ToLower(strings.TrimSpace(sourceSHA256))
	if len(sourceSHA256) != sha256.Size*2 || !isLowerHex(sourceSHA256) {
		return fmt.Errorf("ai code finding source_sha256 is invalid")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.codeWorkspaceSHA256 != "" &&
		(s.codeWorkspaceLockedRev != lockedRevision || s.codeWorkspaceSHA256 != sourceSHA256) {
		return fmt.Errorf("ai code finding source evidence is already bound")
	}
	s.codeWorkspaceLockedRev = lockedRevision
	s.codeWorkspaceSHA256 = sourceSHA256
	return nil
}

func (s *legionAIFocusResultSink) codeWorkspaceEvidence() (string, string, error) {
	if s == nil {
		return "", "", fmt.Errorf("ai code finding result sink is unavailable")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.codeWorkspaceSHA256 == "" {
		return "", "", fmt.Errorf("ai code finding source evidence is not bound")
	}
	return s.codeWorkspaceLockedRev, s.codeWorkspaceSHA256, nil
}

func (s *legionAIFocusResultSink) SubmitCodeAuditReport(
	ctx context.Context,
	kind string,
	report aiFocusCodeAuditReport,
) (aiFocusResultReceipt, error) {
	kind = strings.TrimSpace(kind)
	report.WorkspaceID = strings.TrimSpace(report.WorkspaceID)
	report.Markdown = strings.TrimSpace(report.Markdown)
	if kind == "" {
		return aiFocusResultReceipt{}, fmt.Errorf("report result kind is required")
	}
	if report.WorkspaceID == "" || report.Markdown == "" {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code audit report workspace_id and markdown are required")
	}
	if len(report.Markdown) > maxInlineCodeAuditReportBytes {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code audit report markdown exceeds %d bytes", maxInlineCodeAuditReportBytes)
	}
	if len(report.StructuredSummary) == 0 || len(report.StructuredSummary) > maxInlineCodeAuditSummaryBytes {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code audit report structured_summary must contain at most %d bytes", maxInlineCodeAuditSummaryBytes)
	}
	if !json.Valid(report.StructuredSummary) || string(report.StructuredSummary) == "null" {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code audit report structured_summary must be valid JSON")
	}
	var structured map[string]any
	if err := json.Unmarshal(report.StructuredSummary, &structured); err != nil || structured == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code audit report structured_summary must be a JSON object")
	}
	canonicalSummary, err := json.Marshal(structured)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("canonicalize ai code audit report structured_summary: %w", err)
	}
	report.StructuredSummary = canonicalSummary
	expectedTarget, err := legionCodeWorkspaceSentinel(report.WorkspaceID)
	if err != nil {
		return aiFocusResultReceipt{}, err
	}
	actualTarget, err := normalizeServerFocusURL(s.targetURL)
	if err != nil || actualTarget.String() != expectedTarget {
		return aiFocusResultReceipt{}, fmt.Errorf("ai code audit report workspace_id does not match the authorized target")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("marshal ai code audit report: %w", err)
	}
	eventID := focusResultReportEventID(s.ref.JobID, kind)
	if err := s.publisher.PublishReportWithEventID(ctx, s.ref, eventID, kind, raw); err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("publish ai code audit report: %w", err)
	}
	s.mu.Lock()
	s.publishedResultKinds[kind] = struct{}{}
	s.mu.Unlock()
	return aiFocusResultReceipt{ResultID: eventID, DedupeKey: kind, BackendID: s.ref.JobID}, nil
}

func (s *legionAIFocusResultSink) bindFocusExecutionContract(contract *legionFocusExecutionContract) error {
	if s == nil || contract == nil {
		return fmt.Errorf("Focus execution contract is required")
	}
	required := make(map[string]struct{})
	for _, result := range contract.Results {
		if result.Required {
			required[result.Kind] = struct{}{}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.requiredResultKinds) > 0 {
		if !sameStringSet(s.requiredResultKinds, required) {
			return fmt.Errorf("Focus execution result contract is already bound")
		}
		return nil
	}
	s.requiredResultKinds = required
	return nil
}

func (s *legionAIFocusResultSink) SubmitRisk(
	ctx context.Context,
	risk *schema.Risk,
) (aiFocusResultReceipt, error) {
	if risk == nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus risk is required")
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
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus risk title is required")
	case normalized.RiskType == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus risk type is required")
	case target == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus risk target is required")
	}
	target, err := normalizeServerFocusResultTarget(s.targetURL, target)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus risk target: %w", err)
	}
	normalized.Url = target

	normalized.QuotedRequest = truncateFocusRiskField(normalized.QuotedRequest)
	normalized.QuotedResponse = truncateFocusRiskField(normalized.QuotedResponse)
	normalized.Details = truncateFocusRiskField(normalized.Details)
	normalized.Payload = truncateFocusRiskField(normalized.Payload)

	raw, err := json.Marshal(&normalized)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("marshal ai focus risk: %w", err)
	}
	dedupeKey := focusRiskDedupeKey(&normalized, target)
	eventID := focusRiskEventID(s.ref.JobID, dedupeKey)
	if err := s.publisher.PublishRiskWithEventID(
		ctx,
		s.ref,
		eventID,
		normalized.RiskType,
		normalized.Title,
		target,
		normalized.Severity,
		dedupeKey,
		raw,
	); err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("publish ai focus risk: %w", err)
	}
	s.recordRisk(eventID, target)
	return aiFocusResultReceipt{
		ResultID:  eventID,
		DedupeKey: dedupeKey,
		BackendID: s.ref.JobID,
	}, nil
}

func (s *legionAIFocusResultSink) SubmitAsset(
	ctx context.Context,
	asset aiFocusAssetResult,
) (aiFocusResultReceipt, error) {
	asset.Kind = strings.TrimSpace(asset.Kind)
	asset.Title = strings.TrimSpace(asset.Title)
	asset.Target = strings.TrimSpace(asset.Target)
	asset.IdentityKey = strings.TrimSpace(asset.IdentityKey)
	switch {
	case asset.Kind == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset kind is required")
	case asset.Title == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset title is required")
	case asset.Target == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset target is required")
	case asset.IdentityKey == "":
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset identity_key is required")
	case len(asset.Payload) == 0:
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset payload is required")
	case len(asset.Payload) > maxInlineFocusAssetBytes:
		return aiFocusResultReceipt{}, fmt.Errorf(
			"ai focus asset payload exceeds %d bytes",
			maxInlineFocusAssetBytes,
		)
	case !json.Valid(asset.Payload):
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset payload must be valid JSON")
	}
	normalizedTarget, err := normalizeServerFocusResultTarget(s.targetURL, asset.Target)
	if err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("ai focus asset target: %w", err)
	}
	asset.Target = normalizedTarget

	eventID := focusAssetEventID(s.ref.JobID, asset.Kind, asset.IdentityKey)
	if err := s.publisher.PublishAssetWithEventID(
		ctx,
		s.ref,
		eventID,
		asset.Kind,
		asset.Title,
		asset.Target,
		asset.IdentityKey,
		asset.Payload,
	); err != nil {
		return aiFocusResultReceipt{}, fmt.Errorf("publish ai focus asset: %w", err)
	}
	s.recordAsset(eventID, asset.Target)
	return aiFocusResultReceipt{
		ResultID:  eventID,
		DedupeKey: asset.IdentityKey,
		BackendID: s.ref.JobID,
	}, nil
}

func (s *legionAIFocusResultSink) Succeed(
	ctx context.Context,
	resultJSON []byte,
) error {
	s.mu.Lock()
	missingRequired := make([]string, 0)
	for kind := range s.requiredResultKinds {
		if _, published := s.publishedResultKinds[kind]; !published {
			missingRequired = append(missingRequired, kind)
		}
	}
	s.mu.Unlock()
	if len(missingRequired) > 0 {
		sort.Strings(missingRequired)
		return fmt.Errorf("Focus execution cannot complete without required results: %s", strings.Join(missingRequired, ", "))
	}
	result := make(map[string]any)
	if len(resultJSON) > 0 {
		var decoded map[string]any
		if err := json.Unmarshal(resultJSON, &decoded); err == nil && decoded != nil {
			for key, value := range decoded {
				result[key] = value
			}
		} else {
			result["result_payload"] = string(resultJSON)
		}
	}
	assetIDs, riskIDs, targets := s.resultSummarySnapshot()
	result["schema_version"] = "legion.focus-run-result.v1"
	result["focus_run_id"] = s.focusRunID
	result["focus_mode"] = s.focusMode
	result["focus_release_id"] = s.focusReleaseID
	result["target_url"] = s.targetURL
	result["status"] = "succeeded"
	result["asset_count"] = len(assetIDs)
	result["risk_count"] = len(riskIDs)
	result["asset_result_ids"] = assetIDs
	result["risk_result_ids"] = riskIDs
	result["targets"] = targets
	reportJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal ai focus summary report: %w", err)
	}
	if err := s.publisher.PublishReportWithEventID(
		ctx,
		s.ref,
		focusSummaryReportEventID(s.ref.JobID),
		"ai_focus_summary",
		reportJSON,
	); err != nil {
		return fmt.Errorf("publish ai focus summary report: %w", err)
	}
	return s.publisher.PublishSucceeded(ctx, s.ref, result)
}

func (s *legionAIFocusResultSink) recordAsset(eventID string, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assetIDs[strings.TrimSpace(eventID)] = struct{}{}
	if target = strings.TrimSpace(target); target != "" {
		s.targets[target] = struct{}{}
	}
}

func (s *legionAIFocusResultSink) recordRisk(eventID string, target string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.riskIDs[strings.TrimSpace(eventID)] = struct{}{}
	if target = strings.TrimSpace(target); target != "" {
		s.targets[target] = struct{}{}
	}
}

func (s *legionAIFocusResultSink) resultSummarySnapshot() ([]string, []string, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	assets := sortedFocusResultKeys(s.assetIDs)
	risks := sortedFocusResultKeys(s.riskIDs)
	targets := sortedFocusResultKeys(s.targets)
	return assets, risks, targets
}

func sortedFocusResultKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}

func (s *legionAIFocusResultSink) Fail(
	ctx context.Context,
	code string,
	message string,
	detailJSON []byte,
) error {
	detail := map[string]string{
		"focus_run_id":     s.focusRunID,
		"focus_mode":       s.focusMode,
		"focus_release_id": s.focusReleaseID,
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

func legionCodeFindingDedupeKey(finding aiFocusCodeFinding) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"ai_code_finding.v1",
		finding.File,
		fmt.Sprintf("%d", finding.StartLine),
		fmt.Sprintf("%d", finding.EndLine),
		strings.ToUpper(strings.TrimSpace(finding.CWE)),
		normalizeLegionCodeFindingIdentityText(finding.VulnerabilityType),
		normalizeLegionCodeFindingIdentityText(finding.Category),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func normalizeLegionCodeFindingIdentityText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func focusAssetEventID(
	jobID string,
	assetKind string,
	identityKey string,
) string {
	name := strings.Join([]string{
		strings.TrimSpace(jobID),
		strings.ToLower(strings.TrimSpace(assetKind)),
		strings.TrimSpace(identityKey),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func focusRiskEventID(jobID string, dedupeKey string) string {
	name := strings.Join([]string{
		strings.TrimSpace(jobID),
		"risk",
		strings.TrimSpace(dedupeKey),
	}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func focusSummaryReportEventID(jobID string) string {
	name := strings.Join([]string{strings.TrimSpace(jobID), "ai_focus_summary"}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func focusResultReportEventID(jobID string, kind string) string {
	name := strings.Join([]string{strings.TrimSpace(jobID), strings.TrimSpace(kind)}, "\x00")
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(name)).String()
}

func sameStringSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key := range left {
		if _, ok := right[key]; !ok {
			return false
		}
	}
	return true
}

func isLegionCodeFindingSeverity(severity string) bool {
	switch severity {
	case "critical", "high", "medium", "low", "info":
		return true
	default:
		return false
	}
}

func legionCodeWorkspaceSentinel(workspaceID string) (string, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if !legionCodeWorkspaceIDPattern.MatchString(workspaceID) {
		return "", fmt.Errorf("source_workspace workspace_id is invalid")
	}
	return (&url.URL{Scheme: "https", Host: "workspace.invalid", Path: "/" + workspaceID + "/"}).String(), nil
}

func legionCodeFindingTarget(authorizedTarget, workspaceID, filename string) (string, error) {
	expected, err := legionCodeWorkspaceSentinel(workspaceID)
	if err != nil {
		return "", err
	}
	authorized, err := normalizeServerFocusURL(authorizedTarget)
	if err != nil {
		return "", err
	}
	if authorized.String() != expected {
		return "", fmt.Errorf("ai code finding workspace_id does not match the authorized target")
	}
	cleaned, err := cleanLegionCodeRelativePath(filename, false)
	if err != nil {
		return "", err
	}
	authorized.Path = strings.TrimSuffix(authorized.Path, "/") + "/" + cleaned
	authorized.RawPath = ""
	return authorized.String(), nil
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
