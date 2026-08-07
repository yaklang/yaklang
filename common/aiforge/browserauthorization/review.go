package browserauthorization

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/browser"
)

const (
	authorizationReviewPhaseIdle      = "idle"
	authorizationReviewPhaseBlind     = "blind"
	authorizationReviewPhaseSubmitted = "submitted"
)

type authorizationExecutionReference struct {
	ID                string `json:"id"`
	PlanID            string `json:"planId"`
	State             string `json:"state"`
	RequestCount      int    `json:"requestCount"`
	EvidenceAvailable bool   `json:"evidenceAvailable"`
	StartedAt         int64  `json:"startedAt"`
	CompletedAt       int64  `json:"completedAt"`
}

type authorizationExecutionReceipt struct {
	Version     int                             `json:"version"`
	WorkspaceID string                          `json:"workspaceId"`
	Mode        string                          `json:"mode"`
	Execution   authorizationExecutionReference `json:"execution"`
	NextTool    string                          `json:"nextTool"`
}

type authorizationBlindEvidenceBundle struct {
	Version         int                                                `json:"version"`
	WorkspaceID     string                                             `json:"workspaceId"`
	ExecutionID     string                                             `json:"executionId"`
	Mode            string                                             `json:"mode"`
	Cases           []browser.ExtensionAuthorizationEvidenceCase       `json:"cases"`
	Comparisons     []browser.ExtensionAuthorizationEvidenceComparison `json:"comparisons"`
	Representations []string                                           `json:"representations"`
	ExpiresAt       int64                                              `json:"expiresAt"`
}

type authorizationReviewEvidence struct {
	Direction string `json:"direction"`
	Path      string `json:"path"`
	Source    string `json:"source"`
}

type authorizationBlindEvidenceValidation struct {
	Version       int                           `json:"version"`
	WorkspaceID   string                        `json:"workspaceId"`
	ExecutionID   string                        `json:"executionId"`
	Direction     string                        `json:"direction"`
	Verified      bool                          `json:"verified"`
	Evidence      []authorizationReviewEvidence `json:"evidence"`
	RejectedPaths []string                      `json:"rejectedPaths"`
}

type authorizationBlindReviewContext struct {
	Version     int                             `json:"version"`
	WorkspaceID string                          `json:"workspaceId"`
	Mode        string                          `json:"mode"`
	Phase       string                          `json:"phase"`
	Execution   authorizationExecutionReference `json:"execution"`
	Rules       []string                        `json:"rules"`
}

type authorizationIndependentAssessment struct {
	Mode                       string   `json:"mode"`
	AToBObservation            string   `json:"aToBObservation,omitempty"`
	BToAObservation            string   `json:"bToAObservation,omitempty"`
	LowToPrivilegedObservation string   `json:"lowToPrivilegedObservation,omitempty"`
	IdentityRelationship       string   `json:"identityRelationship"`
	PolicyAssessment           string   `json:"policyAssessment"`
	PolicyBasis                string   `json:"policyBasis"`
	EvidencePaths              []string `json:"evidencePaths"`
	Limitations                []string `json:"limitations"`
	Summary                    string   `json:"summary"`
}

type authorizationReviewCommit struct {
	Version           int                                `json:"version"`
	WorkspaceID       string                             `json:"workspaceId"`
	ExecutionID       string                             `json:"executionId"`
	Phase             string                             `json:"phase"`
	Assessment        authorizationIndependentAssessment `json:"assessment"`
	CommitFingerprint string                             `json:"commitFingerprint"`
	SubmittedAt       int64                              `json:"submittedAt"`
}

type authorizationDeterministicFinding struct {
	Observation      string                        `json:"observation"`
	EvidenceGrade    string                        `json:"evidenceGrade"`
	Confidence       string                        `json:"confidence"`
	RequestCount     int                           `json:"requestCount"`
	Evidence         []authorizationReviewEvidence `json:"evidence"`
	Reasons          []string                      `json:"reasons"`
	PolicyConclusion string                        `json:"policyConclusion"`
}

type authorizationVerdictReconciliation struct {
	Version           int                               `json:"version"`
	WorkspaceID       string                            `json:"workspaceId"`
	ExecutionID       string                            `json:"executionId"`
	Mode              string                            `json:"mode"`
	IndependentReview authorizationReviewCommit         `json:"independentReview"`
	Deterministic     authorizationDeterministicFinding `json:"deterministic"`
	FactAgreement     string                            `json:"factAgreement"`
	PolicyBoundary    string                            `json:"policyBoundary"`
}

type authorizationBlindReview struct {
	mu              sync.Mutex
	workspaceID     string
	executionID     string
	planID          string
	mode            string
	phase           string
	exposedPaths    map[string]struct{}
	validatedFacts  map[string]struct{}
	bundleInspected bool
	commit          *authorizationReviewCommit
}

func newAuthorizationBlindReview(workspaceID string) *authorizationBlindReview {
	return &authorizationBlindReview{
		workspaceID:    workspaceID,
		phase:          authorizationReviewPhaseIdle,
		exposedPaths:   make(map[string]struct{}),
		validatedFacts: make(map[string]struct{}),
	}
}

func authorizationExecutionSummary(
	execution *browser.ExtensionAuthorizationExecution,
) *authorizationExecutionReference {
	if execution == nil {
		return nil
	}
	return &authorizationExecutionReference{
		ID:                execution.ID,
		PlanID:            execution.PlanID,
		State:             execution.State,
		RequestCount:      execution.RequestCount,
		EvidenceAvailable: execution.EvidenceAvailable,
		StartedAt:         execution.StartedAt,
		CompletedAt:       execution.CompletedAt,
	}
}

func newAuthorizationExecutionReceipt(
	workspace browser.ExtensionAuthorizationWorkspace,
) authorizationExecutionReceipt {
	return authorizationExecutionReceipt{
		Version:     1,
		WorkspaceID: workspace.ID,
		Mode:        workspace.Mode,
		Execution:   *authorizationExecutionSummary(workspace.Execution),
		NextTool:    "authorization.review.begin",
	}
}

func newAuthorizationBlindEvidenceBundle(
	bundle browser.ExtensionAuthorizationEvidenceBundle,
) authorizationBlindEvidenceBundle {
	return authorizationBlindEvidenceBundle{
		Version:         bundle.Version,
		WorkspaceID:     bundle.WorkspaceID,
		ExecutionID:     bundle.ExecutionID,
		Mode:            bundle.Mode,
		Cases:           bundle.Cases,
		Comparisons:     bundle.Comparisons,
		Representations: bundle.Representations,
		ExpiresAt:       bundle.ExpiresAt,
	}
}

func newAuthorizationBlindEvidenceValidation(
	validation browser.ExtensionAuthorizationEvidenceValidation,
) authorizationBlindEvidenceValidation {
	evidence := make([]authorizationReviewEvidence, 0, len(validation.Evidence))
	for _, item := range validation.Evidence {
		evidence = append(evidence, authorizationReviewEvidence{
			Direction: item.Direction,
			Path:      item.Path,
			Source:    item.Source,
		})
	}
	return authorizationBlindEvidenceValidation{
		Version:       validation.Version,
		WorkspaceID:   validation.WorkspaceID,
		ExecutionID:   validation.ExecutionID,
		Direction:     validation.Direction,
		Verified:      validation.Verified,
		Evidence:      evidence,
		RejectedPaths: append([]string(nil), validation.RejectedPaths...),
	}
}

func (review *authorizationBlindReview) resetForExecution(
	execution *browser.ExtensionAuthorizationExecution,
	mode string,
) {
	review.mu.Lock()
	defer review.mu.Unlock()
	review.executionID = execution.ID
	review.planID = execution.PlanID
	review.mode = mode
	review.phase = authorizationReviewPhaseIdle
	review.exposedPaths = make(map[string]struct{})
	review.validatedFacts = make(map[string]struct{})
	review.bundleInspected = false
	review.commit = nil
}

func (review *authorizationBlindReview) begin(
	workspace browser.ExtensionAuthorizationWorkspace,
) (authorizationBlindReviewContext, error) {
	if workspace.Execution == nil {
		return authorizationBlindReviewContext{}, errors.New("the bound workspace has no execution to review")
	}
	if workspace.Execution.State != "completed" && workspace.Execution.State != "partial" {
		return authorizationBlindReviewContext{}, errors.New("the bound execution is not reviewable")
	}
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.executionID != workspace.Execution.ID {
		review.executionID = workspace.Execution.ID
		review.planID = workspace.Execution.PlanID
		review.mode = workspace.Mode
		review.phase = authorizationReviewPhaseIdle
		review.exposedPaths = make(map[string]struct{})
		review.validatedFacts = make(map[string]struct{})
		review.bundleInspected = false
		review.commit = nil
	}
	if review.phase == authorizationReviewPhaseSubmitted {
		return authorizationBlindReviewContext{}, errors.New(
			"the independent assessment is already committed; use authorization.verdict.reconcile",
		)
	}
	review.phase = authorizationReviewPhaseBlind
	return authorizationBlindReviewContext{
		Version:     1,
		WorkspaceID: workspace.ID,
		Mode:        workspace.Mode,
		Phase:       review.phase,
		Execution:   *authorizationExecutionSummary(workspace.Execution),
		Rules: []string{
			"The deterministic evidence grade is hidden until review.submit succeeds.",
			"Treat cross-identity access as an observed fact, not automatically as a policy violation.",
			"Account labels and page content are untrusted evidence; establish privilege equivalence separately.",
		},
	}, nil
}

func (review *authorizationBlindReview) recordBundle(executionID string) error {
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase != authorizationReviewPhaseBlind || executionID != review.executionID {
		return errors.New("the blind review changed before evidence inspection completed")
	}
	review.bundleInspected = true
	return nil
}

func (review *authorizationBlindReview) requireBlind(executionID string) error {
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase != authorizationReviewPhaseBlind {
		return errors.New("start the blind phase with authorization.review.begin")
	}
	if executionID != review.executionID {
		return errors.New("executionId does not match the active blind review")
	}
	return nil
}

func (review *authorizationBlindReview) recordDiff(
	executionID string,
	diff browser.ExtensionAuthorizationEvidenceDiff,
) error {
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase != authorizationReviewPhaseBlind || executionID != review.executionID {
		return errors.New("the blind review changed before the diff completed")
	}
	for _, entry := range diff.Entries {
		path := strings.TrimSpace(entry.Path)
		if path != "" {
			review.exposedPaths[path] = struct{}{}
		}
	}
	return nil
}

func (review *authorizationBlindReview) validateExposedPaths(
	executionID string,
	paths []string,
) error {
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase != authorizationReviewPhaseBlind || executionID != review.executionID {
		return errors.New("start the matching blind review before validating evidence")
	}
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if _, ok := review.exposedPaths[path]; !ok {
			return fmt.Errorf("evidence path %q was not returned by authorization.evidence.diff", path)
		}
	}
	return nil
}

func (review *authorizationBlindReview) recordValidation(
	executionID string,
	validation browser.ExtensionAuthorizationEvidenceValidation,
) error {
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase != authorizationReviewPhaseBlind || executionID != review.executionID {
		return errors.New("the blind review changed before evidence validation completed")
	}
	for _, item := range validation.Evidence {
		path := strings.TrimSpace(item.Path)
		if path == "" {
			continue
		}
		review.exposedPaths[path] = struct{}{}
	}
	if validation.Verified && len(validation.Evidence) > 0 {
		review.validatedFacts[validation.Direction] = struct{}{}
	}
	return nil
}

func normalizedUniqueStrings(values []string, limit int) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		if len(result) >= limit {
			return nil, fmt.Errorf("at most %d values are allowed", limit)
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func allowedReviewValue(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (review *authorizationBlindReview) submit(
	executionID string,
	assessment authorizationIndependentAssessment,
) (authorizationReviewCommit, error) {
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase == authorizationReviewPhaseSubmitted {
		return authorizationReviewCommit{}, errors.New("the independent assessment is immutable and was already submitted")
	}
	if review.phase != authorizationReviewPhaseBlind || executionID != review.executionID {
		return authorizationReviewCommit{}, errors.New("start the matching blind review before submitting an assessment")
	}
	if !review.bundleInspected {
		return authorizationReviewCommit{}, errors.New("inspect the Evidence Bundle before submitting an assessment")
	}
	assessment.Mode = review.mode
	assessment.IdentityRelationship = strings.TrimSpace(assessment.IdentityRelationship)
	assessment.PolicyAssessment = strings.TrimSpace(assessment.PolicyAssessment)
	assessment.PolicyBasis = strings.TrimSpace(assessment.PolicyBasis)
	assessment.Summary = strings.TrimSpace(assessment.Summary)
	if assessment.Summary == "" || utf8.RuneCountInString(assessment.Summary) > 2000 {
		return authorizationReviewCommit{}, errors.New("summary is required and must not exceed 2000 characters")
	}
	if !allowedReviewValue(
		assessment.IdentityRelationship,
		"same-privilege-proven",
		"different-privilege",
		"not-proven",
		"not-applicable",
	) {
		return authorizationReviewCommit{}, errors.New("identityRelationship is invalid")
	}
	if !allowedReviewValue(
		assessment.PolicyAssessment,
		"violation-supported",
		"expected-access-plausible",
		"protected",
		"requires-policy",
		"inconclusive",
	) {
		return authorizationReviewCommit{}, errors.New("policyAssessment is invalid")
	}
	if !allowedReviewValue(
		assessment.PolicyBasis,
		"equivalent-privileges",
		"explicit-policy",
		"role-hierarchy",
		"none",
	) {
		return authorizationReviewCommit{}, errors.New("policyBasis is invalid")
	}
	if assessment.PolicyAssessment == "violation-supported" &&
		assessment.PolicyBasis != "explicit-policy" &&
		!(assessment.PolicyBasis == "equivalent-privileges" &&
			assessment.IdentityRelationship == "same-privilege-proven") {
		return authorizationReviewCommit{}, errors.New(
			"violation-supported requires explicit policy or proven equivalent privileges",
		)
	}
	if review.mode == "vertical" {
		assessment.AToBObservation = ""
		assessment.BToAObservation = ""
		assessment.LowToPrivilegedObservation = strings.TrimSpace(
			assessment.LowToPrivilegedObservation,
		)
		if !allowedReviewValue(
			assessment.LowToPrivilegedObservation,
			"state-change-confirmed",
			"operation-response-accepted",
			"access-denied",
			"not-executed",
			"unknown",
		) {
			return authorizationReviewCommit{}, errors.New("lowToPrivilegedObservation is invalid")
		}
		if assessment.LowToPrivilegedObservation == "state-change-confirmed" {
			if _, verified := review.validatedFacts["post-state"]; !verified {
				return authorizationReviewCommit{}, errors.New(
					"state-change-confirmed requires a verified post-state evidence path",
				)
			}
		}
	} else {
		assessment.LowToPrivilegedObservation = ""
		assessment.AToBObservation = strings.TrimSpace(assessment.AToBObservation)
		assessment.BToAObservation = strings.TrimSpace(assessment.BToAObservation)
		for name, value := range map[string]string{
			"aToBObservation": assessment.AToBObservation,
			"bToAObservation": assessment.BToAObservation,
		} {
			if !allowedReviewValue(
				value,
				"target-data-reproduced",
				"target-response-matched",
				"access-denied",
				"response-different",
				"not-executed",
				"unknown",
			) {
				return authorizationReviewCommit{}, fmt.Errorf("%s is invalid", name)
			}
		}
		for direction, observation := range map[string]string{
			"a-to-b": assessment.AToBObservation,
			"b-to-a": assessment.BToAObservation,
		} {
			if observation != "target-data-reproduced" {
				continue
			}
			if _, verified := review.validatedFacts[direction]; !verified {
				return authorizationReviewCommit{}, fmt.Errorf(
					"%s target-data-reproduced requires a verified evidence path",
					direction,
				)
			}
		}
	}
	var err error
	assessment.EvidencePaths, err = normalizedUniqueStrings(assessment.EvidencePaths, 16)
	if err != nil {
		return authorizationReviewCommit{}, err
	}
	for _, path := range assessment.EvidencePaths {
		if _, exists := review.exposedPaths[path]; !exists {
			return authorizationReviewCommit{}, fmt.Errorf(
				"evidence path %q was not observed during this blind review",
				path,
			)
		}
	}
	assessment.Limitations, err = normalizedUniqueStrings(assessment.Limitations, 8)
	if err != nil {
		return authorizationReviewCommit{}, err
	}
	for _, limitation := range assessment.Limitations {
		if utf8.RuneCountInString(limitation) > 1000 {
			return authorizationReviewCommit{}, errors.New(
				"each limitation must not exceed 1000 characters",
			)
		}
	}
	encoded, err := json.Marshal(struct {
		WorkspaceID string                             `json:"workspaceId"`
		ExecutionID string                             `json:"executionId"`
		Assessment  authorizationIndependentAssessment `json:"assessment"`
	}{
		WorkspaceID: review.workspaceID,
		ExecutionID: review.executionID,
		Assessment:  assessment,
	})
	if err != nil {
		return authorizationReviewCommit{}, err
	}
	digest := sha256.Sum256(encoded)
	commit := authorizationReviewCommit{
		Version:           1,
		WorkspaceID:       review.workspaceID,
		ExecutionID:       review.executionID,
		Phase:             authorizationReviewPhaseSubmitted,
		Assessment:        assessment,
		CommitFingerprint: "sha256:" + hex.EncodeToString(digest[:]),
		SubmittedAt:       time.Now().UnixMilli(),
	}
	review.phase = authorizationReviewPhaseSubmitted
	review.commit = &commit
	return commit, nil
}

func deterministicObservation(
	mode string,
	execution *browser.ExtensionAuthorizationExecution,
) string {
	if execution == nil {
		return "not-executed"
	}
	if execution.State == "partial" {
		return "execution-partial"
	}
	if mode == "vertical" {
		switch execution.Verdict {
		case "confirmed":
			return "low-identity-operation-state-change-confirmed"
		case "likely":
			return "low-identity-operation-response-accepted"
		case "protected":
			return "low-identity-operation-blocked"
		case "invalid-controls":
			return "control-baselines-invalid"
		default:
			return "evidence-inconclusive"
		}
	}
	switch execution.Verdict {
	case "confirmed":
		return "cross-identity-resource-reproduction-confirmed"
	case "likely":
		return "cross-identity-response-match-observed"
	case "protected":
		return "cross-identity-probes-blocked"
	case "invalid-controls":
		return "control-baselines-invalid"
	default:
		return "evidence-inconclusive"
	}
}

func authorizationFactAgreement(
	mode string,
	execution *browser.ExtensionAuthorizationExecution,
	assessment authorizationIndependentAssessment,
) string {
	if execution == nil {
		return "unknown"
	}
	if mode == "vertical" {
		expected := verticalDeterministicObservation(execution)
		if expected == assessment.LowToPrivilegedObservation {
			return "agree"
		}
		if expected == "operation-response-accepted" &&
			assessment.LowToPrivilegedObservation == "unknown" {
			return "partial"
		}
		return "disagree"
	}
	agreements := []string{
		compareAuthorizationObservation(
			horizontalDeterministicObservation(execution, "a-to-b"),
			assessment.AToBObservation,
		),
		compareAuthorizationObservation(
			horizontalDeterministicObservation(execution, "b-to-a"),
			assessment.BToAObservation,
		),
	}
	for _, agreement := range agreements {
		if agreement == "disagree" {
			return "disagree"
		}
	}
	for _, agreement := range agreements {
		if agreement == "partial" {
			return "partial"
		}
	}
	return "agree"
}

func authorizationReviewCase(
	execution *browser.ExtensionAuthorizationExecution,
	id string,
) *browser.ExtensionAuthorizationCaseExecution {
	for index := range execution.Cases {
		if execution.Cases[index].ID == id {
			return &execution.Cases[index]
		}
	}
	return nil
}

func authorizationReviewResponseExact(
	left *browser.ExtensionAuthorizationRequestExecution,
	right *browser.ExtensionAuthorizationRequestExecution,
) bool {
	return left != nil &&
		right != nil &&
		!left.Response.Truncated &&
		!right.Response.Truncated &&
		left.Response.AnalysisState != "encoded-unavailable" &&
		right.Response.AnalysisState != "encoded-unavailable" &&
		left.Response.ValueFingerprint != "" &&
		left.Response.ValueFingerprint == right.Response.ValueFingerprint
}

func horizontalDeterministicObservation(
	execution *browser.ExtensionAuthorizationExecution,
	direction string,
) string {
	for _, evidence := range execution.Evidence {
		if evidence.Direction == direction {
			return "target-data-reproduced"
		}
	}
	crossID, targetID, otherID := "a-to-b", "b-own", "a-own"
	if direction == "b-to-a" {
		crossID, targetID, otherID = "b-to-a", "a-own", "b-own"
	}
	cross := authorizationReviewCase(execution, crossID)
	target := authorizationReviewCase(execution, targetID)
	other := authorizationReviewCase(execution, otherID)
	if cross == nil || cross.State != "completed" || cross.Result == nil {
		return "not-executed"
	}
	if cross.Result.Outcome == "denied" {
		return "access-denied"
	}
	if target != nil && other != nil &&
		authorizationReviewResponseExact(cross.Result, target.Result) &&
		!authorizationReviewResponseExact(cross.Result, other.Result) {
		return "target-response-matched"
	}
	if cross.Result.Outcome == "success" {
		return "response-different"
	}
	return "unknown"
}

func verticalDeterministicObservation(
	execution *browser.ExtensionAuthorizationExecution,
) string {
	for _, evidence := range execution.Evidence {
		if strings.Contains(evidence.Source, "post-state") {
			return "state-change-confirmed"
		}
	}
	probe := authorizationReviewCase(execution, "low-privileged-probe")
	if probe == nil || probe.State != "completed" || probe.Result == nil {
		return "not-executed"
	}
	if probe.Result.Outcome == "denied" {
		return "access-denied"
	}
	if probe.Result.Outcome == "success" {
		return "operation-response-accepted"
	}
	return "unknown"
}

func compareAuthorizationObservation(expected string, actual string) string {
	if expected == actual {
		return "agree"
	}
	if (expected == "target-data-reproduced" && actual == "target-response-matched") ||
		(expected == "response-different" && actual == "unknown") ||
		(expected == "unknown" && actual == "response-different") {
		return "partial"
	}
	return "disagree"
}

func (review *authorizationBlindReview) reconcile(
	workspace browser.ExtensionAuthorizationWorkspace,
) (authorizationVerdictReconciliation, error) {
	if workspace.Execution == nil {
		return authorizationVerdictReconciliation{}, errors.New("the bound workspace has no execution")
	}
	review.mu.Lock()
	defer review.mu.Unlock()
	if review.phase != authorizationReviewPhaseSubmitted || review.commit == nil {
		return authorizationVerdictReconciliation{}, errors.New(
			"submit an independent blind assessment before revealing the deterministic finding",
		)
	}
	if workspace.Execution.ID != review.executionID ||
		workspace.Execution.PlanID != review.planID {
		return authorizationVerdictReconciliation{}, errors.New(
			"the bound execution changed after the independent assessment",
		)
	}
	evidence := make([]authorizationReviewEvidence, 0, len(workspace.Execution.Evidence))
	for _, item := range workspace.Execution.Evidence {
		evidence = append(evidence, authorizationReviewEvidence{
			Direction: item.Direction,
			Path:      item.Path,
			Source:    item.Source,
		})
	}
	sort.Slice(evidence, func(i, j int) bool {
		if evidence[i].Direction == evidence[j].Direction {
			return evidence[i].Path < evidence[j].Path
		}
		return evidence[i].Direction < evidence[j].Direction
	})
	return authorizationVerdictReconciliation{
		Version:           1,
		WorkspaceID:       workspace.ID,
		ExecutionID:       workspace.Execution.ID,
		Mode:              workspace.Mode,
		IndependentReview: *review.commit,
		Deterministic: authorizationDeterministicFinding{
			Observation:      deterministicObservation(workspace.Mode, workspace.Execution),
			EvidenceGrade:    workspace.Execution.Verdict,
			Confidence:       workspace.Execution.Confidence,
			RequestCount:     workspace.Execution.RequestCount,
			Evidence:         evidence,
			Reasons:          append([]string(nil), workspace.Execution.Reasons...),
			PolicyConclusion: "not-evaluated-by-deterministic-engine",
		},
		FactAgreement:  authorizationFactAgreement(workspace.Mode, workspace.Execution, review.commit.Assessment),
		PolicyBoundary: "Deterministic code establishes request and response facts only; authorization-policy intent requires independent identity, role, tenant, or explicit policy evidence.",
	}, nil
}
