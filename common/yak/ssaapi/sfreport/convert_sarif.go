package sfreport

import (
	"io"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	// SarifDriverName identifies the scanning tool to GitHub code scanning.
	// It stays stable across runs and versions: GitHub groups alerts by
	// (tool name, category), so renaming it would orphan every existing alert.
	SarifDriverName = "yaklang-code-scan"
	// SarifDriverFullName is the human readable tool name.
	SarifDriverFullName = "Yaklang SyntaxFlow Static Analysis"
	// SarifDriverOrganization is the vendor shown in the GitHub UI.
	SarifDriverOrganization = "yaklang.io"
	// SarifInformationURI points at the tool homepage.
	SarifInformationURI = "https://github.com/yaklang/yaklang"
	// SarifFingerprintKey is the partial fingerprint name used to keep a
	// finding matched to the same alert when a file moves or lines shift.
	SarifFingerprintKey = "yaklangRiskFeatureHash"
	// SarifRunAutomationID identifies this upload among the repository's
	// scanners, so GitHub tracks its alerts as one set.
	SarifRunAutomationID = "diff-code-check"
)

// sarifResultKind is the SARIF "kind" for every finding.
//
// SARIF only allows a fixed enum here (notApplicable/pass/fail/review/open/
// informational). yak's risk_type (command-injection, xss, ...) is *not*
// valid and made the whole document schema-invalid, so GitHub rejected the
// upload. A reported finding is a failure of the check, so we always emit
// "fail"; the category travels in the rule tags instead.
const sarifResultKind = "fail"

// sarifSecuritySeverity maps a yak severity to the CVSS-like score GitHub
// reads from properties["security-severity"] to label an alert
// (critical/high/medium/low). Without it GitHub falls back to the SARIF level
// and every finding renders as an undifferentiated warning.
var sarifSecuritySeverity = map[schema.SyntaxFlowSeverity]string{
	schema.SFR_SEVERITY_CRITICAL: "9.5",
	schema.SFR_SEVERITY_HIGH:     "8.0",
	schema.SFR_SEVERITY_WARNING:  "5.0",
	schema.SFR_SEVERITY_LOW:      "2.0",
	schema.SFR_SEVERITY_INFO:     "1.0",
}

// SarifReport accumulates findings and keeps the SARIF document it will emit
// up to date as they stream in.
//
// The struct is serializable at any point, so a save taken mid-scan (one per
// product stage, see ScanProject) is a complete, valid document describing
// everything found so far. That is what makes a later stage failing harmless:
// the artifact already holds the findings of the stages that finished.
type SarifReport struct {
	report *sarif.Report
	writer io.Writer

	root *SarifContext
	// run is the single run every finding is merged into. GitHub refuses a
	// document whose runs share one tool name, which is what a run per rule
	// used to produce.
	run *sarif.Run
	// driver is run.Tool.Driver, kept for O(1) rule registration.
	driver   *sarif.ToolComponent
	ruleByID map[string]struct{}
}

// SarifContext accumulates the SARIF entities one result contributes (files,
// locations, data-flow steps) while sharing a single artifact table with the
// root context so several results reference the same artifact index.
type SarifContext struct {
	root *SarifContext

	// sha256 -> index
	_artifacts      []*sarif.Artifact
	_ArtifactsTable map[string]int

	// context for result
	locations []*sarif.Location
	codeFlows []*sarif.CodeFlow

	//TODO: only mark cross function path
	stack []*sarif.Stack
}

func (r *SarifReport) SetWriter(writer io.Writer) error {
	if writer == nil {
		return utils.Errorf("writer is nil")
	}
	r.writer = writer
	return nil
}

func (r *SarifReport) AddSyntaxFlowRisks(risks ...*schema.SSARisk) {
	log.Errorf("The sarif format cannot specify only a single risk for generation")
}

func NewSarifReport() (*SarifReport, error) {
	sarifReport, err := sarif.New(sarif.Version210)
	if err != nil {
		log.Errorf("create sarif.New Report failed: %s", err)
		return nil, err
	}

	driver := sarif.NewDriver(SarifDriverName).
		WithFullName(SarifDriverFullName).
		WithOrganization(SarifDriverOrganization).
		WithInformationURI(SarifInformationURI).
		WithVersion(consts.GetYakVersion())
	run := sarif.NewRun(*sarif.NewTool(driver))
	run.WithAutomationDetails(sarif.NewRunAutomationDetails().WithID(SarifRunAutomationID))
	// A run is emitted even with zero results: GitHub closes the alerts of a
	// (tool, category) upload whose results no longer contain them, so an empty
	// run is what retires findings that were fixed.
	sarifReport.Runs = []*sarif.Run{run}

	return &SarifReport{
		report:   sarifReport,
		root:     NewSarifContext(),
		run:      run,
		driver:   driver,
		ruleByID: map[string]struct{}{},
	}, nil
}

var _ IReport = (*SarifReport)(nil)

// AddSyntaxFlowResult folds one SyntaxFlow result (one rule) into the run and
// leaves the report ready to be serialized.
func (r *SarifReport) AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool {
	if r == nil || result == nil {
		return false
	}
	before := len(r.run.Results)
	r.appendResult(result)
	// New findings can bring new artifacts (files) with them.
	r.run.Artifacts = r.root.Artifacts()
	return len(r.run.Results) > before
}

// Save writes the current document.
//
// It runs at every stage boundary and once more when the whole pipeline is
// done, and each call replaces the previous output rather than appending to
// it. The struct is already current, so this is a plain encode of the
// accumulated state.
func (r *SarifReport) Save() error {
	if r == nil {
		return utils.Errorf("report is nil")
	}
	if r.writer == nil {
		return nil
	}
	if err := rewindReportOutput(r.writer); err != nil {
		return err
	}
	return r.report.PrettyWrite(r.writer)
}

// Report exposes the accumulated document for callers that want to serialize
// or inspect it directly. It is always a valid SARIF 2.1.0 log.
func (r *SarifReport) Report() *sarif.Report {
	if r == nil {
		return nil
	}
	return r.report
}

func (r *SarifReport) appendResult(result *ssaapi.SyntaxFlowResult) {
	SFRule := result.GetRule()
	ruleID := codec.Sha256(SFRule.Content)

	for risk := range result.YieldRisk() {
		value, err := result.GetValue(risk.Variable, int(risk.Index))
		if err != nil {
			log.Errorf("get value from result failed: resultId[%d: %s: %d] %s", result.GetResultID(), risk.Variable, risk.Index, err)
			continue
		}

		sctx := r.root.CreateSubSarifContext()
		sctx.AddSSAValue(value)

		res := sarif.NewRuleResult(ruleID).
			WithMessage(sarif.NewTextMessage(sarifRiskMessage(risk))).
			WithLevel(ToSarifLevel(risk.Severity)).
			WithKind(sarifResultKind).
			WithPartialFingerPrints(sarifFingerprint(risk))

		if rg := value.GetRange(); rg != nil && rg.GetEditor() != nil {
			artifactId := r.root.GetArtifactIdFromEditor(rg.GetEditor())
			if artifactId >= 0 {
				if loc := r.root.CreateLocation(artifactId, rg); loc != nil {
					res.WithLocations([]*sarif.Location{loc})
				}
			}
		}

		if len(sctx.codeFlows) > 0 {
			res.WithCodeFlows(sctx.codeFlows)
		}

		r.registerRule(ruleID, SFRule, risk)
		r.run.Results = append(r.run.Results, res)
	}
}

// registerRule appends the result rule to the driver once per rule ID.
func (r *SarifReport) registerRule(ruleID string, rule *schema.SyntaxFlowRule, risk *schema.SSARisk) {
	if _, ok := r.ruleByID[ruleID]; ok {
		return
	}

	title := strings.TrimSpace(rule.Title)
	if title == "" {
		title = rule.RuleName
	}
	if title == "" {
		title = ruleID
	}

	// A non-empty shortDescription is required by the schema; rule content
	// can be large, so it is only used when there is no description.
	short := strings.TrimSpace(rule.Description)
	if short == "" {
		short = title
	}

	descriptor := sarif.NewRule(ruleID).
		WithName(title).
		WithShortDescription(sarif.NewMultiformatMessageString(short)).
		WithDefaultConfiguration(
			sarif.NewReportingConfiguration().WithLevel(ToSarifLevel(risk.Severity)),
		).
		WithProperties(sarif.Properties{
			"tags":              sarifRuleTags(rule, risk),
			"security-severity": sarifSecuritySeverityFor(risk.Severity),
		})

	if text := strings.TrimSpace(rule.Description); text != "" {
		descriptor = descriptor.WithFullDescription(sarif.NewMultiformatMessageString(text))
	}
	if rule.RuleName != "" {
		descriptor = descriptor.WithHelpURI(SarifInformationURI)
	}

	r.ruleByID[ruleID] = struct{}{}
	r.driver.Rules = append(r.driver.Rules, descriptor)
}

func sarifSecuritySeverityFor(severity schema.SyntaxFlowSeverity) string {
	if score, ok := sarifSecuritySeverity[severity]; ok {
		return score
	}
	return "5.0"
}

// sarifRuleTags mirrors what the previous report path attached: a plain
// "security" tag plus the rule identity. GitHub's SARIF parser rejects tags
// that are not strings, so every entry is coerced.
func sarifRuleTags(rule *schema.SyntaxFlowRule, risk *schema.SSARisk) []string {
	seen := map[string]struct{}{}
	tags := make([]string, 0, 4)
	add := func(raw string) {
		value := strings.TrimSpace(raw)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		tags = append(tags, value)
	}

	add("security")
	add(rule.RuleName)
	add(rule.Tag)
	if risk != nil {
		add(risk.RiskType)
	}
	return tags
}

// sarifFingerprint gives GitHub a stable identity for a finding so re-running
// the scan updates the existing alert instead of opening a new one when the
// line number shifts. RiskFeatureHash is derived from the function, rule and
// variable rather than the position, which is exactly the intent here.
func sarifFingerprint(risk *schema.SSARisk) map[string]interface{} {
	value := strings.TrimSpace(risk.RiskFeatureHash)
	if value == "" {
		value = strings.TrimSpace(risk.Hash)
	}
	if value == "" {
		return nil
	}
	return map[string]interface{}{SarifFingerprintKey: value}
}

// sarifRiskMessage builds a one-line title so the GitHub alert list stays
// readable; the long description stays in the rule metadata.
func sarifRiskMessage(risk *schema.SSARisk) string {
	title := strings.TrimSpace(risk.TitleVerbose)
	if title == "" {
		title = strings.TrimSpace(risk.Title)
	}
	if title == "" {
		title = risk.RiskType
	}
	if title == "" {
		title = "SyntaxFlow risk"
	}
	parts := []string{title}
	if rule := strings.TrimSpace(risk.FromRule); rule != "" {
		parts = append(parts, "rule: "+rule)
	}
	if fn := strings.TrimSpace(risk.FunctionName); fn != "" {
		parts = append(parts, "function: "+fn)
	}
	return strings.Join(parts, " | ")
}

func (s *SarifContext) CreateSubSarifContext() *SarifContext {
	return &SarifContext{
		root: s,
	}
}

func (s *SarifContext) ArtifactsExisted(hash string) (int, bool) {
	if s.root == nil {
		if id, ok := s._ArtifactsTable[hash]; ok {
			return id, true
		}
		id := len(s._artifacts)
		s._ArtifactsTable[hash] = id
		return id, false
	}
	return s.root.ArtifactsExisted(hash)
}

func (s *SarifContext) appendArtifacts(art *sarif.Artifact) {
	if s.root == nil {
		s._artifacts = append(s._artifacts, art)
		return
	}
	s.root.appendArtifacts(art)
}

// Artifacts exposes the collected artifacts for the merged run.
func (s *SarifContext) Artifacts() []*sarif.Artifact {
	if s == nil {
		return nil
	}
	if s.root != nil {
		return s.root.Artifacts()
	}
	return s._artifacts
}

func NewSarifContext() *SarifContext {
	return &SarifContext{
		root:            nil,
		_ArtifactsTable: make(map[string]int),
	}
}

func (s *SarifContext) AddSSAValue(v *ssaapi.Value) {
	rg := v.GetRange()
	editor := rg.GetEditor()
	if editor == nil {
		log.Warn("editor is nil (nil editor value cannot be treated as sarif.CodeFlow)")
		return
	}

	artifactId := s.GetArtifactIdFromEditor(editor)
	if artifactId < 0 {
		log.Warn("artifactId < 0 (invalid artifactId value cannot be treated as sarif.CodeFlow)")
		return
	}
	s.CreateCodeFlowsFromPredecessor(v)
}

func (s *SarifContext) createCodeFlowsFromPredecessor(v *ssaapi.Value) {
	// Create a new thread flow for this path
	threadFlow := sarif.NewThreadFlow()
	threadFlows := []*sarif.ThreadFlowLocation{}
	visited := make(map[*ssaapi.Value]bool)

	// Function to add a value to the thread flow
	addValueToFlow := func(val *ssaapi.Value) bool {
		if visited[val] {
			return false
		}
		visited[val] = true

		rg := val.GetRange()
		if rg == nil || rg.GetEditor() == nil {
			return false
		}

		artid := s.GetArtifactIdFromEditor(rg.GetEditor())
		if artid < 0 {
			return false
		}

		loc := s.CreateLocation(artid, rg)
		if loc == nil {
			return false
		}
		loc.WithMessage(sarif.NewTextMessage(rg.GetText()))

		// Create thread flow threadFlow
		threadFlowLocation := sarif.NewThreadFlowLocation().
			WithLocation(loc)

		// Add importance based on whether it's the target value
		if val == v {
			threadFlowLocation.WithImportance("essential")
		} else {
			threadFlowLocation.WithImportance("important")
		}

		threadFlows = append(threadFlows, threadFlowLocation)
		return true
	}

	// Add the current value to the flow
	if !addValueToFlow(v) {
		return
	}

	// Process predecessors
	var processNeighbors func(val *ssaapi.Value)
	processNeighbors = func(val *ssaapi.Value) {
		// Process direct predecessors
		//TODO: fix this dataflow path
		for _, pred := range val.GetPredecessors() {
			if pred.Node != nil && addValueToFlow(pred.Node) {
				processNeighbors(pred.Node)
			}
		}
	}

	// Start processing from the current value
	processNeighbors(v)

	// Only create a code flow if we have more than one location
	if len(threadFlows) > 1 {
		threadFlow.WithLocations(threadFlows)
		codeFlow := sarif.NewCodeFlow().WithThreadFlows([]*sarif.ThreadFlow{threadFlow})
		s.codeFlows = append(s.codeFlows, codeFlow)
	}
}

func (s *SarifContext) CreateCodeFlowsFromPredecessor(v *ssaapi.Value) {
	s.createCodeFlowsFromPredecessor(v)
}

func (s *SarifContext) CreateLocation(artifactId int, rg *memedit.Range) *sarif.Location {
	al := sarif.NewArtifactLocation().WithIndex(artifactId)
	// Include the URI directly in the result location so SARIF viewers
	// (VS Code, GitHub, etc.) can resolve the file without looking up the
	// run.artifacts array by index. Many viewers do not resolve index refs.
	if editor := rg.GetEditor(); editor != nil {
		if uri := sarifArtifactURI(editor); uri != "" {
			al = al.WithUri(uri)
		}
	}
	return sarif.NewLocation().
		WithPhysicalLocation(
			sarif.NewPhysicalLocation().WithArtifactLocation(al).WithRegion(
				sarif.NewRegion().
					WithStartLine(rg.GetStart().GetLine()).
					WithStartColumn(rg.GetStart().GetColumn()).
					WithEndLine(rg.GetEnd().GetLine()).
					WithEndColumn(rg.GetEnd().GetColumn()),
			),
		)
}

func (s *SarifContext) GetArtifactIdFromEditor(editor *memedit.MemEditor) int {
	if editor == nil {
		return -1
	}
	hash := editor.SourceCodeSha256()
	id, ok := s.ArtifactsExisted(hash)
	if ok {
		return id
	}

	url := sarifArtifactURI(editor)
	if url == "" {
		log.Warn("editor file path is empty, it will cause some problems will open in some sarif viewer")
	}
	sourceCode := editor.GetSourceCode()
	art := sarif.NewArtifact().WithLength(len(sourceCode)).WithLocation(
		sarif.NewArtifactLocation().WithUri(url).WithIndex(id),
	).WithContents(
		sarif.NewArtifactContent().WithText(sourceCode),
	).WithHashes(map[string]string{
		"sha256": hash,
		"md5":    editor.SourceCodeMd5(),
		"sha1":   editor.SourceCodeSha1(),
	})
	s.appendArtifacts(art)
	return id
}

// sarifArtifactURI returns the repository-relative path GitHub needs to place
// an annotation on a file.
//
// memedit's GetFilename() is only the basename ("main.go") and GetUrl()
// prefixes the scan's virtual root ("/fs.zip(2025-0923-11:47)/sub/deep/x.go").
// Neither resolves to a path in the repository, so GitHub silently dropped
// every alert. GetFilePath() yields "/<folder>/<name>", which after trimming
// the leading slash is exactly the repo-relative form.
func sarifArtifactURI(editor *memedit.MemEditor) string {
	if editor == nil {
		return ""
	}
	uri := strings.TrimSpace(editor.GetFilePath())
	if uri == "" {
		uri = strings.TrimSpace(editor.GetUrl())
	}
	if uri == "" {
		uri = strings.TrimSpace(editor.GetFilename())
	}
	return normalizeSarifURI(uri)
}

// normalizeSarifURI strips the scan's virtual program root and any leading
// slash so the URI is relative to the repository root, and makes it a valid
// URI reference by escaping spaces and backslashes.
func normalizeSarifURI(raw string) string {
	uri := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if uri == "" {
		return ""
	}
	// "/program(2025-0923-11:47)/a/b.go" -> "a/b.go"
	if idx := strings.Index(uri, ")/"); idx >= 0 {
		uri = uri[idx+2:]
	}
	uri = strings.TrimPrefix(uri, "/")
	return path.Clean(uri)
}

// ConvertSyntaxFlowResultToSarifRun keeps the single-result helper for callers
// that want a run rather than a whole log. Everything inside one SARIF document
// is merged into a single run, so this is only meaningful in isolation.
func ConvertSyntaxFlowResultToSarifRun(result *ssaapi.SyntaxFlowResult) *sarif.Run {
	report, err := NewSarifReport()
	if err != nil {
		log.Errorf("create sarif report failed: %s", err)
		return nil
	}
	if !report.AddSyntaxFlowResult(result) {
		return nil
	}
	return report.run
}

func ConvertSyntaxFlowResultsToSarif(results ...*ssaapi.SyntaxFlowResult) (*sarif.Report, error) {
	report, err := NewSarifReport()
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}
	for _, result := range results {
		report.AddSyntaxFlowResult(result)
	}
	return report.Report(), nil
}

func ToSarifLevel(level schema.SyntaxFlowSeverity) string {
	switch level {
	case schema.SFR_SEVERITY_INFO:
		return "note"
	case schema.SFR_SEVERITY_LOW, schema.SFR_SEVERITY_WARNING:
		return "warning"
	case schema.SFR_SEVERITY_CRITICAL, schema.SFR_SEVERITY_HIGH:
		return "error"
	default:
		return "note"
	}
}
