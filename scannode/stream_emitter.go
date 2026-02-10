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

	"github.com/golang/snappy"
	"github.com/klauspost/compress/zstd"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi/sfreport"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

type StreamEmitter struct {
	agent              *ScanNode
	enabled            bool
	codec              string // "", gzip, zstd, snappy
	chunkSize          int
	inlineMax          int
	mockRiskMultiplier int
	disableSeq         bool
	dropInfo           bool
	perTask            sync.Map

	zstdEncPool sync.Pool

	// Envelope batching reduces per-message overhead in MQ by sending a JSON array of
	// StreamEnvelope objects in one ScanResult message. Disabled by default.
	batchEnabled  bool
	batchMax      int
	batchMaxBytes int
	batchFlushDur time.Duration
}

type taskStreamState struct {
	mu           sync.Mutex
	seq          int64
	started      bool
	ended        bool
	sentFiles    map[string]struct{}
	sentFlows    map[string]struct{}
	lastActivity time.Time

	// Envelope batching state (per task).
	lastRuntimeId string
	lastSubTaskId string
	batchRaws     [][]byte
	batchBytes    int
	batchTimer    *time.Timer
}

func NewStreamEmitter(agent *ScanNode) *StreamEmitter {
	enabled := true
	if raw := os.Getenv("SCANNODE_STREAM_LAYERED"); raw != "" {
		enabled = raw == "1" || raw == "true" || raw == "TRUE"
	}

	codecName := ""
	codecExplicit := false
	if raw := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_CODEC")); raw != "" {
		codecName = strings.ToLower(raw)
		codecExplicit = true
	} else if raw := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_ENCODING")); raw != "" {
		codecName = strings.ToLower(raw)
		codecExplicit = true
	}
	switch codecName {
	case "none", "off", "0", "false":
		codecName = ""
	case "", "gzip", "zstd", "snappy", "auto":
	default:
		// Invalid codec: ignore only if not explicit; if explicit, treat as none.
		if codecExplicit {
			codecName = ""
		} else {
			codecName = ""
		}
	}
	if codecName == "" && !codecExplicit {
		// Backward compatible default: gzip enabled unless explicitly disabled.
		gzipEnabled := true
		if raw := os.Getenv("SCANNODE_STREAM_GZIP"); raw != "" {
			gzipEnabled = raw == "1" || raw == "true" || raw == "TRUE"
		}
		if gzipEnabled {
			codecName = "gzip"
		}
	}
	if codecName == "auto" {
		// Heuristic: localhost / loopback => disable compression (CPU > bandwidth).
		// Otherwise prefer zstd (fast + good ratio).
		if isLocalAddress(agent) {
			codecName = ""
		} else {
			codecName = "zstd"
		}
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

	// If stream mode is minimizing dataflow payload, also minimize dataflow generation to avoid
	// paying for source snippets/dot graph that won't be persisted for audit anyway.
	// This only affects this process (yak mq scannode) and can be overridden by explicitly setting SSA_DATAFLOW_MINIMAL.
	if enabled && strings.TrimSpace(os.Getenv("SSA_DATAFLOW_MINIMAL")) == "" {
		raw := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_FLOW_MINIMAL"))
		flowMinimal := raw == "" || raw == "1" || strings.EqualFold(raw, "true")
		if flowMinimal {
			_ = os.Setenv("SSA_DATAFLOW_MINIMAL", "1")
		}
	}

	batchMax := 0
	if raw := os.Getenv("SCANNODE_STREAM_ENVELOPE_BATCH_MAX"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			batchMax = v
		}
	}
	batchMaxBytes := 256 * 1024
	if raw := os.Getenv("SCANNODE_STREAM_ENVELOPE_BATCH_BYTES"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			batchMaxBytes = v
		}
	}
	flushMs := 10
	if raw := os.Getenv("SCANNODE_STREAM_ENVELOPE_BATCH_FLUSH_MS"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 0 {
			flushMs = v
		}
	}
	unsafeBatch := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_ENVELOPE_BATCH_UNSAFE"))
	batchEnabled := (unsafeBatch == "1" || strings.EqualFold(unsafeBatch, "true")) && batchMax > 0 && batchMaxBytes > 0
	if batchEnabled {
		log.Warnf("stream envelope batching enabled (experimental/unsafe): max=%d bytes=%d flush_ms=%d",
			batchMax, batchMaxBytes, flushMs)
	}

	e := &StreamEmitter{
		agent:              agent,
		enabled:            enabled,
		codec:              codecName,
		chunkSize:          chunkSize,
		inlineMax:          inlineMax,
		mockRiskMultiplier: mockRiskMultiplier,
		disableSeq:         disableSeq,
		dropInfo:           dropInfo,
		batchEnabled:       batchEnabled,
		batchMax:           batchMax,
		batchMaxBytes:      batchMaxBytes,
		batchFlushDur:      time.Duration(flushMs) * time.Millisecond,
	}
	e.zstdEncPool = sync.Pool{
		New: func() any {
			enc, err := zstd.NewWriter(nil,
				zstd.WithEncoderLevel(zstd.SpeedFastest),
				zstd.WithEncoderConcurrency(1),
			)
			if err != nil {
				return nil
			}
			return enc
		},
	}
	return e
}

func (e *StreamEmitter) Enabled() bool {
	return e != nil && e.enabled
}

func (e *StreamEmitter) shouldMinimizeDataflow() bool {
	if e == nil {
		return false
	}
	// Default on: dataflow JSON contains a lot of redundant fields (source snippets/dot graph),
	// but the server only needs node/edge ids + ir_source_hash + offsets to persist audit graph.
	raw := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_FLOW_MINIMAL"))
	if raw == "" {
		return true
	}
	return raw == "1" || strings.EqualFold(raw, "true")
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

	// Flush any buffered envelopes first, so task_end won't be delayed by batch timer.
	e.flushTaskBatch(taskId)

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
func (e *StreamEmitter) EmitSSARisk(taskId, runtimeId, subTaskId string, ev *spec.StreamRiskEvent) error {
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

	out := &spec.StreamRiskEvent{
		RiskHash:       riskHash,
		ProgramName:    programName,
		ReportType:     reportType,
		RiskJSON:       riskJSON,
		FileHashes:     ev.FileHashes,
		DataflowHashes: ev.DataflowHashes,
	}
	e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventRisk, out)

	if e.mockRiskMultiplier > 1 {
		for i := 1; i < e.mockRiskMultiplier; i++ {
			mockHash := fmt.Sprintf("%s-mock-%d", riskHash, i)
			mockJSON := riskJSON
			if v, err := sjson.SetBytes(mockJSON, "hash", mockHash); err == nil {
				mockJSON = v
			}
			mockEv := &spec.StreamRiskEvent{
				RiskHash:       mockHash,
				ProgramName:    programName,
				ReportType:     reportType,
				RiskJSON:       mockJSON,
				FileHashes:     ev.FileHashes,
				DataflowHashes: ev.DataflowHashes,
			}
			e.emitEnvelope(taskId, runtimeId, subTaskId, spec.StreamEventRisk, mockEv)
		}
	}
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

	ev := &spec.StreamTaskStartEvent{
		TaskId:     taskId,
		Program:    programName,
		ReportType: reportType,
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
	// Only batch risk events. Meta events may carry inline file/dataflow payloads and can become
	// large enough to regress latency or hit message size limits.
	allowBatch := eventType == spec.StreamEventRisk
	e.sendEnvelopeRaw(taskId, runtimeId, subTaskId, envRaw, allowBatch)
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
	// Force-send control envelopes (never batch).
	e.sendEnvelopeRaw(taskId, runtimeId, subTaskId, envRaw, false)
}

func (e *StreamEmitter) sendEnvelopeRaw(taskId, runtimeId, subTaskId string, envRaw []byte, allowBatch bool) {
	if e == nil || e.agent == nil || taskId == "" || len(envRaw) == 0 {
		return
	}
	if !allowBatch || !e.batchEnabled || e.batchMax <= 0 || e.batchMaxBytes <= 0 {
		e.agent.feedback(&spec.ScanResult{
			Type:      spec.ScanResult_StreamEvent,
			Content:   envRaw,
			TaskId:    taskId,
			RuntimeId: runtimeId,
			SubTaskId: subTaskId,
		})
		return
	}

	// Very large envelopes should never be batched (avoid oversized messages).
	if len(envRaw) >= e.batchMaxBytes/2 {
		e.flushTaskBatch(taskId)
		e.agent.feedback(&spec.ScanResult{
			Type:      spec.ScanResult_StreamEvent,
			Content:   envRaw,
			TaskId:    taskId,
			RuntimeId: runtimeId,
			SubTaskId: subTaskId,
		})
		return
	}

	state := e.getTaskState(taskId)
	var flushRaws [][]byte
	var flushRuntime, flushSub string
	state.mu.Lock()
	state.lastRuntimeId = runtimeId
	state.lastSubTaskId = subTaskId
	state.batchRaws = append(state.batchRaws, envRaw)
	state.batchBytes += len(envRaw)
	if state.batchTimer == nil && e.batchFlushDur > 0 {
		state.batchTimer = time.AfterFunc(e.batchFlushDur, func() { e.flushTaskBatch(taskId) })
	}
	shouldFlush := len(state.batchRaws) >= e.batchMax || state.batchBytes >= e.batchMaxBytes
	if shouldFlush {
		flushRaws = state.batchRaws
		flushRuntime = state.lastRuntimeId
		flushSub = state.lastSubTaskId
		state.batchRaws = nil
		state.batchBytes = 0
		if state.batchTimer != nil {
			state.batchTimer.Stop()
			state.batchTimer = nil
		}
	}
	state.mu.Unlock()

	if len(flushRaws) > 0 {
		e.sendEnvelopeBatch(taskId, flushRuntime, flushSub, flushRaws)
	}
}

func (e *StreamEmitter) flushTaskBatch(taskId string) {
	if e == nil || !e.batchEnabled || taskId == "" {
		return
	}
	state := e.getTaskState(taskId)
	var flushRaws [][]byte
	var flushRuntime, flushSub string
	state.mu.Lock()
	if len(state.batchRaws) == 0 {
		if state.batchTimer != nil {
			state.batchTimer.Stop()
			state.batchTimer = nil
		}
		state.mu.Unlock()
		return
	}
	flushRaws = state.batchRaws
	flushRuntime = state.lastRuntimeId
	flushSub = state.lastSubTaskId
	state.batchRaws = nil
	state.batchBytes = 0
	if state.batchTimer != nil {
		state.batchTimer.Stop()
		state.batchTimer = nil
	}
	state.mu.Unlock()

	e.sendEnvelopeBatch(taskId, flushRuntime, flushSub, flushRaws)
}

func (e *StreamEmitter) sendEnvelopeBatch(taskId, runtimeId, subTaskId string, envRaws [][]byte) {
	if e == nil || e.agent == nil || taskId == "" || len(envRaws) == 0 {
		return
	}
	// Build JSON array without re-marshalling each envelope.
	var buf bytes.Buffer
	buf.Grow(2 + len(envRaws))
	buf.WriteByte('[')
	first := true
	for _, raw := range envRaws {
		if len(raw) == 0 {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(raw)
	}
	buf.WriteByte(']')
	e.agent.feedback(&spec.ScanResult{
		Type:      spec.ScanResult_StreamEvent,
		Content:   buf.Bytes(),
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

func isLocalAddress(agent *ScanNode) bool {
	if agent == nil {
		return false
	}
	ip := strings.TrimSpace(strings.ToLower(agent.serverIp))
	if ip == "" {
		return false
	}
	if ip == "127.0.0.1" || ip == "localhost" || ip == "::1" {
		return true
	}
	if strings.HasPrefix(ip, "127.") {
		return true
	}
	return false
}

func (e *StreamEmitter) maybeCompress(raw []byte) ([]byte, string) {
	if e == nil || e.codec == "" || len(raw) < 1024 {
		return raw, ""
	}

	codecName := e.codec
	var enc []byte
	switch codecName {
	case "gzip":
		var buf bytes.Buffer
		zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if err != nil {
			return raw, ""
		}
		_, _ = zw.Write(raw)
		_ = zw.Close()
		enc = buf.Bytes()
	case "zstd":
		v := e.zstdEncPool.Get()
		encWriter, _ := v.(*zstd.Encoder)
		if encWriter == nil {
			return raw, ""
		}
		enc = encWriter.EncodeAll(raw, nil)
		e.zstdEncPool.Put(encWriter)
	case "snappy":
		enc = snappy.Encode(nil, raw)
	default:
		return raw, ""
	}

	// Keep compression only when it helps enough (avoid wasting CPU for tiny wins).
	// Source code and JSON are usually highly compressible.
	threshold := len(raw) / 10 // 10% gain
	if codecName == "snappy" {
		threshold = len(raw) / 20 // 5% gain (snappy is very fast)
	}
	if len(enc) >= len(raw)-threshold {
		return raw, ""
	}
	return append([]byte(nil), enc...), codecName
}
