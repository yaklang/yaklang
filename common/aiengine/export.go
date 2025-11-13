package aiengine

var Exports = map[string]interface{}{
	// invoke re-act
	"InvokeReAct":      InvokeReAct,
	"InvokeReActAsync": InvokeReActAsync,

	// new ai engine
	"NewAIEngine": NewAIEngine,

	// config options
	"focus":                 WithFocus,
	"timeout":               WithTimeout,
	"context":               WithContext,
	"aiService":             WithAIService,
	"maxIteration":          WithMaxIteration,
	"sessionID":             WithSessionID,
	"workdir":               WithWorkdir,
	"language":              WithLanguage,
	"disableToolUse":        WithDisableToolUse,
	"disableAIForge":        WithDisableAIForge,
	"disableMCPServers":     WithDisableMCPServers,
	"enableAISearchTool":    WithEnableAISearchTool,
	"enableForgeSearchTool": WithEnableForgeSearchTool,
	"includeToolNames":      WithIncludeToolNames,
	"excludeToolNames":      WithExcludeToolNames,
	"keywords":              WithKeywords,
	"allowUserInteract":     WithAllowUserInteract,
	"reviewPolicy":          WithReviewPolicy,
	"userInteractLimit":     WithUserInteractLimit,
	"timelineContentLimit":  WithTimelineContentLimit,
	"debugMode":             WithDebugMode,
	"onEvent":               WithOnEvent,
	"onStream":              WithOnStream,
	"onStreamEnd":           WithOnStreamEnd,
	"onData":                WithOnData,
	"onFinished":            WithOnFinished,
	"onInputRequiredRaw":    WithOnInputRequiredRaw,
	"onInputRequired":       WithOnInputRequired,
	"yoloMode":              WithYOLOMode,
	"manualMode":            WithManualMode,
	"aiReviewMode":          WithAIReviewMode,
	"aiCallback":            WithAICallback,
	"aiConfig":              WithAIConfig,

	// ai forge
	// "BuildAIForge":      BuildAIForge,
	// "forgePlan":         WithForgePlan,
	// "forgePlanTask":     WithForgePlanTask,
	// "forgeResultSchema": WithForgeResultSchema,
	// "extAForge":         WithExtAIForge,
}
