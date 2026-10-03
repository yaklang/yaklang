package liteforge

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func init() {
	aispec.RegisterStructuredOutputExecutor(ExecuteFunctionCall)
}

// ExecuteFunctionCall adapts ai.FunctionCall's field map to the same streaming
// executor as public LiteForge. The caller retains provider/model selection.
func ExecuteFunctionCall(input string, fields map[string]any, chat aispec.GeneralChatter, opts ...aispec.AIConfigOption) (map[string]any, error) {
	if len(fields) == 0 {
		return nil, fmt.Errorf("no fields configured for structured output")
	}
	if chat == nil {
		return nil, fmt.Errorf("structured output requires a model callback")
	}
	schema := fields
	if fields["type"] != "object" || fields["properties"] == nil {
		properties, required := make(map[string]any, len(fields)), make([]string, 0, len(fields))
		for name, field := range fields {
			property := map[string]any{}
			switch value := field.(type) {
			case string:
				property["description"] = value
			case map[string]any:
				property = value
			default:
				property["description"] = fmt.Sprintf("输出字段，声明类型为 %T", field)
			}
			properties[name] = property
			required = append(required, name)
		}
		// Stable required ordering keeps the function/schema prefix reusable.
		sort.Strings(required)
		schema = map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": true}
	}
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	config := aispec.NewDefaultAIConfig(opts...)
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}
	options := []aicommon.ConfigOption{aicommon.WithFastAICallback(aicommon.AIChatToAICallbackType(chat)),
		aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(int64(config.FunctionCallRetryTimes))}
	if config.FunctionCallMode != nil {
		options = append(options, aicommon.WithEnableFunctionCallMode(*config.FunctionCallMode))
	}
	result, err := Execute(ctx, Request{Name: "ai.FunctionCall", ActionName: "object", Schema: string(encoded), Prompt: input}, options...)
	if err != nil {
		return nil, err
	}
	value := result.Action.GetParams()
	delete(value, "@action")
	return value, nil
}
