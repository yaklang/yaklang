package aicommon

const (
	AttachedResourceTypeDefault         = "default"
	AttachedResourceTypeFile            = CONTEXT_PROVIDER_TYPE_FILE
	AttachedResourceTypeHTTPFlowID      = "http_flow_id"
	AttachedResourceTypeKnowledgeBase   = "knowledge_base"
	AttachedResourceTypeSelected        = "selected"
	AttachedResourceTypeHTTPFuzzRequest = "http_fuzz_request"

	AttachedResourceKeyID      = "id"
	AttachedResourceKeyContent = "content"
	AttachedResourceKeyIsHTTPS = "is_https"

	AttachedHTTPFlowRequestInlineLimit  = 3 * 1024
	AttachedHTTPFlowResponseInlineLimit = 3 * 1024
	AttachedHTTPFlowListInlineLimit     = 30 * 1024
	AttachedSelectedTextInlineLimit     = 5 * 1024
	AttachedHTTPPacketInlineLimit       = 8 * 1024
	AttachedDefaultResourceInlineLimit  = 8 * 1024
	AttachedFilePreviewLimit            = 1024
)

// AttachedCodeSelection is the JSON payload for Type=selected, Key=content from Yak Runner code chips.
type AttachedCodeSelection struct {
	Path      string `json:"path"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Language  string `json:"language"`
	Content   string `json:"content"`
}

type AttachedResource struct {
	Type  string
	Key   string
	Value string
}

func NewAttachedResource(typ string, key string, value string) *AttachedResource {
	return &AttachedResource{
		Type:  typ,
		Key:   key,
		Value: value,
	}
}
