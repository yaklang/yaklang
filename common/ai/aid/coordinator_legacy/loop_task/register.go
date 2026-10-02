package loop_task

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_default"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_PE_TASK,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := append(loop_default.BaseOptions(r.GetConfig()), []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildPETaskInitTask(r)),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
			}...)

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
