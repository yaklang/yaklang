package generate_index_tool

import (
	"context"
	"fmt"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge/contracts"
)

//go:embed prompt/init.txt
var defaultProcessPrompt string

// AIContentProcessor AI内容处理器
type AIContentProcessor struct {
	prompt    string
	liteForge contracts.LiteForge
}

// NewAIContentProcessor 创建AI内容处理器
func NewAIContentProcessor(liteForge contracts.LiteForge, customPrompt ...string) *AIContentProcessor {
	prompt := defaultProcessPrompt
	if len(customPrompt) > 0 && customPrompt[0] != "" {
		prompt = customPrompt[0]
	}

	return &AIContentProcessor{
		prompt:    prompt,
		liteForge: liteForge,
	}
}

// ProcessContent 处理原始内容，返回清洗后的内容
func (p *AIContentProcessor) ProcessContent(ctx context.Context, rawContent string) (string, error) {
	if p.liteForge == nil {
		return "", fmt.Errorf("liteForge is not initialized")
	}

	// 构建工具选项
	toolOptions := []aitool.ToolOption{
		aitool.WithStringParam("language", aitool.WithParam_Required(true), aitool.WithParam_Description("语言，固定为chinese")),
		aitool.WithStringParam("description", aitool.WithParam_Required(true), aitool.WithParam_Description("内容功能描述")),
		aitool.WithStringArrayParam("keywords", aitool.WithParam_Required(true), aitool.WithParam_Description("关键词数组")),
	}

	// 使用 LiteForge 接口执行
	params, err := p.liteForge.SimpleExecute(ctx, fmt.Sprintf("%s\n\n内容: %s", p.prompt, rawContent), toolOptions)
	if err != nil {
		return "", err
	}

	// 提取结果
	description := params.GetString("description")
	keywords := params.GetStringSlice("keywords")

	// 组合描述和关键词作为处理后的内容
	processedContent := fmt.Sprintf("描述: %s\n关键词: %v", description, keywords)

	return processedContent, nil
}

// SimpleContentProcessor 简单内容处理器（不使用AI）
type SimpleContentProcessor struct{}

// NewSimpleContentProcessor 创建简单内容处理器
func NewSimpleContentProcessor() *SimpleContentProcessor {
	return &SimpleContentProcessor{}
}

// ProcessContent 简单处理内容（直接返回原内容）
func (p *SimpleContentProcessor) ProcessContent(ctx context.Context, rawContent string) (string, error) {
	return rawContent, nil
}
