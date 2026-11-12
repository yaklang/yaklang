package aicommon

func init() {
	RegisterAttachedResourceDataFactory(
		CONTEXT_PROVIDER_TYPE_AIFORGE,
		func() AttachedResourceData { return &AttachedAIForgeResourceData{} },
	)
}

// AttachedAIForgeResourceData is the concrete attachment type for frontend
// AIForge mentions. Prompt and runtime behavior are intentionally left for a
// follow-up change.
type AttachedAIForgeResourceData struct {
	Raw string
}

func (d *AttachedAIForgeResourceData) Type() string {
	return CONTEXT_PROVIDER_TYPE_AIFORGE
}

func (d *AttachedAIForgeResourceData) Unmarshal(raw string) error {
	d.Raw = raw
	return nil
}

func (d *AttachedAIForgeResourceData) BindLoopData(ReActLoopIF) error {
	return nil
}

func (d *AttachedAIForgeResourceData) ToAttachData(ReActLoopIF) string {
	return ""
}
