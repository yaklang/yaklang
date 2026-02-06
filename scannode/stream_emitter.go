package scannode

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type StreamEmitter struct {
	agent              *ScanNode
	enabled            bool
	gzipEnabled        bool
	chunkSize          int
	inlineMax          int
	mockRiskMultiplier int
	disableSeq         bool
	dropInfo           bool
	perTask            sync.Map
}

type taskStreamState struct {
	mu           sync.Mutex
	seq          int64
	started      bool
	ended        bool
	sentFiles    map[string]struct{}
	sentFlows    map[string]struct{}
	lastActivity time.Time
}

func NewStreamEmitter(agent *ScanNode) *StreamEmitter {
	enabled := true
	if raw := os.Getenv("SCANNODE_STREAM_LAYERED"); raw != "" {
		enabled = raw == "1" || raw == "true" || raw == "TRUE"
	}
	gzipEnabled := true
	if raw := os.Getenv("SCANNODE_STREAM_GZIP"); raw != "" {
		gzipEnabled = raw == "1" || raw == "true" || raw == "TRUE"
	}
	chunkSize := 256 * 1024
	if raw := os.Getenv("SCANNODE_STREAM_CHUNK_SIZE"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			chunkSize = v
		}
	}
	inlineMax := 16 * 1024
	if raw := os.Getenv("SCANNODE_STREAM_INLINE_MAX"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			inlineMax = v
		}
	}
	disableSeq := false
	if raw := os.Getenv("SCANNODE_STREAM_DISABLE_SEQ"); raw != "" {
		disableSeq = raw == "1" || raw == "true" || raw == "TRUE"
	}
	dropInfo := false
	if raw := os.Getenv("SCANNODE_STREAM_DROP_INFO"); raw != "" {
		dropInfo = raw == "1" || raw == "true" || raw == "TRUE"
	}
	mockRiskMultiplier := 1
	if raw := os.Getenv("SCANNODE_STREAM_MOCK_RISK_MULTIPLIER"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 1 {
			if v > 1000 {
				v = 1000
			}
			mockRiskMultiplier = v
		}
	}
	if mockRiskMultiplier > 1 {
		log.Infof("stream mock risk multiplier enabled: %d", mockRiskMultiplier)
	}
	return &StreamEmitter{
		agent:              agent,
		enabled:            enabled,
		gzipEnabled:        gzipEnabled,
		chunkSize:          chunkSize,
		inlineMax:          inlineMax,
		mockRiskMultiplier: mockRiskMultiplier,
		disableSeq:         disableSeq,
		dropInfo:           dropInfo,
	}
}

func (e *StreamEmitter) Enabled() bool {
	return e != nil && e.enabled
}

// EmitTaskEnd should be called when the Yak script/task truly finishes (process exit / ReturnData),
// not per streamed SSA report chunk. Otherwise the server may finalize a task prematurely.
func (e *StreamEmitter) EmitTaskEnd(taskId, runtimeId, subTaskId string, totalRisks, totalFiles, totalFlows int64) {
	if !e.Enabled() || taskId == "" {
		return
	}
	v, ok := e.perTask.Load(taskId)
	if !ok {
		return
	}
	state := v.(*taskStreamState)

	state.mu.Lock()
	started := state.started
	alreadyEnded := state.ended
	if started && !alreadyEnded {
		state.ended = true
	}
	state.mu.Unlock()

	if !started || alreadyEnded {
		return
	}

	ev := &spec.StreamTaskEndEvent{
		TaskId:      taskId,
		TotalRisks:  totalRisks,
		TotalFiles:  totalFiles,
		TotalFlows:  totalFlows,
		FinishedAt:  time.Now().Unix(),
		FinalStatus: "done",
	}
	// Force seq for task_end to preserve ordering even when disableSeq is enabled.
	e.emitEnvelopeForceSeq(taskId, runtimeId, subTaskId, spec.StreamEventTaskEnd, ev)

	// Best-effort cleanup to avoid per-task state leaking forever.
	time.AfterFunc(2*time.Minute, func() { e.perTask.Delete(taskId) })
}

func (e *StreamEmitter) EmitSSAReportJSON(taskId, runtimeId, subTaskId, reportJSON string) error {
	if !e.Enabled() {
		return nil
	}
	if reportJSON == "" {
		return nil
	}

	var report sfreport.Report
	if err := json.Unmarshal([]byte(reportJSON), &report); err != nil {
		return err
	}

	state := e.getTaskState(taskId)
	state.lastActivity = time.Now()
	state.emitStartOnce(e, taskId, runtimeId, subTaskId, &report)

	fileIndex := make(map[string]*sfreport.File, len(report.File))
	riskFiles := make(map[string][]string)
	for _, f := range report.File {
		if f == nil {
			continue
		}
		fileHash := fileHashFor(f)
		if fileHash == "" {
			continue
		}
		fileIndex[fileHash] = f
		for _, riskHash := range f.Risks {
			riskFiles[riskHash] = append(riskFiles[riskHash], fileHash)
		}
	}

	riskFlows := make(map[string][]string)
	flowPayloads := make(map[string][]byte)

	for _, risk := range report.Risks {
		if risk == nil || len(risk.DataFlowPaths) == 0 {
			continue
		}
		for _, path := range risk.DataFlowPaths {
			if path == nil {
				continue
			}
			raw, err := json.Marshal(path)
			if err != nil {
				log.Errorf("stream marshal dataflow failed: %v", err)
				continue
			}
			hash := codec.Sha256(raw)
			if hash == "" {
				continue
			}
			flowPayloads[hash] = raw
			riskFlows[risk.Hash] = append(riskFlows[risk.Hash], hash)
		}
	}

	for fileHash, file := range fileIndex {
		if !state.markFileSent(fileHash) {
			continue
		}
		e.emitFile(fileHash, file, taskId, runtimeId, subTaskId)
	}

	for flowHash, payload := range flowPayloads {
		if !state.markFlowSent(flowHash) {
			continue
		}
		e.emitDataflow(flowHash, payload, taskId, runtimeId, subTaskId)
	}

	for _, risk := range report.Risks {
		if risk == nil {
			continue
		}
		if e.dropInfo && strings.EqualFold(risk.Severity, "info") {
			continue
		}
		riskCopy := *risk
		riskCopy.DataFlowPaths = nil
		if riskCopy.Hash == "" {
			riskCopy.Hash = codec.Sha256(fmt.Sprintf("%s|%s|%s|%s|%s",
				riskCopy.Title,
				riskCopy.CodeSourceURL,
				riskCopy.CodeRange,
				riskCopy.ProgramName,
				riskCopy.RiskType,
			))
		}
		baseHash := riskCopy.Hash
		riskRaw, err := json.Marshal(&riskCopy)
		if err != nil {
			log.Errorf("stream marshal risk failed: %v", err)
			continue
		}

		ev := &spec.StreamRiskEvent{
			RiskHash:       riskCopy.Hash,
			ProgramName:    coalesceString(risk.ProgramName, report.ProgramName),
			ReportType:     string(report.ReportType),
			RiskJSON:       riskRaw,
			FileHashes:     riskFiles[risk.Hash],
			DataflowHashes: riskFlows[risk.Hash],
		}
		e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventRisk, ev)

		if e.mockRiskMultiplier > 1 {
			for i := 1; i < e.mockRiskMultiplier; i++ {
				mockRisk := riskCopy
				mockRisk.Hash = fmt.Sprintf("%s-mock-%d", baseHash, i)
				if mockRisk.Title != "" {
					mockRisk.Title = fmt.Sprintf("%s [mock-%d]", riskCopy.Title, i)
				}
				if mockRisk.TitleVerbose != "" {
					mockRisk.TitleVerbose = fmt.Sprintf("%s [mock-%d]", riskCopy.TitleVerbose, i)
				}
				if mockRisk.CodeRange != "" {
					mockRisk.CodeRange = fmt.Sprintf("%s;mock-%d", riskCopy.CodeRange, i)
				} else {
					mockRisk.CodeRange = fmt.Sprintf("mock-%d", i)
				}
				if mockRisk.CodeSourceURL != "" {
					mockRisk.CodeSourceURL = fmt.Sprintf("%s?mock=%d", riskCopy.CodeSourceURL, i)
				} else {
					mockRisk.CodeSourceURL = fmt.Sprintf("/mock/%d", i)
				}
				mockRaw, err := json.Marshal(&mockRisk)
				if err != nil {
					log.Errorf("stream marshal mock risk failed: %v", err)
					continue
				}
				mockEv := &spec.StreamRiskEvent{
					RiskHash:       mockRisk.Hash,
					ProgramName:    coalesceString(risk.ProgramName, report.ProgramName),
					ReportType:     string(report.ReportType),
					RiskJSON:       mockRaw,
					FileHashes:     riskFiles[risk.Hash],
					DataflowHashes: riskFlows[risk.Hash],
				}
				e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventRisk, mockEv)
			}
		}
	}

	return nil
}

func (s *taskStreamState) emitStartOnce(e *StreamEmitter, taskId, runtimeId, subTaskId string, report *sfreport.Report) {
	if s == nil || e == nil || report == nil {
		return
	}
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	ev := &spec.StreamTaskStartEvent{
		TaskId:     taskId,
		Program:    report.ProgramName,
		ReportType: string(report.ReportType),
	}
	// Force seq for task_start to preserve ordering relative to risk/file/flow events.
	e.emitEnvelopeForceSeq(taskId, runtimeId, subTaskId, spec.StreamEventTaskStart, ev)
}

func (e *StreamEmitter) emitFile(fileHash string, file *sfreport.File, taskId, runtimeId, subTaskId string) {
	rawContent := []byte(file.Content)
	content, encoding := e.maybeCompress(rawContent)
	meta := &spec.StreamFileMetaEvent{
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
		e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventFileMeta, meta)
		return
	}
	e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventFileMeta, meta)
	e.emitChunks(taskId, runtimeId, subTaskId, spec.StreamEventFileChunk, fileHash, content)
}

func (e *StreamEmitter) emitDataflow(flowHash string, payload []byte, taskId, runtimeId, subTaskId string) {
	rawPayload := payload
	payload, encoding := e.maybeCompress(rawPayload)
	meta := &spec.StreamDataflowMetaEvent{
		DataflowHash: flowHash,
		Size:         int64(len(rawPayload)),
		Encoding:     encoding,
	}
	if e.inlineMax > 0 && len(payload) > 0 && len(payload) <= e.inlineMax {
		meta.InlineContent = payload
		e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventDataflowMeta, meta)
		return
	}
	e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventDataflowMeta, meta)
	e.emitChunks(taskId, runtimeId, subTaskId, spec.StreamEventDataflowChunk, flowHash, payload)
}

func (e *StreamEmitter) emitChunks(taskId, runtimeId, subTaskId string, eventType spec.StreamEventType, key string, payload []byte) {
	if len(payload) == 0 {
		switch eventType {
		case spec.StreamEventFileChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.StreamFileChunkEvent{
				FileHash:   key,
				ChunkIndex: 0,
				Data:       nil,
				IsLast:     true,
			})
		case spec.StreamEventDataflowChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.StreamDataflowChunkEvent{
				DataflowHash: key,
				ChunkIndex:   0,
				Data:         nil,
				IsLast:       true,
			})
		}
		return
	}
	chunkSize := e.chunkSize
	for i, offset := 0, 0; offset < len(payload); i, offset = i+1, offset+chunkSize {
		end := offset + chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		data := payload[offset:end]
		isLast := end >= len(payload)

		switch eventType {
		case spec.StreamEventFileChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.StreamFileChunkEvent{
				FileHash:   key,
				ChunkIndex: i,
				Data:       data,
				IsLast:     isLast,
			})
		case spec.StreamEventDataflowChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.StreamDataflowChunkEvent{
				DataflowHash: key,
				ChunkIndex:   i,
				Data:         data,
				IsLast:       isLast,
			})
		}
	}
}

func (e *StreamEmitter) emitEnvelope(taskId, runtimeId, subTaskId string, eventType spec.StreamEventType, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("stream marshal payload failed: %v", err)
		return
	}
	env := &spec.StreamEnvelope{
		TaskId:      taskId,
		RuntimeId:   runtimeId,
		SubTaskId:   subTaskId,
		EventId:     utils.RandStringBytes(20),
		Seq:         e.nextSeq(taskId),
		Timestamp:   time.Now().Unix(),
		Type:        eventType,
		Payload:     raw,
		PayloadHash: codec.Md5(raw),
		PayloadSize: int64(len(raw)),
	}
	if e.disableSeq {
		env.Seq = 0
	}
	envRaw, err := json.Marshal(env)
	if err != nil {
		log.Errorf("stream marshal envelope failed: %v", err)
		return
	}
	e.agent.feedback(&spec.ScanResult{
		Type:      spec.ScanResult_StreamEvent,
		Content:   envRaw,
		TaskId:    taskId,
		RuntimeId: runtimeId,
		SubTaskId: subTaskId,
	})
}

func (e *StreamEmitter) emitEnvelopeForceSeq(taskId, runtimeId, subTaskId string, eventType spec.StreamEventType, payload any) {
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Errorf("stream marshal payload failed: %v", err)
		return
	}
	env := &spec.StreamEnvelope{
		TaskId:      taskId,
		RuntimeId:   runtimeId,
		SubTaskId:   subTaskId,
		EventId:     utils.RandStringBytes(20),
		Seq:         e.nextSeq(taskId),
		Timestamp:   time.Now().Unix(),
		Type:        eventType,
		Payload:     raw,
		PayloadHash: codec.Md5(raw),
		PayloadSize: int64(len(raw)),
	}
	envRaw, err := json.Marshal(env)
	if err != nil {
		log.Errorf("stream marshal envelope failed: %v", err)
		return
	}
	e.agent.feedback(&spec.ScanResult{
		Type:      spec.ScanResult_StreamEvent,
		Content:   envRaw,
		TaskId:    taskId,
		RuntimeId: runtimeId,
		SubTaskId: subTaskId,
	})
}

func (e *StreamEmitter) getTaskState(taskId string) *taskStreamState {
	if taskId == "" {
		taskId = "default"
	}
	v, _ := e.perTask.LoadOrStore(taskId, &taskStreamState{
		sentFiles: make(map[string]struct{}),
		sentFlows: make(map[string]struct{}),
	})
	return v.(*taskStreamState)
}

func (e *StreamEmitter) nextSeq(taskId string) int64 {
	state := e.getTaskState(taskId)
	state.mu.Lock()
	defer state.mu.Unlock()
	state.seq++
	return state.seq
}

func (s *taskStreamState) markFileSent(hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sentFiles[hash]; ok {
		return false
	}
	s.sentFiles[hash] = struct{}{}
	return true
}

func (s *taskStreamState) markFlowSent(hash string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.sentFlows[hash]; ok {
		return false
	}
	s.sentFlows[hash] = struct{}{}
	return true
}

func fileHashFor(file *sfreport.File) string {
	if file == nil {
		return ""
	}
	if file.IrSourceHash != "" {
		return file.IrSourceHash
	}
	return codec.Md5(file.Path + ":" + utils.InterfaceToString(file.Hash))
}

func coalesceString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (e *StreamEmitter) maybeCompress(raw []byte) ([]byte, string) {
	if !e.gzipEnabled || len(raw) < 1024 {
		return raw, ""
	}
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return raw, ""
	}
	_, _ = zw.Write(raw)
	_ = zw.Close()

	enc := buf.Bytes()
	// Keep gzip only when it helps enough (avoid wasting CPU for tiny wins).
	// Source code and JSON are usually highly compressible, so this triggers often.
	if len(enc) >= len(raw)-(len(raw)/10) { // <10% gain
		return raw, ""
	}
	return append([]byte(nil), enc...), "gzip"
}
