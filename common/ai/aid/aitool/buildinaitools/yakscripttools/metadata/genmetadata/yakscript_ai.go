package genmetadata

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const defaultYakScriptAIMetadataTimeout = 45 * time.Second

type YakScriptAIFields struct {
	Description string
	Keywords    []string
	Usage       string
}

func ShouldGenerateYakScriptAIFields(script *schema.YakScript) bool {
	if script == nil {
		return false
	}
	if !supportsYakScriptAIType(script.Type) {
		return false
	}
	if !script.EnableForAI {
		return false
	}
	return script.AIDesc == "" || script.AIKeywords == "" || script.AIUsage == ""
}

func CompleteYakScriptAIFields(ctx context.Context, script *schema.YakScript, opts ...any) error {
	if script == nil {
		return utils.Error("nil YakScript")
	}
	if !script.EnableForAI {
		return nil
	}

	applyYakScriptEmbeddedMetadata(script)
	if !ShouldGenerateYakScriptAIFields(script) {
		return nil
	}

	fields, err := GenerateYakScriptAIFields(ctx, script, opts...)
	if err != nil {
		return err
	}

	if script.AIDesc == "" {
		script.AIDesc = strings.TrimSpace(fields.Description)
	}
	if script.AIKeywords == "" && len(fields.Keywords) > 0 {
		script.AIKeywords = strings.Join(cleanYakScriptAIKeywords(fields.Keywords), ",")
	}
	if script.AIUsage == "" {
		script.AIUsage = strings.TrimSpace(fields.Usage)
	}
	return nil
}

func GenerateYakScriptAIFields(ctx context.Context, script *schema.YakScript, opts ...any) (*YakScriptAIFields, error) {
	if script == nil {
		return nil, utils.Error("nil YakScript")
	}
	if !supportsYakScriptAIType(script.Type) {
		return nil, utils.Errorf("unsupported YakScript type %q for AI metadata generation", script.Type)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultYakScriptAIMetadataTimeout)
		defer cancel()
	}

	liteforgeOpts := []any{
		aicommon.WithContext(ctx),
		aicommon.WithLiteForgeOutputSchemaFromAIToolOptions(
			aitool.WithStringParam("ai_desc",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("A concise description of what this YakScript does"),
			),
			aitool.WithStringArrayParam("ai_keywords",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Keywords for AI retrieval, including technical terms or aliases when helpful"),
			),
			aitool.WithStringParam("ai_usage",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Usage guidance for AI agents, including when to use it, important parameters, and expected output"),
			),
		),
		aicommon.WithAICallback(aicommon.MustGetSpeedPriorityAIModelCallback()),
	}
	liteforgeOpts = append(liteforgeOpts, opts...)

	result, err := aicommon.InvokeLiteForge(generateYakScriptAIPrompt(script), liteforgeOpts...)
	if err != nil {
		return nil, utils.Errorf("invoke liteforge failed: %v", err)
	}

	fields := &YakScriptAIFields{
		Description: strings.TrimSpace(result.GetString("ai_desc")),
		Keywords:    cleanYakScriptAIKeywords(result.GetStringSlice("ai_keywords")),
		Usage:       strings.TrimSpace(result.GetString("ai_usage")),
	}
	if fields.Description == "" {
		return nil, utils.Error("ai_desc is empty")
	}
	if len(fields.Keywords) == 0 {
		return nil, utils.Error("ai_keywords is empty")
	}
	if fields.Usage == "" {
		return nil, utils.Error("ai_usage is empty")
	}
	return fields, nil
}

func applyYakScriptEmbeddedMetadata(script *schema.YakScript) {
	if script == nil || script.Content == "" {
		return
	}
	if !script.EnableForAI {
		return
	}

	meta, err := metadata.ParseYakScriptMetadata(script.ScriptName, script.Content)
	if err != nil || meta == nil {
		return
	}

	if script.AIDesc == "" {
		script.AIDesc = strings.TrimSpace(meta.Description)
	}
	if script.AIKeywords == "" && len(meta.Keywords) > 0 {
		script.AIKeywords = strings.Join(cleanYakScriptAIKeywords(meta.Keywords), ",")
	}
	if script.AIUsage == "" {
		script.AIUsage = strings.TrimSpace(meta.Usage)
	}
}

func cleanYakScriptAIKeywords(keywords []string) []string {
	var results []string
	seen := make(map[string]struct{})
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		dedupKey := strings.ToLower(keyword)
		if _, ok := seen[dedupKey]; ok {
			continue
		}
		seen[dedupKey] = struct{}{}
		results = append(results, keyword)
	}
	return results
}

func supportsYakScriptAIType(typ string) bool {
	switch typ {
	case "yak", "mitm", "port-scan":
		return true
	default:
		return false
	}
}

func generateYakScriptAIPrompt(script *schema.YakScript) string {
	var prompt strings.Builder

	prompt.WriteString("你是 YakScript 的 AI 元数据生成器。\n")
	prompt.WriteString("请只根据插件的真实功能、输入输出和适用场景，生成适合 AI 检索与调用的元数据。\n\n")
	prompt.WriteString("输出要求：\n")
	prompt.WriteString("1. 当前插件已经明确启用了 enable_for_ai，请只补充 ai_desc、ai_keywords、ai_usage。\n")
	prompt.WriteString("2. ai_desc: 1-2 句简洁说明插件做什么，不要复述实现细节。\n")
	prompt.WriteString("3. ai_keywords: 5-12 个关键词，优先保留检索价值高的中文/英文技术词，去重。\n")
	prompt.WriteString("4. ai_usage: 面向 AI 的使用说明，说明何时使用、关键参数、预期产出，保持简洁实用。\n")
	prompt.WriteString("5. 忽略夸张注释，优先依据代码行为、参数、help、tags 进行判断。\n\n")

	prompt.WriteString(fmt.Sprintf("ScriptName: %s\n", script.ScriptName))
	prompt.WriteString(fmt.Sprintf("Type: %s\n", script.Type))
	if help := strings.TrimSpace(script.Help); help != "" {
		prompt.WriteString(fmt.Sprintf("Help: %s\n", help))
	}
	if tags := strings.TrimSpace(script.Tags); tags != "" {
		prompt.WriteString(fmt.Sprintf("Tags: %s\n", tags))
	}

	prompt.WriteString("Params:\n")
	prompt.WriteString(normalizeYakScriptJSONPayload(script.Params))
	prompt.WriteString("\n\nCode:\n```yak\n")
	prompt.WriteString(utils.ShrinkString(script.Content, 16000))
	prompt.WriteString("\n```")

	return prompt.String()
}

func normalizeYakScriptJSONPayload(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "[]"
	}
	if unquoted, err := strconv.Unquote(raw); err == nil && strings.TrimSpace(unquoted) != "" {
		return utils.ShrinkString(unquoted, 4000)
	}
	return utils.ShrinkString(raw, 4000)
}
