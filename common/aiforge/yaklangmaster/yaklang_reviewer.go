package yaklangmaster

import (
	"context"
	_ "embed"
	"fmt"
	"github.com/yaklang/yaklang/common/schema"
	"regexp"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/fstools"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yaklangtools"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/static_analyzer/result"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

//go:embed yaklang_reviewer_prompts/init.txt
var _initPrompt string

//go:embed yaklang_reviewer_prompts/plan.txt
var _codeReviewPlanMock string

//go:embed yaklang_reviewer_prompts/persistent.txt
var _persistentPrompt string

//go:embed yaklang_reviewer_prompts/set-code.json
var _setCodeSchema string

var magicCode = "yaklang-reviewer-code"

func newYaklangMasterForge(callback func(string)) *aiforge.ForgeBlueprint {
	yaklangTools, err := yaklangtools.CreateYaklangTools()
	if err != nil {
		log.Errorf("create yaklang tools: %v", err)
		return nil
	}
	fileOp, err := fstools.CreateSystemFSTools()
	if err != nil {
		log.Errorf("create system fs tools tools: %v", err)
		return nil
	}
	extTools := append(fileOp, yaklangTools...)

	forge := aiforge.NewForgeBlueprint(
		"yaklang-reviewer",
		aiforge.WithTools(extTools...),
		aiforge.WithInitializePrompt(_initPrompt),
		aiforge.WithPlanMocker(func(config *aid.Config) *aid.PlanResponse {
			res, err := aid.ExtractPlan(config, _codeReviewPlanMock)
			if err != nil {
				config.EmitError("yak review plan mock failed: %s", err)
				return nil
			}
			return res
		}),

		aiforge.WithPersistentPrompt(_persistentPrompt),
		aiforge.WithOriginYaklangCliCode(fmt.Sprintf(`
cli.String("%s", cli.setRequired(true),cli.setVerboseName("yaklang代码"), cli.setHelp("代码内容"))
cli.check()
`, magicCode)),
		aiforge.WithAIDOptions(
			aid.WithAgreeManual(),
			aid.WithResultHandler(func(config *aid.Config) {
				code, _ := config.GetMemory().GetPersistentData("code")
				callback(code)
			}),
			aid.WithExtendedActionCallback("set-code", func(config *aid.Config, action *aicommon.Action) {
				codeContent := action.GetString("content")
				config.GetMemory().SetPersistentData(magicCode, codeContent)
			}),
			aid.WithManualAssistantCallback(func(ctx context.Context, config *aid.Config) (aitool.InvokeParams, error) {
				m := config.GetMemory()
				_, eventIns, ok := m.GetInteractiveEventLast()
				if !ok {
					return nil, utils.Error("Interactive Event Not Found")
				}
				if eventIns.InteractiveEvent.Type == schema.EVENT_TYPE_TASK_REVIEW_REQUIRE {
					return analyzeToolCallResult(m), nil
				}
				return nil, nil
			}),
		),
	)
	return forge
}

//go:embed yaklang_reviewer_prompts/suggestion.txt
var _planSuggestionPrompt string

func analyzeToolCallResult(m *aid.PromptContextProvider) aitool.InvokeParams {
	params := make(map[string]any)
	toolCallResults := m.CurrentTaskToolCallResults()
	if toolCallResults == nil {
		return params
	}
	allSuggestions := make([]*FixSuggestion, 0)
	for _, callResult := range toolCallResults {
		if callResult.Name == yaklangtools.YaklangToolName_SyntaxCheck {
			syntaxRes, _ := callResult.Data.(*aitool.ToolExecutionResult).Result.([]*result.StaticAnalyzeResult)
			allSuggestions = append(allSuggestions, analyzeResultToSuggestion(syntaxRes)...)
		}
	}
	if len(allSuggestions) > 0 {
		params["suggestion"] = "adjust_plan"
		params["plan"] = renderSuggestion(m, allSuggestions)
	}
	return params
}

func renderSuggestion(m *aid.PromptContextProvider, suggestions []*FixSuggestion) string {
	// 解析模板
	tmpl, err := template.New("suggestion").Parse(_planSuggestionPrompt)
	if err != nil {
		log.Error(err)
		return ""
	}
	// 渲染模板
	var promptBuilder strings.Builder
	code, _ := m.UserDataGet("code")
	err = tmpl.Execute(&promptBuilder, map[string]any{
		"Suggestions": suggestions,
		"Code":        code,
	})
	if err != nil {
		log.Errorf("error executing suggestion template: %v", err)
		return ""
	}
	return promptBuilder.String()
}

type FixSuggestion struct {
	RecommendedTool string
	ToolParam       map[string]string
	Suggestion      string
	Reason          string
	StartLine       int64
	EndLine         int64
}

var libFuncNameRegex = regexp.MustCompile("ExternLib.*?\\[([^\\]]+)\\]")

func analyzeResultToSuggestion(results []*result.StaticAnalyzeResult) []*FixSuggestion {
	var suggestions []*FixSuggestion
	for _, res := range results {
		suggestion := &FixSuggestion{
			StartLine: res.StartLineNumber,
			EndLine:   res.EndLineNumber,
		}
		message := res.Message
		if strings.Contains(message, "ExternLib") {
			suggestion.RecommendedTool = yaklangtools.YaklangToolName_Document
			suggestion.Suggestion = "推荐使用工具查询指定库的函数，修复函数名错误"
			suggestion.Reason = "库函数名错误"
			suggestion.ToolParam = make(map[string]string)
			libFuncName := libFuncNameRegex.FindStringSubmatch(message)
			if len(libFuncName) > 0 {
				suggestion.ToolParam["lib"] = libFuncName[1]
			}
		} else if strings.Contains(message, "Error Unhandled") {
			suggestion.Suggestion = "根据yaklang错误处理风格，处理未处理的错误"
			suggestion.Reason = "有未处理的错误"
		} else {
			suggestion.Suggestion = "根据提供的语法示例，处理语法错误"
			suggestion.Reason = message
		}
		suggestions = append(suggestions, suggestion)
	}
	return suggestions
}

func init() {
	err := aiforge.RegisterForgeExecutor("yaklang-reviewer", func(ctx context.Context, items []*ypb.ExecParamItem, option ...aid.Option) (*aiforge.ForgeResult, error) {
		var res = &aiforge.ForgeResult{}
		forge := newYaklangMasterForge(func(code string) {
			res.Formated = code
		})
		ins, err := forge.CreateCoordinator(ctx, items, option...)
		if err != nil {
			return nil, utils.Errorf("create coordinator failed: %s", err)
		}
		err = ins.Run()
		if err != nil {
			log.Errorf("yaklang-master failed: %s", err)
		}
		return res, nil
	})
	if err != nil {
		log.Errorf("register yaklang master forge failed: %s", err)
	} else {
		log.Infof("register yaklang master forge success")
	}
}
