package sfreport

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// SSAResultParts is a streaming-friendly representation of one SyntaxFlowResult.
// Files/dataflows are dedupable by hash, and each risk carries references
// (file_hashes/dataflow_hashes) to avoid embedding heavy payloads.
type SSAResultParts struct {
	ProgramName string                 `json:"program_name"`
	ReportType  string                 `json:"report_type"`
	Files       []*File                `json:"files,omitempty"`
	Dataflows   []*SSADataflowPart     `json:"dataflows,omitempty"`
	Risks       []*SSARiskPart         `json:"risks,omitempty"`
	Stats       map[string]interface{} `json:"stats,omitempty"`
}

type SSADataflowPart struct {
	DataflowHash string          `json:"dataflow_hash"`
	Payload      json.RawMessage `json:"payload"`
}

type SSARiskPart struct {
	RiskHash       string          `json:"risk_hash"`
	RiskJSON       json.RawMessage `json:"risk_json"`
	FileHashes     []string        `json:"file_hashes,omitempty"`
	DataflowHashes []string        `json:"dataflow_hashes,omitempty"`
}

// Compatibility aliases for existing callers.
type StreamSingleResultParts = SSAResultParts
type StreamDataflowPart = SSADataflowPart
type StreamRiskPart = SSARiskPart

type StreamPartsOptions struct {
	StreamKey        string
	ReportType       ReportType
	ShowDataflowPath bool
	ShowFileContent  bool
	WithFile         bool
	DedupFileContent bool
	DedupDataflow    bool
}

type StreamPartsOption func(*StreamPartsOptions)

func NewStreamPartsOptions(opts ...StreamPartsOption) StreamPartsOptions {
	o := StreamPartsOptions{
		ReportType:       IRifyFullReportType,
		ShowDataflowPath: true,
		ShowFileContent:  true,
		WithFile:         true,
		DedupDataflow:    true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.ReportType == "" {
		o.ReportType = IRifyFullReportType
	}
	return o
}

func WithStreamKey(v string) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.StreamKey = strings.TrimSpace(v)
	}
}

func WithStreamReportType(v ReportType) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.ReportType = v
	}
}

func WithStreamShowDataflowPath(v bool) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.ShowDataflowPath = v
	}
}

func WithStreamShowFileContent(v bool) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.ShowFileContent = v
	}
}

func WithStreamWithFile(v bool) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.WithFile = v
	}
}

func WithStreamDedupFileContent(v bool) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.DedupFileContent = v
	}
}

func WithStreamDedupDataflow(v bool) StreamPartsOption {
	return func(o *StreamPartsOptions) {
		o.DedupDataflow = v
	}
}

// ConvertSingleResultToSSAResultPartsJSON returns raw JSON of SSAResultParts and basic stats.
func ConvertSingleResultToSSAResultPartsJSON(result *ssaapi.SyntaxFlowResult, opts StreamPartsOptions) (string, map[string]any, error) {
	parts, err := ConvertSingleResultToSSAResultParts(result, opts)
	if err != nil {
		return "", nil, err
	}
	if parts == nil {
		return "", map[string]any{"has_payload": false}, nil
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return "", nil, err
	}
	stats := map[string]any{
		"has_payload": true,
		"risk_count":  len(parts.Risks),
		"file_count":  len(parts.Files),
		"flow_count":  len(parts.Dataflows),
	}
	return string(raw), stats, nil
}

// ConvertSingleResultToSSAResultParts converts one SyntaxFlowResult into stream-friendly parts.
func ConvertSingleResultToSSAResultParts(result *ssaapi.SyntaxFlowResult, opts StreamPartsOptions) (*SSAResultParts, error) {
	if result == nil {
		return nil, nil
	}
	_ = opts.StreamKey
	if opts.ReportType == "" {
		opts.ReportType = IRifyFullReportType
	}

	// Keep this tiny context object only for NewRisk/NewFile behavior switches.
	// We no longer build an intermediate Report from the whole result.
	reportCtx := NewReport(opts.ReportType)
	if opts.ShowDataflowPath {
		reportCtx.config.showDataflowPath = true
	}
	if opts.ShowFileContent {
		reportCtx.config.showFileContent = true
	}

	out := &SSAResultParts{
		ProgramName: strings.TrimSpace(result.GetProgramName()),
		ReportType:  string(reportCtx.ReportType),
	}

	var ruleName string
	if rule := result.GetRule(); rule != nil {
		ruleName = strings.TrimSpace(rule.RuleName)
	}

	fileByHash := make(map[string]*File, 256)
	riskFiles := make(map[string][]string, 256)
	riskFlows := make(map[string][]string, 256)
	flowPayloads := make(map[string]json.RawMessage, 256)
	riskSeen := make(map[string]struct{}, 256)

	for ssarisk := range result.YieldRisk() {
		if ssarisk == nil {
			continue
		}
		value, err := result.GetValue(ssarisk.Variable, int(ssarisk.Index))
		if err != nil {
			log.Errorf("stream parts: get value failed variable=%s index=%d err=%v", ssarisk.Variable, ssarisk.Index, err)
			continue
		}

		risk, toAddIrSourceHashes := NewRisk(ssarisk, reportCtx, value)
		if risk == nil {
			continue
		}
		riskHash := strings.TrimSpace(risk.Hash)
		if riskHash == "" {
			riskHash = strings.TrimSpace(ssarisk.Hash)
		}
		if riskHash == "" {
			continue
		}
		if _, exists := riskSeen[riskHash]; exists {
			continue
		}
		riskSeen[riskHash] = struct{}{}

		risk.Hash = riskHash
		if out.ProgramName == "" && risk.ProgramName != "" {
			out.ProgramName = risk.ProgramName
		}
		if risk.ProgramName == "" {
			risk.ProgramName = out.ProgramName
		}
		if risk.RuleName == "" {
			risk.RuleName = ruleName
		}

		if opts.WithFile {
			if value != nil && value.GetRange() != nil {
				if editor := value.GetRange().GetEditor(); editor != nil {
					if file := upsertStreamFileByEditor(fileByHash, editor, reportCtx); file != nil {
						file.AddRisk(risk)
						if hash := strings.TrimSpace(file.IrSourceHash); hash != "" {
							riskFiles[riskHash] = append(riskFiles[riskHash], hash)
						}
					}
				}
			}
			for _, h := range toAddIrSourceHashes {
				h = strings.TrimSpace(h)
				if h == "" {
					continue
				}
				file, err := upsertStreamFileByHash(fileByHash, h, reportCtx)
				if err != nil {
					log.Errorf("stream parts: load file by hash failed hash=%s err=%v", h, err)
					continue
				}
				if file != nil {
					file.AddRisk(risk)
					riskFiles[riskHash] = append(riskFiles[riskHash], h)
				}
			}
		}

		for _, p := range risk.DataFlowPaths {
			raw, err := MarshalMinimalDataFlowPath(p)
			if err != nil || len(raw) == 0 {
				continue
			}
			flowHash := codec.Sha256(raw)
			if flowHash == "" {
				continue
			}
			if _, exists := flowPayloads[flowHash]; !exists {
				flowPayloads[flowHash] = raw
			}
			riskFlows[riskHash] = append(riskFlows[riskHash], flowHash)
		}

		rc := *risk
		rc.DataFlowPaths = nil
		riskJSON, err := json.Marshal(&rc)
		if err != nil {
			continue
		}
		out.Risks = append(out.Risks, &SSARiskPart{
			RiskHash:       riskHash,
			RiskJSON:       riskJSON,
			FileHashes:     dedupStrings(riskFiles[riskHash]),
			DataflowHashes: dedupStrings(riskFlows[riskHash]),
		})
	}

	if len(out.Risks) == 0 {
		return nil, nil
	}

	if opts.WithFile {
		fileHashes := make([]string, 0, len(fileByHash))
		for h := range fileByHash {
			fileHashes = append(fileHashes, h)
		}
		sort.Strings(fileHashes)
		out.Files = make([]*File, 0, len(fileHashes))
		for _, h := range fileHashes {
			f := fileByHash[h]
			if f == nil {
				continue
			}
			ff := *f
			if ff.Content != "" && !utf8.ValidString(ff.Content) {
				ff.Content = utils.EscapeInvalidUTF8Byte([]byte(ff.Content))
			}
			ff.Risks = dedupStrings(ff.Risks)
			out.Files = append(out.Files, &ff)
		}
	}

	flowHashes := make([]string, 0, len(flowPayloads))
	for h := range flowPayloads {
		flowHashes = append(flowHashes, h)
	}
	sort.Strings(flowHashes)
	out.Dataflows = make([]*SSADataflowPart, 0, len(flowHashes))
	for _, h := range flowHashes {
		out.Dataflows = append(out.Dataflows, &SSADataflowPart{
			DataflowHash: h,
			Payload:      flowPayloads[h],
		})
	}

	sort.Slice(out.Risks, func(i, j int) bool {
		return out.Risks[i].RiskHash < out.Risks[j].RiskHash
	})

	out.Stats = map[string]interface{}{
		"risk_count": len(out.Risks),
		"file_count": len(out.Files),
		"flow_count": len(out.Dataflows),
	}
	return out, nil
}

func ConvertSingleResultToStreamPartsJSON(result *ssaapi.SyntaxFlowResult, opts StreamPartsOptions) (string, map[string]any, error) {
	return ConvertSingleResultToSSAResultPartsJSON(result, opts)
}

func ConvertSingleResultToStreamParts(result *ssaapi.SyntaxFlowResult, opts StreamPartsOptions) (*StreamSingleResultParts, error) {
	return ConvertSingleResultToSSAResultParts(result, opts)
}

func upsertStreamFileByEditor(fileByHash map[string]*File, editor *memedit.MemEditor, reportCtx *Report) *File {
	if editor == nil {
		return nil
	}
	hash := strings.TrimSpace(editor.GetIrSourceHash())
	if hash == "" {
		return nil
	}
	if f, ok := fileByHash[hash]; ok && f != nil {
		return f
	}
	f := NewFile(editor, reportCtx)
	fileByHash[hash] = f
	return f
}

func upsertStreamFileByHash(fileByHash map[string]*File, irSourceHash string, reportCtx *Report) (*File, error) {
	irSourceHash = strings.TrimSpace(irSourceHash)
	if irSourceHash == "" {
		return nil, nil
	}
	if f, ok := fileByHash[irSourceHash]; ok && f != nil {
		return f, nil
	}
	editor, err := ssadb.GetEditorByHash(irSourceHash)
	if err != nil {
		return nil, err
	}
	f := NewFile(editor, reportCtx)
	fileByHash[irSourceHash] = f
	return f, nil
}

// buildFiles extracts unique files from the report and returns risk→fileHashes mapping.
func buildFiles(out *StreamSingleResultParts, report *Report, withFile bool, dedup *streamDedupState, dedupFile bool) map[string][]string {
	riskFiles := make(map[string][]string)
	if !withFile || len(report.File) == 0 {
		return riskFiles
	}

	// Build unique file index, preferring fileByHash if available.
	unique := make(map[string]*File, len(report.File))
	if len(report.fileByHash) > 0 {
		for h, f := range report.fileByHash {
			if h != "" && f != nil {
				unique[h] = f
			}
		}
	} else {
		for _, f := range report.File {
			if f == nil || f.IrSourceHash == "" {
				continue
			}
			if _, ok := unique[f.IrSourceHash]; !ok {
				unique[f.IrSourceHash] = f
			}
		}
	}

	out.Files = make([]*File, 0, len(unique))
	var fileDedup *streamDedupState
	if dedupFile {
		fileDedup = dedup
	}

	for h, f := range unique {
		// Always track risk→file associations, even for deduped files.
		for _, rh := range f.Risks {
			if rh = strings.TrimSpace(rh); rh != "" {
				riskFiles[rh] = append(riskFiles[rh], h)
			}
		}

		if !fileDedup.markSeen("file:", h) {
			continue
		}

		ff := *f
		if ff.Content != "" && !utf8.ValidString(ff.Content) {
			ff.Content = utils.EscapeInvalidUTF8Byte([]byte(ff.Content))
		}
		out.Files = append(out.Files, &ff)
	}

	sort.Slice(out.Files, func(i, j int) bool {
		return out.Files[i].IrSourceHash < out.Files[j].IrSourceHash
	})
	return riskFiles
}

// buildDataflowsAndRisks builds dataflow parts and risk parts in a single pass over report.Risks.
func buildDataflowsAndRisks(
	out *StreamSingleResultParts,
	report *Report,
	dedup *streamDedupState,
	dedupDataflow bool,
	riskFiles map[string][]string,
) {
	flowPayloads := make(map[string]json.RawMessage)
	riskFlows := make(map[string][]string)

	var flowDedup *streamDedupState
	if dedupDataflow {
		flowDedup = dedup
	}

	out.Risks = make([]*StreamRiskPart, 0, len(report.Risks))
	out.Dataflows = make([]*StreamDataflowPart, 0)

	for rh, r := range report.Risks {
		if r == nil {
			continue
		}
		riskHash := strings.TrimSpace(rh)
		if riskHash == "" {
			riskHash = strings.TrimSpace(r.Hash)
		}
		if riskHash == "" {
			continue
		}
		if out.ProgramName == "" && r.ProgramName != "" {
			out.ProgramName = r.ProgramName
		}

		// Collect dataflow hashes for this risk.
		for _, p := range r.DataFlowPaths {
			raw, err := MarshalMinimalDataFlowPath(p)
			if err != nil || len(raw) == 0 {
				continue
			}
			flowHash := codec.Sha256(raw)
			if flowHash == "" {
				continue
			}
			if _, exists := flowPayloads[flowHash]; !exists {
				flowPayloads[flowHash] = raw
				if flowDedup.markSeen("flow:", flowHash) {
					out.Dataflows = append(out.Dataflows, &StreamDataflowPart{
						DataflowHash: flowHash,
						Payload:      raw,
					})
				}
			}
			riskFlows[riskHash] = append(riskFlows[riskHash], flowHash)
		}

		// Build risk part (strip heavy DataFlowPaths).
		rc := *r
		rc.Hash = riskHash
		rc.DataFlowPaths = nil
		riskJSON, err := json.Marshal(&rc)
		if err != nil {
			continue
		}
		out.Risks = append(out.Risks, &StreamRiskPart{
			RiskHash:       riskHash,
			RiskJSON:       riskJSON,
			FileHashes:     dedupStrings(riskFiles[riskHash]),
			DataflowHashes: dedupStrings(riskFlows[riskHash]),
		})
	}

	sort.Slice(out.Dataflows, func(i, j int) bool {
		return out.Dataflows[i].DataflowHash < out.Dataflows[j].DataflowHash
	})
}

func dedupStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	set := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v = strings.TrimSpace(v); v == "" {
			continue
		}
		if _, ok := set[v]; ok {
			continue
		}
		set[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
