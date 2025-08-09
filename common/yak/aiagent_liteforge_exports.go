package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

/*
TDD:

action = liteforge.Execute(<<<PROMPT

PROMPT, liteforge.output(jsonschema.ActionObject(
	jsonschema.paramString("key", jsonschema.description("The query to execute in the LiteForge context")),
)), liteforge.context(ctx))~
action.Get("obj")
*/

var LiteForgeExport = map[string]interface{}{
	"Execute": _withOutputJSONSchema,

	"output": _withOutputJSONSchema,
	"action": _withOutputAction,
	"id": func(id string) liteForgeOption {
		return func(cfg *liteforgeConfig) {
			cfg.id = id
		}
	},
	"context": func(ctx context.Context) liteForgeOption {
		return func(cfg *liteforgeConfig) {
			cfg.ctx = ctx
		}
	},
	"verboseName": func(opts ...aid.Option) liteForgeOption {
		return func(cfg *liteforgeConfig) {
			cfg.aidOptions = append(cfg.aidOptions, opts...)
		}
	},
}

type liteforgeConfig struct {
	query       string
	output      string
	action      string
	id          string
	verboseName string
	ctx         context.Context

	aidOptions []aid.Option
}

type liteForgeOption func(*liteforgeConfig)

func _withOutputJSONSchema(output string) liteForgeOption {
	return func(cfg *liteforgeConfig) {
		cfg.output = output
	}
}

func _withOutputAction(action string) liteForgeOption {
	return func(cfg *liteforgeConfig) {
		cfg.action = action
	}
}

// liteforge.Execute can create a temporary LiteForge instance and execute it with the given query.
// Example:
// ```
// result = liteforge.Execute(<<<PROMPT
// PROMPT, liteforge.output(jsonschema.ActionObject(jsonschema.paramString("value"))),
// ```
func _executeLiteForgeTemp(query string, opts ...any) (*aiforge.ForgeResult, error) {
	cfg := &liteforgeConfig{
		query:  query,
		action: "object",
	}
	for _, optRaw := range opts {
		switch opt := optRaw.(type) {
		case liteForgeOption:
			opt(cfg)
		case aid.Option:
			cfg.aidOptions = append(cfg.aidOptions, opt)
		}
	}

	if utils.InterfaceToString(jsonpath.Find(cfg.output, "$..properties..const")) != cfg.action {
		return nil, utils.Errorf("jsonschema output must have '@action' - const value '%s', lite: ..."+`.."@action": {"const": "`+cfg.action+`"}`+"...", cfg.action)
	}

	liteforgeIns, err := aiforge.NewLiteForge(cfg.id, aiforge.WithLiteForge_OutputSchemaRaw(cfg.action, cfg.output))
	if err != nil {
		return nil, utils.Errorf("new liteforge failed: %s", err)
	}
	fr, err := liteforgeIns.Execute(cfg.ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: cfg.query},
	}, cfg.aidOptions...)
	if err != nil {
		return nil, utils.Errorf("execute liteforge failed: %s", err)
	}
	if fr == nil {
		return nil, utils.Errorf("execute liteforge result is nil")
	}
	return fr, nil
}
