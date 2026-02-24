// TODO: MQ 不适合批量数据传输，当前流式协议是过渡方案，后续改为对象存储 + MQ 通知。
package scannode

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/spec"
	"github.com/yaklang/yaklang/common/utils"
)

type StreamEmitter struct {
	agent      *ScanNode
	enabled    bool
	codec      string // "", snappy
	chunkSize  int
	inlineMax  int
	disableSeq bool
	dropInfo   bool
	perTask    sync.Map

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

// TODO: 以下环境变量是压测阶段快速调参的临时方案，应重构为 StreamEmitterConfig 结构体，通过配置文件或 Server 下发统一管理。
func NewStreamEmitter(agent *ScanNode) *StreamEmitter {
	enabled := true
	if raw := os.Getenv("SCANNODE_STREAM_LAYERED"); raw != "" {
		enabled = isTruthy(raw)
	}
	codecName := resolveCodecName(agent)
	chunkSize := readIntEnv("SCANNODE_STREAM_CHUNK_SIZE", 256*1024, func(v int) bool { return v > 0 })
	inlineMax := readIntEnv("SCANNODE_STREAM_INLINE_MAX", 16*1024, func(v int) bool { return v >= 0 })
	disableSeq := isTruthy(os.Getenv("SCANNODE_STREAM_DISABLE_SEQ"))
	dropInfo := isTruthy(os.Getenv("SCANNODE_STREAM_DROP_INFO"))
	batchMax := readIntEnv("SCANNODE_STREAM_ENVELOPE_BATCH_MAX", 0, func(v int) bool { return v > 0 })
	batchMaxBytes := readIntEnv("SCANNODE_STREAM_ENVELOPE_BATCH_BYTES", 256*1024, func(v int) bool { return v > 0 })
	flushMs := readIntEnv("SCANNODE_STREAM_ENVELOPE_BATCH_FLUSH_MS", 10, func(v int) bool { return v >= 0 })
	batchEnabled := isTruthy(os.Getenv("SCANNODE_STREAM_ENVELOPE_BATCH_UNSAFE")) && batchMax > 0 && batchMaxBytes > 0
	if batchEnabled {
		log.Warnf("stream envelope batching enabled (experimental/unsafe): max=%d bytes=%d flush_ms=%d",
			batchMax, batchMaxBytes, flushMs)
	}

	e := &StreamEmitter{
		agent:         agent,
		enabled:       enabled,
		codec:         codecName,
		chunkSize:     chunkSize,
		inlineMax:     inlineMax,
		disableSeq:    disableSeq,
		dropInfo:      dropInfo,
		batchEnabled:  batchEnabled,
		batchMax:      batchMax,
		batchMaxBytes: batchMaxBytes,
		batchFlushDur: time.Duration(flushMs) * time.Millisecond,
	}
	return e
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

	ev := &spec.SSAStreamTaskEndEvent{
		TaskId:      taskId,
		TotalRisks:  totalRisks,
		TotalFiles:  totalFiles,
		TotalFlows:  totalFlows,
		FinishedAt:  time.Now().Unix(),
		FinalStatus: "done",
	}
	// Force seq for task_end to preserve ordering even when disableSeq is enabled.
	e.emitEnvelopeForceSeq(taskId, runtimeId, subTaskId, spec.SSAStreamEventTaskEnd, ev)

	// Best-effort cleanup to avoid per-task state leaking forever.
	time.AfterFunc(2*time.Minute, func() { e.perTask.Delete(taskId) })
}

func (e *StreamEmitter) emitChunks(taskId, runtimeId, subTaskId string, eventType spec.SSAStreamEventType, key string, payload []byte) {
	if len(payload) == 0 {
		switch eventType {
		case spec.SSAStreamEventFileChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.SSAStreamFileChunkEvent{
				FileHash:   key,
				ChunkIndex: 0,
				Data:       nil,
				IsLast:     true,
			})
		case spec.SSAStreamEventDataflowChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.SSAStreamDataflowChunkEvent{
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
		case spec.SSAStreamEventFileChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.SSAStreamFileChunkEvent{
				FileHash:   key,
				ChunkIndex: i,
				Data:       data,
				IsLast:     isLast,
			})
		case spec.SSAStreamEventDataflowChunk:
			e.emitEnvelope(taskId, runtimeId, subTaskId, eventType, &spec.SSAStreamDataflowChunkEvent{
				DataflowHash: key,
				ChunkIndex:   i,
				Data:         data,
				IsLast:       isLast,
			})
		}
	}
}

func (e *StreamEmitter) emitEnvelope(taskId, runtimeId, subTaskId string, eventType spec.SSAStreamEventType, payload any) {
	// Only batch risk events. Meta events may carry inline file/dataflow payloads and can become
	// large enough to regress latency or hit message size limits.
	allowBatch := eventType == spec.SSAStreamEventRisk
	e.emitEnvelopeInternal(taskId, runtimeId, subTaskId, eventType, payload, allowBatch, false)
}

func (e *StreamEmitter) emitEnvelopeForceSeq(taskId, runtimeId, subTaskId string, eventType spec.SSAStreamEventType, payload any) {
	// Force-send control envelopes (never batch).
	e.emitEnvelopeInternal(taskId, runtimeId, subTaskId, eventType, payload, false, true)
}

func (e *StreamEmitter) emitEnvelopeInternal(
	taskId, runtimeId, subTaskId string,
	eventType spec.SSAStreamEventType,
	payload any,
	allowBatch bool,
	forceSeq bool,
) {
	env := &spec.SSAStreamEnvelope{
		TaskId:    taskId,
		RuntimeId: runtimeId,
		SubTaskId: subTaskId,
		EventId:   utils.RandStringBytes(20),
		Seq:       e.nextSeq(taskId),
		Timestamp: time.Now().Unix(),
		Type:      eventType,
	}
	// Single-marshal path: payload is encoded together with envelope.
	// Receiver decodes into SSAStreamEnvelope.Payload(raw JSON) by event type.
	wire := struct {
		*spec.SSAStreamEnvelope
		Payload any `json:"payload"`
	}{
		SSAStreamEnvelope: env,
		Payload:           payload,
	}
	if !forceSeq && e.disableSeq {
		wire.Seq = 0
	}
	envRaw, err := json.Marshal(&wire)
	if err != nil {
		log.Errorf("stream marshal envelope failed: %v", err)
		return
	}
	e.sendEnvelopeRaw(taskId, runtimeId, subTaskId, envRaw, allowBatch)
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

func isTruthy(raw string) bool {
	raw = strings.TrimSpace(raw)
	return raw == "1" || strings.EqualFold(raw, "true")
}

func readIntEnv(name string, defaultValue int, valid func(int) bool) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return defaultValue
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return defaultValue
	}
	if valid != nil && !valid(v) {
		return defaultValue
	}
	return v
}

func resolveCodecName(agent *ScanNode) string {
	codecName := "snappy"
	if raw := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_CODEC")); raw != "" {
		codecName = strings.ToLower(raw)
	} else if raw := strings.TrimSpace(os.Getenv("SCANNODE_STREAM_ENCODING")); raw != "" {
		codecName = strings.ToLower(raw)
	}

	switch codecName {
	case "none", "off", "0", "false":
		return ""
	case "auto":
		// Heuristic: localhost / loopback => disable compression (CPU > bandwidth).
		// Otherwise use snappy as the only built-in compression algorithm.
		if isLocalAddress(agent) {
			return ""
		}
		return "snappy"
	case "gzip", "zstd":
		// Keep backward compatibility for old env values.
		log.Warnf("stream codec %q is deprecated, fallback to snappy", codecName)
		return "snappy"
	case "snappy", "":
		return "snappy"
	default:
		return "snappy"
	}
}

func (e *StreamEmitter) maybeCompress(raw []byte) ([]byte, string) {
	if e == nil || e.codec == "" || len(raw) < 1024 {
		return raw, ""
	}

	codecName := e.codec
	var enc []byte
	switch codecName {
	case "snappy":
		enc = snappy.Encode(nil, raw)
	default:
		return raw, ""
	}

	// Keep compression only when it helps enough (avoid wasting CPU for tiny wins).
	// Source code and JSON are usually highly compressible.
	threshold := len(raw) / 20 // 5% gain (snappy is very fast)
	if len(enc) >= len(raw)-threshold {
		return raw, ""
	}
	return append([]byte(nil), enc...), codecName
}
