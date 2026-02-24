// TODO: MQ 不适合批量数据传输，当前流式协议是过渡方案，后续改为对象存储 + MQ 通知。
package spec

import "encoding/json"

type StreamEventType string

// SSAStreamEventType is the semantic name for current stream event domain.
// It is kept as an alias to preserve compatibility with existing references.
type SSAStreamEventType = StreamEventType

const (
	SSAStreamEventUnknown       StreamEventType = "unknown"
	SSAStreamEventTaskStart     StreamEventType = "task_start"
	SSAStreamEventTaskEnd       StreamEventType = "task_end"
	SSAStreamEventRisk          StreamEventType = "risk"
	SSAStreamEventFileMeta      StreamEventType = "file_meta"
	SSAStreamEventFileChunk     StreamEventType = "file_chunk"
	SSAStreamEventDataflowMeta  StreamEventType = "dataflow_meta"
	SSAStreamEventDataflowChunk StreamEventType = "dataflow_chunk"
)

const (
	// Deprecated: use SSAStreamEvent* names. Keep aliases for compatibility.
	StreamEventUnknown       = SSAStreamEventUnknown
	StreamEventTaskStart     = SSAStreamEventTaskStart
	StreamEventTaskEnd       = SSAStreamEventTaskEnd
	StreamEventRisk          = SSAStreamEventRisk
	StreamEventFileMeta      = SSAStreamEventFileMeta
	StreamEventFileChunk     = SSAStreamEventFileChunk
	StreamEventDataflowMeta  = SSAStreamEventDataflowMeta
	StreamEventDataflowChunk = SSAStreamEventDataflowChunk
)

type SSAStreamEnvelope struct {
	TaskId    string          `json:"task_id"`
	RuntimeId string          `json:"runtime_id"`
	SubTaskId string          `json:"sub_task_id"`
	EventId   string          `json:"event_id"`
	Seq       int64           `json:"seq"`
	Timestamp int64           `json:"timestamp"`
	Type      StreamEventType `json:"type"`

	Payload     json.RawMessage `json:"payload"`
	PayloadHash string          `json:"payload_hash,omitempty"`
	PayloadSize int64           `json:"payload_size,omitempty"`

	Tags map[string]string `json:"tags,omitempty"`
}

// StreamEnvelope is kept as a compatibility alias.
type StreamEnvelope = SSAStreamEnvelope

type SSAStreamTaskStartEvent struct {
	TaskId     string `json:"task_id"`
	Program    string `json:"program"`
	ReportType string `json:"report_type"`
}

type SSAStreamTaskEndEvent struct {
	TaskId      string `json:"task_id"`
	TotalRisks  int64  `json:"total_risks"`
	TotalFiles  int64  `json:"total_files"`
	TotalFlows  int64  `json:"total_flows"`
	FinishedAt  int64  `json:"finished_at"`
	FinalStatus string `json:"final_status"`
}

type SSAStreamRiskEvent struct {
	RiskHash       string          `json:"risk_hash"`
	ProgramName    string          `json:"program_name"`
	ReportType     string          `json:"report_type"`
	RiskJSON       json.RawMessage `json:"risk_json"`
	FileHashes     []string        `json:"file_hashes,omitempty"`
	DataflowHashes []string        `json:"dataflow_hashes,omitempty"`
}

type SSAStreamFileMetaEvent struct {
	FileHash  string            `json:"file_hash"`
	Path      string            `json:"path"`
	Length    int64             `json:"length"`
	LineCount int               `json:"line_count"`
	Hash      map[string]string `json:"hash"`
	// ContentSize is the size of the original (decoded) content in bytes.
	ContentSize int64 `json:"content_size"`
	// Encoding indicates how InlineContent / chunk Data is encoded (e.g. "gzip").
	Encoding      string `json:"encoding,omitempty"`
	InlineContent []byte `json:"inline_content,omitempty"`
}

type SSAStreamFileChunkEvent struct {
	FileHash   string `json:"file_hash"`
	ChunkIndex int    `json:"chunk_index"`
	Data       []byte `json:"data"`
	IsLast     bool   `json:"is_last"`
}

type SSAStreamDataflowMetaEvent struct {
	DataflowHash string `json:"dataflow_hash"`
	// Size is the size of the original (decoded) payload in bytes.
	Size int64 `json:"size"`
	// Encoding indicates how InlineContent / chunk Data is encoded (e.g. "gzip").
	Encoding      string `json:"encoding,omitempty"`
	InlineContent []byte `json:"inline_content,omitempty"`
}

type SSAStreamDataflowChunkEvent struct {
	DataflowHash string `json:"dataflow_hash"`
	ChunkIndex   int    `json:"chunk_index"`
	Data         []byte `json:"data"`
	IsLast       bool   `json:"is_last"`
}

// Deprecated: use SSAStream* names. Keep aliases for compatibility.
type StreamTaskStartEvent = SSAStreamTaskStartEvent
type StreamTaskEndEvent = SSAStreamTaskEndEvent
type StreamRiskEvent = SSAStreamRiskEvent
type StreamFileMetaEvent = SSAStreamFileMetaEvent
type StreamFileChunkEvent = SSAStreamFileChunkEvent
type StreamDataflowMetaEvent = SSAStreamDataflowMetaEvent
type StreamDataflowChunkEvent = SSAStreamDataflowChunkEvent
