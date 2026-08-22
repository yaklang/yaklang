package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	// blank-import aive 触发价值评估 submitter 的 init() 注册 (默认开启).
	// 关键词: aive blank import, RegisterValueFeedbackSubmitter 触发
	_ "github.com/yaklang/yaklang/common/ai/aid/aive"
	// blank-import aistats 触发命中统计 (UserAIStats) recorder 的 init() 注册 (默认开启).
	// 关键词: aistats blank import, RegisterStatsRecorder 触发
	_ "github.com/yaklang/yaklang/common/ai/aid/aistats"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools/metadata/genmetadata"
	"github.com/yaklang/yaklang/common/aiforge"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func init() {
	yakscripttools.RegisterYakScriptAiToolsCovertHandle(YakTool2AITool) // avoid cycle import
}

func schemaNodeAllowsObject(node map[string]any) bool {
	if node == nil {
		return false
	}
	if typ, _ := node["type"].(string); typ == "object" {
		return true
	}
	for _, unionKey := range []string{"oneOf", "anyOf"} {
		variants, _ := node[unionKey].([]any)
		for _, variant := range variants {
			variantMap, _ := variant.(map[string]any)
			if schemaNodeAllowsObject(variantMap) {
				return true
			}
		}
	}
	return false
}

func isEmptyArrayLike(value any) bool {
	if value == nil {
		return false
	}
	rv := reflect.ValueOf(value)
	return rv.IsValid() && (rv.Kind() == reflect.Array || rv.Kind() == reflect.Slice) && rv.Len() == 0
}

// Optional object fields are sometimes emitted by form UIs as [] instead of being
// omitted. The public schema explicitly opts compatible fields into this behavior;
// normalize those empty arrays before cli.Json sees them, and report the correction
// in captured output so ReAct/timeline users can understand the effective request.
func normalizeEmptyObjectParams(tool *mcp.Tool, params aitool.InvokeParams) (aitool.InvokeParams, []string) {
	normalized := make(aitool.InvokeParams, len(params))
	for key, value := range params {
		normalized[key] = value
	}
	if tool == nil || tool.InputSchema.Properties == nil {
		return normalized, nil
	}

	var notes []string
	for key, value := range normalized {
		if !isEmptyArrayLike(value) {
			continue
		}
		property, ok := tool.InputSchema.Properties.Get(key)
		if !ok {
			continue
		}
		rawSchema, err := json.Marshal(property)
		if err != nil {
			continue
		}
		var schemaNode map[string]any
		if err := json.Unmarshal(rawSchema, &schemaNode); err != nil || !schemaNodeAllowsObject(schemaNode) {
			continue
		}
		normalized[key] = map[string]any{}
		notes = append(notes, fmt.Sprintf("normalized parameter %q from empty array [] to empty object {}; omit the field or pass {} next time", key))
	}
	return normalized, notes
}

func YakTool2AITool(aitools []*schema.AIYakTool) []*aitool.Tool {
	tools := []*aitool.Tool{}
	for _, aiTool := range aitools {
		tool := mcp.NewTool(aiTool.Name)
		tool.Description = aiTool.Description
		dataMap := map[string]any{}
		err := json.Unmarshal([]byte(aiTool.Params), &dataMap)
		if err != nil {
			log.Errorf("unmarshal aiTool.Params failed: %v", err)
			continue
		}
		tool.InputSchema.FromMap(dataMap)
		at, err := aitool.NewFromMCPTool(
			tool,
			aitool.WithDescription(aiTool.Description),
			aitool.WithKeywords(strings.Split(aiTool.Keywords, ",")),
			aitool.WithVerboseName(aiTool.VerboseName),
			aitool.WithVerboseNameZh(aiTool.VerboseNameZh),
			aitool.WithUsage(aiTool.Usage),
			aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()
				params, normalizationNotes := normalizeEmptyObjectParams(tool, params)
				for _, note := range normalizationNotes {
					fmt.Fprintf(stdout, "[warning] input compatibility: %s. Execution will continue.\n", note)
				}

				var runtimeId string
				var runtimeFeedBacker func(result *ypb.ExecResult) error
				var riskSaveHandler func(context.Context, *schema.Risk) error
				if runtimeConfig != nil {
					runtimeId = runtimeConfig.RuntimeID
					runtimeFeedBacker = runtimeConfig.FeedBacker
					riskSaveHandler = runtimeConfig.RiskSaveHandler
				}

				yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
					if ret := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result, aiTool.EnableAIOutputLog == 2); ret != "" {
						stdout.Write([]byte(ret))
						stdout.Write([]byte("\n"))
					}
					if runtimeFeedBacker != nil {
						return runtimeFeedBacker(result)
					}
					return nil
				}, runtimeId)

				engine := NewYakitVirtualClientScriptEngine(yakitClient)

				var args []string
				for k, v := range params {
					switch ret := v.(type) {
					case bool:
						if ret {
							args = append(args, "--"+k)
						}
					default:
						args = append(args, "--"+k, nativeYakPluginArgString(ret))
					}

				}
				cliApp := GetHookCliApp(args)
				var browserTracker browserSessionTracker
				if runtimeConfig != nil {
					browserTracker = runtimeConfig.BrowserSessionTracker
				}
				registerBrowserSessionHooks(engine, browserTracker)
				engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
					pluginContext := CreateYakitPluginContext(
						runtimeId,
					).WithContext(
						ctx,
					).WithContextCancel(
						cancel,
					).WithCliApp(
						cliApp,
					).WithYakitClient(
						yakitClient,
					)
					if riskSaveHandler != nil {
						pluginContext.WithRiskSaveHandler(riskSaveHandler)
					}
					if runtimeFeedBacker != nil {
						pluginContext.WithAfterSaveHTTPFlowHandler(func(flow *schema.HTTPFlow) {
							if flow == nil {
								return
							}
							level, msg := yaklib.MarshalYakitOutput(flow)
							if level == "" {
								return
							}
							if err := runtimeFeedBacker(yaklib.NewYakitLogExecResult(level, msg)); err != nil {
								log.Warnf("emit saved httpflow via runtime feedbacker failed: %v", err)
							}
						})
					}
					BindYakitPluginContextToEngine(
						ae,
						pluginContext,
					)
					return nil
				})

				executedEngine, err := engine.ExecuteExWithContext(ctx, aiTool.Content, map[string]interface{}{
					"RUNTIME_ID":   runtimeId,
					"CTX":          ctx,
					"PLUGIN_NAME":  aiTool.Name + ".yak",
					"YAK_FILENAME": aiTool.Name + ".yak",
				})
				if err != nil {
					log.Errorf("execute ex with context failed: %v", err)
					stderr.Write([]byte(err.Error()))
					return nil, err
				}
				// RESULT is the explicit semantic result channel for Yak-backed AI
				// tools (also used by other Yak execution paths). Log output remains
				// captured separately as stdout/stderr observations. Do not fall back
				// to lowercase `result`: many scripts use that name as a loop-local
				// value, especially concurrent HTTP batches.
				if executedEngine != nil {
					if result, ok := executedEngine.GetVar("RESULT"); ok {
						return result, nil
					}
				}
				return nil, nil
			}))
		if err != nil {
			log.Errorf(`at.NewFromMCPTool(tool): %v`, err)
			return nil
		}
		tools = append(tools, at)
	}
	return tools
}

// exports to yaklang
var AIAgentExport = map[string]any{
	/*
		ai forge api
	*/
	// exec ai forge
	"ExecuteForge": ExecuteForge,
	//  create ai forge blue print
	"CreateForge":         NewForgeBlueprint,
	"NewExecutor":         NewForgeExecutor,
	"NewExecutorFromJson": NewExecutorFromJson,
	"CreateLiteForge":     NewLiteForge,

	"planAICallback": aicommon.WithQualityPriorityAICallback,
	"taskAICallback": aicommon.WithSpeedPriorityAICallback,
	"aiCallback":     aicommon.WithAICallback,

	// liteforge options
	"liteForgePrompt":          aiforge.WithLiteForge_Prompt,
	"liteForgeOutputSchema":    aiforge.WithLiteForge_OutputSchema,
	"liteForgedRequireParams":  aiforge.WithLiteForge_RequireParams,
	"liteForgeOutputSchemaRaw": aiforge.WithLiteForge_OutputSchemaRaw,
	// "liteForgeOutputMemoryOP":  aiforge.WithLiteForge_OutputMemoryOP, // !已废弃

	// forge
	"tools":                 aicommon.WithTools,
	"forgeTools":            aiforge.WithTools,
	"initPrompt":            aiforge.WithInitializePrompt,
	"initializePrompt":      aiforge.WithInitializePrompt, // alias for initPrompt
	"persistentPrompt":      aiforge.WithPersistentPrompt,
	"persistentPromptForge": aiforge.WithPersistentPrompt, // similar to persistentPrompt above
	"resultPrompt":          aiforge.WithResultPrompt,
	"resultPromptForge":     aiforge.WithResultPrompt, // similar to resultPrompt above
	"plan":                  aid.WithPlanMocker,       // plan mocker
	"forgePlanMocker":       aiforge.WithPlanMocker,

	"resultHandlerForge":   aiforge.WithResultHandler,
	"toolKeywords":         aiforge.WithToolKeywords,
	"originYaklangCliCode": aiforge.WithOriginYaklangCliCode,

	/*
		aid api
	*/
	"offsetSeq":                    aicommon.WithSequence,
	"tool":                         aicommon.WithTool,
	"agreeAuto":                    aicommon.WithAgreeAuto,
	"agreeYOLO":                    aicommon.WithAgreeYOLO,
	"agreePolicyAI":                aicommon.WithAIAgree,
	"agreeManual":                  aicommon.WithAgreeManual,
	"agreePolicy":                  aicommon.WithAgreePolicy,
	"extendedActionCallback":       aicommon.WithExtendedActionCallback,
	"resultHandler":                aid.WithResultHandler,
	"forgeName":                    WithForgeName,
	"context":                      WithContext,
	"extendAIDOptions":             WithExtendAICommonOptions,
	"disallowRequireForUserPrompt": aicommon.WithDisallowRequireForUserPrompt,
	"manualAssistantCallback":      aicommon.WithManualAssistantCallback,
	"allowRequireForUserInteract":  aicommon.WithAllowRequireForUserInteract,
	"coordinatorAICallback":        aicommon.WithQualityPriorityAICallback,
	"systemFileOperator":           aicommon.WithSystemFileOperator,
	"omniSearchTool":               aicommon.WithOmniSearchTool,
	"debugPrompt":                  aicommon.WithDebugPrompt,
	"debug":                        aicommon.WithDebug,
	"appendPersistentMemory":       aicommon.WithAppendPersistentContext,
	"timelineContentLimit":         aicommon.WithTimelineContentLimit,
	"disableToolUse":               aicommon.WithDisableToolUse,
	"aiAutoRetry":                  aicommon.WithAIAutoRetry,
	"aiTransactionRetry":           aicommon.WithAITransactionRetry,
	"disableOutputType":            aicommon.WithDisableOutputEvent,

	/*
		ai utils api
	*/
	"ExtractPlan":               aid.ExtractPlan,
	"ExtractAction":             aicommon.ExtractAction,
	"GetDefaultContextProvider": aid.GetDefaultContextProvider,
	"AllYakScriptAiTools":       AllYakScriptTools,
	"UpdateYakScriptMetaData":   genmetadata.UpdateYakScriptMetaData,
	"ParseYakScriptToAiTools":   yakscripttools.LoadYakScriptToAiTools,
}
