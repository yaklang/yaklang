package spec

const (
	// SSAArtifactFormatPartsNDJSONV1 stores a sequence of SSAResultParts JSON
	// objects (one per line/record), then applies outer codec compression.
	SSAArtifactFormatPartsNDJSONV1 = "ssa-result-parts-ndjson-v1"
	// SSAArtifactFormatSegmentsManifestV1 stores a JSON manifest that points
	// to multiple segment objects. Each segment stores NDJSON parts with its own codec.
	SSAArtifactFormatSegmentsManifestV1 = "ssa-result-segments-manifest-v1"
)

type SSAArtifactSegment struct {
	Seq              int    `json:"seq"`
	ObjectKey        string `json:"object_key"`
	Codec            string `json:"codec"`
	CompressedSize   int64  `json:"compressed_size"`
	UncompressedSize int64  `json:"uncompressed_size"`
	UploadMS         int64  `json:"upload_ms,omitempty"`
	SHA256           string `json:"sha256,omitempty"`
}

type SSAArtifactManifestV1 struct {
	Version string `json:"version"`
	Format  string `json:"format"`
	TaskID  string `json:"task_id,omitempty"`

	ProgramName string `json:"program_name,omitempty"`
	ReportType  string `json:"report_type,omitempty"`

	TotalSegments         int                  `json:"total_segments"`
	TotalCompressedSize   int64                `json:"total_compressed_size"`
	TotalUncompressedSize int64                `json:"total_uncompressed_size"`
	Segments              []SSAArtifactSegment `json:"segments"`

	ProducedAt int64 `json:"produced_at"`
}

// SSAArtifactReadyEvent is sent by ScanNode after uploading full SSA report
// artifact to object storage. Server should consume this as a control-plane
// notification and run async import from object storage.
type SSAArtifactReadyEvent struct {
	ObjectKey        string `json:"object_key"`
	Codec            string `json:"codec"` // "zstd" | "gzip" | "identity"
	ArtifactFormat   string `json:"artifact_format,omitempty"`
	CompressedSize   int64  `json:"compressed_size"`
	UncompressedSize int64  `json:"uncompressed_size"`
	SHA256           string `json:"sha256"`

	ProgramName string `json:"program_name,omitempty"`
	ReportType  string `json:"report_type,omitempty"`
	TotalLines  int64  `json:"total_lines,omitempty"`
	RiskCount   int64  `json:"risk_count,omitempty"`
	FileCount   int64  `json:"file_count,omitempty"`
	FlowCount   int64  `json:"flow_count,omitempty"`

	ProducedAt int64 `json:"produced_at"`
}
