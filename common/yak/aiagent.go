package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
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
			aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtimeConfig *aitool.ToolRuntimeConfig, stdout io.Writer, stderr io.Writer) (any, error) {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				var runtimeId string
				var runtimeFeedBacker func(result *ypb.ExecResult) error
				if runtimeConfig != nil {
					runtimeId = runtimeConfig.RuntimeID
					runtimeFeedBacker = runtimeConfig.FeedBacker
				}

				yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(result *ypb.ExecResult) error {
					if ret := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); ret != "" {
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
					args = append(args, "--"+k, fmt.Sprint(v))
				}
				cliApp := GetHookCliApp(args)
				engine.RegisterEngineHooks(func(ae *antlr4yak.Engine) error {
					BindYakitPluginContextToEngine(
						ae,
						CreateYakitPluginContext(
							runtimeId,
						).WithContext(
							ctx,
						).WithContextCancel(
							cancel,
						).WithCliApp(
							cliApp,
						).WithYakitClient(
							yakitClient,
						),
					)
					return nil
				})

				_, err = engine.ExecuteExWithContext(ctx, aiTool.Content, map[string]interface{}{
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
				return "", nil
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
	"appendPersistentMemory":       aicommon.WithAppendPersistentMemory,
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
