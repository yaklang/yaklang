package aicommon

const (
	AttachedResourceTypeHTTPFlowID = "http_flow_id"
	AttachedResourceTypeSelected   = "selected"

	AttachedResourceKeyID      = "id"
	AttachedResourceKeyContent = "content"

	AttachedHTTPFlowRequestInlineLimit  = 3 * 1024
	AttachedHTTPFlowResponseInlineLimit = 3 * 1024
	AttachedSelectedTextInlineLimit     = 5 * 1024
)

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
