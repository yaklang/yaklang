package loop_syntaxflow_scan

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			state := NewSyntaxFlowState()
			preset := []reactloops.ReActLoopOption{
				reactloops.WithInitTask(buildSyntaxflowOrchestratorInit(r, state)),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, r, preset...)
		},
		reactloops.WithVerboseName("IRify · SyntaxFlow Scan"),
		reactloops.WithVerboseNameZh("IRify · SyntaxFlow 扫描"),
		reactloops.WithLoopDescription("Multi-stage SyntaxFlow scan: orchestrator resolves task_id / sf_scan_config_json / project_path, then compiles and runs the interpret sub-loop (reload_syntaxflow_scan_session, reload_ssa_risk_overview, set_ssa_risk_review_target)."),
		reactloops.WithLoopDescriptionZh("SyntaxFlow 多阶段扫描：编排层解析入参与校验后编译起扫，再由解读子循环拉会话与风险；工具同前。可 WithVar：syntaxflow_task_id、sf_scan_config_json、project_path。"),
		reactloops.WithLoopUsagePrompt("Provide syntaxflow_task_id, sf_scan_config_json, or project_path (or natural-language project path for LiteForge). Interpret phase uses reload_syntaxflow_scan_session, reload_ssa_risk_overview, set_ssa_risk_review_target."),
		reactloops.WithLoopOutputExample(`
* SyntaxFlow 扫描会话：
  {"@action": "syntaxflow_scan", "human_readable_thought": "需要解读已注入的 SyntaxFlow 扫描任务（task_id 由引擎或变量提供）"}
* 重新加载另一扫描任务摘要：
  {"@action": "reload_syntaxflow_scan_session", "human_readable_thought": "用户提供了新的 task_id", "task_id": "550e8400-e29b-41d4-a716-446655440000"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop %v failed: %v", schema.AI_REACT_LOOP_NAME_SYNTAXFLOW_SCAN, err)
	}
}
