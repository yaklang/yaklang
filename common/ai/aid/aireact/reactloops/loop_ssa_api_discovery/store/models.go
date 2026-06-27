package store

import (
	"time"
)

// DiscoverySession is the root row for one SSA API discovery run.
type DiscoverySession struct {
	ID        uint   `gorm:"primaryKey"`
	UUID      string `gorm:"uniqueIndex;not null;size:36"`
	CreatedAt time.Time
	UpdatedAt time.Time

	CodeRootPath      string
	CodePathOK        bool
	CodePathError     string
	TargetRaw         string
	TargetHost        string
	TargetPort        string
	TargetScheme      string
	TargetReachable   bool
	TargetProbeMethod string
	TargetProbeDetail string
	TargetProbedAt    *time.Time

	Language string

	SSAProgramName  string
	SSACompileOK    bool
	SSACompileError string
	SSAFileCount    int

	Phase string
	Notes string

	// EndpointHarvestMetaJSON：多手段端点搜集（静态 Spring 等）与完备性摘要，见 EndpointHarvestReport。
	EndpointHarvestMetaJSON string `gorm:"type:text"`

	// SyntaxFlowScanMetaJSON：Phase3 SyntaxFlow 扫描元数据（filter 摘要、规则全量名、条数、来源等），供报告与阶段门使用。
	SyntaxFlowScanMetaJSON string `gorm:"type:text"`

	// Deprecated: RoutingProfileJSON 历史字段；主路径使用 ApiBaseCalibrationMetaJSON + forwarding_profile.json。
	RoutingProfileJSON string `gorm:"type:text"`

	// ApiPreanalysisMetaJSON：程序化预分析摘要（模块、配置 base 候选、OpenAPI 路径、路由文件候选等），完整 JSON 见 workdir/ssa_discovery/api_preanalysis.json。
	ApiPreanalysisMetaJSON string `gorm:"type:text"`

	// ApiSpecImportMetaJSON：OpenAPI/Swagger 导入摘要。
	ApiSpecImportMetaJSON string `gorm:"type:text"`

	// ApiBaseCalibrationMetaJSON：base URL / context-path 程序化探测评分摘要。
	ApiBaseCalibrationMetaJSON string `gorm:"type:text"`

	// CodeReadingRoutesMetaJSON：Phase1B AI 代码通读产出的 discovered_apis 同步摘要。
	CodeReadingRoutesMetaJSON string `gorm:"type:text"`

	// PipelineWaiversJSON：流水线阶段契约豁免记录（JSON 数组）。
	PipelineWaiversJSON string `gorm:"type:text"`

	// SyntaxFlowSkipReason：SSA 不可用时 SyntaxFlow 跳过原因。
	SyntaxFlowSkipReason string `gorm:"type:text"`
}

// ArchitectureComponent is a logical module or subsystem.
type ArchitectureComponent struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	Name          string
	Kind          string
	Summary       string
	PathHintsJSON string
	Confidence    int
	Source        string
}

// ConfigArtifact is a configuration file or env slice worth highlighting.
type ConfigArtifact struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	RelPath               string
	Format                string
	Summary               string
	SensitiveKeyKindsJSON string
}

// DependencyRef is a third-party dependency (Maven/npm/...).
type DependencyRef struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	Name      string
	Version   string
	Ecosystem string
}

// Endpoint validation status constants.
const (
	EndpointStatusPendingValidation = "pending_validation"
	EndpointStatusCandidate         = "candidate"
	EndpointStatusAlive             = "alive"
	EndpointStatusRejected          = "rejected"
	EndpointStatusAuthFailed        = "auth_failed"
	EndpointStatusUnreachable       = "unreachable"
	EndpointStatusQuarantined       = "quarantined"
)

// HttpEndpoint is an HTTP route / handler pair (possibly inferred by AI).
type HttpEndpoint struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	Method        string
	PathPattern   string
	HandlerClass  string
	HandlerMethod string
	AuthzHint     string
	Source        string

	Status          string     `gorm:"size:32;default:'pending_validation'"`
	LastProbedAt    *time.Time
	ProbeStatusCode int
	ProbeEvidence   string `gorm:"type:text"`
	RejectReason    string
	FunctionScore   int
}

// SecurityMechanism records authn/z, csrf, cors, crypto surface, etc.
type SecurityMechanism struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	Category         string
	Description      string
	EvidenceRefsJSON string
}

// BusinessCapability is a coarse business/domain capability.
type BusinessCapability struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	Name            string
	Description     string
	LayerHint       string
	ScopePathsJSON  string `gorm:"type:text"`
	ModuleHintsJSON string `gorm:"type:text"`
}

// DiscoveryEvent is a lightweight timeline / audit row.
type DiscoveryEvent struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	SessionID uint

	Level       string
	Message     string
	PayloadJSON string
}

// VerifiedHttpApi is a Phase1-confirmed API row with probe evidence and code context.
type VerifiedHttpApi struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint `gorm:"not null;uniqueIndex:ux_verified_http_api_sess_method_path"`

	Method      string `gorm:"size:16;not null;uniqueIndex:ux_verified_http_api_sess_method_path"`
	PathPattern string `gorm:"size:512;not null;uniqueIndex:ux_verified_http_api_sess_method_path"`

	FullSampleURL   string
	EffectiveBase   string `gorm:"size:256"`
	URLSpace        string `gorm:"size:64"`
	QueryParamsJSON string `gorm:"type:text"`
	BodyHintJSON    string `gorm:"type:text"`

	AuthRequired    bool
	AuthHeadersJSON string `gorm:"type:text"`

	HandlerFile    string `gorm:"size:512"`
	HandlerSymbol  string `gorm:"size:256"`
	CodeSnippet    string `gorm:"type:text"`

	BusinessCapabilityID uint
	ForwardChainJSON     string `gorm:"type:text"`

	ProbeStatusCode  int
	ContentType      string `gorm:"size:128"`
	ResponseExcerpt  string `gorm:"type:text"`
	ProbeAttemptsJSON string `gorm:"type:text"`
	VerdictReason    string `gorm:"type:text"`

	Verified   bool
	Confidence int
	Source     string `gorm:"size:32"`
	Notes      string `gorm:"type:text"`
	RejectReason string `gorm:"type:text"`
}

// VerifiedEndpoint stores HTTP probe outcome for a discovered endpoint (or ad-hoc URL).
type VerifiedEndpoint struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	HttpEndpointID uint
	Method         string
	PathPattern    string
	RequestURL     string
	StatusCode     int
	Verified       bool   // true if response indicates the API surface likely exists
	ResponseHint   string
	Notes          string

	// Phase3 batch_probe：鉴权探测时记录的请求头（JSON）与响应摘要，供 Phase5 灰盒与报告使用。
	RequestHeaders string `gorm:"type:text"`
	ResponseBody   string `gorm:"type:text"`
	ContentType    string
	ResponseSize   int
}

// DiscoverySyntaxFlowFinding stores a simplified SyntaxFlow / SSARisk hit for this session.
type DiscoverySyntaxFlowFinding struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	RiskHash     string
	RuleName     string
	Severity     string
	Title        string
	Description  string
	MatchedFile  string
	MatchedLine  int
	DataFlowHint string
	Confidence   int
}

// Coverage work-item kind for HTTP endpoint verification (Phase3 gate).
const (
	CoverageKindHttpEndpoint = "http_endpoint"
)

// Coverage work-item status.
const (
	CoverageStatusPending    = "pending"
	CoverageStatusDone       = "done"
	CoverageStatusSkipped    = "skipped"
	CoverageStatusBlocked    = "blocked"
	CoverageStatusInProgress = "in_progress"
	CoverageStatusRejected   = "rejected"
)

// CoverageWorkItem is a legacy unit for deprecated batch-probe Phase3; retained for historical sessions only.
type CoverageWorkItem struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint `gorm:"not null;uniqueIndex:ux_cov_sess_kind_ref"`

	Kind      string `gorm:"size:32;not null;uniqueIndex:ux_cov_sess_kind_ref"` // e.g. http_endpoint
	RefID     uint   `gorm:"not null;uniqueIndex:ux_cov_sess_kind_ref"`         // http_endpoints.id
	RefLabel  string `gorm:"size:512"`
	Priority  int    `gorm:"default:0"`
	Status    string `gorm:"size:32;not null;default:'pending'"`
	Evidence  string `gorm:"type:text"`
	BlockedReason string `gorm:"type:text"`
}

// VulnVerification stores AI + dynamic check outcome for a syntaxflow or greybox finding.
type VulnVerification struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	SyntaxFlowFindingID uint
	DynamicFindingID    uint   `gorm:"index"`
	Source              string `gorm:"size:32"` // syntaxflow | dynamic
	Status              string `gorm:"size:32"` // confirmed | safe | uncertain
	Confidence          int
	ExploitPayload      string `gorm:"type:text"`
	ExploitResponse     string `gorm:"type:text"`
	AIAnalysis          string `gorm:"type:text"`
	Fix                 string `gorm:"type:text"`
}

// Auth credential refresh state constants.
const (
	AuthRefreshStateFresh   = "fresh"
	AuthRefreshStateExpiring = "expiring"
	AuthRefreshStateStale   = "stale"
)

// AuthCredential stores authentication tokens/cookies obtained during auth bootstrap or Phase5 Step1.
type AuthCredential struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	AuthType    string     // cookie_session | jwt_bearer | basic_auth | api_key
	Username    string
	TokenValue  string     `gorm:"type:text"`
	HeaderName  string     // e.g. Authorization, Cookie, X-API-Key (backward compat)
	HeaderValue string     `gorm:"type:text"` // single header value (backward compat)
	URLSpace    string     `gorm:"size:64"`   // routing_profile url_spaces.id (e.g. stage_0_admin, public)
	AuthRealm           string     `gorm:"size:32"`   // admin | web | api | oauth | member
	CredentialGroupID   string     `gorm:"size:32"`   // user input group: admin | user | web | api
	MountPrefix         string     `gorm:"size:128"`  // e.g. /admin, /api
	LoginPath           string     `gorm:"size:256"`  // login endpoint path used to acquire this credential
	LoginEvidenceJSON   string     `gorm:"type:text"` // login POST metadata (no password)
	ValidUntil  *time.Time
	Verified    bool
	VerifyURL   string
	Notes       string

	HeadersJSON      string     `gorm:"type:text"` // canonical source: {"Cookie":"...","X-CSRF-Token":"..."}
	HeadersText      string     `gorm:"type:text"` // derived: "Header: value\r\nHeader2: value2"
	AcquireRecipeID  uint
	LastAcquiredAt   *time.Time
	LastVerifiedAt   *time.Time
	ExpiresHint      string     `gorm:"type:text"` // JSON: {"strategy":"ttl_seconds","ttl_seconds":3600}
	RefreshState     string     `gorm:"size:32;default:'fresh'"`
	ReacquireCount   int
}

// AuthAcquisitionRecipe records how to replay a login flow for credential refresh.
type AuthAcquisitionRecipe struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID    uint
	CredentialID uint

	Method                    string // login_form_post|login_form_json|basic_auth|bearer_input|api_key_input|multi_step
	LoginURL                  string
	StepsJSON                 string `gorm:"type:text"` // []Step JSON
	ExtractRulesJSON          string `gorm:"type:text"`
	VariableSlotsJSON         string `gorm:"type:text"` // {"USERNAME":"admin","PASSWORD":"<from:user_input>"}
	VerifyURL                 string
	VerifySuccessPatternsJSON string `gorm:"type:text"`
	VerifyFailurePatternsJSON string `gorm:"type:text"`
	HeadersMappingJSON        string `gorm:"type:text"` // extracted variables -> final Headers map
	Notes                     string
}

// EndpointValidationAttempt is an audit log entry for each endpoint validation probe.
type EndpointValidationAttempt struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	SessionID        uint
	HttpEndpointID   uint
	AttemptNo        int
	URL              string
	Method           string `gorm:"size:16"`
	RequestHeaders   string `gorm:"type:text"`
	StatusCode       int
	ResponseSnippet  string `gorm:"type:text"`
	Verdict          string `gorm:"size:32"` // alive|rejected|auth_failed|unreachable|needs_ai
	Reason           string
	AIAnalysis       string `gorm:"type:text"`
}

// DynamicVulnFinding stores a vulnerability discovered during Phase5 Step3 greybox scanning.
type DynamicVulnFinding struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint

	HttpEndpointID uint
	VulnType       string // sqli | xss | cmdi | ssrf | path_traversal | upload_bypass
	Severity       string // critical | high | medium | low
	Confidence     int    // 0-100
	Payload        string
	RequestURL     string
	RequestRaw     string `gorm:"type:text"`
	ResponseRaw    string `gorm:"type:text"`
	Evidence       string `gorm:"type:text"`
	Status         string // confirmed | uncertain | false_positive
	AIAnalysis     string `gorm:"type:text"`
	CodeContext     string `gorm:"type:text"`
}

// EndpointVulnProbe stores per-endpoint per-vuln_type deep-mining probe outcome.
type EndpointVulnProbe struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint `gorm:"unique_index:idx_evprobe_session_api_type"`

	VerifiedHttpApiID uint   `gorm:"unique_index:idx_evprobe_session_api_type"`
	VulnType          string `gorm:"size:64;unique_index:idx_evprobe_session_api_type"`
	Status            string `gorm:"size:32"` // confirmed | safe | uncertain | skipped
	SkipReason        string `gorm:"type:text"`
	Payload           string `gorm:"type:text"`
	RequestURL        string `gorm:"type:text"`
	ResponseExcerpt   string `gorm:"type:text"`
	AIAnalysis        string `gorm:"type:text"`
	Source            string `gorm:"size:32"` // deep_mining
}

func (EndpointVulnProbe) TableName() string { return "endpoint_vuln_probes" }

func (DiscoverySession) TableName() string      { return "discovery_sessions" }
func (ArchitectureComponent) TableName() string { return "architecture_components" }
func (ConfigArtifact) TableName() string        { return "config_artifacts" }
func (DependencyRef) TableName() string         { return "dependency_refs" }
func (HttpEndpoint) TableName() string          { return "http_endpoints" }
func (SecurityMechanism) TableName() string     { return "security_mechanisms" }
func (BusinessCapability) TableName() string    { return "business_capabilities" }
func (DiscoveryEvent) TableName() string        { return "discovery_events" }
func (VerifiedHttpApi) TableName() string       { return "verified_http_apis" }
func (VerifiedEndpoint) TableName() string      { return "verified_endpoints" }
func (DiscoverySyntaxFlowFinding) TableName() string {
	return "discovery_syntaxflow_findings"
}
func (VulnVerification) TableName() string    { return "vuln_verifications" }
func (AuthCredential) TableName() string            { return "auth_credentials" }
func (AuthAcquisitionRecipe) TableName() string     { return "auth_acquisition_recipes" }
func (EndpointValidationAttempt) TableName() string { return "endpoint_validation_attempts" }
func (DynamicVulnFinding) TableName() string        { return "dynamic_vuln_findings" }
func (CoverageWorkItem) TableName() string          { return "coverage_work_items" }

// Vuln checklist status constants.
const (
	VulnChecklistStatusPending  = "pending"
	VulnChecklistStatusVerified = "verified"
	VulnChecklistStatusSkipped  = "skipped"
)

// VulnChecklistItem links a static SyntaxFlow finding to an HTTP endpoint for Phase5 verification.
type VulnChecklistItem struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint `gorm:"not null;index"`

	FindingID         uint
	EndpointID        uint
	VerifiedHttpApiID uint
	RuleName          string
	Severity          string
	Title             string
	MatchedFile       string
	DataFlowHint      string `gorm:"type:text"`
	Method            string
	PathPattern       string
	FullSampleURL     string
	HandlerClass      string
	Priority          int
	AssocConfidence   string `gorm:"size:16;default:'none'"`
	Status            string `gorm:"size:16;default:'pending'"`
}

// DiscoveryFileOperation records a single file-level pipeline action for audit/debug.
type DiscoveryFileOperation struct {
	ID            uint `gorm:"primaryKey"`
	CreatedAt     time.Time
	SessionID     uint   `gorm:"not null;index:idx_file_op_sess_stage;index:idx_file_op_sess_path;index:idx_file_op_sess_op"`
	PipelineStage string `gorm:"size:32;not null;index:idx_file_op_sess_stage"`
	Operation     string `gorm:"size:32;not null;index:idx_file_op_sess_op"`
	RelPath       string `gorm:"size:512;index:idx_file_op_sess_path"`
	RuleID        string `gorm:"size:64"`
	ToolName      string `gorm:"size:64"`
	Outcome       string `gorm:"size:32"`
	Summary       string `gorm:"size:512"`
	DetailJSON    string `gorm:"type:text"`
	DurationMs    int
}

func (DiscoveryFileOperation) TableName() string { return "discovery_file_operations" }

// File operation stage / operation / outcome constants.
const (
	FileOpStageBootstrap   = "bootstrap"
	FileOpStagePhase1A     = "phase1a"
	FileOpStagePhase1BPre  = "phase1b_pre"
	FileOpStagePhase1BReact = "phase1b_react"
	FileOpStagePhase1C     = "phase1c"

	FileOpScanSkip       = "scan_skip"
	FileOpFilterExclude  = "filter_exclude"
	FileOpFilterInclude  = "filter_include"
	FileOpStaticHarvest  = "static_harvest"
	FileOpAIRead         = "ai_read"
	FileOpReactReadFile  = "react_read_file"

	FileOpOutcomeIncluded  = "included"
	FileOpOutcomeExcluded  = "excluded"
	FileOpOutcomeProcessed = "processed"
	FileOpOutcomeSkipped   = "skipped"
	FileOpOutcomeFailed     = "failed"
)

// Phase artifact kind constants (large JSON blobs from Yak tools / prep phases).
const (
	ArtifactStaticRouteHints         = "static_route_hints"
	ArtifactAuthSurface              = "auth_surface"
	ArtifactAuthEvidence             = "auth_evidence"
	ArtifactDependencies             = "dependencies"
	ArtifactPhase1PrepBundle         = "phase1_prep_bundle"
	ArtifactCodeReadingPlan          = "code_reading_plan"
	ArtifactCodeReadingPlanBuildMeta = "code_reading_plan_build_meta"
	ArtifactBackendScope             = "backend_scope"
	ArtifactForwardingProfile        = "forwarding_profile"
	ArtifactApiPreanalysisFull       = "api_preanalysis_full"
	ArtifactSyntaxflowSummary        = "syntaxflow_summary"
	ArtifactApiArchMap               = "api_arch_map"
	ArtifactProjectProfile           = "project_profile"
	ArtifactApiCatalog               = "api_catalog"
	ArtifactCodeReadingStage         = "code_reading_stage"
	ArtifactPhase1Recon              = "phase1_recon"
	ArtifactPhase1AuthFailure        = "phase1_auth_failure"
	ArtifactJavaBusinessScopeInventory = "java_business_scope_inventory"
	ArtifactCodeScopeInventory       = "code_scope_inventory"
	ArtifactTechArchitecture         = "tech_architecture"
	ArtifactBusinessFunctionMap      = "business_function_map"
	ArtifactFeatureInventory         = "feature_inventory"
	ArtifactAuthSurfaceMap           = "auth_surface_map"
	ArtifactAuthRealmInventory       = "auth_realm_inventory"
	ArtifactAuthMechanismMap         = "auth_mechanism_map"
	ArtifactFailureSemantics         = "failure_semantics"
	ArtifactServletRoutingMap        = "servlet_routing_map"
	ArtifactFrontendAPIHarvest       = "frontend_api_harvest"
	ArtifactFrontendAPIInventory     = "frontend_api_inventory"
	ArtifactCombinedAPICatalog       = "combined_api_catalog"
	ArtifactFrameworkToolkitSelection = "framework_toolkit_selection"
	ArtifactAuthCalibration          = "auth_calibration"
	ArtifactFeatureApiMap            = "feature_api_map"
	ArtifactComponentPackageMap      = "component_package_map"
	ArtifactProjectContextSummary    = "project_context_summary"
	ArtifactCodeUnitRegistry         = "code_unit_registry"
	ArtifactFeatureCodeAnalysisMap   = "feature_code_analysis_map"
	ArtifactCoverageSignal          = "coverage_signal"
	ArtifactDirectoryTree          = "directory_tree"

	// New unified endpoint extraction artifacts
	ArtifactJavaFrameworkDetector = "java_framework_detector"
	ArtifactUnifiedEndpoints     = "unified_endpoints"
	ArtifactValidationReport    = "validation_report"
)

// PhaseArtifact stores a versioned JSON payload for a session phase output.
type PhaseArtifact struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	SessionID uint   `gorm:"not null;uniqueIndex:ux_phase_artifact_sess_kind"`
	Kind      string `gorm:"size:64;not null;uniqueIndex:ux_phase_artifact_sess_kind"`
	Version   int    `gorm:"default:1"`
	PayloadJSON string `gorm:"type:text"`
}

func (VulnChecklistItem) TableName() string { return "vuln_checklist_items" }
func (PhaseArtifact) TableName() string    { return "phase_artifacts" }
