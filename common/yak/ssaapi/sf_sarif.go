package ssaapi

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sarif"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strconv"
	"strings"
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
		s._ArtifactsTable[hash] = len(s._artifacts)
		return -1, false
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
	s.locations = append(s.locations, s.CreateLocation(artifactId, rg))
	cf := sarif.NewCodeFlow()
	cf.WithTextMessage(v.StringWithRange())
	if len(extraMsg) > 0 {
		pb := sarif.NewPropertyBag()
		pb.AddString("extraMessage", fmt.Sprintf("%s:\nt%s:%s", extraMsg[0], strconv.FormatInt(v.GetId(), 10), v.StringWithRange()))
		cf.AttachPropertyBag(pb)
	}
	cf.WithThreadFlows(s.CreateThreadFlowsFromPredecessor(v))
	s.codeFlows = append(s.codeFlows, cf)
}

func (s *SarifContext) createThreadFlowsFromPredecessor(v *Value, add func(flow *sarif.ThreadFlow), visited map[int64]struct{}) {
	if visited == nil {
		visited = make(map[int64]struct{})
	}

	if _, ok := visited[v.GetId()]; ok {
		return
	}
	visited[v.GetId()] = struct{}{}

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
		}).WithTextMessage(tf.Info.Label)
		add(tfInstance)

		s.createThreadFlowsFromPredecessor(v, add, visited)
	}

	if len(preds) > 0 {
		return
	}

	for _, dep := range v.DependOn {
		s.createThreadFlowsFromPredecessor(dep, add, visited)
	}
	for _, eff := range v.EffectOn {
		s.createThreadFlowsFromPredecessor(eff, add, visited)
	}
}

func (s *SarifContext) CreateThreadFlowsFromPredecessor(v *Value) []*sarif.ThreadFlow {
	var flows []*sarif.ThreadFlow

	addFlow := func(tf *sarif.ThreadFlow) {
		flows = append(flows, tf)
	}
	s.createThreadFlowsFromPredecessor(v, addFlow, nil)
	return flows
}

func (s *SarifContext) CreateLocation(artifactId int, rg *ssa.Range) *sarif.Location {
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

	url := editor.GetUrl()
	if url == "" {
		log.Warn("editor.GetUrl() is empty, it will cause some problems will open in some sarif viewer")
	}
	if url != "" && !strings.HasPrefix(url, "file://") {
		url = "file://" + url
	}
	sourceCode := editor.GetSourceCode()
	art := sarif.NewArtifact().WithLength(len(sourceCode)).WithLocation(
		sarif.NewArtifactLocation().WithUri(url),
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

func ConvertSyntaxFlowResultToSarif(r ...*sfvm.SFFrameResult) (*sarif.Report, error) {
	report, err := sarif.New(sarif.Version210, false)
	if err != nil {
		return nil, utils.Wrap(err, "create sarif.New Report failed")
	}

	root := NewSarifContext()
	var results []*sarif.Result
	for _, frame := range r {
		haveResult := false
		sctx := root.CreateSubSarifContext()
		if frame.AlertSymbolTable == nil {
			frame.AlertSymbolTable = make(map[string]sfvm.ValueOperator)
		}
		for k, v := range frame.AlertSymbolTable {
			v.Recursive(func(operator sfvm.ValueOperator) error {
				if raw, ok := operator.(*Value); ok {
					if msg := frame.AlertMsgTable[k]; msg != "" {
						sctx.AddSSAValue(raw, msg)
					} else {
						sctx.AddSSAValue(raw)
					}
					haveResult = true
				}
				return nil
			})
		}
		if !haveResult {
			continue
		}
		msgRaw, _ := frame.Description.MarshalJSON()
		results = append(results, sarif.NewRuleResult(codec.Sha256(frame.Rule)).WithMessage(sarif.NewTextMessage(frame.Rule)).WithCodeFlows(
			sctx.codeFlows,
		).WithLocations(
			sctx.locations,
		).WithMessage(sarif.NewTextMessage(string(msgRaw))))
		//).WithGraphs([]*sarif.Graph{}))
	}

	report.AddRun(
		sarif.NewRun(
			*sarif.NewSimpleTool("syntaxflow"),
		).WithDefaultSourceLanguage(
			"java",
		).WithDefaultEncoding(
			"utf-8",
		).WithArtifacts(root._artifacts).WithResults(results),
	)
	return report, nil
}
