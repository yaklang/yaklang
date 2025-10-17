package sfreport

import (
	"io"

	"github.com/yaklang/yaklang/common/go-funk"

	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SarifReport struct {
	report *sarif.Report
}

func (r *SarifReport) AddSyntaxFlowRisks(risks []*schema.SSARisk) {
	log.Errorf("The sarif format cannot specify only a single risk for generation")
}

func NewSarifReport() (*SarifReport, error) {
	sarifReport, err := sarif.New(sarif.Version210, false)
	if err != nil {
		log.Errorf("create sarif.New Report failed: %s", err)
		return nil, err
	}
	return &SarifReport{
		report: sarifReport,
	}, nil
}

var _ IReport = (*SarifReport)(nil)

func (r *SarifReport) AddSyntaxFlowResult(result *ssaapi.SyntaxFlowResult) bool {
	run := ConvertSyntaxFlowResultToSarifRun(result)
	if !funk.IsEmpty(run) {
		r.report.AddRun(run)
		return true
	}
	return false
}

func (r *SarifReport) Save(writer io.Writer) error {
	return r.report.PrettyWrite(writer)
}

// ====================== sarif context ======================

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
		threadFlow := sarif.NewThreadFlowLocation().
			WithLocation(loc)

		// Add importance based on whether it's the target value
		if val == v {
			threadFlow.WithImportance("essential")
		} else {
			threadFlow.WithImportance("important")
		}

		threadFlows = append(threadFlows, threadFlow)
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
		// Reverse the locations to show flow from source to sink
		// for i, j := 0, len(locations)-1; i < j; i, j = i+1, j-1 {
		// 	locations[i], locations[j] = locations[j], locations[i]
		// }

		threadFlow.WithLocations(threadFlows)
		codeFlow := sarif.NewCodeFlow().WithThreadFlows([]*sarif.ThreadFlow{threadFlow})
		s.codeFlows = append(s.codeFlows, codeFlow)
		//TODO
	}
}

func (s *SarifContext) CreateCodeFlowsFromPredecessor(v *ssaapi.Value) {
	s.createCodeFlowsFromPredecessor(v)
}

func (s *SarifContext) CreateLocation(artifactId int, rg *memedit.Range) *sarif.Location {
	return sarif.NewLocation().
		WithPhysicalLocation(
			sarif.NewPhysicalLocation().WithArtifactLocation(
				sarif.NewArtifactLocation().WithIndex(artifactId),
			).WithRegion(
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

	url := editor.GetFilename()
	if url == "" {
		log.Warn("editor.GetFilename() is empty, it will cause some problems will open in some sarif viewer")
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

func ConvertSyntaxFlowResultToSarifRun(result *ssaapi.SyntaxFlowResult) *sarif.Run {
	var results []*sarif.Result

	root := NewSarifContext()

	// if len(result.GetAlertVariables()) == 0 {
	// 	return nil
	// }

	SFRule := result.GetRule()
	ruleId := codec.Sha256(SFRule.Content)
	rule := sarif.NewRule(ruleId).
		WithName(SFRule.Title).
		WithDescription(SFRule.Description)
	// .WithFullDescription(sarif.NewMultiformatMessageString(SFRule.Content))

	for risk := range result.YieldRisk() {
		value, err := result.GetValue(risk.Variable, int(risk.Index))
		if err != nil {
			log.Errorf("get value from result failed: resultId[%d: %s: %d] %s", result.GetResultID(), risk.Variable, risk.Index, err)
			continue
		}
		sctx := root.CreateSubSarifContext()
		sctx.AddSSAValue(value)

		result := sarif.NewRuleResult(ruleId).
			WithMessage(sarif.NewTextMessage(risk.String())).
			WithLevel(ToSarifLevel(risk.Severity)).
			WithKind(risk.RiskType)

		// Add locations for the current value
		if rg := value.GetRange(); rg != nil && rg.GetEditor() != nil {
			artifactId := root.GetArtifactIdFromEditor(rg.GetEditor())
			if artifactId >= 0 {
				loc := root.CreateLocation(artifactId, rg)
				if loc != nil {
					loc.WithMessage(sarif.NewTextMessage("location message "))
					result.WithLocations([]*sarif.Location{loc})
				}
			}
		}

		// Add code flows if they exist
		if len(sctx.codeFlows) > 0 {
			result.WithCodeFlows(sctx.codeFlows)
		}

		results = append(results, result)
	}

	if len(results) == 0 {
		return nil
	}
	driver := sarif.NewDriver("SyntaxFlow").
		WithFullName("SyntaxFlow Static Analysis").
		WithOrganization("yaklang.io").
		WithRules([]*sarif.ReportingDescriptor{rule})
	tool := sarif.NewTool(driver)
	run := sarif.NewRun(*tool)

	// Add artifacts if they exist
	if len(root._artifacts) > 0 {
		run.WithArtifacts(root._artifacts)
	}

	run.WithResults(results)
	return run
}

func ConvertSyntaxFlowResultsToSarif(results ...*ssaapi.SyntaxFlowResult) (*sarif.Report, error) {
	report, err := sarif.New(sarif.Version210, false)
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}

	for _, result := range results {
		run := ConvertSyntaxFlowResultToSarifRun(result)
		if funk.IsEmpty(run) {
			continue
		}
		report.AddRun(run)
	}
	return report, nil
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
