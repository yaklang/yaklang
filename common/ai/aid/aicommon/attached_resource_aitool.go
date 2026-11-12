package aicommon

func init() {
	RegisterAttachedResourceDataFactory(
		CONTEXT_PROVIDER_TYPE_AITOOL,
		func() AttachedResourceData { return &AttachedAIToolResourceData{} },
	)
}

// AttachedAIToolResourceData is the concrete attachment type for frontend
// AITool mentions. Prompt and runtime behavior are intentionally left for a
// follow-up change.
type AttachedAIToolResourceData struct {
	Raw string
}

func (d *AttachedAIToolResourceData) Type() string {
	return CONTEXT_PROVIDER_TYPE_AITOOL
}

func (d *AttachedAIToolResourceData) Unmarshal(raw string) error {
	d.Raw = raw
	return nil
}

func (d *AttachedAIToolResourceData) BindLoopData(ReActLoopIF) error {
	return nil
}

func (d *AttachedAIToolResourceData) ToAttachData(ReActLoopIF) string {
	return ""
}
