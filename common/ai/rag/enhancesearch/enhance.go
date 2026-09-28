package enhancesearch

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type SearchHandler interface {
	ExtractKeywords(ctx context.Context, query string) ([]string, error)
	HypotheticalAnswer(ctx context.Context, query string) (string, error)
	SplitQuery(ctx context.Context, query string) ([]string, error)
	GeneralizeQuery(ctx context.Context, query string) ([]string, error)
}

type LiteForgeSearchHandler struct {
}

func NewDefaultSearchHandler() *LiteForgeSearchHandler {
	return NewSearchHandler()
}

func NewSearchHandler() *LiteForgeSearchHandler {
	return &LiteForgeSearchHandler{}
}

// ====================================================================================
// P3-X1: 4 个 LiteForge 子调用的静态指令独立抽出, 跨调用 byte-identical.
//
// 这些指令不含任何 nonce / query 拼接, 通过 aicommon.LiteForgeStaticInstruction
// marker 进入 LiteForge 模板的 semi-dynamic 段; 调用方传入的 query 通过
// aicommon.InvokeLiteForge 第 1 参数 (cfg.query) 进入 dynamic <params_NONCE> 段,
// LiteForge 模板自带 dynamic 段 nonce 包装防 prompt-injection.
//
// 改造前: 这些指令文本 + nonce + query 全部塞给 InvokeLiteForge 第 1 参数,
//        最终被当作 cfg.query 拼进 dynamic <params_NONCE>, 1-3KB 静态文本
//        每次随 nonce/query 变化重发, 客户端 LCP / 上游 KV cache 都无法命中.
// 改造后: 静态部分进 semi-dynamic (跨同一 method 调用 byte-identical), 仅 query
//        每次变化, 让 4 个子调用都能享受 system + semi-dynamic 段的缓存命中.
//
// 关键词: aicache, P3-X1, enhancesearch StaticInstruction 下沉, semi-dynamic 段稳定
// ====================================================================================

// extractKeywordsStaticInstruction 是 ExtractKeywords 的稳定指令头,
// query 由 LiteForge 自动包到 dynamic <params_NONCE> 段, 此处不再嵌入.
//
// 关键词: extractKeywordsStaticInstruction, ExtractKeywords semi-dynamic 段
var extractKeywordsStaticInstruction = promptloader.MustLoad("inline/ai/rag/enhancesearch/enhance/extractKeywordsStaticInstruction.txt")

// hydeStaticInstruction 是 HypotheticalAnswer 的稳定指令头.
// 关键词: hydeStaticInstruction, HypotheticalAnswer semi-dynamic 段
var hydeStaticInstruction = promptloader.MustLoad("inline/ai/rag/enhancesearch/enhance/hydeStaticInstruction.txt")

// splitQueryStaticInstruction 是 SplitQuery 的稳定指令头.
// 关键词: splitQueryStaticInstruction, SplitQuery semi-dynamic 段
var splitQueryStaticInstruction = promptloader.MustLoad("inline/ai/rag/enhancesearch/enhance/splitQueryStaticInstruction.txt")

// generalizeQueryStaticInstruction 是 GeneralizeQuery 的稳定指令头.
// 关键词: generalizeQueryStaticInstruction, GeneralizeQuery semi-dynamic 段
var generalizeQueryStaticInstruction = promptloader.MustLoad("inline/ai/rag/enhancesearch/enhance/generalizeQueryStaticInstruction.txt")

// ExtractKeywords 从问题中提取核心关键词，用于精确的词条搜索。
// 关键词: ExtractKeywords, LiteForgeStaticInstruction, P3-X1 静态下沉
func (h *LiteForgeSearchHandler) ExtractKeywords(ctx context.Context, query string) ([]string, error) {
	result, err := aicommon.InvokeLiteForge(
		query,
		aicommon.WithContext(ctx),
		aicommon.LiteForgeStaticInstruction(extractKeywordsStaticInstruction),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStringArrayParam(
				"search_keywords",
				aitool.WithParam_Description("从问题中提取的核心搜索关键词列表，用于精确的词条检索"),
			),
		),
		aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback()),
	)
	if err != nil {
		return nil, err
	}
	keywords := result.GetStringSlice("search_keywords")
	return keywords, nil
}

// HypotheticalAnswer 生成详细的假设回答，有助于搜索到更多相关结果.
// 关键词: HypotheticalAnswer, LiteForgeStaticInstruction, P3-X1 静态下沉
func (h *LiteForgeSearchHandler) HypotheticalAnswer(ctx context.Context, query string) (string, error) {
	result, err := aicommon.InvokeLiteForge(
		query,
		aicommon.WithContext(ctx),
		aicommon.LiteForgeStaticInstruction(hydeStaticInstruction),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStringParam(
				"hypothetical_answer",
				aitool.WithParam_Description("假设文档内容，搜索会使用假设文档作为 rag 搜索的查询内容"),
			),
		),
		aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback()),
	)
	if err != nil {
		return "", err
	}

	document_paragraph := result.GetString("hypothetical_answer")
	return document_paragraph, nil
}

// SplitQuery 将复杂问题拆分为多个子问题，有助于精确搜索多个领域的问题.
// 关键词: SplitQuery, LiteForgeStaticInstruction, P3-X1 静态下沉
func (h *LiteForgeSearchHandler) SplitQuery(ctx context.Context, query string) ([]string, error) {
	result, err := aicommon.InvokeLiteForge(
		query,
		aicommon.WithContext(ctx),
		aicommon.LiteForgeStaticInstruction(splitQueryStaticInstruction),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStringArrayParam(
				"sub_questions",
				aitool.WithParam_Description("拆分后的子问题列表，若无法拆分则返回原问题作为唯一子问题"),
			),
		),
		aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback()),
	)
	if err != nil {
		return nil, err
	}

	sub_questions := result.GetStringSlice("sub_questions")
	return sub_questions, nil
}

// GeneralizeQuery 把问题泛化，有助于扩大搜索范围.
// 关键词: GeneralizeQuery, LiteForgeStaticInstruction, P3-X1 静态下沉
func (h *LiteForgeSearchHandler) GeneralizeQuery(ctx context.Context, query string) ([]string, error) {
	result, err := aicommon.InvokeLiteForge(
		query,
		aicommon.WithContext(ctx),
		aicommon.LiteForgeStaticInstruction(generalizeQueryStaticInstruction),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStringArrayParam(
				"generalized_query",
				aitool.WithParam_Description("泛化后的主题级问题，若无法泛化则返回原问题"),
			),
		),
		aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback()),
	)
	if err != nil {
		return nil, err
	}

	generalized_query := result.GetStringSlice("generalized_query")
	return generalized_query, nil
}
