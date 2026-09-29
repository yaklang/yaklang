package aimem

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// tagSelectionStaticInstruction 是 SelectTags 的系统侧静态指令
// 通过 aicommon.WithLiteForgeStaticInstruction 传入 LiteForge，进入 high-static 段，跨调用稳定哈希
// 关键词: aicache, PROMPT_SECTION, StaticInstruction, tag-selection, B 档
var tagSelectionStaticInstruction = promptloader.MustLoad("inline/ai/aid/aimem/aimemory_tag_selection/tagSelectionStaticInstruction.txt")

// SelectTags 从文本中生成标签以便搜索
// B 档改造：去掉内层 INPUT/TAGS nonce；通过 StaticInstruction 把任务说明送入 high-static 段
// 关键词: aicache, PROMPT_SECTION, SelectTags, B 档去 nonce
func (r *AIMemoryTriage) SelectTags(ctx context.Context, i any) (tags []string, err error) {
	err = utils.Error("tag selection auxiliary task returned no action")
	r.invoker.GetConfig().ScheduleAuxiliaryTask(ctx,
		aicommon.CallerLabelTextagSelection,
		func() string {
			var prompt string
			prompt, err = r.buildTagSelectionPrompt(i)
			return prompt
		},
		func(action *aicommon.Action) {
			tags = action.GetStringSlice("tags")
			if tags == nil {
				tags = []string{}
			}
			err = nil
		},
		aicommon.WithAuxiliaryOnError(func(cause error) { err = cause }),
		aicommon.WithAuxiliaryOutputs(
			aitool.WithStringArrayParam("tags", aitool.WithParam_Description("从上面的输入中提取出相关的标签（领域），如果上面的标签已经足够了，就不需要再创建新的标签了")),
		),
		aicommon.WithAuxiliaryOpts(aicommon.WithLiteForgeStaticInstruction(tagSelectionStaticInstruction)),
	)
	return tags, err
}

func (r *AIMemoryTriage) buildTagSelectionPrompt(i any) (string, error) {
	// summarize existing tags
	existed, err := r.GetDynamicContextWithTags()
	if err != nil {
		return "", utils.Errorf("GetDynamicContextWithTags failed: %v", err)
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
		return "", utils.Errorf("RenderTemplate failed: %v", err)
	}

	return prompt, nil
}
