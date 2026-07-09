package loop_code_security_audit

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/persist"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/orchestrator"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			state := model.NewAuditState()

			cfg := r.GetConfig()
			if c, ok := cfg.(interface{ GetOrCreateWorkDir() string }); ok {
				state.WorkDir = c.GetOrCreateWorkDir()
				log.Infof("[CodeAudit] workdir=%s", state.WorkDir)
				if loaded, ok := persist.TryLoadAuditStateFromWorkDir(state.WorkDir); ok {
					state = loaded
					log.Infof("[CodeAudit] restored completed audit state from workdir (phase=%s)", state.GetPhase())
				}
			}

			preset := []reactloops.ReActLoopOption{
				reactloops.WithInitTask(orchestrator.BuildInitTask(r, state)),
			}

			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT, r, append(opts, preset...)...)
		},
		reactloops.WithLoopDescription("Code security audit mode: a four-phase pipeline (project reconnaissance -> structured finding scan -> finding-by-finding verification -> report generation)."),
		reactloops.WithLoopDescriptionZh("代码安全审计模式：四阶段流水线（项目探索→结构化扫描→逐 finding 验证→报告生成）。"),
		reactloops.WithVerboseName("Code Security Audit"),
		reactloops.WithVerboseNameZh("代码安全审计"),
		reactloops.WithLoopUsagePrompt(`当用户需要使用 AI 独立对整个代码项目进行安全审计时使用此流程。流程分四阶段：Phase 1 项目探索 → Phase 2 结构化 Finding 扫描 → Phase 3 并行 Finding 验证（fork 子 Agent） → Phase 4 Markdown 报告生成。

前端 AttachedResourceInfo 与 ai_skill_audit 完全相同（仅 FocusModeLoop 不同）：
- Type=file, Key=directory_path — 扫描根目录
- Type=file, Key=file_path — 当前打开文件（Phase2 聚焦）
- Type=selected, Key=content — 选中代码片段 JSON（Phase2 聚焦）
旧版兼容 Key：code_audit_target_path（新前端无需传递）`),
		reactloops.WithLoopOutputExample(`
* 当需要进行项目级别的代码安全审计时：
  {"@action": "code_security_audit", "human_readable_thought": "需要对项目进行全面的安全审计"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_CODE_SECURITY_AUDIT, err)
	}
}
