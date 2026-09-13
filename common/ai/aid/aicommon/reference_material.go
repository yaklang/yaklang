package aicommon

import (
	"fmt"
	"strings"
)

const aiResponseReferenceTitle = "AI 响应原文"

func formatAIReferenceMaterial(title, content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return fmt.Sprintf("【%s】\n\n%s", title, content)
}

// EmitAIResponseReferenceMaterial attaches the raw response without duplicating
// the full request prompt in reference-material events.
func EmitAIResponseReferenceMaterial(emitter *Emitter, eventID, responseContent string) {
	if emitter == nil || strings.TrimSpace(eventID) == "" {
		return
	}

	if responsePayload := formatAIReferenceMaterial(aiResponseReferenceTitle, responseContent); responsePayload != "" {
		_, _ = emitter.EmitTextReferenceMaterial(eventID, responsePayload)
	}
}
