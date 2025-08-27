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

请生成多个子问题，每个子问题应该某个维度视角的代表性问题，并且提问需要详细准确专业。
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

请生成一个泛化后的问题，问题需要详细准确专业。`
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
