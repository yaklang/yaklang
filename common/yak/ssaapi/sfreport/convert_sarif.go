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
)

// sarifKindByDefault is the SARIF "kind" for every finding.
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

type SarifReport struct {
	report *sarif.Report
	writer io.Writer

	// single-run accumulation
	root     *SarifContext
	ruleByID map[string]*sarif.ReportingDescriptor
	ruleIDs  []string
	results  []*sarif.Result
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
	return &SarifReport{
		report:   sarifReport,
		root:     NewSarifContext(),
		ruleByID: map[string]*sarif.ReportingDescriptor{},
	}, nil
}

var _ IReport = (*SarifReport)(nil)

// AddSyntaxFlowResult folds one SyntaxFlow result (one rule) into the single
// output run. GitHub refuses a document whose runs share the same tool name,
// and a run per rule used to produce that, so everything is merged here.
func (r *SarifReport) AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool {
	if r == nil || result == nil {
		return false
	}
	before := len(r.results)
	r.appendResult(result)
	return len(r.results) > before
}

// Save writes the accumulated report. Callers that scan in several passes
// (ScanProject runs one StartScan per product stage) hold a single report and
// save once at the end, so this is expected to run once per report.
func (r *SarifReport) Save() error {
	if r == nil {
		return utils.Errorf("report is nil")
	}
	if r.writer == nil {
		return nil
	}
	return r.finalize().PrettyWrite(r.writer)
}

// finalize builds the single run that represents this scan. The run is
// emitted even with zero results: GitHub closes the alerts of a
// (tool, category) upload whose results no longer contain them, so an empty
// run is what retires findings that were fixed.
func (r *SarifReport) finalize() *sarif.Report {
	r.report.Runs = nil

	rules := make([]*sarif.ReportingDescriptor, 0, len(r.ruleIDs))
	for _, id := range r.ruleIDs {
		rules = append(rules, r.ruleByID[id])
	}

	driver := sarif.NewDriver(SarifDriverName).
		WithFullName(SarifDriverFullName).
		WithOrganization(SarifDriverOrganization).
		WithInformationURI(SarifInformationURI).
		WithVersion(consts.GetYakVersion()).
		WithRules(rules)

	run := sarif.NewRun(*sarif.NewTool(driver))
	// Distinguishes this upload from other scanners in the same repository.
	run.WithAutomationDetails(sarif.NewRunAutomationDetails().WithID("diff-code-check"))
	if artifacts := r.root.Artifacts(); len(artifacts) > 0 {
		run.WithArtifacts(artifacts)
	}
	run.WithResults(r.results)

	// Results is a required field on Run, so a run must always carry a slice.
	if run.Results == nil {
		run.Results = []*sarif.Result{}
	}
	r.report.AddRun(run)
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
		r.results = append(r.results, res)
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

	r.ruleByID[ruleID] = descriptor
	r.ruleIDs = append(r.ruleIDs, ruleID)
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

// ConvertSyntaxFlowResultToSarifRun is kept for callers that want a run from a
// single result. Everything that ends up inside one SARIF document is merged
// into a single run by the report, so this returns a run only for isolated use.
func ConvertSyntaxFlowResultToSarifRun(result *ssaapi.SyntaxFlowResult) *sarif.Run {
	report, err := NewSarifReport()
	if err != nil {
		log.Errorf("create sarif report failed: %s", err)
		return nil
	}
	if !report.AddSyntaxFlowResult(result) {
		return nil
	}
	finalized := report.finalize()
	if len(finalized.Runs) == 0 {
		return nil
	}
	return finalized.Runs[0]
}

func ConvertSyntaxFlowResultsToSarif(results ...*ssaapi.SyntaxFlowResult) (*sarif.Report, error) {
	report, err := NewSarifReport()
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}
	for _, result := range results {
		report.AddSyntaxFlowResult(result)
	}
	return report.finalize(), nil
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
