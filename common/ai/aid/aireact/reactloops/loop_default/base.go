package loop_default

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/instruction.txt
var instruction string

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveDataTemplate string

const reActPostSummary = `
请根据你刚才执行的所有步骤，以 **Markdown 格式** 输出一份结构化总结，格式如下：

【注意：回答过程中，保持克制，不要使用任何 EMOJI，这是一个工业生产级别的系统】

---

简要描述本次任务的目标和整体结果。说明任务是否完成，以及核心产出是什么。

---

## 可选后续（不属于本次完成条件）

仅列出本次目标之外、不会阻塞本次完成的可选扩展，例如需要用户新选择、超出本次范围的长期可观测性、回归、调试或优化。

不要把仍属于本次目标、已有证据支持、当前可以执行且会实质提高结果质量的行动放在这里；这类行动应在 finish 前升级为 TODO 并执行。

没有合适的非阻塞可选项时，省略整个章节。

`

// resolveMaxIterations computes the iteration ceiling for a loop from the
// caller config. When goal mode is enabled it raises a too-small ceiling to
// GoalMinIterations + buffer via EnsureGoalModeMaxIterations, so the finish
// gate can actually open before the loop exhausts its iterations. This is the
// single loop-level resolution point; the gRPC entry point
// (ConvertYPBAIStartParamsToReActConfig) performs the same bump on the config
// field as an idempotent safety net, so both programmatic and gRPC entry paths
// are covered regardless of which one runs first.
func resolveMaxIterations(cfg aicommon.AICallerConfigIf) int {
	if cfg == nil {
		return 100
	}
	maxIterations := cfg.GetMaxIterationCount()
	if typedCfg, ok := cfg.(*aicommon.Config); ok && typedCfg.GetEnableGoalMode() {
		maxIterations = aicommon.EnsureGoalModeMaxIterations(maxIterations, typedCfg.GetGoalMinIterations())
	}
	if maxIterations <= 0 {
		return 100
	}
	return int(maxIterations)
}

func buildDefaultReactiveDataBuilder() reactloops.ReActLoopOption {
	return reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
		renderMap := map[string]any{
			"Nonce":            nonce,
			"FeedbackMessages": feedbacker.String(),
			"IsLastIteration":  loop.GetCurrentIterationIndex()+1 >= loop.GetMaxIterations(),
		}
		return utils.RenderTemplate(reactiveDataTemplate, renderMap)
	})
}

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(true),
				reactloops.WithAllowPlanAndExec(true),
				reactloops.WithPlanExecActionType(schema.AI_REACT_LOOP_ACTION_REQUEST_PLAN_EXECUTION),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithMaxIterations(resolveMaxIterations(r.GetConfig())),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithOutputExample(outputExample),
				buildDefaultReactiveDataBuilder(),
				reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
					if !isDone {
						return
					}
					lastAction := loop.GetLastValidAction()
					if lastAction == nil {
						log.Warnf("iteration %d: skip final summary because last action is empty", iteration)
						return
					}
					if lastAction.ActionType == schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER {
						log.Infof("iteration %d: action is directly answer, exiting loop and returning final answer", iteration)
						return
					}
					if strings.TrimSpace(loop.Get("intent_hint")) == "simple_query" {
						log.Infof("iteration %d: simple query task, skip post-iteration summary", iteration)
						return
					}

					directlySummary, _ := loop.GetInvoker().DirectlyAnswer(
						task.GetContext(), reActPostSummary, nil, nil,
					)
					if directlySummary != "" {
						loop.GetInvoker().AddToTimeline("final_summary", directlySummary)
					}
				}),
			}

			preset = append(preset, opts...)
			loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r, preset...)
			return loop, err
		},
		reactloops.WithLoopDescription("General-purpose assistant mode for mixed tasks, combining reasoning, tools, RAG, and AI forges."),
		reactloops.WithLoopDescriptionZh("通用助手模式：适合处理混合型任务，综合使用推理、工具、RAG 与 AI 蓝图完成工作。"),
		reactloops.WithLoopUsagePrompt("Use as the primary fallback mode when the request does not require a specialized focused mode. Suitable for broad problem solving, multi-step coordination, and direct responses."),
		reactloops.WithLoopOutputExample(`
* When the task is general and no specialized focused mode is needed:
  {"@action": "default", "human_readable_thought": "The request is broad, so I will continue in the default assistant mode and solve it step by step"}
`),
		reactloops.WithLoopIsHidden(true),
		reactloops.WithVerboseName("Default Assistant"),
		reactloops.WithVerboseNameZh("默认助手模式"),
	)
	if err != nil {
		log.Errorf("build default react loop failed: %v", err)
	}

	err = reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_PE_TASK,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildPETaskInitTask(r)),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithMaxIterations(resolveMaxIterations(r.GetConfig())),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithOutputExample(outputExample),
				buildDefaultReactiveDataBuilder(),
			}

			preset = append(preset, opts...)
			loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r, preset...)
			return loop, err
		},
		reactloops.WithLoopDescription("Plan-execution task mode for structured PE workflows with predefined objectives and execution context."),
		reactloops.WithLoopDescriptionZh("渗透任务执行模式：面向结构化渗透测试工作流，在既定目标和上下文下推进任务执行。"),
		reactloops.WithLoopUsagePrompt("Used internally for PE task orchestration when the system has already prepared execution-oriented initialization context and constraints."),
		reactloops.WithLoopOutputExample(`
* When entering a structured PE execution task:
  {"@action": "pe_task", "human_readable_thought": "I will execute the prepared PE task flow with the provided constraints and goals"}
`),
		reactloops.WithLoopIsHidden(true),
		reactloops.WithVerboseName("PE Task Executor"),
		reactloops.WithVerboseNameZh("渗透任务执行模式"),
	)
	if err != nil {
		log.Errorf("build default react loop failed: %v", err)
	}
}
