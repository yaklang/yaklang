package aicommon

import (
	"fmt"
	"strings"
)

const (
	aiRequestReferenceTitle  = "AI 请求原文"
	aiResponseReferenceTitle = "AI 响应原文"
)

func formatAIReferenceMaterial(title, content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return fmt.Sprintf("【%s】\n\n%s", title, content)
}

func EmitAIRequestAndResponseReferenceMaterials(emitter *Emitter, eventID, requestContent, responseContent string) {
	if emitter == nil || strings.TrimSpace(eventID) == "" {
		return
	}

	if requestPayload := formatAIReferenceMaterial(aiRequestReferenceTitle, requestContent); requestPayload != "" {
		_, _ = emitter.EmitTextReferenceMaterial(eventID, requestPayload)
	}
	if responsePayload := formatAIReferenceMaterial(aiResponseReferenceTitle, responseContent); responsePayload != "" {
		_, _ = emitter.EmitTextReferenceMaterial(eventID, responsePayload)
	}
}
