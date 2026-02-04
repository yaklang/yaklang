package aireact

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// EnhanceKnowledgeGetterEx 支持多种 EnhancePlan 的知识增强获取器
// enhancePlans 参数可选，支持：
//   - nil 或空切片：使用默认完整增强流程（hypothetical_answer, generalize_query, split_query, exact_keyword_search）
//   - []string{"exact_keyword_search"}: 仅使用精准关键词搜索（跳过关键词生成，适用于 keyword 搜索模式）
//   - []string{"hypothetical_answer"}: 仅使用 HyDE 假设回答
//   - []string{"split_query"}: 仅使用拆分查询
//   - []string{"generalize_query"}: 仅使用泛化查询
//   - 可组合使用: []string{"hypothetical_answer", "generalize_query"}
func (r *ReAct) EnhanceKnowledgeGetterEx(ctx context.Context, userQuery string, enhancePlans []string, collections ...string) (string, error) {
	if utils.IsNil(ctx) {
		ctx = r.config.GetContext()
	}

	currentTask := r.GetCurrentTask()
	enhanceID := uuid.NewString()

	// Get or create EnhanceKnowledgeManager
	// If user configured a custom manager (e.g., for testing), use it
	// Otherwise, create a default RAG-based manager with the specified options
	ekm := r.config.EnhanceKnowledgeManager
	if ekm == nil {
		log.Warnf("EnhanceKnowledgeManager is not configured, using default RAG knowledge manager")
		// Create RAG manager with options for enhancePlans and collections
		var ragOpts []rag.RAGSystemConfigOption
		if len(enhancePlans) > 0 {
			ragOpts = append(ragOpts, rag.WithRAGEnhance(enhancePlans...))
		}
		if len(collections) > 0 {
			ragOpts = append(ragOpts, rag.WithRAGCollectionNames(collections...))
		}
		ekm = rag.NewRagEnhanceKnowledgeManagerWithOptions(ragOpts...)
		ekm.SetEmitter(r.Emitter)
	}

	// Fetch knowledge through the manager
	knowledgeCh, err := ekm.FetchKnowledgeWithCollections(ctx, collections, userQuery)
	if err != nil {
		return "", utils.Errorf("EnhanceKnowledgeManager.FetchKnowledge(%s) failed: %v", userQuery, err)
	}

	// Collect all knowledge items for summary artifact
	var knowledgeList []aicommon.EnhanceKnowledge

	// Process knowledge from the manager
	for knowledge := range knowledgeCh {
		// Emit knowledge event
		r.EmitKnowledge(enhanceID, knowledge)
		// Collect for artifact generation
		knowledgeList = append(knowledgeList, knowledge)
		// Append to manager's knowledge map for current task
		if currentTask != nil {
			ekm.AppendKnowledge(currentTask.GetId(), knowledge)
		}
	}

	// Save all knowledge to a single artifact file
	if len(knowledgeList) > 0 {
		r.EmitKnowledgeReferenceArtifact(knowledgeList, userQuery)
	}

	// Get enhanced data
	enhance := r.DumpCurrentEnhanceData()
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
	fmt.Println(enhance)
	fmt.Println("----------------------------------------------------------------")
	fmt.Println("----------------------------------------------------------------")
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
			enhance = enhancePayload
		}
	}

	return enhance, nil
}
