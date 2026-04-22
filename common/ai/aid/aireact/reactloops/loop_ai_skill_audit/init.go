package loop_ai_skill_audit

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
)

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_AI_SKILL_AUDIT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			return BuildSkillAuditLoop(r, opts...)
		},
		reactloops.WithLoopDescription("AI Skill security audit mode: a three-phase pipeline (directory exploration → static malicious-behavior analysis → report generation). Detects reverse shells, data exfiltration, backdoors, cryptominers, and intent mismatches between SKILL.md declarations and script implementations."),
		reactloops.WithLoopDescriptionZh("AI Skill 安全审计模式：三阶段流水线（目录探索 → 静态恶意行为分析 → 报告生成）。检测反弹 Shell、数据外传、后门植入、挖矿代码，并核查 SKILL.md 声明与脚本实现的意图一致性。"),
		reactloops.WithVerboseName("AI Skill Security Auditor"),
		reactloops.WithVerboseNameZh("AI Skill 安全审计"),
		reactloops.WithLoopUsagePrompt(`当用户需要对 Agent Skill（包含 SKILL.md 的目录）进行安全审计、安全扫描或安全检查时使用此流程。流程分三阶段：Phase 1 目录探索 → Phase 2 静态安全分析（检测恶意行为模式）→ Phase 3 报告生成。用户需提供 Skill 目录的绝对路径。`),
		reactloops.WithLoopOutputExample(`
* 当需要对 AI Skill 进行安全审计时：
  {"@action": "ai_skill_audit", "human_readable_thought": "需要对此 AI Skill 目录进行安全审计，检查是否存在恶意行为"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_AI_SKILL_AUDIT, err)
	}
}
