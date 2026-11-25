package yak

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

// bashCommandGuard is the bash tool's own preflight deny hook.
// Returning nil means continue; a map is the semantic RESULT for a hard deny.
func bashCommandGuard(ctx context.Context, disable bool, command string) map[string]any {
	if disable {
		return nil
	}
	cmd := strings.TrimSpace(utils.InterfaceToString(command))
	if cmd == "" {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	check := aicommon.CheckBashCommand(ctx, cmd,
		aicommon.WithDisableBuiltinBashRules(false),
		aicommon.WithIncludeCustomBashRules(true),
	)
	if check.Blocked() {
		return check.BlockedResult(aitool.BashToolName)
	}
	return nil
}

// exportedBashCommandGuard is the default aiagent.BashCommandGuard for scripts
// that are not running under the AI tool host callback.
func exportedBashCommandGuard(command string) map[string]any {
	return bashCommandGuard(context.Background(), false, command)
}

// bindBashCommandGuard installs a per-invocation guard that honors the runtime
// master switch and the call's cancel context.
func bindBashCommandGuard(engine *antlr4yak.Engine, ctx context.Context, runtimeConfig *aitool.ToolRuntimeConfig) {
	if engine == nil {
		return
	}
	disable := runtimeConfig != nil && runtimeConfig.DisableBuiltinSyntaxFlowRules
	guard := func(command string) map[string]any {
		return bashCommandGuard(ctx, disable, command)
	}
	exports := make(map[string]any)
	if value, ok := engine.GetVar("aiagent"); ok {
		if original, ok := value.(map[string]any); ok {
			for name, value := range original {
				exports[name] = value
			}
		}
	}
	exports["BashCommandGuard"] = guard
	engine.SetVars(map[string]any{"aiagent": exports})
}
