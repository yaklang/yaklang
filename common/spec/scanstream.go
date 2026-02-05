package spec

import "encoding/json"

type StreamEventType string

const (
	StreamEventUnknown       StreamEventType = "unknown"
	StreamEventTaskStart     StreamEventType = "task_start"
	StreamEventTaskEnd       StreamEventType = "task_end"
	StreamEventRisk          StreamEventType = "risk"
	StreamEventFileMeta      StreamEventType = "file_meta"
	StreamEventFileChunk     StreamEventType = "file_chunk"
	StreamEventDataflowMeta  StreamEventType = "dataflow_meta"
	StreamEventDataflowChunk StreamEventType = "dataflow_chunk"
)

type StreamEnvelope struct {
	TaskId    string          `json:"task_id"`
	RuntimeId string          `json:"runtime_id"`
	SubTaskId string          `json:"sub_task_id"`
	EventId   string          `json:"event_id"`
	Seq       int64           `json:"seq"`
	Timestamp int64           `json:"timestamp"`
	Type      StreamEventType `json:"type"`

	Payload     json.RawMessage `json:"payload"`
	PayloadHash string          `json:"payload_hash"`
	PayloadSize int64           `json:"payload_size"`

	Tags map[string]string `json:"tags,omitempty"`
}

type StreamTaskStartEvent struct {
	TaskId     string `json:"task_id"`
	Program    string `json:"program"`
	ReportType string `json:"report_type"`
}

type StreamTaskEndEvent struct {
	TaskId      string `json:"task_id"`
	TotalRisks  int64  `json:"total_risks"`
	TotalFiles  int64  `json:"total_files"`
	TotalFlows  int64  `json:"total_flows"`
	FinishedAt  int64  `json:"finished_at"`
	FinalStatus string `json:"final_status"`
}

type StreamRiskEvent struct {
	RiskHash       string          `json:"risk_hash"`
	ProgramName    string          `json:"program_name"`
	ReportType     string          `json:"report_type"`
	RiskJSON       json.RawMessage `json:"risk_json"`
	FileHashes     []string        `json:"file_hashes,omitempty"`
	DataflowHashes []string        `json:"dataflow_hashes,omitempty"`
}

type StreamFileMetaEvent struct {
	FileHash      string            `json:"file_hash"`
	Path          string            `json:"path"`
	Length        int64             `json:"length"`
	LineCount     int               `json:"line_count"`
	Hash          map[string]string `json:"hash"`
	ContentSize   int64             `json:"content_size"`
	InlineContent []byte            `json:"inline_content,omitempty"`
}

type StreamFileChunkEvent struct {
	FileHash   string `json:"file_hash"`
	ChunkIndex int    `json:"chunk_index"`
	Data       []byte `json:"data"`
	IsLast     bool   `json:"is_last"`
}

type StreamDataflowMetaEvent struct {
	DataflowHash  string `json:"dataflow_hash"`
	Size          int64  `json:"size"`
	InlineContent []byte `json:"inline_content,omitempty"`
}

type StreamDataflowChunkEvent struct {
	DataflowHash string `json:"dataflow_hash"`
	ChunkIndex   int    `json:"chunk_index"`
	Data         []byte `json:"data"`
	IsLast       bool   `json:"is_last"`
}
