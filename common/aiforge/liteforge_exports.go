package aiforge

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/imageutils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

/*
TDD:

action = liteforge.Execute(<<<PROMPT

PROMPT, liteforge.output(jsonschema.ActionObject
	jsonschema.paramString("key", jsonschema.description("The query to execute in the LiteForge context")),
)), liteforge.context(ctx))~
action.Get("obj")
*/

var LiteForgeExport = map[string]interface{}{
	"Execute":          _executeLiteForgeTemp,
	"AnalyzeImage":     AnalyzeImage,
	"AnalyzeImageFile": AnalyzeImageFile,
	"AnalyzeAudioFile": AnalyzeAudioFile,
	"imageExtraPrompt": ImageWithExtraPrompt, // use for analyzeImage and analyzeImageFile

	"output":      _withOutputJSONSchema,
	"action":      _withOutputAction,
	"image":       _withImage,
	"imageFile":   _withImageFile,
	"id":          _withID,
	"context":     LiteForgeExecWithContext,
	"verboseName": _withVerboseName,
	"forceImage":  _withForceImage,
}

type liteforgeConfig struct {
	query      string
	output     string
	action     string
	id         string
	ctx        context.Context
	images     []*aicommon.ImageData
	forceImage bool

	aidOptions []aid.Option
}

type LiteForgeExecOption func(*liteforgeConfig)

// liteforge.output is an option for liteforge.Execute
// it can limit the output of the liteforge.Execute
// use `jsonschema.ActionObject` to limit the output to an object
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENTN
// PROMPT, liteforge.output(jsonschema.ActionObject(jsonschema.paramString("value"))),
// ```
func _withOutputJSONSchema(output string) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.output = output
	}
}

// liteforge.forceImage is an option for liteforge.Execute
// it forces the execution to require image input
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.forceImage(true))
// ```
func _withForceImage(force ...bool) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		if len(force) > 0 {
			cfg.forceImage = force[0]
		} else {
			cfg.forceImage = true
		}
	}
}

// liteforge.action is an option for liteforge.Execute
// it sets the action type for the liteforge execution,
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.action("analyze"))
// ```
func _withOutputAction(action string) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.action = action
	}
}

// liteforge.imageFile is an option for liteforge.Execute
// it adds image files to the execution context
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.imageFile("path/to/image.jpg"))
// ```
func _withImageFile(filename ...string) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		for _, file := range filename {
			imgC, err := imageutils.ExtractImageFromFile(file)
			if err != nil {
				utils.Errorf("extract image from file %s failed: %s", file, err)
				continue
			}
			for img := range imgC {
				log.Info("Extracted image from file:", file, "MIMEType:", img.MIMEType)
				cfg.images = append(cfg.images, &aicommon.ImageData{
					IsBase64: true,
					Data:     []byte(img.Base64()),
				})
			}
		}
	}
}

// liteforge.image is an option for liteforge.Execute
// it adds image data to the execution context
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.image(imageData))
// ```
func _withImage(anyImageInput ...any) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		for _, anyImg := range anyImageInput {
			for img := range imageutils.ExtractImage(anyImg) {
				cfg.images = append(cfg.images, &aicommon.ImageData{
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
func _executeLiteForgeTemp(query string, opts ...any) (*ForgeResult, error) {
	cfg := &liteforgeConfig{
		query:  query,
		action: "object",
		id:     "temporary-liteforge",
		ctx:    context.Background(),
	}
	for _, optRaw := range opts {
		switch opt := optRaw.(type) {
		case LiteForgeExecOption:
			opt(cfg)
		case aid.Option:
			cfg.aidOptions = append(cfg.aidOptions, opt)
		}
	}

	if ret := utils.InterfaceToString(jsonpath.FindFirst(cfg.output, "$..properties..const")); ret != cfg.action {
		return nil, utils.Errorf("jsonschema output must have '@action' - const value '%s', lite: ..."+`.."@action": {"const": "`+cfg.action+`"}`+"..., found: %v, expect: %v", cfg.action, ret, cfg.action)
	}

	liteforgeIns, err := NewLiteForge(cfg.id, WithLiteForge_OutputSchemaRaw(cfg.action, cfg.output))
	if err != nil {
		return nil, utils.Errorf("new liteforge failed: %s", err)
	}

	if cfg.forceImage && len(cfg.images) == 0 {
		return nil, utils.Error("force image is true, but no image provided")
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

// liteforge.id is an option for liteforge.Execute
// it sets the ID for the liteforge instance
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.id("my-forge-instance"))
// ```
func _withID(id string) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.id = id
	}
}

// liteforge.context is an option for liteforge.Execute
// it sets the context for the liteforge execution
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.context(ctx))
// ```
func LiteForgeExecWithContext(ctx context.Context) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.ctx = ctx
	}
}

// liteforge.verboseName is an option for liteforge.Execute
// it adds verbose naming options to the execution
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.verboseName("my-forge-instance"))
// ```
func _withVerboseName(opts ...aid.Option) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.aidOptions = append(cfg.aidOptions, opts...)
	}
}
