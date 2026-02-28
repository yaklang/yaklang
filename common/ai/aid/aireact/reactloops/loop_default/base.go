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
