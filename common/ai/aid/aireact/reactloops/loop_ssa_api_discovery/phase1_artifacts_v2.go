package loop_ssa_api_discovery

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	artifactV2SchemaVersion = 1
)

// ComponentPackageMapV1 maps modules/packages to controller layers (T2 output).
type ComponentPackageMapV1 struct {
	SchemaVersion int                       `json:"schema_version"`
	GeneratedAt   string                    `json:"generated_at"`
	Language      string                    `json:"language,omitempty"`
	Components    []ComponentPackageEntry   `json:"components"`
}

type ComponentPackageEntry struct {
	ID               string   `json:"id"`
	Label            string   `json:"label,omitempty"`
	ModuleName       string   `json:"module_name,omitempty"`
	PackagePatterns  []string `json:"package_patterns"`
	ControllerLayer  string   `json:"controller_layer,omitempty"` // admin|web|api|internal
	EvidenceRefs     []string `json:"evidence_refs,omitempty"`
}

// ProjectContextSummaryV1 describes the project and first-party vs third-party boundaries (P0 output).
type ProjectContextSummaryV1 struct {
	SchemaVersion    int                     `json:"schema_version"`
	GeneratedAt      string                  `json:"generated_at"`
	ProjectName      string                  `json:"project_name,omitempty"`
	ProjectType      string                  `json:"project_type,omitempty"`
	Summary          string                  `json:"summary"`
	PrimaryLanguage  string                  `json:"primary_language,omitempty"`
	Frameworks       []string                `json:"frameworks,omitempty"`
	BusinessModules  []ProjectBusinessModule `json:"business_modules,omitempty"`
	FirstPartyBoundary  ProjectCodeBoundary  `json:"first_party_boundary"`
	ThirdPartyBoundary  ProjectCodeBoundary  `json:"third_party_boundary"`
	EvidenceRefs     []string                `json:"evidence_refs,omitempty"`
}

type ProjectBusinessModule struct {
	ModuleRoot   string `json:"module_root"`
	Role         string `json:"role,omitempty"`
	JavaPathHint string `json:"java_path_hint,omitempty"`
}

type ProjectCodeBoundary struct {
	Description     string   `json:"description,omitempty"`
	ModuleRoots     []string `json:"module_roots,omitempty"`
	PathPatterns    []string `json:"path_patterns,omitempty"`
	PackageRoots    []string `json:"package_roots,omitempty"`
	PackagePrefixes []string `json:"package_prefixes,omitempty"`
	Examples        []string `json:"examples,omitempty"`
}

// FeatureInventoryV1 lists business features with package ownership (F1 output).
type FeatureInventoryV1 struct {
	SchemaVersion int                      `json:"schema_version"`
	GeneratedAt   string                   `json:"generated_at"`
	Language      string                   `json:"language,omitempty"`
	Features      []FeatureInventoryEntry  `json:"features"`
	Coverage      FeatureCoverageResult    `json:"coverage"`
}

type FeatureInventoryEntry struct {
	FeatureID       string   `json:"feature_id"`
	Label           string   `json:"label,omitempty"`
	Description     string   `json:"description,omitempty"`
	PackagePatterns []string `json:"package_patterns"`
	SurfaceKind     string   `json:"surface_kind,omitempty"` // http_api | code_only
	EntryFiles      []string `json:"entry_files,omitempty"`
	ControllerFiles []string `json:"controller_files,omitempty"` // legacy alias; use EntryFiles
	NoHttpReason    string   `json:"no_http_reason,omitempty"`
	BusinessSurface string   `json:"business_surface,omitempty"`
}

const (
	SurfaceKindHTTPAPI  = "http_api"
	SurfaceKindCodeOnly = "code_only"
)

type FeatureCoverageResult struct {
	Policy         string   `json:"policy"`
	TotalRequired  int      `json:"total_required"`
	Covered        int      `json:"covered"`
	Complete       bool     `json:"complete"`
	UncoveredPaths []string `json:"uncovered_paths,omitempty"`
}

// AuthRealmInventoryV1 lists discovered auth realms (A1 output).
type AuthRealmInventoryV1 struct {
	SchemaVersion int                `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	MultiAuth     bool               `json:"multi_auth"`
	Realms        []AuthRealmSummary `json:"realms"`
}

type AuthRealmSummary struct {
	AuthRealm   string `json:"auth_realm"`
	Label       string `json:"label,omitempty"`
	URLSpace    string `json:"url_space,omitempty"`
	MountPrefix string `json:"mount_prefix,omitempty"`
	Evidence    string `json:"evidence,omitempty"`
}

// AuthMechanismDetailV1 per-realm mechanism (A2 output, merged into auth_surface_map).
type AuthMechanismDetailV1 struct {
	AuthRealm           string           `json:"auth_realm"`
	SessionMechanism    string           `json:"session_mechanism,omitempty"`
	SessionFields       []string         `json:"session_fields,omitempty"`
	PasswordTransform   string           `json:"password_transform,omitempty"`
	FilterChain         []string         `json:"filter_chain,omitempty"`
	InterceptorChain    []string         `json:"interceptor_chain,omitempty"`
	LoginMethod         string           `json:"login_method,omitempty"`
	LoginPath           string           `json:"login_path,omitempty"`
	LoginPageKind       string           `json:"login_page_kind,omitempty"` // backend|frontend|unknown
	LoginPagePath       string           `json:"login_page_path,omitempty"`
	LoginPostPath       string           `json:"login_post_path,omitempty"`
	LoginFormFields     []LoginFormField `json:"login_form_fields,omitempty"`
	ContentType         string           `json:"content_type,omitempty"`
	MechanismDetail     string           `json:"mechanism_detail,omitempty"`
	CodeEvidence        []string         `json:"code_evidence,omitempty"`
}

// LoginFormField describes one input from login page HTML/JS evidence.
type LoginFormField struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
	Default  string `json:"default,omitempty"`
	Evidence string `json:"evidence,omitempty"`
}

// AuthMechanismMapV1 persists per-realm mechanism details from A2.
type AuthMechanismMapV1 struct {
	SchemaVersion int                              `json:"schema_version"`
	GeneratedAt   string                           `json:"generated_at"`
	Realms        map[string]AuthMechanismDetailV1 `json:"realms"`
}

// AuthSurfaceMapV1 binds packages/paths to auth realms (A3 output).
type AuthSurfaceMapV1 struct {
	SchemaVersion int                `json:"schema_version"`
	GeneratedAt   string             `json:"generated_at"`
	MultiAuth     bool               `json:"multi_auth"`
	Surfaces      []AuthSurfaceEntry `json:"surfaces"`
}

type AuthSurfaceEntry struct {
	AuthRealm         string           `json:"auth_realm"`
	URLSpace          string           `json:"url_space,omitempty"`
	MountPrefix       string           `json:"mount_prefix,omitempty"`
	PackagePatterns   []string         `json:"package_patterns"`
	PathPrefixes      []string         `json:"path_prefixes,omitempty"`
	SessionMechanism  string           `json:"session_mechanism,omitempty"`
	SessionFields     []string         `json:"session_fields,omitempty"`
	PasswordTransform string           `json:"password_transform,omitempty"`
	LoginPath         string           `json:"login_path,omitempty"`
	LoginMethod       string           `json:"login_method,omitempty"`
	LoginPageKind     string           `json:"login_page_kind,omitempty"`
	LoginPagePath     string           `json:"login_page_path,omitempty"`
	LoginPostPath     string           `json:"login_post_path,omitempty"`
	LoginFormFields   []LoginFormField `json:"login_form_fields,omitempty"`
	ContentType       string           `json:"content_type,omitempty"`
	MechanismDetail   string           `json:"mechanism_detail,omitempty"`
	FilterChain       []string         `json:"filter_chain,omitempty"`
	CodeEvidence      []string         `json:"code_evidence,omitempty"`
}

// FailureSemanticsV1 documents how the app signals probe failures (X1 output).
type FailureSemanticsV1 struct {
	SchemaVersion int                        `json:"schema_version"`
	GeneratedAt   string                     `json:"generated_at"`
	Categories    []FailureSemanticsCategory `json:"categories"`
}

type FailureSemanticsCategory struct {
	Kind            string   `json:"kind"` // wrong_path|wrong_method|wrong_param|unauthorized|success
	Description     string   `json:"description,omitempty"`
	StatusCodes     []int    `json:"status_codes,omitempty"`
	BodyPatterns    []string `json:"body_patterns,omitempty"`
	HeaderPatterns  []string `json:"header_patterns,omitempty"`
	ContentTypeHint string   `json:"content_type_hint,omitempty"`
	CodeEvidence    []string `json:"code_evidence,omitempty"`
	RouteVerdict    string   `json:"route_verdict,omitempty"`
}

// AuthCalibrationV1 records per-realm live calibration (C1 output).
type AuthCalibrationV1 struct {
	SchemaVersion int                     `json:"schema_version"`
	GeneratedAt   string                  `json:"generated_at"`
	AllCalibrated bool                    `json:"all_calibrated"`
	Realms        []AuthCalibrationRealm  `json:"realms"`
}

type AuthCalibrationRealm struct {
	AuthRealm    string                  `json:"auth_realm"`
	MountPrefix  string                  `json:"mount_prefix,omitempty"`
	CredentialID uint                    `json:"credential_id,omitempty"`
	Calibrated   bool                    `json:"calibrated"`
	Detail       string                  `json:"detail,omitempty"`
	Probes       []AuthCalibrationProbe  `json:"probes"`
}

type AuthCalibrationProbe struct {
	Method          string `json:"method"`
	Path            string `json:"path"`
	FullURL         string `json:"full_url,omitempty"`
	StatusCode      int    `json:"status_code,omitempty"`
	ResponseExcerpt string `json:"response_excerpt,omitempty"`
	ClassifiedAs    string `json:"classified_as,omitempty"`
	ExpectedKind    string `json:"expected_kind,omitempty"`
	Passed          bool   `json:"passed"`
	Notes           string `json:"notes,omitempty"`
}

// FeatureApiMapV1 per-feature API discovery+verify results (V output).
type FeatureApiMapV1 struct {
	SchemaVersion int                  `json:"schema_version"`
	GeneratedAt   string               `json:"generated_at"`
	Features      []FeatureApiMapEntry `json:"features"`
}

type FeatureApiMapEntry struct {
	FeatureID    string            `json:"feature_id"`
	Label        string            `json:"label,omitempty"`
	ApiCount     int               `json:"api_count"`
	NoApiReason  string            `json:"no_api_reason,omitempty"`
	Processed    bool              `json:"processed"`
	Apis         []FeatureApiEntry `json:"apis,omitempty"`
}

type FeatureApiEntry struct {
	Method        string `json:"method"`
	PathPattern   string `json:"path_pattern"`
	HandlerFile   string `json:"handler_file,omitempty"`
	HandlerClass  string `json:"handler_class,omitempty"`
	HandlerSymbol string `json:"handler_symbol,omitempty"`
	AuthRealm     string `json:"auth_realm,omitempty"`
	AuthAccess    string `json:"auth_access,omitempty"`
	Verified      bool   `json:"verified"`
	RejectReason  string `json:"reject_reason,omitempty"`
	VerdictReason string `json:"verdict_reason,omitempty"`
	FullSampleURL string `json:"full_sample_url,omitempty"`
}

// EntryFilesForFeature returns the dispatch entry file list, preferring entry_files over legacy controller_files.
func EntryFilesForFeature(f FeatureInventoryEntry) []string {
	if len(f.EntryFiles) > 0 {
		return append([]string(nil), f.EntryFiles...)
	}
	if len(f.ControllerFiles) > 0 {
		return append([]string(nil), f.ControllerFiles...)
	}
	return nil
}

type CodeAnalysisFunction struct {
	Name      string   `json:"name"`
	Signature string   `json:"signature,omitempty"`
	Line      int      `json:"line,omitempty"`
	Role      string   `json:"role,omitempty"`
	CallsOut  []string `json:"calls_out,omitempty"`
}

type CodeAnalysisUnitResult struct {
	EntryFile        string                 `json:"entry_file"`
	FeatureID        string                 `json:"feature_id"`
	Summary          string                 `json:"summary,omitempty"`
	Functions        []CodeAnalysisFunction `json:"functions,omitempty"`
	Stats            map[string]any         `json:"stats,omitempty"`
	Capabilities     []string               `json:"capabilities,omitempty"`
	NoCallableReason string                 `json:"no_callable_reason,omitempty"`
}

type FeatureCodeAnalysisMapV1 struct {
	SchemaVersion int                      `json:"schema_version"`
	GeneratedAt   string                   `json:"generated_at,omitempty"`
	Units         []CodeAnalysisUnitResult `json:"units"`
}

func writeArtifactJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return writeJSONFile(path, b)
}

func persistPhaseArtifact(rt *Runtime, kind string, payload string) {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return
	}
	_ = rt.Repo.UpsertPhaseArtifact(rt.Session.ID, kind, payload)
}

func loadJSONArtifact[T any](path string, target *T) (*T, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, target); err != nil {
		return nil, err
	}
	return target, nil
}

func loadFeatureInventory(workDir string) (*FeatureInventoryV1, error) {
	var inv FeatureInventoryV1
	return loadJSONArtifact(store.FeatureInventoryPath(workDir), &inv)
}

func loadAuthSurfaceMap(workDir string) (*AuthSurfaceMapV1, error) {
	var m AuthSurfaceMapV1
	return loadJSONArtifact(store.AuthSurfaceMapPath(workDir), &m)
}

func loadAuthRealmInventory(workDir string) (*AuthRealmInventoryV1, error) {
	var inv AuthRealmInventoryV1
	return loadJSONArtifact(store.AuthRealmInventoryPath(workDir), &inv)
}

func loadAuthMechanismMap(workDir string) (*AuthMechanismMapV1, error) {
	var m AuthMechanismMapV1
	return loadJSONArtifact(store.AuthMechanismMapPath(workDir), &m)
}

func loadFailureSemantics(workDir string) (*FailureSemanticsV1, error) {
	var fs FailureSemanticsV1
	return loadJSONArtifact(store.FailureSemanticsPath(workDir), &fs)
}

func loadAuthCalibration(workDir string) (*AuthCalibrationV1, error) {
	var c AuthCalibrationV1
	return loadJSONArtifact(store.AuthCalibrationPath(workDir), &c)
}

func loadFeatureApiMap(workDir string) (*FeatureApiMapV1, error) {
	var m FeatureApiMapV1
	return loadJSONArtifact(store.FeatureApiMapPath(workDir), &m)
}

func loadFeatureCodeAnalysisMap(workDir string) (*FeatureCodeAnalysisMapV1, error) {
	var m FeatureCodeAnalysisMapV1
	return loadJSONArtifact(store.FeatureCodeAnalysisMapPath(workDir), &m)
}

func loadComponentPackageMap(workDir string) (*ComponentPackageMapV1, error) {
	var m ComponentPackageMapV1
	return loadJSONArtifact(store.ComponentPackageMapPath(workDir), &m)
}

func loadProjectContextSummary(workDir string) (*ProjectContextSummaryV1, error) {
	var summary ProjectContextSummaryV1
	return loadJSONArtifact(store.ProjectContextSummaryPath(workDir), &summary)
}

func persistProjectContextSummary(rt *Runtime, summary *ProjectContextSummaryV1) error {
	if summary == nil {
		return utils.Error("nil project context summary")
	}
	if summary.SchemaVersion == 0 {
		summary.SchemaVersion = artifactV2SchemaVersion
	}
	if summary.GeneratedAt == "" {
		summary.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(summary, "", "  ")
	if err := writeArtifactJSON(store.ProjectContextSummaryPath(rt.WorkDir), summary); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactProjectContextSummary, string(b))
	return nil
}

func persistFeatureInventory(rt *Runtime, inv *FeatureInventoryV1) error {
	if inv == nil {
		return utils.Error("nil feature inventory")
	}
	if inv.SchemaVersion == 0 {
		inv.SchemaVersion = artifactV2SchemaVersion
	}
	if inv.GeneratedAt == "" {
		inv.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(inv, "", "  ")
	if err := writeArtifactJSON(store.FeatureInventoryPath(rt.WorkDir), inv); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactFeatureInventory, string(b))
	return nil
}

func persistAuthSurfaceMap(rt *Runtime, m *AuthSurfaceMapV1) error {
	if m == nil {
		return utils.Error("nil auth surface map")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = artifactV2SchemaVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if rt != nil {
		if sm, err := loadServletRoutingMap(rt.WorkDir); err == nil && sm != nil {
			mergeServletMapIntoAuthSurface(sm, m)
		}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := writeArtifactJSON(store.AuthSurfaceMapPath(rt.WorkDir), m); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactAuthSurfaceMap, string(b))
	// Sync login endpoints into auth_evidence; preserve DB-backed fields when refreshing.
	ev := authEvidenceFromSurfaceMap(m)
	if ev != nil {
		if existing, err := loadAuthEvidenceFromWorkDir(rt.WorkDir); err == nil && existing != nil {
			mergeAuthEvidencePreservingCredentials(existing, ev)
		}
		raw, _ := json.MarshalIndent(ev, "", "  ")
		_ = writeJSONFile(store.AuthEvidencePath(rt.WorkDir), raw)
		persistPhaseArtifact(rt, store.ArtifactAuthEvidence, string(raw))
	}
	_ = RefreshAuthEvidenceFromDB(rt)
	return MergeAuthSurfaceIntoRoutingProfile(rt)
}

func mergeAuthEvidencePreservingCredentials(existing, next *AuthEvidenceRecord) {
	if existing == nil || next == nil {
		return
	}
	if existing.Verified && !next.Verified {
		next.Verified = existing.Verified
	}
	if strings.TrimSpace(existing.VerificationDetail) != "" && strings.TrimSpace(next.VerificationDetail) == "" {
		next.VerificationDetail = existing.VerificationDetail
	}
	if len(existing.CredentialBindings) > 0 && len(next.CredentialBindings) == 0 {
		next.CredentialBindings = existing.CredentialBindings
		next.CredentialID = existing.CredentialID
	}
	for i := range next.LoginEndpoints {
		ep := &next.LoginEndpoints[i]
		for _, old := range existing.LoginEndpoints {
			if old.CredentialID != 0 && credentialMatchesEndpoint(AuthCredentialBinding{
				CredentialID: old.CredentialID,
				AuthRealm:    NormalizeAuthRealm(old.AuthRealm),
				URLSpace:     old.URLSpace,
				MountPrefix:  normURLPath(old.MountPrefix),
			}, *ep) {
				ep.CredentialID = old.CredentialID
				break
			}
		}
	}
}

func authEvidenceFromSurfaceMap(m *AuthSurfaceMapV1) *AuthEvidenceRecord {
	if m == nil {
		return nil
	}
	ev := &AuthEvidenceRecord{
		MultiAuth: m.MultiAuth,
		Verified:  false,
	}
	for _, s := range m.Surfaces {
		ep := AuthLoginEndpoint{
			Method:            firstNonEmpty(s.LoginMethod, "POST"),
			Path:              s.LoginPath,
			AuthRealm:         s.AuthRealm,
			URLSpace:          s.URLSpace,
			MountPrefix:       s.MountPrefix,
			ContentType:       s.ContentType,
			PasswordTransform: s.PasswordTransform,
			CodeEvidence:      strings.Join(s.CodeEvidence, "; "),
		}
		if ep.Path != "" {
			ev.LoginEndpoints = append(ev.LoginEndpoints, ep)
		}
		if s.SessionMechanism != "" && ev.SessionMechanism == "" {
			ev.SessionMechanism = s.SessionMechanism
		}
	}
	return ev
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func persistAuthRealmInventory(rt *Runtime, inv *AuthRealmInventoryV1) error {
	if inv == nil {
		return utils.Error("nil auth realm inventory")
	}
	if inv.SchemaVersion == 0 {
		inv.SchemaVersion = artifactV2SchemaVersion
	}
	if inv.GeneratedAt == "" {
		inv.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(inv, "", "  ")
	if err := writeArtifactJSON(store.AuthRealmInventoryPath(rt.WorkDir), inv); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactAuthRealmInventory, string(b))
	return nil
}

func persistAuthMechanismDetail(rt *Runtime, detail *AuthMechanismDetailV1) error {
	if rt == nil || detail == nil {
		return utils.Error("nil runtime or mechanism detail")
	}
	realm := NormalizeAuthRealm(detail.AuthRealm)
	if realm == "" {
		return utils.Error("auth_realm required for mechanism map")
	}
	m, err := loadAuthMechanismMap(rt.WorkDir)
	if err != nil || m == nil {
		m = &AuthMechanismMapV1{
			SchemaVersion: artifactV2SchemaVersion,
			Realms:        map[string]AuthMechanismDetailV1{},
		}
	}
	if m.Realms == nil {
		m.Realms = map[string]AuthMechanismDetailV1{}
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = artifactV2SchemaVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	detail.AuthRealm = realm
	m.Realms[realm] = *detail
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := writeArtifactJSON(store.AuthMechanismMapPath(rt.WorkDir), m); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactAuthMechanismMap, string(b))
	return nil
}

func persistAuthMechanismMap(rt *Runtime, mechanisms map[string]AuthMechanismDetailV1) error {
	if rt == nil || len(mechanisms) == 0 {
		return nil
	}
	for _, d := range mechanisms {
		detail := d
		if err := persistAuthMechanismDetail(rt, &detail); err != nil {
			return err
		}
	}
	return nil
}

func persistFailureSemantics(rt *Runtime, fs *FailureSemanticsV1) error {
	if fs == nil {
		return utils.Error("nil failure semantics")
	}
	if fs.SchemaVersion == 0 {
		fs.SchemaVersion = artifactV2SchemaVersion
	}
	if fs.GeneratedAt == "" {
		fs.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(fs, "", "  ")
	if err := writeArtifactJSON(store.FailureSemanticsPath(rt.WorkDir), fs); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactFailureSemantics, string(b))
	return nil
}

func persistAuthCalibration(rt *Runtime, c *AuthCalibrationV1) error {
	if c == nil {
		return utils.Error("nil auth calibration")
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = artifactV2SchemaVersion
	}
	if c.GeneratedAt == "" {
		c.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	if err := writeArtifactJSON(store.AuthCalibrationPath(rt.WorkDir), c); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactAuthCalibration, string(b))
	return RefreshAuthEvidenceFromDB(rt)
}

func persistFeatureApiMap(rt *Runtime, m *FeatureApiMapV1) error {
	if m == nil {
		return utils.Error("nil feature api map")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = artifactV2SchemaVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := writeArtifactJSON(store.FeatureApiMapPath(rt.WorkDir), m); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactFeatureApiMap, string(b))
	return nil
}

func persistFeatureCodeAnalysisMap(rt *Runtime, m *FeatureCodeAnalysisMapV1) error {
	if m == nil {
		return utils.Error("nil feature code analysis map")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = artifactV2SchemaVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := writeArtifactJSON(store.FeatureCodeAnalysisMapPath(rt.WorkDir), m); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactFeatureCodeAnalysisMap, string(b))
	return nil
}

func mergeFeatureCodeAnalysisUnit(m *FeatureCodeAnalysisMapV1, unit CodeAnalysisUnitResult) {
	if m == nil {
		return
	}
	for i := range m.Units {
		if m.Units[i].EntryFile == unit.EntryFile {
			m.Units[i] = unit
			return
		}
	}
	m.Units = append(m.Units, unit)
}

func ensureFeatureCodeAnalysisMap(rt *Runtime) (*FeatureCodeAnalysisMapV1, error) {
	m, err := loadFeatureCodeAnalysisMap(rt.WorkDir)
	if err == nil && m != nil {
		return m, nil
	}
	return &FeatureCodeAnalysisMapV1{SchemaVersion: artifactV2SchemaVersion, Units: []CodeAnalysisUnitResult{}}, nil
}

func persistComponentPackageMap(rt *Runtime, m *ComponentPackageMapV1) error {
	if m == nil {
		return utils.Error("nil component package map")
	}
	if m.SchemaVersion == 0 {
		m.SchemaVersion = artifactV2SchemaVersion
	}
	if m.GeneratedAt == "" {
		m.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := writeArtifactJSON(store.ComponentPackageMapPath(rt.WorkDir), m); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactComponentPackageMap, string(b))
	return nil
}

func mergeFeatureApiMapEntry(m *FeatureApiMapV1, entry FeatureApiMapEntry) {
	if m == nil {
		return
	}
	for i := range m.Features {
		if m.Features[i].FeatureID == entry.FeatureID {
			m.Features[i] = entry
			return
		}
	}
	m.Features = append(m.Features, entry)
}

func ensureFeatureApiMap(rt *Runtime) (*FeatureApiMapV1, error) {
	m, err := loadFeatureApiMap(rt.WorkDir)
	if err == nil && m != nil {
		return m, nil
	}
	return &FeatureApiMapV1{SchemaVersion: artifactV2SchemaVersion, Features: []FeatureApiMapEntry{}}, nil
}

func buildEmbeddedContextBlock(title string, workDir string, loaders ...func(string) (string, error)) string {
	var parts []string
	parts = append(parts, "## "+title)
	for _, load := range loaders {
		if load == nil {
			continue
		}
		s, err := load(workDir)
		if err != nil || strings.TrimSpace(s) == "" {
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n\n")
}

func readArtifactExcerpt(path string, max int) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := string(b)
	if max > 0 && len(s) > max {
		s = s[:max] + "\n...(truncated)"
	}
	return s, nil
}

// -------------------------------------------------------------------------------------
// Phase D: Directory Analysis Artifacts
// -------------------------------------------------------------------------------------

// DirectoryTreeV1 is the output of Phase D (directory BFS analysis).
type DirectoryTreeV1 struct {
	SchemaVersion int             `json:"schema_version"`
	GeneratedAt   string          `json:"generated_at"`
	BackendRoot   string         `json:"backend_root"` // e.g. publiccms-core/src/main/java
	TotalSizeKB   int64          `json:"total_size_kb"`
	TotalDirs     int            `json:"total_dirs"`
	TotalFiles    int            `json:"total_files"`
	Nodes         []DirectoryNode `json:"nodes"` // flat list; parent_id links to parent
}

// DirectoryNode is a single node in the directory tree.
type DirectoryNode struct {
	ID            string       `json:"id"`            // UUID
	ParentID      string       `json:"parent_id"`      // "" for root
	RelPath       string       `json:"rel_path"`       // relative to BackendRoot
	Depth         int          `json:"depth"`          // root = 0
	DirectSizeKB  int64        `json:"direct_size_kb"`
	TotalSizeKB   int64        `json:"total_size_kb"`
	FileCount     int          `json:"file_count"`
	FileNames     []string     `json:"file_names"`    // immediate child file names
	PackageHint   string       `json:"package_hint"`   // package prefix from first .java file
	Analysis      *DirAnalysis `json:"analysis,omitempty"`
}

// DirAnalysis is the result of AI-analyzing a single directory.
type DirAnalysis struct {
	FunctionDesc  string   `json:"function_desc"`   // Chinese description (max 50 chars)
	TechLayers   []string `json:"tech_layers"`    // tech:*
	BizDomains   []string `json:"biz_domains"`    // biz:*
	DbFeatures   []string `json:"db_features"`    // db:*
	BfsControl   string   `json:"bfs_control"`   // bfs:stop | bfs:continue | bfs:leaf
	IsBusiness   bool     `json:"is_business"`   // true if business code
	IsHttpEntry  bool     `json:"is_http_entry"` // true if contains HTTP entry
	HasDB        bool     `json:"has_db"`         // true if contains DB operations
	HttpEntryFiles []string `json:"http_entry_files,omitempty"` // controller file names
	// ApiFeature, AuthFeature, RouteSpace are filled in auth mechanism detection phase.
	DepInfo *DepInfo `json:"dependency_info,omitempty"` // non-nil when is_business=false
}

// DepInfo describes a third-party dependency library.
type DepInfo struct {
	Name        string `json:"name"`
	Group       string `json:"group"`
	Version     string `json:"version"`
	Description string `json:"description"`
}

// WorkUnitKB is the baseline unit size for splitting/merging work units.
const WorkUnitKB = 8 * 1024 // 8KB (http_entry avg ~7.2KB)

// WorkUnit represents a unit of work derived from directory analysis.
type WorkUnit struct {
	ID          string   `json:"id"`          // UUID
	FeatureID   string   `json:"feature_id"` // derived from label
	Label       string   `json:"label"`       // feature name
	Description string   `json:"description"` // from function_desc
	// Source
	DirIDs     []string `json:"dir_ids"`     // directory IDs included
	EntryFiles []string `json:"entry_files"` // HTTP entry files
	// Effort
	EstimatedKB int64 `json:"estimated_kb"`
	// Priority (lower = higher priority)
	Priority int `json:"priority"`
	// Tags (merged from directory analysis)
	TechLayers []string `json:"tech_layers"`
	BizDomains []string `json:"biz_domains"`
	DbFeatures []string `json:"db_features"`
	// ApiFeature, AuthFeature, RouteSpace filled in auth phase.
	SurfaceKind string `json:"surface_kind"` // http_api | code_only
}

// GetNode returns the node with the given ID, or nil.
func (t *DirectoryTreeV1) GetNode(id string) *DirectoryNode {
	for i := range t.Nodes {
		if t.Nodes[i].ID == id {
			return &t.Nodes[i]
		}
	}
	return nil
}

// persistDirectoryTree writes directory_analysis.json to workDir.
func persistDirectoryTree(rt *Runtime, tree *DirectoryTreeV1) error {
	if tree == nil {
		return utils.Error("nil directory tree")
	}
	if tree.SchemaVersion == 0 {
		tree.SchemaVersion = artifactV2SchemaVersion
	}
	if tree.GeneratedAt == "" {
		tree.GeneratedAt = time.Now().UTC().Format(time.RFC3339)
	}
	b, _ := json.MarshalIndent(tree, "", "  ")
	if err := writeArtifactJSON(store.DirectoryAnalysisPath(rt.WorkDir), tree); err != nil {
		return err
	}
	persistPhaseArtifact(rt, store.ArtifactDirectoryTree, string(b))
	return nil
}

// loadDirectoryTree loads directory_analysis.json from workDir.
func loadDirectoryTree(workDir string) (*DirectoryTreeV1, error) {
	var tree DirectoryTreeV1
	return loadJSONArtifact(store.DirectoryAnalysisPath(workDir), &tree)
}
