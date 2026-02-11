package sfreport

import (
	"encoding/json"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// StreamSingleResultParts is a streaming-friendly representation of one SyntaxFlowResult.
// Files/dataflows are dedupable by hash, and each risk carries references
// (file_hashes/dataflow_hashes) to avoid embedding heavy payloads.
type StreamSingleResultParts struct {
	ProgramName string                 `json:"program_name"`
	ReportType  string                 `json:"report_type"`
	Files       []*File                `json:"files,omitempty"`
	Dataflows   []*StreamDataflowPart  `json:"dataflows,omitempty"`
	Risks       []*StreamRiskPart      `json:"risks,omitempty"`
	Stats       map[string]interface{} `json:"stats,omitempty"`
}

type StreamDataflowPart struct {
	DataflowHash string          `json:"dataflow_hash"`
	Payload      json.RawMessage `json:"payload"`
}

type StreamRiskPart struct {
	RiskHash       string          `json:"risk_hash"`
	RiskJSON       json.RawMessage `json:"risk_json"`
	FileHashes     []string        `json:"file_hashes,omitempty"`
	DataflowHashes []string        `json:"dataflow_hashes,omitempty"`
}

type StreamPartsOptions struct {
	StreamKey        string
	ReportType       ReportType
	ShowDataflowPath bool
	ShowFileContent  bool
	WithFile         bool
	DedupFileContent bool
	DedupDataflow    bool
}

// ConvertSingleResultToStreamPartsJSON returns raw JSON of StreamSingleResultParts and basic stats.
func ConvertSingleResultToStreamPartsJSON(result *ssaapi.SyntaxFlowResult, opts StreamPartsOptions) (string, map[string]any, error) {
	parts, err := ConvertSingleResultToStreamParts(result, opts)
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

// ConvertSingleResultToStreamParts converts one SyntaxFlowResult into stream-friendly parts.
func ConvertSingleResultToStreamParts(result *ssaapi.SyntaxFlowResult, opts StreamPartsOptions) (*StreamSingleResultParts, error) {
	if result == nil {
		return nil, nil
	}

	report := NewReport(opts.ReportType)
	if opts.ShowDataflowPath {
		report.config.showDataflowPath = true
	}
	if opts.ShowFileContent {
		report.config.showFileContent = true
	}
	report.AddSyntaxFlowResult(result)

	if !opts.WithFile {
		report.File = nil
		report.IrSourceHashes = make(map[string]struct{})
		report.FileCount = 0
	}
	if len(report.Risks) == 0 {
		return nil, nil
	}

	out := &StreamSingleResultParts{
		ProgramName: report.ProgramName,
		ReportType:  string(report.ReportType),
	}

	// dedup state
	var dedup *streamDedupState
	if opts.StreamKey != "" && (opts.DedupFileContent || opts.DedupDataflow) {
		dedup = getStreamDedupState(opts.StreamKey)
	}

	riskFiles := buildFiles(out, report, opts.WithFile, dedup, opts.DedupFileContent)
	buildDataflowsAndRisks(out, report, dedup, opts.DedupDataflow, riskFiles)

	maybeSweepStreamDedup()

	out.Stats = map[string]interface{}{
		"risk_count": len(out.Risks),
		"file_count": len(out.Files),
		"flow_count": len(out.Dataflows),
	}
	return out, nil
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
			raw, err := MarshalStreamMinimalDataFlowPath(p)
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
