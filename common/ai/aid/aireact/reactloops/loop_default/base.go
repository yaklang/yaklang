package loop_default

import (
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

//go:embed prompts/instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

const reActPostSummary = `
请根据你刚才执行的所有步骤，以 **Markdown 格式** 输出一份结构化总结，格式如下：

【注意：回答过程中，保持克制，不要使用任何 EMOJI，这是一个工业生产级别的系统】

---

## 执行总结

简要描述本次任务的目标和整体结果。

---

## 执行过程回顾

按步骤列出你做了什么，每一步的关键操作和结果：

1. **步骤一**：...
2. **步骤二**：...
3. **步骤三**：...

---

## 最终结果

说明任务是否完成，以及核心产出是什么。

---

## 下一步建议

[友善地引导用户进行下一步行动]，可以说：

1. "如果您觉得我的结果有缺陷，请您不吝赐教"
2. "如果您有任何其他问题或需要进一步的帮助，请随时告诉我！"
3. "我们可以继续做xxx，您允许的话我们马上开始！"

引发用户的兴趣和参与，鼓励他们继续互动。

【注意：下一步建议并不一定需要出现，也不需要太死板，以引导用户交互为核心目标，上述列表只需要选择性表达即可】

`

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_DEFAULT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithAllowAIForge(true),
				reactloops.WithAllowPlanAndExec(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, operator *reactloops.OnPostIterationOperator) {
					if !isDone {
						return
					}
					if loop.GetLastAction().ActionType == schema.AI_REACT_LOOP_ACTION_DIRECTLY_ANSWER {
						log.Infof("iteration %d: action is directly answer, exiting loop and returning final answer", iteration)
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

			// 检查是否有 GetEnableSelfReflection 方法（向后兼容）
			if config := r.GetConfig(); config != nil {
				if reactConfig, ok := config.(interface{ GetEnableSelfReflection() bool }); ok {
					preset = append(preset, reactloops.WithEnableSelfReflection(reactConfig.GetEnableSelfReflection()))
				}
			}
			preset = append(preset, opts...)
			loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r, preset...)
			return loop, err
		},
		reactloops.WithLoopDescription("General-purpose assistant mode for mixed tasks, combining reasoning, tools, RAG, and AI forges."),
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
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
			}

			// 检查是否有 GetEnableSelfReflection 方法（向后兼容）
			if config := r.GetConfig(); config != nil {
				if reactConfig, ok := config.(interface{ GetEnableSelfReflection() bool }); ok {
					preset = append(preset, reactloops.WithEnableSelfReflection(reactConfig.GetEnableSelfReflection()))
				}
			}
			preset = append(preset, opts...)
			loop, err := reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_DEFAULT, r, preset...)
			return loop, err
		},
		reactloops.WithLoopDescription("Plan-execution task mode for structured PE workflows with predefined objectives and execution context."),
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
