package aireact

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

func (r *ReAct) EnhanceKnowledgeAnswer(ctx context.Context, userQuery string) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()
	config := r.config

	if config.enhanceKnowledgeManager == nil {
		return "", utils.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check main loop prompt!")
	}

	enhanceData, err := config.enhanceKnowledgeManager.FetchKnowledge(ctx, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		config.enhanceKnowledgeManager.AppendKnowledge(currentTask.GetId(), enhanceDatum)
	}

	var queryBuf bytes.Buffer
	queryBuf.WriteString(userQuery)

	enhance := r.DumpCurrentEnhanceData()
	if enhance != "" {
		enhancePayload, err := utils.RenderTemplate(`<|ENHANCE_DATA_{{ .Nonce }}|>
{{ .EnhanceData }}
<|ENHANCE_DATA_{{ .Nonce }}|>
`, map[string]interface{}{
			"Nonce":       nonce(),
			"EnhanceData": enhance,
		})
		if err != nil {
			log.Warnf("enhanceKnowledgeAnswer.DumpCurrentEnhanceData() failed: %v", err)
		}
		if enhancePayload != "" {
			queryBuf.WriteString("\n\n")
			queryBuf.WriteString(enhancePayload)
		}
	}

	finalResult, err := r.DirectlyAnswer(ctx, queryBuf.String(), nil)
	if finalResult != "" {
		r.EmitTextArtifact("enhance_directly_answer", finalResult)
	}
	return finalResult, err
}
