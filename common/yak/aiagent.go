package yak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
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
			aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				runtimeId := params.GetString("runtime_id")
				if runtimeId == "" {
					runtimeId = uuid.New().String()
				}
				yakitClient := yaklib.NewVirtualYakitClientWithRuntimeID(func(i *ypb.ExecResult) error {
					if i.IsMessage {
						stdout.Write([]byte(yaklib.ConvertExecResultIntoLog(i)))
						stdout.Write([]byte("\n"))
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
					"PLUGIN_NAME":  runtimeId + ".yak",
					"YAK_FILENAME": runtimeId + ".yak",
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
	"ExecuteForge":   ExecuteForge,
	"planAICallback": WithPlanAICallback,
	"taskAICallback": WithTaskAICallback,
	"aiCallback":     WithAICallback,

	// todo: need to split?

	//  create ai forge blue print
	"CreateForge":           NewForgeBlueprint,
	"NewExecutor":           NewForgeExecutor,
	"NewExecutorFromJson":   NewExecutorFromJson,
	"tools":                 WithTools,
	"initPrompt":            WithInitializePrompt,
	"persistentPrompt":      WithPersistentPrompt,
	"resultPrompt":          WithResultPrompt,
	"plan":                  WithPlanMocker,
	"forgePlanMocker":       WithForgePlanMocker,
	"initializePrompt":      WithInitializePrompt, // similar to initPrompt above
	"resultPromptForge":     WithResultPrompt,     // similar to resultPrompt above
	"resultHandlerForge":    WithResultHandlerForge,
	"persistentPromptForge": WithPersistentPrompt, // similar to persistentPrompt above
	"toolKeywords":          WithToolKeywords,
	"forgeTools":            WithForgeTools,
	"originYaklangCliCode":  WithOriginYaklangCliCode,

	/*
		aid api
	*/
	"agreeAuto":                    WithAgreeAuto,
	"agreeYOLO":                    WithAgreeYOLO,
	"agreePolicyAI":                WithAIAgree,
	"agreeManual":                  WithAgreeManual,
	"extendedActionCallback":       WithExtendedActionCallback,
	"resultHandler":                WithResultHandler,
	"forgeName":                    WithForgeName,
	"context":                      WithContext,
	"extendAIDOptions":             WithExtendAIDOptions,
	"runtimeID":                    WithRuntimeID,
	"offsetSeq":                    WithOffsetSeq,
	"tool":                         WithTool,
	"disallowRequireForUserPrompt": WithDisallowRequireForUserPrompt,
	"manualAssistantCallback":      WithManualAssistantCallback,
	"agreePolicy":                  WithAgreePolicy,
	"aiAgree":                      WithAIAgree,
	"allowRequireForUserInteract":  WithAllowRequireForUserInteract,
	"toolManager":                  WithToolManager,
	"memory":                       WithMemory,
	"coordinatorAICallback":        WithCoordinatorAICallback,
	"systemFileOperator":           WithSystemFileOperator,
	"jarOperator":                  WithJarOperator,
	"omniSearchTool":               WithOmniSearchTool,
	"aiToolsSearchTool":            WithAiToolsSearchTool,
	"debugPrompt":                  WithDebugPrompt,
	"eventHandler":                 WithEventHandler,
	"eventInputChan":               WithEventInputChan,
	"debug":                        WithDebug,
	"appendPersistentMemory":       WithAppendPersistentMemory,
	"timeLineLimit":                WithTimeLineLimit,
	"timelineContentLimit":         WithTimelineContentLimit,
	"forgeParams":                  WithForgeParams,
	"disableToolUse":               WithDisableToolUse,
	"aiAutoRetry":                  WithAIAutoRetry,
	"aiTransactionRetry":           WithAITransactionRetry,

	/*
		ai utils api
	*/
	"ExtractPlan":   aid.ExtractPlan,
	"ExtractAction": aid.ExtractAction,
}
