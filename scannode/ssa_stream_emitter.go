package scannode

import (
	"fmt"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// EmitSSATaskStart emits a task_start stream event.
// Producer (yak script) is expected to call this once before sending file/dataflow/risk parts.
func (e *StreamEmitter) EmitSSATaskStart(taskId, runtimeId, subTaskId, programName, reportType string) {
	if !e.Enabled() || taskId == "" {
		return
	}
	state := e.getTaskState(taskId)
	state.lastActivity = time.Now()
	state.emitStartOnce(e, taskId, runtimeId, subTaskId, programName, reportType)
}

// EmitSSAFile emits one file payload (deduped by ir_source_hash per task).
func (e *StreamEmitter) EmitSSAFile(taskId, runtimeId, subTaskId string, file *sfreport.File) error {
	if !e.Enabled() {
		return nil
	}
	if file == nil {
		return nil
	}
	fileHash := strings.TrimSpace(file.IrSourceHash)
	if fileHash == "" {
		return nil
	}
	state := e.getTaskState(taskId)
	state.lastActivity = time.Now()
	// Best-effort: if producer forgot to send task_start, keep pipeline moving.
	state.emitStartOnce(e, taskId, runtimeId, subTaskId, "", "")
	if !state.markFileSent(fileHash) {
		return nil
	}
	e.emitFile(fileHash, file, taskId, runtimeId, subTaskId)
	return nil
}

// EmitSSADataflow emits one dataflow payload (deduped by dataflow_hash per task).
func (e *StreamEmitter) EmitSSADataflow(taskId, runtimeId, subTaskId string, flowHash string, payload []byte) error {
	if !e.Enabled() {
		return nil
	}
	flowHash = strings.TrimSpace(flowHash)
	if flowHash == "" || len(payload) == 0 {
		return nil
	}
	state := e.getTaskState(taskId)
	state.lastActivity = time.Now()
	// Best-effort: if producer forgot to send task_start, keep pipeline moving.
	state.emitStartOnce(e, taskId, runtimeId, subTaskId, "", "")
	if !state.markFlowSent(flowHash) {
		return nil
	}
	e.emitDataflow(flowHash, payload, taskId, runtimeId, subTaskId)
	return nil
}

// EmitSSARisk emits one risk payload. It should carry references to file/dataflow hashes.
func (e *StreamEmitter) EmitSSARisk(taskId, runtimeId, subTaskId string, ev *spec.SSAStreamRiskEvent) error {
	if !e.Enabled() {
		return nil
	}
	if ev == nil {
		return nil
	}
	riskHash := strings.TrimSpace(ev.RiskHash)
	riskJSON := ev.RiskJSON
	if riskHash == "" && len(riskJSON) > 0 {
		riskHash = strings.TrimSpace(gjson.GetBytes(riskJSON, "hash").String())
	}
	if riskHash == "" && len(riskJSON) > 0 {
		riskHash = calcFallbackRiskHashFromFields(
			gjson.GetBytes(riskJSON, "title").String(),
			gjson.GetBytes(riskJSON, "code_source_url").String(),
			gjson.GetBytes(riskJSON, "code_range").String(),
			gjson.GetBytes(riskJSON, "program_name").String(),
			gjson.GetBytes(riskJSON, "risk_type").String(),
		)
	}
	if riskHash == "" || len(riskJSON) == 0 {
		return nil
	}
	if e.dropInfo && strings.EqualFold(gjson.GetBytes(riskJSON, "severity").String(), "info") {
		return nil
	}
	// Ensure hash field is present and data_flow_paths is not duplicated inside risk_json.
	if v, err := sjson.DeleteBytes(riskJSON, "data_flow_paths"); err == nil {
		riskJSON = v
	}
	if v, err := sjson.SetBytes(riskJSON, "hash", riskHash); err == nil {
		riskJSON = v
	}

	programName := strings.TrimSpace(ev.ProgramName)
	if programName == "" {
		programName = strings.TrimSpace(gjson.GetBytes(riskJSON, "program_name").String())
	}
	reportType := strings.TrimSpace(ev.ReportType)

	state := e.getTaskState(taskId)
	state.lastActivity = time.Now()
	state.emitStartOnce(e, taskId, runtimeId, subTaskId, programName, reportType)

	out := &spec.SSAStreamRiskEvent{
		RiskHash:       riskHash,
		ProgramName:    programName,
		ReportType:     reportType,
		RiskJSON:       riskJSON,
		FileHashes:     ev.FileHashes,
		DataflowHashes: ev.DataflowHashes,
	}
	e.emitEnvelope(taskId, runtimeId, subTaskId, spec.SSAStreamEventRisk, out)
	return nil
}

func calcFallbackRiskHashFromFields(title, codeSourceURL, codeRange, programName, riskType string) string {
	return codec.Sha256(fmt.Sprintf("%s|%s|%s|%s|%s",
		title,
		codeSourceURL,
		codeRange,
		programName,
		riskType,
	))
}

func (s *taskStreamState) emitStartOnce(e *StreamEmitter, taskId, runtimeId, subTaskId, programName, reportType string) {
	if s == nil || e == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	ev := &spec.SSAStreamTaskStartEvent{
		TaskId:     taskId,
		Program:    programName,
		ReportType: reportType,
	}
	// Force seq for task_start to preserve ordering relative to risk/file/flow events.
	e.emitEnvelopeForceSeq(taskId, runtimeId, subTaskId, spec.SSAStreamEventTaskStart, ev)
}

func (e *StreamEmitter) emitFile(fileHash string, file *sfreport.File, taskId, runtimeId, subTaskId string) {
	rawContent := []byte(file.Content)
	content, encoding := e.maybeCompress(rawContent)
	meta := &spec.SSAStreamFileMetaEvent{
		FileHash:    fileHash,
		Path:        file.Path,
		Length:      file.Length,
		LineCount:   file.LineCount,
		Hash:        file.Hash,
		ContentSize: int64(len(rawContent)),
		Encoding:    encoding,
	}
	if e.inlineMax > 0 && len(content) > 0 && len(content) <= e.inlineMax {
		meta.InlineContent = content
		e.emitEnvelope(taskId, runtimeId, subTaskId, spec.SSAStreamEventFileMeta, meta)
		return
	}
	e.emitEnvelope(taskId, runtimeId, subTaskId, spec.SSAStreamEventFileMeta, meta)
	e.emitChunks(taskId, runtimeId, subTaskId, spec.SSAStreamEventFileChunk, fileHash, content)
}

func (e *StreamEmitter) emitDataflow(flowHash string, payload []byte, taskId, runtimeId, subTaskId string) {
	rawPayload := payload
	payload, encoding := e.maybeCompress(rawPayload)
	meta := &spec.SSAStreamDataflowMetaEvent{
		DataflowHash: flowHash,
		Size:         int64(len(rawPayload)),
		Encoding:     encoding,
	}
	if e.inlineMax > 0 && len(payload) > 0 && len(payload) <= e.inlineMax {
		meta.InlineContent = payload
		e.emitEnvelope(taskId, runtimeId, subTaskId, spec.SSAStreamEventDataflowMeta, meta)
		return
	}
	e.emitEnvelope(taskId, runtimeId, subTaskId, spec.SSAStreamEventDataflowMeta, meta)
	e.emitChunks(taskId, runtimeId, subTaskId, spec.SSAStreamEventDataflowChunk, flowHash, payload)
}
