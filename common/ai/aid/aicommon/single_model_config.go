package aicommon

// applySingleModelModeDefaults is called during NewConfig when single-model
// simple mode is enabled. It activates existing subsystem disable switches so
// that internal AI calls (memory triage, intent recognition, session title,
// interval review, RAG enhance) are not triggered.
//
// This centralizes the "auto-flip existing switches" logic so callers only
// need to set WithSingleAIModelMode(true) and all subsystem gates follow.
func (c *Config) applySingleModelModeDefaults() {
	// Memory triage: disable embedding/DB/AI calls for memory processing.
	if !c.DisableMemoryTriage {
		c.DisableMemoryTriage = true
	}

	// Deep intent recognition: disable to avoid sub-loop AI calls.
	if !c.DisableIntentRecognition {
		c.DisableIntentRecognition = true
	}

	// Session title / naming: disable via the config key checked in
	// re-act_mainloop.go.
	c.SetConfig("disable_session_title_generation", true)

	// Interval review: disable periodic AI review during tool execution.
	if !c.DisableIntervalReview {
		c.DisableIntervalReview = true
	}

	// RAG enhance: do not inject EnhanceKnowledgeManager so RAG-related
	// AI calls are not triggered.
	c.EnhanceKnowledgeManager = nil
}
