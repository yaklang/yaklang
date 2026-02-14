package aimem

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// TriageTagsFromText 从文本中生成标签以便搜索
func (r *AIMemoryTriage) SelectTags(ctx context.Context, i any) ([]string, error) {
	// summarize existing tags
	existed, err := r.GetDynamicContextWithTags()
	if err != nil {
		return nil, utils.Errorf("GetDynamicContextWithTags failed: %v", err)
	}

	prompt, err := utils.RenderTemplate(`
<|INPUT_{{ .Nonce }}|>
{{ .Input }}
<|INPUT_END_{{ .Nonce }}|>

<|TAGS_{{ .Nonce }}|>
{{ .Existed }}
<|TAGS_END_{{ .Nonce }}|>
`, map[string]any{
		"Nonce":   utils.RandStringBytes(4),
		"Existed": existed,
		"Input":   utils.InterfaceToString(i),
	})
	if err != nil {
		return nil, utils.Errorf("RenderTemplate failed: %v", err)
	}

	action, err := r.invoker.InvokeLiteForge(ctx, "tag-selection", prompt, []aitool.ToolOption{
		aitool.WithStringArrayParam("tags", aitool.WithParam_Description("从上面的输入中提取出相关的标签（领域），如果上面的标签已经足够了，就不需要再创建新的标签了")), // tags
	})
	if err != nil {
		return nil, err
	}
	tags := action.GetStringSlice("tags")
	if len(tags) > 0 {
		return tags, nil
	}
	return []string{}, nil
}
