package enhancesearch

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge/contracts"
	"github.com/yaklang/yaklang/common/utils"
)

// 需要调用之前注入Simpleliteforge
var Simpleliteforge contracts.LiteForge

// ExtractKeywords 从问题中提取核心关键词，用于精确的词条搜索。
func ExtractKeywords(ctx context.Context, query string) ([]string, error) {
	if Simpleliteforge == nil {
		return nil, utils.Errorf("Simpleliteforge is not injected")
	}
	prompt := `# 角色
你是一位精通各领域知识的首席信息检索专家（Chief Information Retrieval Expert）。
# 任务
你的任务是分析用户的【问题】，并从中提炼出一组约10个**高价值、高信噪比**的核心搜索关键词。这组关键词将用于精准的、基于词条的检索引擎，目的是快速定位到最相关的专业文档。
# 行动准则
**1. 核心实体优先 (Core Entity First):**
    -   首先识别并提取问题中最重要的**核心实体、技术、专有名词、产品名或人名**。这是关键词列表的基石。
**2. 关联与扩展 (Association & Expansion):**
    -   不要局限于问题中的字面词汇。基于核心实体，扩展出与之紧密相关的**技术标准、替代品、核心组件或相关概念**。
    -   *示例1:*  问题含 'gRPC'，可扩展出 'Protobuf', 'HTTP/2', '远程过程调用'。
    -   *示例2:*  问题含 'React状态管理'，可扩展出 'Redux', 'MobX', 'useState', 'Context API'。
**3. 标准化与规范 (Normalization & Canonization):**
    -   提取概念时，应同时包含其**常用缩写和完整全称**，因为两者都可能作为索引词。
    -   *示例:*  问题含 'CSRF'，关键词中应包含 'CSRF' 和 'Cross-Site Request Forgery'。
**4. 高信噪比原则 (High Signal-to-Noise Ratio):**
    -   **必须过滤掉**所有对精确搜索无意义的词：
        -   疑问词：如何、什么、为什么、哪里
        -   停用词：的、和、一个、关于、在
        -   模糊描述：最佳实践、优缺点、性能差异（应提取具体比较对象，而非这些描述词本身）
**5. 质量与数量 (Quality & Quantity):**
    -   生成 **8-12个** 最具代表性的关键词。
    -   **质量远比数量重要**。如果一个简单问题只能提炼出3-4个高质量关键词，这是完全可以接受的。
---
<|问题_{{ .nonce }}_START|>
{{ .query }}
<|问题_{{ .nonce }}_END|>
---
请立即开始分析，并生成最终的关键词列表。`
	prompt, err := utils.RenderTemplate(prompt, map[string]any{
		"nonce": utils.RandStringBytes(4),
		"query": query,
	})
	if err != nil {
		return nil, err
	}
	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam(
				"search_keywords",
				aitool.WithParam_Description("从问题中提取的核心搜索关键词列表，用于精确的词条检索"),
			),
		},
	)
	if err != nil {
		return nil, err
	}
	keywords := result.GetStringSlice("search_keywords")
	return keywords, nil
}

// HypotheticalAnswer 生成详细的假设回答，有助于搜索到更多相关结果
func HypotheticalAnswer(ctx context.Context, query string) (string, error) {
	if Simpleliteforge == nil {
		return "", utils.Errorf("Simpleliteforge is not injected")
	}
	prompt, err := utils.RenderTemplate(`
你是一个精通信息检索的AI助手。
# 任务
根据用户提出的【问题】，精准地提炼出其核心概念，并生成一段**信息密度极高**的“理想答案摘要”。这段摘要将作为搜索引擎的最佳查询依据，以找到最相关的知识。
# 核心指令
1.  **高度浓缩**: 抛弃所有不必要的背景描述和过渡性语言，直接切入主题。
2.  **关键词聚焦**: 围绕问题的核心实体、技术原理、关键特性和应用场景，组合成一个连贯的句子或短段落。
3.  **全称优先**: 如果问题中包含缩写，应在摘要中包含其全称，以增加检索覆盖面。
4.  **严格简短**: **最终输出严格控制在100字以内。**
---
<|问题_{{ .nonce }}_START|>
{{ .query }}
<|问题_{{ .nonce }}_END|>
---
请立即开始生成这段高度浓缩的“理想答案摘要”。
`, map[string]any{
		"nonce": utils.RandStringBytes(4),
		"query": query,
	})
	if err != nil {
		return "", err
	}

	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{
			aitool.WithStringParam(
				"hypothetical_answer",
				aitool.WithParam_Description("假设文档内容，搜索会使用假设文档作为 rag 搜索的查询内容"),
			),
		},
	)
	if err != nil {
		return "", err
	}

	document_paragraph := result.GetString("hypothetical_answer")
	return document_paragraph, nil
}

// SplitQuery 将复杂问题拆分为多个子问题，有助于精确搜索多个领域的问题
func SplitQuery(ctx context.Context, query string) ([]string, error) {
	if Simpleliteforge == nil {
		return nil, utils.Errorf("Simpleliteforge is not injected")
	}
	prompt := `# 角色
你是一位顶级的首席信息分析师和搜索策略师。
# 任务
你的核心任务是将一个复杂的探寻，分解为一系列可以**并行执行**、**精准检索**的独立子问题，以实现最高效、最全面的信息获取。
# 行动准则
**1.  守真原则 (Principle of Fidelity) [最高优先级]:**
    -   如果【问题】本身已经是一个清晰、原子化且可直接检索的简单问题（例如：“什么是Transformer模型？”），则**不应进行拆分**。直接将原问题作为唯一的子问题返回。
**2.  拆分策略 (Decomposition Strategy):**
    -   **多主题拆分:** 如果【问题】明显包含多个领域或主题（例如：“比较一下React和Vue的优缺点，并分析它们在移动端的应用场景”），请按这些自然边界进行拆分。
    -   **多维度拆分:** 如果【问题】是单一主题的复杂问题（例如：“分析一下大型语言模型（LLM）的风险”），请从不同维度进行深入细化，形成有代表性的子问题。**强烈建议参考以下维度：**
        -   核心定义 (What): “什么是大型语言模型（LLM）？”
        -   工作原理 (How): “大型语言模型（LLM）的工作原理是什么？”
        -   关键风险 (Risk): “使用大型语言模型（LLM）存在哪些主要风险？”
        -   应对策略 (Solution): “如何缓解或管理大型语言模型（LLM）的风险？”
        -   实际案例 (Example): “有哪些关于大型语言模型（LLM）风险的真实案例？”
        -   未来趋势 (Trend): “大型语言模型（LLM）风险未来的发展趋势是什么？”
**3.  质量与数量 (Quality & Quantity):**
    -   **质量优先:** 追求子问题的**实质性价值**，而非数量。每个子问题都应是一个有意义的、值得独立检索的探寻点。
    -   **数量适中:** 理想的子问题数量在 **2-5 个**之间。避免生成过于琐碎或高度重叠的子问题。
**4.  格式规范:**
    -   子问题必须是完整、清晰的句子。
    -   如果涉及缩写，应优先使用全称，或在首次出现时采用“**全称（简称）**”的格式。
---
<|问题_{{ .nonce }}_START|>
{{ .query }}
<|问题_{{ .nonce }}_END|>
---
请根据上述准则，开始你的分析和拆分工作。
`
	prompt, err := utils.RenderTemplate(prompt, map[string]any{
		"nonce": utils.RandStringBytes(4),
		"query": query,
	})

	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam(
				"sub_questions",
				aitool.WithParam_Description("拆分后的子问题列表，若无法拆分则返回原问题作为唯一子问题"),
			),
		},
	)
	if err != nil {
		return nil, err
	}

	sub_questions := result.GetStringSlice("sub_questions")
	return sub_questions, nil
}

// GeneralizeQuery 把问题泛化，有助于扩大搜索范围
func GeneralizeQuery(ctx context.Context, query string) ([]string, error) {
	if Simpleliteforge == nil {
		return nil, utils.Errorf("Simpleliteforge is not injected")
	}
	prompt := `# 角色
你是一位专业的知识架构师和信息检索策略师。
# 任务
你的任务是将一个具体、细节性的【问题】，通过“**概念升维**”操作，转化为多个更具概括性的主题级问题。
战略目标是：打破原问题的关键词限制，从而能检索到关于该主题的**宏观论述、原理介绍、领域综述或对比分析**类的文档，为用户提供更广阔的视角。
# 行动准则
**1.  天花板原则 (Ceiling Principle) [最高优先级]:**
    -   如果【问题】本身已经是一个广泛、抽象的主题（例如：“什么是计算机科学？”），它已经触及了泛化的“天花板”，则**无需泛化**。在这种情况下，直接返回原问题。
**2.  概念升维操作 (Conceptual Ascension):**
    -   **第一步：识别核心。** 找出【问题】中的核心实体、技术或概念。
    -   **第二步：升维一级。** 确定该核心概念的**直接上层类别**或所属的宏观主题。
        -   *示例：* 'gRPC' 的上层是 '远程过程调用（RPC）框架'；'React useState Hook' 的上层是 'React状态管理'。
    -   **第三步：重构问题。** 围绕这个更高维度的“上层类别”重新构建一个完整、专业的问题。
**3.  质量标准:**
    -   泛化后的问题必须逻辑清晰，仍然是一个有意义的、可被回答的问题。
    -   如果涉及缩写，应优先使用全称，或在首次出现时采用“**全称（简称）**”的格式。
---
<|问题_{{ .nonce }}_START|>
{{ .query }}
<|问题_{{ .nonce }}_END|>
---
请以知识架构师的身份，执行“概念升维”任务。`
	prompt, err := utils.RenderTemplate(prompt, map[string]any{
		"nonce": utils.RandStringBytes(4),
		"query": query,
	})
	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam(
				"generalized_query",
				aitool.WithParam_Description("泛化后的主题级问题，若无法泛化则返回原问题"),
			),
		},
	)
	if err != nil {
		return nil, err
	}

	generalized_query := result.GetStringSlice("generalized_query")
	return generalized_query, nil
}
