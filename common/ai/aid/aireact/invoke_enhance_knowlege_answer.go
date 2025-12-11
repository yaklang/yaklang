package aireact

import (
	"bytes"
	"context"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
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

	ekm := config.EnhanceKnowledgeManager

	if ekm == nil {
		log.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check config! use temp rag knowledge manager")
		ekm = rag.NewRagEnhanceKnowledgeManager()
		ekm.SetEmitter(r.Emitter)
	}

	enhanceData, err := ekm.FetchKnowledge(ctx, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	// Collect all knowledge items for summary artifact
	var knowledgeList []aicommon.EnhanceKnowledge
	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		ekm.AppendKnowledge(currentTask.GetId(), enhanceDatum)
		knowledgeList = append(knowledgeList, enhanceDatum)
	}
	knowledgeCount := len(knowledgeList)

	// Save all knowledge to a single artifact file
	if knowledgeCount > 0 {
		r.EmitKnowledgeReferenceArtifact(knowledgeList, userQuery)
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

	// Build reference material content with original query and knowledge data
	referenceMaterial := ""
	if enhance != "" {
		referenceMaterial, _ = utils.RenderTemplate(`<|ORIGINAL_QUERY|>
{{ .OriginalQuery }}
<|ORIGINAL_QUERY_END|>

<|KNOWLEDGE_ENHANCED_DATA|>
{{ .EnhanceData }}
<|KNOWLEDGE_ENHANCED_DATA_END|>

知识条目数量: {{ .KnowledgeCount }}
`, map[string]any{
			"OriginalQuery":  userQuery,
			"EnhanceData":    enhance,
			"KnowledgeCount": knowledgeCount,
		})
	}

	// Pass reference material to DirectlyAnswer for emission with stream
	var opts []any
	if referenceMaterial != "" {
		opts = append(opts, WithReferenceMaterial(referenceMaterial, 1))
	}

	finalResult, err := r.DirectlyAnswer(ctx, queryBuf.String(), nil, opts...)
	// Note: DirectlyAnswer already emits the result via stream
	// EmitTextArtifact only saves to file for reference, doesn't show duplicate UI
	if finalResult != "" {
		r.EmitTextArtifact("enhance_directly_answer", finalResult)
	}
	return finalResult, err
}

func (r *ReAct) EnhanceKnowledgeGetter(ctx context.Context, userQuery string) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()
	config := r.config

	ekm := config.EnhanceKnowledgeManager
	if ekm == nil {
		log.Errorf("enhanceKnowledgeManager is not configured, but ai choice knowledge enhance answer action, check config! use temp rag knowledge manager")
		ekm = rag.NewRagEnhanceKnowledgeManager()
		ekm.SetEmitter(r.Emitter)
	}

	enhanceData, err := ekm.FetchKnowledge(ctx, userQuery)
	if err != nil {
		return "", utils.Errorf("enhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	for enhanceDatum := range enhanceData {
		r.EmitKnowledge(enhanceID, enhanceDatum)
		ekm.AppendKnowledge(currentTask.GetId(), enhanceDatum)
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

	return enhance, nil
}
