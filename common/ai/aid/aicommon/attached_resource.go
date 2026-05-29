package aicommon

const (
	AttachedResourceTypeHTTPFlowID = "http_flow_id"
	AttachedResourceTypeSelected   = "selected"

	AttachedResourceKeyID      = "id"
	AttachedResourceKeyContent = "content"

	AttachedHTTPFlowRequestInlineLimit  = 3 * 1024
	AttachedHTTPFlowResponseInlineLimit = 3 * 1024
	AttachedHTTPFlowListInlineLimit     = 30 * 1024
	AttachedSelectedTextInlineLimit     = 5 * 1024
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
