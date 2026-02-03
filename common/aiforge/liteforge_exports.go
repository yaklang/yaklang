package aiforge

import (
	"context"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/jsonpath"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
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
	"imageExtraPrompt": WithExtraPrompt, // use for analyzeImage and analyzeImageFile

	"analyzeCtx":          WithAnalyzeContext,    // use for analyzeContext
	"analyzeLog":          WithAnalyzeLog,        // use for analyzeLog
	"analyzeStatusCard":   WithAnalyzeStatusCard, // use for analyzeStatusCard
	"output":              WithOutputJSONSchema,
	"action":              WithOutputAction,
	"image":               _withImage,
	"imageFile":           _withImageFile,
	"id":                  _withID,
	"context":             LiteForgeExecWithContext,
	"verboseName":         _withVerboseName,
	"forceImage":          _withForceImage,
	"fieldStreamCallback": WithLiteForgeExec_FieldStreamCallback,

	"knowledgeBaseName":    RefineWithKnowledgeBaseName,
	"knowledgeBaseDesc":    RefineWithKnowledgeBaseDesc,
	"knowledgeBaseType":    RefineWithKnowledgeBaseType,
	"knowledgeEntryLength": RefineWithKnowledgeEntryLength,
	"refinePrompt":         _refine_WithRefinePrompt,
	"strictRefine":         _refine_WithStrict,
}

type liteforgeConfig struct {
	query      string
	output     string
	action     string
	id         string
	ctx        context.Context
	images     []*aicommon.ImageData
	forceImage bool

	aidOptions []aicommon.ConfigOption

	jsonExtractHook []jsonextractor.CallbackOption

	// fieldStreamCallback is called with the streaming content of a specific field
	fieldStreamCallback LiteForgeFieldStreamCallback
	streamFieldKey      string // the JSON field key to stream
}

type LiteForgeExecOption func(*liteforgeConfig)

func WithJsonExtractHook(opts ...jsonextractor.CallbackOption) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.jsonExtractHook = append(cfg.jsonExtractHook, opts...)
	}
}

// WithLiteForgeExec_FieldStreamCallback sets a callback to receive the streaming content of a specific JSON field
// fieldKey: the JSON field name to extract and stream (e.g., "hypothetical_answer", "search_keywords")
// callback: called with the field key and a reader containing the extracted field content
func WithLiteForgeExec_FieldStreamCallback(fieldKey string, callback LiteForgeFieldStreamCallback) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.streamFieldKey = fieldKey
		cfg.fieldStreamCallback = callback
	}
}

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
func WithOutputJSONSchema(output string) LiteForgeExecOption {
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
func WithOutputAction(action string) LiteForgeExecOption {
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

// liteforge.image is an option for liteforge.Execute
// it adds image data to the execution context
//
// example:
// ```
// liteforge.Execute(<<<PROMPT
// SOME_CONTENT
// PROMPT, liteforge.image(imageData))
// ```
func _withImageCompress(anyImageInput ...any) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		for _, anyImg := range anyImageInput {
			for img := range imageutils.ExtractImage(anyImg) {
				if len(img.RawImage) > 300*1024 {
					raw, err := ffmpegutils.CompressImageRaw(img.RawImage)
					if err == nil {
						img.RawImage = raw
					}
				}
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

	// Collect aispec.AIConfigOption to build aicommon.WithAIChatInfo
	var aiSpecConfig aispec.AIConfig
	var hasAiSpecOpts bool

	for _, optRaw := range opts {
		switch opt := optRaw.(type) {
		case LiteForgeExecOption:
			opt(cfg)
		case aicommon.ConfigOption:
			cfg.aidOptions = append(cfg.aidOptions, opt)
		case aispec.AIConfigOption:
			// Collect aispec options to extract Type and Model
			opt(&aiSpecConfig)
			hasAiSpecOpts = true
		}
	}

	// Convert collected aispec options to aicommon.ConfigOption
	if hasAiSpecOpts {
		if aiSpecConfig.Type != "" || aiSpecConfig.Model != "" {
			cfg.aidOptions = append(cfg.aidOptions, aicommon.WithAIChatInfo(aiSpecConfig.Type, aiSpecConfig.Model))
		}
	}

	if cfg.ctx == nil {
		cfg.ctx = context.Background()
	}

	// When cfg.output is set via LiteForgeExecOption, validate the schema here.
	// When schema is passed via aicommon.ConfigOption (in cfg.aidOptions), skip validation here
	// and let ExecuteEx handle it - it will extract schema from coordinator's config.
	if cfg.output != "" {
		if ret := utils.InterfaceToString(jsonpath.FindFirst(cfg.output, "$..properties..const")); ret != cfg.action {
			return nil, utils.Errorf("jsonschema output must have '@action' - const value '%s', lite: ..."+`.."@action": {"const": "`+cfg.action+`"}`+"..., found: %v, expect: %v", cfg.action, ret, cfg.action)
		}
	}

	var liteForgeOpts []LiteForgeOption
	liteForgeOpts = append(liteForgeOpts, WithLiteForge_OutputJsonHook(cfg.jsonExtractHook...))
	if cfg.output != "" {
		liteForgeOpts = append(liteForgeOpts, WithLiteForge_OutputSchemaRaw(cfg.action, cfg.output))
	}
	if cfg.fieldStreamCallback != nil && cfg.streamFieldKey != "" {
		liteForgeOpts = append(liteForgeOpts, WithLiteForge_FieldStreamCallback(cfg.streamFieldKey, cfg.fieldStreamCallback))
	}

	// Check if LiteForgeFieldStreamCallback is set via aicommon.ConfigOption
	// Safely extract callback info by applying options to a minimal config with panic recovery
	if cfg.fieldStreamCallback == nil && len(cfg.aidOptions) > 0 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Warnf("failed to extract liteforge stream callback from config options: %v", r)
				}
			}()
			// Create a minimal config with required fields initialized
			tempConfig := aicommon.NewMinimalConfigForOptionExtraction()
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Warnf("panic recovered while applying config options to minimal config: %v", r)
						utils.PrintCurrentGoroutineRuntimeStack()
					}
				}()
				for _, opt := range cfg.aidOptions {
					if opt != nil {
						opt(tempConfig)
					}
				}
			}()

			if tempConfig.LiteForgeFieldStreamCallback != nil && tempConfig.LiteForgeStreamFieldKey != "" {
				liteForgeOpts = append(liteForgeOpts, WithLiteForge_FieldStreamCallback(
					tempConfig.LiteForgeStreamFieldKey,
					tempConfig.LiteForgeFieldStreamCallback,
				))
			}
		}()
	}

	liteforgeIns, err := NewLiteForge(cfg.id, liteForgeOpts...)
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
func _withVerboseName(opts ...aicommon.ConfigOption) LiteForgeExecOption {
	return func(cfg *liteforgeConfig) {
		cfg.aidOptions = append(cfg.aidOptions, opts...)
	}
}
