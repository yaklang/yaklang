package ssaapi

import (
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type SarifContext struct {
	root *SarifContext

	// sha256 -> index
	_artifacts      []*sarif.Artifact
	_ArtifactsTable map[string]int

	// context for result
	locations   []*sarif.Location
	codeFlows   []*sarif.CodeFlow
	invocations []*sarif.Invocation
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

func (s *SarifContext) AddSSAValue(v *Value, extraMsg ...string) {
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

func (s *SarifContext) createCodeFlowsFromPredecessor(v *Value, add func(flow *sarif.CodeFlow), visited map[int64]struct{}) {
	if visited == nil {
		visited = make(map[int64]struct{})
	}

	if _, ok := visited[v.GetId()]; ok {
		return
	}
	visited[v.GetId()] = struct{}{}

	if len(v.Predecessors) > 0 {
		log.Infof("create codeflow preds: %v from: %v", len(v.Predecessors), v)
	}

	preds := v.Predecessors
	for _, tf := range preds {
		rg := tf.Node.GetRange()
		if rg == nil {
			continue
		}
		if rg.GetEditor() == nil {
			continue
		}

		artid := s.GetArtifactIdFromEditor(rg.GetEditor())
		if artid < 0 {
			continue
		}
		tfInstance := sarif.NewThreadFlow().WithLocations([]*sarif.ThreadFlowLocation{
			sarif.NewThreadFlowLocation().WithLocation(
				s.CreateLocation(artid, rg),
			),
		}).WithMessage(sarif.NewTextMessage(tf.Info.Label))

		codeflow := sarif.NewCodeFlow().WithThreadFlows([]*sarif.ThreadFlow{
			tfInstance,
		}).WithTextMessage(tf.Node.StringWithRange())

		add(codeflow)
		s.locations = append(s.locations, s.CreateLocation(artid, rg))
		s.createCodeFlowsFromPredecessor(tf.Node, add, visited)
		invoc := sarif.NewInvocation().WithArguments([]string{tf.Info.Label})
		s.invocations = append(s.invocations, invoc)
	}

	if len(preds) > 0 {
		return
	}

	for _, dep := range v.DependOn {
		rg := dep.GetRange()
		if rg == nil {
			continue
		}
		if rg.GetEditor() == nil {
			continue
		}

		artid := s.GetArtifactIdFromEditor(rg.GetEditor())
		if artid < 0 {
			continue
		}
		s.locations = append(s.locations, s.CreateLocation(artid, rg))
		s.createCodeFlowsFromPredecessor(dep, add, visited)
	}
	for _, eff := range v.EffectOn {
		rg := eff.GetRange()
		if rg == nil {
			continue
		}
		if rg.GetEditor() == nil {
			continue
		}

		artid := s.GetArtifactIdFromEditor(rg.GetEditor())
		if artid < 0 {
			continue
		}
		s.locations = append(s.locations, s.CreateLocation(artid, rg))
		s.createCodeFlowsFromPredecessor(eff, add, visited)
	}
}

func (s *SarifContext) CreateCodeFlowsFromPredecessor(v *Value) []*sarif.CodeFlow {
	var flows []*sarif.CodeFlow

	addFlow := func(tf *sarif.CodeFlow) {
		s.codeFlows = append(s.codeFlows, tf)
	}
	s.createCodeFlowsFromPredecessor(v, addFlow, nil)
	return flows
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

func convertSyntaxFlowFrameToSarifRun(root *SarifContext, frameResult *sfvm.SFFrameResult) []*sarif.Run {
	var results []*sarif.Result
	var ctxs []*SarifContext

	haveResult := false
	if frameResult.AlertSymbolTable == nil {
		frameResult.AlertSymbolTable = make(map[string]sfvm.ValueOperator)
	}

	SFRule := frameResult.GetRule()
	ruleId := codec.Sha256(SFRule.Content)

	rule := sarif.NewRule(ruleId).WithName(frameResult.Name()).WithDescription(frameResult.GetDescription())
	rule.WithFullDescription(sarif.NewMultiformatMessageString(SFRule.Content))

	if len(frameResult.AlertSymbolTable) == 0 {
		for _, name := range frameResult.CheckParams {
			checkResult, ok := frameResult.SymbolTable.Get(name)
			if !ok {
				continue
			}
			frameResult.AlertSymbolTable[name] = checkResult
		}
	}

	var defaultEditor *memedit.MemEditor = nil

	for k, v := range frameResult.AlertSymbolTable {
		v.Recursive(func(operator sfvm.ValueOperator) error {
			sctx := root.CreateSubSarifContext()
			if raw, ok := operator.(*Value); ok {
				if utils.IsNil(defaultEditor) {
					defaultEditor = raw.GetRange().GetEditor()
				}
				if m := frameResult.GetRule().AlertDesc[k]; m != nil {
					if m.OnlyMsg {
						sctx.AddSSAValue(raw, m.Msg)
					} else {
						sctx.AddSSAValue(raw, codec.AnyToString(m))
					}
				} else {
					sctx.AddSSAValue(raw)
				}
				haveResult = true
			}
			if len(sctx.codeFlows) > 0 {
				ctxs = append(ctxs, sctx)
			}
			return nil
		})
	}

	if !haveResult {
		return nil
	}
	msgRaw, _ := frameResult.Description.MarshalJSON()
	result, ok := frameResult.Description.Get("title")
	if ok {
		msgRaw = []byte(result)
	}

	tool := sarif.NewTool(&sarif.ToolComponent{
		FullName:     bizhelper.StrP("syntaxflow"),
		Name:         "syntaxflow",
		Organization: bizhelper.StrP("yaklang.io"),
		Rules:        []*sarif.ReportingDescriptor{rule},
		Taxa:         []*sarif.ReportingDescriptor{rule},
	})

	var runs []*sarif.Run
	for _, sctx := range ctxs {
		if len(sctx.codeFlows) > 0 {
			log.Infof("codeflows fetch: %v, location: %v", len(sctx.codeFlows), len(sctx.locations))
		}
		results = append(results, sarif.NewRuleResult(
			ruleId,
		).WithMessage(
			sarif.NewTextMessage(SFRule.Content),
		).WithCodeFlows(
			sctx.codeFlows,
		).WithLocations(
			sctx.locations,
		).WithAnalysisTarget(
			sarif.NewArtifactLocation().WithIndex(sctx.GetArtifactIdFromEditor(defaultEditor)),
		).WithMessage(
			sarif.NewTextMessage(string(msgRaw)),
		).WithRule(
			sarif.NewReportingDescriptorReference().WithId(ruleId),
		))

		run := sarif.NewRun(*tool).WithDefaultSourceLanguage(
			"java",
		).WithDefaultEncoding(
			"utf-8",
		).WithArtifacts(
			root._artifacts,
		).WithResults(
			results,
		).WithInvocations(
			sctx.invocations,
		)

		runs = append(runs, run)
	}
	return runs
}

func ConvertSyntaxFlowResultToSarif(r ...*SyntaxFlowResult) (*sarif.Report, error) {
	report, err := sarif.New(sarif.Version210, false)
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}

	root := NewSarifContext()
	for _, frame := range r {
		for _, run := range convertSyntaxFlowFrameToSarifRun(root, frame.memResult) {
			if funk.IsEmpty(run) {
				continue
			}
			report.AddRun(run)
		}
	}
	return report, nil
}
