package ssaapi

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SarifContext struct {
	root *SarifContext

	// sha256 -> index
	_artifacts      []*sarif.Artifact
	_ArtifactsTable map[string]int

	// context for result
	locations []*sarif.Location
	codeFlows []*sarif.CodeFlow
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

func (s *SarifContext) AddSSAValue(v *Value) {
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

func (s *SarifContext) createCodeFlowsFromPredecessor(v *Value) {
	// Create a new thread flow for this path
	threadFlow := sarif.NewThreadFlow()
	locations := []*sarif.ThreadFlowLocation{}
	visited := make(map[*Value]bool)

	// Function to add a value to the thread flow
	addValueToFlow := func(val *Value) bool {
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

		// Create thread flow location
		tfLoc := sarif.NewThreadFlowLocation().
			WithLocation(loc).
			WithNestingLevel(len(locations))

		// Add importance based on whether it's the target value
		if val == v {
			tfLoc.WithImportance("essential")
		} else {
			tfLoc.WithImportance("important")
		}

		locations = append(locations, tfLoc)
		return true
	}

	// Add the current value to the flow
	if !addValueToFlow(v) {
		return
	}

	// Process predecessors
	var processNeighbors func(val *Value)
	processNeighbors = func(val *Value) {
		// Process direct predecessors
		for _, pred := range val.GetPredecessors() {
			if pred.Node != nil && addValueToFlow(pred.Node) {
				processNeighbors(pred.Node)
			}
		}
	}

	// Start processing from the current value
	processNeighbors(v)

	// Only create a code flow if we have more than one location
	if len(locations) > 1 {
		// Reverse the locations to show flow from source to sink
		// for i, j := 0, len(locations)-1; i < j; i, j = i+1, j-1 {
		// 	locations[i], locations[j] = locations[j], locations[i]
		// }

		threadFlow.WithLocations(locations)
		codeFlow := sarif.NewCodeFlow().WithThreadFlows([]*sarif.ThreadFlow{threadFlow})
		s.codeFlows = append(s.codeFlows, codeFlow)
	}
}

func (s *SarifContext) CreateCodeFlowsFromPredecessor(v *Value) {
	s.createCodeFlowsFromPredecessor(v)
}

func (s *SarifContext) CreateLocation(artifactId int, rg memedit.RangeIf) *sarif.Location {
	return sarif.NewLocation().WithPhysicalLocation(
		sarif.NewPhysicalLocation().WithArtifactLocation(
			sarif.NewArtifactLocation().WithIndex(artifactId),
		).WithRegion(
			sarif.NewRegion().
				WithStartLine(rg.GetStart().GetLine()).
				WithStartColumn(rg.GetStart().GetColumn()).
				WithEndLine(rg.GetEnd().GetLine()).
				WithEndColumn(rg.GetEnd().GetColumn()),
		))
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

func convertSyntaxFlowFrameToSarifRun(frameResult *SyntaxFlowResult) []*sarif.Run {
	var results []*sarif.Result
	var runs []*sarif.Run

	root := NewSarifContext()

	if len(frameResult.GetAlertVariables()) == 0 {
		return nil
	}

	SFRule := frameResult.GetRule()
	ruleId := codec.Sha256(SFRule.Content)

	for _, name := range frameResult.GetAlertVariables() {
		values := frameResult.GetValues(name)
		info, ok := frameResult.GetAlertInfo(name)
		if !ok {
			info = SFRule.GetInfo()
		}
		for _, value := range values {
			sctx := root.CreateSubSarifContext()
			sctx.AddSSAValue(value)

			result := sarif.NewRuleResult(
				ruleId,
			).WithMessage(
				sarif.NewTextMessage(info.Description),
			)

			// Add locations for the current value
			if rg := value.GetRange(); rg != nil && rg.GetEditor() != nil {
				artid := root.GetArtifactIdFromEditor(rg.GetEditor())
				if artid >= 0 {
					loc := root.CreateLocation(artid, rg)
					if loc != nil {
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
	}

	if len(results) > 0 {
		driver := sarif.NewDriver("SyntaxFlow").WithFullName("SyntaxFlow Static Analysis")
		tool := sarif.NewTool(driver)
		run := sarif.NewRun(*tool)

		// Add artifacts if they exist
		if len(root._artifacts) > 0 {
			run.WithArtifacts(root._artifacts)
		}

		run.WithResults(results)
		runs = append(runs, run)
	}

	return runs
}

func ConvertSyntaxFlowResultToSarif(r ...*SyntaxFlowResult) (*sarif.Report, error) {
	report, err := sarif.New(sarif.Version210, false)
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}

	for _, frame := range r {
		for _, run := range convertSyntaxFlowFrameToSarifRun(frame) {
			if funk.IsEmpty(run) {
				continue
			}
			report.AddRun(run)
		}
	}
	return report, nil
}
