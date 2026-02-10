package sfreport

import (
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// StreamSingleResultParts is a streaming-friendly representation of one SyntaxFlowResult.
// It is intentionally "transport-shaped": files/dataflows are dedupable by hash, and each
// risk carries references (file_hashes/dataflow_hashes) to avoid embedding heavy payloads.
//
// This is meant to be produced in yak (producer), then delivered to ScanNode via yakit logs,
// and finally re-packaged into spec.StreamEnvelope events by StreamEmitter.
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

func dedupUniqueStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	set := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
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

// ConvertSingleResultToStreamPartsPayload converts one SyntaxFlowResult into stream-friendly parts.
//
// Notes:
// - If dedupFileContent is enabled, files already seen in streamKey are not returned again.
// - Dataflows are always minimized to StreamMinimalDataFlowPath (enough for audit persistence).
func ConvertSingleResultToStreamPartsPayload(
	result *ssaapi.SyntaxFlowResult,
	streamKey string,
	reportType ReportType,
	showDataflowPath bool,
	showFileContent bool,
	withFile bool,
	dedupFileContent bool,
	dedupDataflow bool,
) (map[string]any, error) {
	parts, err := ConvertSingleResultToStreamPartsWithOptions(
		result,
		streamKey,
		reportType,
		showDataflowPath,
		showFileContent,
		withFile,
		dedupFileContent,
		dedupDataflow,
	)
	if err != nil {
		return nil, err
	}
	if parts == nil {
		return map[string]any{"has_payload": false}, nil
	}
	raw, err := json.Marshal(parts)
	if err != nil {
		return nil, err
	}
	// Return as a generic map for yak runtime friendliness.
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	m["has_payload"] = len(parts.Risks) > 0
	return m, nil
}

func ConvertSingleResultToStreamPartsWithOptions(
	result *ssaapi.SyntaxFlowResult,
	streamKey string,
	reportType ReportType,
	showDataflowPath bool,
	showFileContent bool,
	withFile bool,
	dedupFileContent bool,
	dedupDataflow bool,
) (*StreamSingleResultParts, error) {
	if result == nil {
		return nil, nil
	}
	report := NewReport(reportType)
	if showDataflowPath {
		report.config.showDataflowPath = true
	}
	if showFileContent {
		report.config.showFileContent = true
	}
	report.AddSyntaxFlowResult(result)
	if !withFile {
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
		Files:       nil,
		Dataflows:   nil,
		Risks:       make([]*StreamRiskPart, 0, len(report.Risks)),
		Stats:       make(map[string]interface{}),
	}

	// Dedup file payloads across the task (streamKey).
	var fileSeen *streamDedupState
	if dedupFileContent && streamKey != "" {
		fileSeen = getStreamDedupState(streamKey)
	}
	shouldSendFile := func(h string) bool {
		if h == "" {
			return false
		}
		if fileSeen == nil {
			return true
		}
		fileSeen.mu.Lock()
		defer fileSeen.mu.Unlock()
		k := "file:" + h
		if _, ok := fileSeen.seen[k]; ok {
			return false
		}
		fileSeen.seen[k] = struct{}{}
		fileSeen.lastUsed = time.Now()
		return true
	}

	flowSeen := fileSeen
	if !dedupDataflow {
		flowSeen = nil
	}
	shouldSendFlow := func(h string) bool {
		if h == "" {
			return false
		}
		if flowSeen == nil {
			return true
		}
		flowSeen.mu.Lock()
		defer flowSeen.mu.Unlock()
		k := "flow:" + h
		if _, ok := flowSeen.seen[k]; ok {
			return false
		}
		flowSeen.seen[k] = struct{}{}
		flowSeen.lastUsed = time.Now()
		return true
	}

	// Build risk -> fileHashes from report.File[].Risks.
	riskFiles := make(map[string][]string)
	if withFile && len(report.File) > 0 {
		// Prefer report.fileByHash if present (dedup duplicates in report.File).
		unique := make(map[string]*File, 0)
		if report.fileByHash != nil && len(report.fileByHash) > 0 {
			for h, f := range report.fileByHash {
				if h != "" && f != nil {
					unique[h] = f
				}
			}
		} else {
			for _, f := range report.File {
				if f != nil && f.IrSourceHash != "" {
					if _, ok := unique[f.IrSourceHash]; !ok {
						unique[f.IrSourceHash] = f
					}
				}
			}
		}
		out.Files = make([]*File, 0, len(unique))
		for h, f := range unique {
			if f == nil {
				continue
			}
			if !shouldSendFile(h) {
				// Already sent in this stream; still keep the association via riskFiles.
			} else {
				// Ensure JSON-safe UTF-8 for transport (some repos may contain non-utf8 blobs).
				ff := *f
				if ff.Content != "" && !utf8.ValidString(ff.Content) {
					ff.Content = utils.EscapeInvalidUTF8Byte([]byte(ff.Content))
				}
				out.Files = append(out.Files, &ff)
			}
			for _, rh := range f.Risks {
				rh = strings.TrimSpace(rh)
				if rh == "" {
					continue
				}
				riskFiles[rh] = append(riskFiles[rh], h)
			}
		}
		// Keep deterministic ordering for tests/logs.
		sort.Slice(out.Files, func(i, j int) bool {
			if out.Files[i] == nil || out.Files[j] == nil {
				return i < j
			}
			return out.Files[i].IrSourceHash < out.Files[j].IrSourceHash
		})
	}

	// Build dataflow parts and risk -> flowHashes mapping.
	flowPayloads := make(map[string]json.RawMessage)
	riskFlows := make(map[string][]string)
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
		if len(r.DataFlowPaths) > 0 {
			for _, p := range r.DataFlowPaths {
				raw, err := MarshalStreamMinimalDataFlowPath(p)
				if err != nil || len(raw) == 0 {
					continue
				}
				flowHash := codec.Sha256(raw)
				if flowHash == "" {
					continue
				}
				flowPayloads[flowHash] = raw
				riskFlows[riskHash] = append(riskFlows[riskHash], flowHash)
			}
		}
	}
	out.Dataflows = make([]*StreamDataflowPart, 0, len(flowPayloads))
	for h, payload := range flowPayloads {
		if !shouldSendFlow(h) {
			continue
		}
		out.Dataflows = append(out.Dataflows, &StreamDataflowPart{
			DataflowHash: h,
			Payload:      payload,
		})
	}
	sort.Slice(out.Dataflows, func(i, j int) bool { return out.Dataflows[i].DataflowHash < out.Dataflows[j].DataflowHash })

	// Build risk parts.
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
		// Strip heavy dataflow payload from risk itself (we send dataflows separately).
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
			FileHashes:     dedupUniqueStrings(riskFiles[riskHash]),
			DataflowHashes: dedupUniqueStrings(riskFlows[riskHash]),
		})
	}

	maybeSweepStreamDedup()

	out.Stats["risk_count"] = len(out.Risks)
	out.Stats["file_count"] = len(out.Files)
	out.Stats["flow_count"] = len(out.Dataflows)
	return out, nil
}
