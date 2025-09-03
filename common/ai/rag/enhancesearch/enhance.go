package enhancesearch

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge/contracts"
	"github.com/yaklang/yaklang/common/utils"
)

// 需要调用之前注入Simpleliteforge
var Simpleliteforge contracts.LiteForge

// HypotheticalAnswer 生成详细的假设回答，有助于搜索到更多相关结果
func HypotheticalAnswer(ctx context.Context, query string) (string, error) {
	if Simpleliteforge == nil {
		return "", utils.Errorf("Simpleliteforge is not injected")
	}
	prompt := `请根据以下问题，撰写一个能够完美回答该问题的事实性文档段落。

要求：
1. 内容必须客观、准确、基于事实
2. 语言简洁明了，逻辑清晰
3. 避免主观判断和推测性表述
4. 如果涉及技术概念，请提供准确的定义和解释
5. 段落长度适中，信息密度合理
6. 使用陈述句，避免疑问句或感叹句
7. 如问题包含缩写/简称，请先还原为规范全称；必要时在首次出现处采用“全称（简称）”的表述，以确保理解与表达准确

问题: %s

请生成一个专业、权威的文档段落来回答上述问题。`
	prompt = fmt.Sprintf(prompt, query)

	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{aitool.WithStringParam("document_paragraph")},
	)
	if err != nil {
		return "", err
	}

	document_paragraph := result.GetString("document_paragraph")
	return document_paragraph, nil
}

// SplitQuery 将复杂问题拆分为多个子问题，有助于精确搜索多个领域的问题
func SplitQuery(ctx context.Context, query string) ([]string, error) {
	if Simpleliteforge == nil {
		return nil, utils.Errorf("Simpleliteforge is not injected")
	}
	prompt := `请将以下复杂问题拆分为多个子问题：

问题: %s

拆分规则：
1. 若原问题本身包含多个子问题或涉及多个领域/主题，请直接按这些自然边界进行拆分。
2. 若原问题为单一问题，请从不同维度/视角进行细化（如：原因、影响、原理、步骤、风险、对策、最佳实践、实例、边界条件、对比等），形成具代表性的子问题。
3. 子问题需相互独立、表述清晰，可直接用于检索或回答。
4. 若问题中出现缩写/简称，请优先使用规范全称，并在必要时在首次出现处采用“全称（简称）”的方式提升可读性。
`
	prompt = fmt.Sprintf(prompt, query)

	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{aitool.WithStringArrayParam("sub_questions")},
	)
	if err != nil {
		return nil, err
	}

	sub_questions := result.GetStringSlice("sub_questions")
	return sub_questions, nil
}

// GeneralizeQuery 把问题泛化，有助于扩大搜索范围
func GeneralizeQuery(ctx context.Context, query string) (string, error) {
	if Simpleliteforge == nil {
		return "", utils.Errorf("Simpleliteforge is not injected")
	}
	prompt := `请将以下问题泛化：

问题: %s

请生成一个泛化后的问题，问题需要详细准确专业。若存在缩写/简称，请先统一为规范全称；必要时在首次出现处使用“全称（简称）”的表述以提升清晰度。`
	prompt = fmt.Sprintf(prompt, query)

	inputPrompt := prompt
	result, err := Simpleliteforge.SimpleExecute(ctx,
		inputPrompt,
		[]aitool.ToolOption{aitool.WithStringParam("generalized_query")},
	)
	if err != nil {
		return "", err
	}

	generalized_query := result.GetString("generalized_query")
	return generalized_query, nil
}
