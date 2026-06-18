package loop_http_fuzztest

const (
	loopHTTPUploadRequestSummaryKey     = "upload_request_summary"
	loopHTTPUploadFileResourceRefsKey   = "upload_file_resource_refs"
	loopHTTPUploadOriginalPromptSafeKey = "original_request_prompt_safe"
	loopHTTPUploadCurrentPromptSafeKey  = "current_request_prompt_safe"
	loopHTTPUploadRepresentativeSafeKey = "representative_request_prompt_safe"

	loopHTTPUploadPartExternalizeThreshold = 16 * 1024
	loopHTTPUploadBodyExternalizeThreshold = 64 * 1024
	loopHTTPUploadPreviewMaxBytes          = 256
)

type loopHTTPUploadPartSummary struct {
	FieldName   string `json:"field_name"`
	IsFile      bool   `json:"is_file"`
	FileName    string `json:"file_name,omitempty"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size"`
	Preview     string `json:"preview,omitempty"`
	Digest      string `json:"digest,omitempty"`
	ResourceID  string `json:"resource_id,omitempty"`
}

type loopHTTPUploadRequestSummary struct {
	IsMultipart bool                        `json:"is_multipart"`
	Boundary    string                      `json:"boundary,omitempty"`
	Parts       []loopHTTPUploadPartSummary `json:"parts,omitempty"`
}

type loopHTTPUploadFileResource struct {
	ID          string `json:"id"`
	FieldName   string `json:"field_name"`
	FileName    string `json:"file_name"`
	ContentType string `json:"content_type"`
	Path        string `json:"path"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}
