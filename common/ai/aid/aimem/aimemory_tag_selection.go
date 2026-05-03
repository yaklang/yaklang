package aimem

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// tagSelectionStaticInstruction 是 SelectTags 的系统侧静态指令
// 通过 aicommon.WithLiteForgeStaticInstruction 传入 LiteForge，进入 high-static 段，跨调用稳定哈希
// 关键词: aicache, PROMPT_SECTION, StaticInstruction, tag-selection, B 档
const tagSelectionStaticInstruction = `你是一个标签选择助手，负责从输入文本中提取领域标签。

任务说明：
1. 阅读 <input> 中的文本，识别其涉及的领域、技术栈、主题。
2. 参考 <existing_tags> 中已有的标签集合，优先复用已有标签，避免重复创建语义相同但名称不同的新标签。
3. 如果已有标签已经能够完整描述输入文本，则不需要创建新标签。
4. 输出选中的标签列表，每个标签使用具体的领域术语（如 "sql-injection"、"jwt"、"php-security" 等），避免过于宽泛。`

// SelectTags 从文本中生成标签以便搜索
// B 档改造：去掉内层 INPUT/TAGS nonce；通过 StaticInstruction 把任务说明送入 high-static 段
// 关键词: aicache, PROMPT_SECTION, SelectTags, B 档去 nonce
func (r *AIMemoryTriage) SelectTags(ctx context.Context, i any) ([]string, error) {
	// summarize existing tags
	existed, err := r.GetDynamicContextWithTags()
	if err != nil {
		return nil, utils.Errorf("GetDynamicContextWithTags failed: %v", err)
	}

	// 关键词: aicache, dynamic, tag-selection, 去内层 nonce
	// 内层 <input>/<existing_tags> 不再带 nonce，外层 PROMPT_SECTION_dynamic_NONCE 已防 prompt-injection
	prompt, err := utils.RenderTemplate(`<input>
{{ .Input }}
</input>

<existing_tags>
{{ .Existed }}
</existing_tags>
`, map[string]any{
		"Existed": existed,
		"Input":   utils.InterfaceToString(i),
	})
	if err != nil {
		return nil, utils.Errorf("RenderTemplate failed: %v", err)
	}

	action, err := r.invoker.InvokeSpeedPriorityLiteForge(ctx, "tag-selection", prompt, []aitool.ToolOption{
		aitool.WithStringArrayParam("tags", aitool.WithParam_Description("从上面的输入中提取出相关的标签（领域），如果上面的标签已经足够了，就不需要再创建新的标签了")), // tags
	}, aicommon.WithLiteForgeStaticInstruction(tagSelectionStaticInstruction))
	if err != nil {
		return nil, err
	}
	tags := action.GetStringSlice("tags")
	if len(tags) > 0 {
		return tags, nil
	}
	return []string{}, nil
}
