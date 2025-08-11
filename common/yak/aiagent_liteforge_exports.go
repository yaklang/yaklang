package yak

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/imageutils"
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
	"Execute": _executeLiteForgeTemp,

	"output":    _withOutputJSONSchema,
	"action":    _withOutputAction,
	"image":     _withImage,
	"imageFile": _withImageFile,
	"id": func(id string) liteForgeOption {
		return func(cfg *liteforgeConfig) {
			cfg.id = id
		}
	},
	"context": _withLiteForgeCtx,
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
	images      []*aid.ImageData

	aidOptions []aid.Option
}

type liteForgeOption func(*liteforgeConfig)

func _withLiteForgeCtx(ctx context.Context) liteForgeOption {
	return func(cfg *liteforgeConfig) {
		cfg.ctx = ctx
	}
}

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

func _withImageFile(filename ...string) liteForgeOption {
	return func(cfg *liteforgeConfig) {
		for _, file := range filename {
			imgC, err := imageutils.ExtractImageFromFile(file)
			if err != nil {
				utils.Errorf("extract image from file %s failed: %s", file, err)
				continue
			}
			for img := range imgC {
				log.Info("Extracted image from file:", file, "MIMEType:", img.MIMEType)
				cfg.images = append(cfg.images, &aid.ImageData{
					IsBase64: true,
					Data:     []byte(img.Base64()),
				})
			}
		}
	}
}

func _withImage(anyImageInput ...any) liteForgeOption {
	return func(cfg *liteforgeConfig) {
		for _, anyImg := range anyImageInput {
			for img := range imageutils.ExtractImage(anyImg) {
				cfg.images = append(cfg.images, &aid.ImageData{
					IsBase64: true,
					Data:     []byte(img.Base64()),
				})
			}
		}
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
		id:     "temporary-liteforge",
		ctx:    context.Background(),
	}
	for _, optRaw := range opts {
		switch opt := optRaw.(type) {
		case liteForgeOption:
			opt(cfg)
		case aid.Option:
			cfg.aidOptions = append(cfg.aidOptions, opt)
		}
	}

	if ret := utils.InterfaceToString(jsonpath.FindFirst(cfg.output, "$..properties..const")); ret != cfg.action {
		return nil, utils.Errorf("jsonschema output must have '@action' - const value '%s', lite: ..."+`.."@action": {"const": "`+cfg.action+`"}`+"..., found: %v, expect: %v", cfg.action, ret, cfg.action)
	}

	liteforgeIns, err := aiforge.NewLiteForge(cfg.id, aiforge.WithLiteForge_OutputSchemaRaw(cfg.action, cfg.output))
	if err != nil {
		return nil, utils.Errorf("new liteforge failed: %s", err)
	}
	fr, err := liteforgeIns.ExecuteEx(cfg.ctx, []*ypb.ExecParamItem{
		{Key: "query", Value: cfg.query},
	}, cfg.images, cfg.aidOptions...)
	if err != nil {
		return nil, utils.Errorf("execute liteforge failed: %s", err)
	}
	if fr == nil {
		return nil, utils.Errorf("execute liteforge result is nil")
	}
	return fr, nil
}
