package aireact

import (
	"context"
	"io"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/ai/rag"
	"github.com/yaklang/yaklang/common/ai/rag/vectorstore"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
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

	// 构建 RAG 查询选项
	ragOpts := []rag.RAGSystemConfigOption{
		rag.WithRAGCtx(ctx),
		rag.WithEveryQueryResultCallback(func(data *rag.ScoredResult) {
			r.EmitKnowledge(enhanceID, data)
			if currentTask != nil && r.config.EnhanceKnowledgeManager != nil {
				r.config.EnhanceKnowledgeManager.AppendKnowledge(currentTask.GetId(), data)
			}
		}),
	}

	// 设置集合名称限制
	if len(collections) > 0 {
		ragOpts = append(ragOpts, rag.WithRAGCollectionNames(collections...))
	}

	// 设置 EnhancePlan
	if len(enhancePlans) > 0 {
		ragOpts = append(ragOpts, rag.WithRAGEnhance(enhancePlans...))
	}
	// 如果 enhancePlans 为空，使用 RAG 默认的完整增强流程

	// 配置日志输出
	if r.Emitter != nil {
		ragOpts = append(ragOpts, rag.WithRAGLogReaderWithInfo(func(reader io.Reader, info *vectorstore.SubQueryLogInfo, referenceMaterialCallback func(content string)) {
			var event *schema.AiOutputEvent
			var err error
			event, err = r.Emitter.EmitDefaultStreamEvent(
				"enhance-query",
				reader,
				"",
				func() {
					if info.ResultBuffer != nil && info.ResultBuffer.Len() > 0 {
						streamId := ""
						if event != nil {
							streamId = event.GetContentJSONPath(`$.event_writer_id`)
						}
						if streamId != "" {
							_, emitErr := r.Emitter.EmitTextReferenceMaterial(streamId, info.ResultBuffer.String())
							if emitErr != nil {
								log.Warnf("failed to emit reference material: %v", emitErr)
							}
						}
					}
				},
			)
			if err != nil {
				log.Warnf("failed to emit enhance-query stream event: %v", err)
				return
			}
		}))
	}

	// 执行 RAG 查询，返回的 channel 包含查询结果
	resultCh, err := rag.QueryYakitProfile(userQuery, ragOpts...)
	if err != nil {
		return "", utils.Errorf("RAG QueryYakitProfile(%s) failed: %v", userQuery, err)
	}

	// 消费结果 channel，等待查询完成
	// channel 关闭时表示查询完成
	for range resultCh {
		// 结果已通过 WithEveryQueryResultCallback 处理，这里只是等待 channel 关闭
	}

	// 获取增强数据
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
			enhance = enhancePayload
		}
	}

	return enhance, nil
}
