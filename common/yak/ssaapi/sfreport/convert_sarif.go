package sfreport

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

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
	writer io.Writer

	// Streaming state. Each result is converted to a SARIF run as soon as it
	// is produced and streamed to the writer immediately, so a scan that emits
	// risks over minutes no longer defers all conversion work to Save() (that
	// deferral turned into a multi-minute silent tail after the scan finished).
	//
	// The writer is a shared io.Writer, so every emitted run is serialized as a
	// whole JSON value; the surrounding object/array punctuation is written
	// around it. A report with no result is still emitted as a valid, empty
	// SARIF document.
	streamMu    sync.Mutex
	streamErr   error
	streaming   bool
	wroteHeader bool
	wroteAnyRun bool
	saved       bool
	convertCost time.Duration
	writeCost   time.Duration
	resultCount int
}

func (r *SarifReport) SetWriter(writer io.Writer) error {
	if writer == nil {
		return utils.Errorf("writer is nil")
	}
	r.streamMu.Lock()
	defer r.streamMu.Unlock()
	r.writer = writer
	// Results may have been collected before the writer was attached; flush
	// them in order so the streamed document contains every run.
	if len(r.report.Runs) > 0 {
		pending := r.report.Runs
		r.report.Runs = nil
		for _, run := range pending {
			if err := r.writeRunLocked(run); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *SarifReport) AddSyntaxFlowRisks(risks ...*schema.SSARisk) {
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
	start := time.Now()
	run := ConvertSyntaxFlowResultToSarifRun(result)
	convertCost := time.Since(start)
	if funk.IsEmpty(run) {
		return false
	}

	r.streamMu.Lock()
	defer r.streamMu.Unlock()
	r.convertCost += convertCost
	r.resultCount++
	if convertCost > 5*time.Second {
		log.Infof("[sarif] convert result cost=%v (results=%d)", convertCost, r.resultCount)
	}
	// Stream the run out as it is produced. The writer may be nil (in-memory
	// callers that only inspect the report), in which case runs accumulate and
	// Save writes the whole document.
	if r.writer != nil {
		if r.saved {
			// The document was already closed; a late result cannot be added
			// without corrupting it, so drop it rather than write past the end.
			log.Errorf("[sarif] dropping result for %s: report already saved", result.GetProgramName())
			return false
		}
		if err := r.writeRunLocked(run); err != nil && r.streamErr == nil {
			r.streamErr = err
		}
		return true
	}
	r.report.AddRun(run)
	return true
}

func (r *SarifReport) Save() error {
	if r == nil {
		return utils.Errorf("report is nil")
	}
	r.streamMu.Lock()
	defer r.streamMu.Unlock()
	if r.streamErr != nil {
		return r.streamErr
	}
	if r.writer == nil {
		return nil
	}
	if r.streaming {
		// Close the runs array and the document; every run was already written
		// as it was produced.
		if r.saved {
			return nil
		}
		_, err := io.WriteString(r.writer, "]\n}\n")
		if err != nil && r.streamErr == nil {
			r.streamErr = err
		}
		r.saved = true
		return err
	}
	start := time.Now()
	err := r.report.PrettyWrite(r.writer)
	log.Infof("[sarif] save report results=%d convert_total=%v marshal_write=%v",
		r.resultCount, r.convertCost, time.Since(start))
	return err
}

// writeRunLocked emits one run into the report document currently being
// streamed. Callers must hold streamMu.
func (r *SarifReport) writeRunLocked(run *sarif.Run) error {
	start := time.Now()
	defer func() { r.writeCost += time.Since(start) }()

	if !r.wroteHeader {
		header := fmt.Sprintf("{\n  \"version\": %q,\n  \"runs\": [", r.report.Version)
		if _, err := io.WriteString(r.writer, header); err != nil {
			return err
		}
		r.wroteHeader = true
		r.streaming = true
	}

	// Matches what PrettyWrite did for the whole document before the runs were
	// streamed out one at a time.
	if err := run.DedupeArtifacts(); err != nil {
		return err
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	if r.wroteAnyRun {
		if _, err := io.WriteString(r.writer, ",\n"); err != nil {
			return err
		}
	}
	if _, err := r.writer.Write(raw); err != nil {
		return err
	}
	r.wroteAnyRun = true
	return nil
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
	al := sarif.NewArtifactLocation().WithIndex(artifactId)
	// Include the URI directly in the result location so SARIF viewers
	// (VS Code, GitHub, etc.) can resolve the file without looking up the
	// run.artifacts array by index. Many viewers do not resolve index refs.
	if editor := rg.GetEditor(); editor != nil {
		if uri := editor.GetFilename(); uri != "" {
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
