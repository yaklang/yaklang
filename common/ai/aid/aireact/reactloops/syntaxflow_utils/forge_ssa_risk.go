package syntaxflow_utils

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

// ForgeSSARiskIDFromUserInput extracts a numeric SSA risk primary key from free-form user text via LiteForge.
func ForgeSSARiskIDFromUserInput(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string) (id int64, reason string, err error) {
	if r == nil {
		return 0, "", utils.Error("nil invoker")
	}
	userInput = strings.TrimSpace(userInput)
	if userInput == "" {
		return 0, "", nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rendered, err := utils.RenderTemplate(`Extract SSA Risk primary key (database row id) if present.

## User message
<|USER_INPUT_{{ .Nonce }}|>
{{ .UserInput }}
<|USER_INPUT_END_{{ .Nonce }}|>

Rules:
1. risk_id must be a positive decimal integer referring to SSARisk.id.
2. Return risk_id=0 if none is clearly stated.
3. Do not guess.`,
		map[string]any{
			"Nonce":     utils.RandStringBytes(4),
			"UserInput": userInput,
		})
	if err != nil {
		return 0, "", utils.Wrap(err, "render forge prompt")
	}

	res, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"extract-ssa-risk-id",
		rendered,
		[]aitool.ToolOption{
			aitool.WithIntegerParam("risk_id",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Positive SSA risk primary key, or 0")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("Brief justification")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "reason"),
	)
	if err != nil {
		return 0, "", err
	}
	id = int64(res.GetInt("risk_id"))
	reason = strings.TrimSpace(res.GetString("reason"))
	if id <= 0 {
		return 0, reason, nil
	}
	return id, reason, nil
}
