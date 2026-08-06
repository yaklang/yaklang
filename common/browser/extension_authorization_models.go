package browser

import (
	"encoding/json"
	"regexp"
	"time"
)

const (
	extensionAuthorizationWorkspaceTTL  = 30 * time.Minute
	maxExtensionAuthorizationWorkspaces = 32
	maxAuthorizationWorkspaceTombstones = 128
	extensionAuthorizationTombstoneTTL  = 2 * time.Hour
	maxAuthorizationTransformProfiles   = 64
)

var (
	extensionAuthorizationCanaryPathPattern = regexp.MustCompile(
		`^body(\.[A-Za-z0-9_-]+|\[[0-9]+\])+$`,
	)
	authorizationNumericPathSegmentPattern = regexp.MustCompile(`^\d+$`)
	authorizationUUIDPathSegmentPattern    = regexp.MustCompile(
		`(?i)^[0-9a-f]{8}-[0-9a-f-]{27,}$`,
	)
	authorizationHexPathSegmentPattern    = regexp.MustCompile(`(?i)^[0-9a-f]{12,}$`)
	authorizationOpaquePathSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,}$`)
	authorizationBodyPathSegmentPattern   = regexp.MustCompile(
		`(^|\.)([A-Za-z0-9_-]+)|\[([0-9]+)\]`,
	)
	authorizationGraphQLNamePattern = regexp.MustCompile(
		`^[A-Za-z_][A-Za-z0-9_]{0,127}$`,
	)
	authorizationGraphQLFallbackNamePattern = regexp.MustCompile(
		`^(anonymous-[1-9][0-9]*|batch-overflow-[1-9][0-9]*)$`,
	)
)

type ExtensionAuthorizationIdentityInput struct {
	DeviceID     string `json:"deviceId"`
	TabID        int    `json:"tabId"`
	FrameID      int    `json:"frameId"`
	AccountLabel string `json:"accountLabel,omitempty"`
}

type ExtensionAuthorizationWorkspaceInput struct {
	Mode  string                              `json:"mode"`
	Left  ExtensionAuthorizationIdentityInput `json:"left"`
	Right ExtensionAuthorizationIdentityInput `json:"right"`
}

type ExtensionAuthorizationTarget struct {
	TabID      int    `json:"tabId"`
	FrameID    int    `json:"frameId"`
	DocumentID string `json:"documentId"`
}

type ExtensionAuthorizationAuthentication struct {
	Status            string   `json:"status"`
	CookieCount       int      `json:"cookieCount"`
	StorageEntryCount int      `json:"storageEntryCount"`
	AuthCookieNames   []string `json:"authCookieNames"`
	AuthStorageKeys   []string `json:"authStorageKeys"`
}

type extensionAuthorizationContextBase struct {
	Version            int                                  `json:"version"`
	ID                 string                               `json:"id"`
	DeviceID           string                               `json:"deviceId"`
	InstallationID     string                               `json:"installationId"`
	IsolationContextID string                               `json:"isolationContextId"`
	CookieStoreID      string                               `json:"cookieStoreId"`
	Origin             string                               `json:"origin"`
	GrantID            string                               `json:"grantId"`
	Target             ExtensionAuthorizationTarget         `json:"target"`
	Fingerprint        string                               `json:"fingerprint"`
	Authentication     ExtensionAuthorizationAuthentication `json:"authentication"`
	CreatedAt          int64                                `json:"createdAt"`
	ExpiresAt          int64                                `json:"expiresAt"`
}

type extensionAuthorizationHandle struct {
	extensionAuthorizationContextBase
	SlotID           string `json:"slotId"`
	AccountLabel     string `json:"accountLabel,omitempty"`
	IsolationProofID string `json:"isolationProofId"`
}

type extensionAuthorizationAttestation struct {
	extensionAuthorizationContextBase
}

type extensionAuthorizationIsolationProof struct {
	Version                   int      `json:"version"`
	ID                        string   `json:"id"`
	LeftContextID             string   `json:"leftContextId"`
	RightContextID            string   `json:"rightContextId"`
	LeftTabID                 int      `json:"leftTabId"`
	RightTabID                int      `json:"rightTabId"`
	SameOrigin                bool     `json:"sameOrigin"`
	CookieStoreRelation       string   `json:"cookieStoreRelation"`
	AccountEvidenceRelation   string   `json:"accountEvidenceRelation"`
	RequestCredentialRelation string   `json:"requestCredentialRelation"`
	RefreshCheck              string   `json:"refreshCheck"`
	Level                     string   `json:"level"`
	Reasons                   []string `json:"reasons"`
	CreatedAt                 int64    `json:"createdAt"`
	ExpiresAt                 int64    `json:"expiresAt"`
}

type ExtensionAuthorizationContextReference struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type ExtensionAuthorizationIdentitySlot struct {
	Side               string                                 `json:"side"`
	AccountLabel       string                                 `json:"accountLabel,omitempty"`
	DeviceID           string                                 `json:"deviceId"`
	InstallationID     string                                 `json:"installationId"`
	IsolationContextID string                                 `json:"isolationContextId"`
	CookieStoreID      string                                 `json:"cookieStoreId"`
	Origin             string                                 `json:"origin"`
	GrantID            string                                 `json:"grantId"`
	Target             ExtensionAuthorizationTarget           `json:"target"`
	ContextReference   ExtensionAuthorizationContextReference `json:"contextReference"`
	Fingerprint        string                                 `json:"fingerprint"`
	Authentication     ExtensionAuthorizationAuthentication   `json:"authentication"`
	ExpiresAt          int64                                  `json:"expiresAt"`
}

type ExtensionAuthorizationProof struct {
	ID                        string   `json:"id"`
	Source                    string   `json:"source"`
	SourceProofID             string   `json:"sourceProofId,omitempty"`
	Level                     string   `json:"level"`
	SameOrigin                bool     `json:"sameOrigin"`
	CookieStoreRelation       string   `json:"cookieStoreRelation"`
	AccountEvidenceRelation   string   `json:"accountEvidenceRelation"`
	RequestCredentialRelation string   `json:"requestCredentialRelation"`
	RefreshCheck              string   `json:"refreshCheck"`
	Reasons                   []string `json:"reasons"`
	CreatedAt                 int64    `json:"createdAt"`
	ExpiresAt                 int64    `json:"expiresAt"`
}

type ExtensionAuthorizationBaselineField struct {
	Location         string `json:"location"`
	Path             string `json:"path"`
	ValueType        string `json:"valueType"`
	ByteLength       int    `json:"byteLength"`
	ValueFingerprint string `json:"valueFingerprint"`
	Category         string `json:"category"`
}

type ExtensionAuthorizationBaselineRequest struct {
	Method               string                                `json:"method"`
	URL                  string                                `json:"url"`
	Path                 string                                `json:"path"`
	ContentType          string                                `json:"contentType"`
	Protocol             string                                `json:"protocol,omitempty"`
	OperationFingerprint string                                `json:"operationFingerprint,omitempty"`
	OperationNames       []string                              `json:"operationNames,omitempty"`
	ActionFingerprint    string                                `json:"actionFingerprint"`
	HeaderNames          []string                              `json:"headerNames"`
	Fields               []ExtensionAuthorizationBaselineField `json:"fields"`
}

type ExtensionAuthorizationLogicalRequestValidation struct {
	ProofLevel string   `json:"proofLevel"`
	Summary    string   `json:"summary"`
	Warnings   []string `json:"warnings"`
}

type ExtensionAuthorizationLogicalRequestBinding struct {
	Version            int                                            `json:"version"`
	Source             string                                         `json:"source"`
	BaselineID         string                                         `json:"baselineId"`
	ProfileID          string                                         `json:"profileId"`
	ProfileName        string                                         `json:"profileName"`
	IsolationContextID string                                         `json:"isolationContextId"`
	CookieStoreID      string                                         `json:"cookieStoreId"`
	Target             ExtensionAuthorizationTarget                   `json:"target"`
	Origin             string                                         `json:"origin"`
	Request            ExtensionAuthorizationBaselineRequest          `json:"request"`
	OutputDestinations []string                                       `json:"outputDestinations"`
	Validation         ExtensionAuthorizationLogicalRequestValidation `json:"validation"`
	BindingFingerprint string                                         `json:"bindingFingerprint"`
	ProfileUpdatedAt   int64                                          `json:"profileUpdatedAt"`
	ReplayUpdatedAt    int64                                          `json:"replayUpdatedAt"`
	CreatedAt          int64                                          `json:"createdAt"`
	ExpiresAt          int64                                          `json:"expiresAt"`
}

type ExtensionAuthorizationBaseline struct {
	Version              int                                          `json:"version"`
	ID                   string                                       `json:"id"`
	DeviceID             string                                       `json:"deviceId"`
	InstallationID       string                                       `json:"installationId"`
	IsolationContextID   string                                       `json:"isolationContextId"`
	CookieStoreID        string                                       `json:"cookieStoreId"`
	Origin               string                                       `json:"origin"`
	GrantID              string                                       `json:"grantId"`
	Target               ExtensionAuthorizationTarget                 `json:"target"`
	AuthContextReference ExtensionAuthorizationContextReference       `json:"authContextReference"`
	NetworkRequestID     string                                       `json:"networkRequestId"`
	Request              ExtensionAuthorizationBaselineRequest        `json:"request"`
	LogicalRequest       *ExtensionAuthorizationLogicalRequestBinding `json:"logicalRequest,omitempty"`
	CreatedAt            int64                                        `json:"createdAt"`
	ExpiresAt            int64                                        `json:"expiresAt"`
}

type ExtensionAuthorizationBaselineSet struct {
	Left         *ExtensionAuthorizationBaseline `json:"left,omitempty"`
	Right        *ExtensionAuthorizationBaseline `json:"right,omitempty"`
	Verification *ExtensionAuthorizationBaseline `json:"verification,omitempty"`
}

type ExtensionAuthorizationResourceCandidate struct {
	ID                     string   `json:"id"`
	Source                 string   `json:"source"`
	Location               string   `json:"location"`
	Path                   string   `json:"path"`
	Category               string   `json:"category"`
	Confidence             string   `json:"confidence"`
	RequiresLogicalBinding bool     `json:"requiresLogicalBinding"`
	Reasons                []string `json:"reasons"`
}

type ExtensionAuthorizationBaselinePair struct {
	State               string                                     `json:"state"`
	ActionFingerprint   string                                     `json:"actionFingerprint,omitempty"`
	Reasons             []string                                   `json:"reasons"`
	ResourceCandidates  []ExtensionAuthorizationResourceCandidate  `json:"resourceCandidates"`
	OperationCandidates []ExtensionAuthorizationOperationCandidate `json:"operationCandidates"`
}

type ExtensionAuthorizationOperationCandidate struct {
	ID                     string   `json:"id"`
	TemplateSide           string   `json:"templateSide"`
	AuthContextSide        string   `json:"authContextSide"`
	LowControlSide         string   `json:"lowControlSide"`
	Method                 string   `json:"method"`
	Path                   string   `json:"path"`
	ActionFingerprint      string   `json:"actionFingerprint"`
	Eligible               bool     `json:"eligible"`
	SideEffect             bool     `json:"sideEffect"`
	RequiresDynamicRebuild bool     `json:"requiresDynamicRebuild"`
	AuthenticationPaths    []string `json:"authenticationPaths"`
	MissingAuthPaths       []string `json:"missingAuthPaths"`
	DynamicPaths           []string `json:"dynamicPaths"`
	Reasons                []string `json:"reasons"`
}

type ExtensionAuthorizationPlanSelector struct {
	Source   string `json:"source"`
	Location string `json:"location"`
	Path     string `json:"path"`
}

type ExtensionAuthorizationPlanCase struct {
	ID                  string `json:"id"`
	Label               string `json:"label"`
	RequestBaselineSide string `json:"requestBaselineSide"`
	AuthContextSide     string `json:"authContextSide"`
	ResourceValueSide   string `json:"resourceValueSide"`
	Method              string `json:"method"`
	Path                string `json:"path"`
	SideEffect          bool   `json:"sideEffect"`
}

type ExtensionAuthorizationTransformBinding struct {
	Version            int                          `json:"version"`
	BaselineID         string                       `json:"baselineId"`
	ProfileID          string                       `json:"profileId"`
	ProfileName        string                       `json:"profileName"`
	IsolationContextID string                       `json:"isolationContextId"`
	CookieStoreID      string                       `json:"cookieStoreId"`
	Target             ExtensionAuthorizationTarget `json:"target"`
	Origin             string                       `json:"origin"`
	DynamicPaths       []string                     `json:"dynamicPaths"`
	BindingFingerprint string                       `json:"bindingFingerprint"`
	CreatedAt          int64                        `json:"createdAt"`
	ExpiresAt          int64                        `json:"expiresAt"`
}

type ExtensionAuthorizationTransformPair struct {
	Left  ExtensionAuthorizationTransformBinding `json:"left"`
	Right ExtensionAuthorizationTransformBinding `json:"right"`
}

type ExtensionAuthorizationTransformProfileInput struct {
	Left  string `json:"left"`
	Right string `json:"right"`
}

type ExtensionAuthorizationTransformProfileCandidate struct {
	ID                    string   `json:"id"`
	Name                  string   `json:"name"`
	Methods               []string `json:"methods"`
	URLPattern            string   `json:"urlPattern"`
	OutputDestinations    []string `json:"outputDestinations"`
	Eligible              bool     `json:"eligible"`
	Reasons               []string `json:"reasons"`
	DynamicFieldsEligible bool     `json:"dynamicFieldsEligible"`
	DynamicFieldsReasons  []string `json:"dynamicFieldsReasons"`
	LogicalBodyEligible   bool     `json:"logicalBodyEligible"`
	LogicalBodyReasons    []string `json:"logicalBodyReasons"`
	UpdatedAt             int64    `json:"updatedAt"`
}

type ExtensionAuthorizationTransformProfileCandidates struct {
	Left  []ExtensionAuthorizationTransformProfileCandidate `json:"left"`
	Right []ExtensionAuthorizationTransformProfileCandidate `json:"right"`
}

type ExtensionAuthorizationPlan struct {
	Version                int                                  `json:"version"`
	ID                     string                               `json:"id"`
	WorkspaceID            string                               `json:"workspaceId"`
	Mode                   string                               `json:"mode"`
	ProofID                string                               `json:"proofId"`
	CandidateID            string                               `json:"candidateId"`
	CanaryPaths            []string                             `json:"canaryPaths"`
	State                  string                               `json:"state"`
	Selector               ExtensionAuthorizationPlanSelector   `json:"selector"`
	Operation              *ExtensionAuthorizationPlanOperation `json:"operation,omitempty"`
	Cases                  []ExtensionAuthorizationPlanCase     `json:"cases"`
	RequestBudget          int                                  `json:"requestBudget"`
	RequiresDynamicRebuild bool                                 `json:"requiresDynamicRebuild"`
	Transforms             *ExtensionAuthorizationTransformPair `json:"transforms,omitempty"`
	Reasons                []string                             `json:"reasons"`
	CreatedAt              int64                                `json:"createdAt"`
	ExpiresAt              int64                                `json:"expiresAt"`
}

type ExtensionAuthorizationPlanOperation struct {
	TemplateBaselineSide   string                                           `json:"templateBaselineSide"`
	AuthContextSide        string                                           `json:"authContextSide"`
	LowControlSide         string                                           `json:"lowControlSide"`
	AuthenticationPaths    []string                                         `json:"authenticationPaths"`
	DynamicPaths           []string                                         `json:"dynamicPaths"`
	VerificationBaselineID string                                           `json:"verificationBaselineId,omitempty"`
	Transform              *ExtensionAuthorizationOperationTransformBinding `json:"transform,omitempty"`
}

type ExtensionAuthorizationOperationTransformBinding struct {
	Version            int                          `json:"version"`
	AuthBaselineID     string                       `json:"authBaselineId"`
	TemplateBaselineID string                       `json:"templateBaselineId"`
	ProfileID          string                       `json:"profileId"`
	ProfileName        string                       `json:"profileName"`
	ProfileUpdatedAt   int64                        `json:"profileUpdatedAt"`
	IsolationContextID string                       `json:"isolationContextId"`
	CookieStoreID      string                       `json:"cookieStoreId"`
	Target             ExtensionAuthorizationTarget `json:"target"`
	Origin             string                       `json:"origin"`
	ActionFingerprint  string                       `json:"actionFingerprint"`
	DynamicPaths       []string                     `json:"dynamicPaths"`
	BindingFingerprint string                       `json:"bindingFingerprint"`
	CreatedAt          int64                        `json:"createdAt"`
	ExpiresAt          int64                        `json:"expiresAt"`
}

type ExtensionAuthorizationResourceValue struct {
	Version                   int    `json:"version"`
	BaselineID                string `json:"baselineId"`
	Source                    string `json:"source"`
	Location                  string `json:"location"`
	Path                      string `json:"path"`
	ValueType                 string `json:"valueType"`
	ByteLength                int    `json:"byteLength"`
	ValueBase64               string `json:"valueBase64"`
	ValueFingerprint          string `json:"valueFingerprint"`
	LogicalBindingFingerprint string `json:"logicalBindingFingerprint,omitempty"`
}

type extensionAuthorizationCompiledRequest struct {
	Version                   int                                `json:"version"`
	BaselineID                string                             `json:"baselineId"`
	Selector                  ExtensionAuthorizationPlanSelector `json:"selector"`
	Method                    string                             `json:"method"`
	URL                       string                             `json:"url"`
	IsHTTPS                   bool                               `json:"isHttps"`
	RawRequestBase64          string                             `json:"rawRequestBase64"`
	ResourceValueFingerprint  string                             `json:"resourceValueFingerprint"`
	LogicalBindingFingerprint string                             `json:"logicalBindingFingerprint,omitempty"`
	PacketFingerprint         string                             `json:"packetFingerprint"`
}

type extensionAuthorizationBaselinePacket struct {
	Version           int    `json:"version"`
	BaselineID        string `json:"baselineId"`
	Method            string `json:"method"`
	URL               string `json:"url"`
	IsHTTPS           bool   `json:"isHttps"`
	RawRequestBase64  string `json:"rawRequestBase64"`
	PacketFingerprint string `json:"packetFingerprint"`
}

type extensionAuthorizationTransformHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type extensionAuthorizationTransformPacket struct {
	Method     string                                  `json:"method"`
	URL        string                                  `json:"url"`
	Headers    []extensionAuthorizationTransformHeader `json:"headers"`
	BodyBase64 string                                  `json:"bodyBase64"`
}

type extensionAuthorizationTransformExecution struct {
	ProfileID     string                                  `json:"profileId"`
	Direction     string                                  `json:"direction"`
	URL           string                                  `json:"url"`
	BodyBase64    string                                  `json:"bodyBase64"`
	SetHeaders    []extensionAuthorizationTransformHeader `json:"setHeaders"`
	RemoveHeaders []string                                `json:"removeHeaders"`
	LogicalInput  json.RawMessage                         `json:"logicalInput"`
	LogicalOutput json.RawMessage                         `json:"logicalOutput"`
	NodeDurations []struct {
		NodeID     string  `json:"nodeId"`
		DurationMS float64 `json:"durationMs"`
	} `json:"nodeDurations"`
	DurationMS float64 `json:"durationMs"`
}

type ExtensionAuthorizationResponseSummary struct {
	ContentType            string `json:"contentType"`
	ContentEncoding        string `json:"contentEncoding,omitempty"`
	CapturedBytes          int    `json:"capturedBytes"`
	AnalysisBytes          int    `json:"analysisBytes"`
	DeclaredBytes          *int   `json:"declaredBytes,omitempty"`
	Truncated              bool   `json:"truncated"`
	Decoded                bool   `json:"decoded"`
	AnalysisState          string `json:"analysisState"`
	AnalysisRepresentation string `json:"analysisRepresentation"`
	ValueFingerprint       string `json:"valueFingerprint"`
	ShapeFingerprint       string `json:"shapeFingerprint,omitempty"`
}

type ExtensionAuthorizationRequestTiming struct {
	DNSMS      float64 `json:"dnsMs"`
	ConnectMS  float64 `json:"connectMs"`
	TLSMS      float64 `json:"tlsMs"`
	TTFBMS     float64 `json:"ttfbMs"`
	TransferMS float64 `json:"transferMs"`
	TotalMS    float64 `json:"totalMs"`
}

type ExtensionAuthorizationRequestExecution struct {
	Version            int                                   `json:"version"`
	BaselineID         string                                `json:"baselineId"`
	Selector           ExtensionAuthorizationPlanSelector    `json:"selector"`
	Method             string                                `json:"method"`
	URL                string                                `json:"url"`
	Status             int                                   `json:"status"`
	StatusText         string                                `json:"statusText"`
	Outcome            string                                `json:"outcome"`
	Response           ExtensionAuthorizationResponseSummary `json:"response"`
	DroppedHeaderNames []string                              `json:"droppedHeaderNames"`
	DurationMS         float64                               `json:"durationMs"`
	Timing             ExtensionAuthorizationRequestTiming   `json:"timing"`
	CompletedAt        int64                                 `json:"completedAt"`
	responseBody       []byte
	requestPacket      []byte
	responsePacket     []byte
	requestTruncated   bool
	responseTruncated  bool
}

type ExtensionAuthorizationCaseExecution struct {
	ID                string                                  `json:"id"`
	Label             string                                  `json:"label"`
	AuthContextSide   string                                  `json:"authContextSide"`
	ResourceValueSide string                                  `json:"resourceValueSide"`
	State             string                                  `json:"state"`
	Result            *ExtensionAuthorizationRequestExecution `json:"result,omitempty"`
	Error             string                                  `json:"error,omitempty"`
}

type ExtensionAuthorizationCanaryEvidence struct {
	Direction        string `json:"direction"`
	Path             string `json:"path"`
	ValueFingerprint string `json:"valueFingerprint"`
	Source           string `json:"source"`
}

type ExtensionAuthorizationExecution struct {
	Version           int                                    `json:"version"`
	ID                string                                 `json:"id"`
	WorkspaceID       string                                 `json:"workspaceId"`
	PlanID            string                                 `json:"planId"`
	State             string                                 `json:"state"`
	Verdict           string                                 `json:"verdict"`
	Confidence        string                                 `json:"confidence"`
	Cases             []ExtensionAuthorizationCaseExecution  `json:"cases"`
	RequestCount      int                                    `json:"requestCount"`
	Evidence          []ExtensionAuthorizationCanaryEvidence `json:"evidence"`
	EvidenceAvailable bool                                   `json:"evidenceAvailable"`
	Reasons           []string                               `json:"reasons"`
	StartedAt         int64                                  `json:"startedAt"`
	CompletedAt       int64                                  `json:"completedAt"`
}

// ExtensionAuthorizationRecovery is an explicit, non-automatic recovery
// instruction for a stale authorization workspace. A recovery code is safe to
// expose to UI and Agent callers; the original transport error is deliberately
// not copied into it.
type ExtensionAuthorizationRecovery struct {
	Code      string `json:"code"`
	Scope     string `json:"scope"`
	Message   string `json:"message"`
	Automatic bool   `json:"automatic"`
}

type ExtensionAuthorizationWorkspaceLifecycleReason string

const (
	ExtensionAuthorizationWorkspaceExpired               ExtensionAuthorizationWorkspaceLifecycleReason = "expired"
	ExtensionAuthorizationWorkspaceEvicted               ExtensionAuthorizationWorkspaceLifecycleReason = "evicted"
	ExtensionAuthorizationWorkspaceEngineInstanceChanged ExtensionAuthorizationWorkspaceLifecycleReason = "engine_instance_changed"
	ExtensionAuthorizationWorkspaceNotFound              ExtensionAuthorizationWorkspaceLifecycleReason = "not_found"
	ExtensionAuthorizationWorkspaceReplaced              ExtensionAuthorizationWorkspaceLifecycleReason = "replaced"
)

// ExtensionAuthorizationWorkspaceLifecycleError preserves why an in-memory
// workspace cannot be used. Callers should branch on Reason rather than parse
// the human-readable message.
type ExtensionAuthorizationWorkspaceLifecycleError struct {
	Reason                 ExtensionAuthorizationWorkspaceLifecycleReason `json:"reason"`
	WorkspaceID            string                                         `json:"workspaceId"`
	EngineInstanceID       string                                         `json:"engineInstanceId"`
	ExpiresAt              int64                                          `json:"expiresAt,omitempty"`
	ReplacementWorkspaceID string                                         `json:"replacementWorkspaceId,omitempty"`
}

func (e *ExtensionAuthorizationWorkspaceLifecycleError) Error() string {
	if e == nil {
		return "authorization workspace is unavailable"
	}
	switch e.Reason {
	case ExtensionAuthorizationWorkspaceExpired:
		return "authorization workspace expired; rebuild the identity workspace"
	case ExtensionAuthorizationWorkspaceEvicted:
		return "authorization workspace was evicted because the in-memory capacity was reached"
	case ExtensionAuthorizationWorkspaceEngineInstanceChanged:
		return "authorization workspace belongs to a previous engine instance; rebuild it after reconnecting"
	case ExtensionAuthorizationWorkspaceReplaced:
		if e.ReplacementWorkspaceID != "" {
			return "authorization workspace was replaced by " + e.ReplacementWorkspaceID
		}
		return "authorization workspace was replaced by a newer workspace"
	default:
		return "authorization workspace was not found"
	}
}

type extensionAuthorizationWorkspaceTombstone struct {
	Reason                 ExtensionAuthorizationWorkspaceLifecycleReason
	WorkspaceID            string
	EngineInstanceID       string
	ExpiresAt              int64
	ReplacementWorkspaceID string
	RemovedAt              int64
}

type ExtensionAuthorizationWorkspace struct {
	Version          int                                `json:"version"`
	ID               string                             `json:"id"`
	EngineInstanceID string                             `json:"engineInstanceId"`
	Mode             string                             `json:"mode"`
	State            string                             `json:"state"`
	Left             ExtensionAuthorizationIdentitySlot `json:"left"`
	Right            ExtensionAuthorizationIdentitySlot `json:"right"`
	Proof            ExtensionAuthorizationProof        `json:"proof"`
	Baselines        ExtensionAuthorizationBaselineSet  `json:"baselines"`
	BaselinePair     ExtensionAuthorizationBaselinePair `json:"baselinePair"`
	Plan             *ExtensionAuthorizationPlan        `json:"plan,omitempty"`
	Execution        *ExtensionAuthorizationExecution   `json:"execution,omitempty"`
	CreatedAt        int64                              `json:"createdAt"`
	ExpiresAt        int64                              `json:"expiresAt"`
	StaleReason      string                             `json:"staleReason,omitempty"`
	Recovery         *ExtensionAuthorizationRecovery    `json:"recovery,omitempty"`
	comparisonKey    string
}

type ExtensionAuthorizationBaselineInput struct {
	WorkspaceID      string `json:"workspaceId"`
	Side             string `json:"side"`
	NetworkRequestID string `json:"networkRequestId"`
	Clear            bool   `json:"clear,omitempty"`
}

type ExtensionAuthorizationLogicalBindingInput struct {
	WorkspaceID       string                                      `json:"workspaceId"`
	TransformProfiles ExtensionAuthorizationTransformProfileInput `json:"transformProfiles"`
}

type ExtensionAuthorizationPlanInput struct {
	WorkspaceID                 string                                       `json:"workspaceId"`
	CandidateID                 string                                       `json:"candidateId"`
	CanaryPaths                 []string                                     `json:"canaryPaths,omitempty"`
	TransformProfiles           *ExtensionAuthorizationTransformProfileInput `json:"transformProfiles,omitempty"`
	OperationTransformProfileID string                                       `json:"operationTransformProfileId,omitempty"`
}

type ExtensionAuthorizationExecutionInput struct {
	WorkspaceID        string `json:"workspaceId"`
	PlanID             string `json:"planId"`
	ApproveSideEffects bool   `json:"approveSideEffects,omitempty"`
}
