package loop_ssa_api_discovery

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// SsaDiscoveryModeFullPipeline 需要本地代码目录并跑六阶段流水线。
	SsaDiscoveryModeFullPipeline = "full_pipeline"
	// SsaDiscoveryModeQAReview 仅问答/复盘/方法论，不跑 SQLite 流水线、不覆盖报告产物。
	SsaDiscoveryModeQAReview = "qa_review"
)

// MergeParsedPreferBase 仅用 overlay 填补 base 中的空字段。
func MergeParsedPreferBase(base, overlay *ParsedUserInput) *ParsedUserInput {
	if base == nil {
		return overlay
	}
	if overlay == nil {
		return base
	}
	out := *base
	if strings.TrimSpace(out.CodePath) == "" {
		out.CodePath = overlay.CodePath
	}
	if strings.TrimSpace(out.TargetRaw) == "" {
		out.TargetRaw = overlay.TargetRaw
	}
	if strings.TrimSpace(out.LanguageHint) == "" {
		out.LanguageHint = overlay.LanguageHint
	}
	if out.PipelineMaxStage == 0 && overlay.PipelineMaxStage > 0 {
		out.PipelineMaxStage = overlay.PipelineMaxStage
	}
	if strings.TrimSpace(out.SessionDBDSN) == "" {
		out.SessionDBDSN = overlay.SessionDBDSN
	}
	if strings.TrimSpace(out.SessionUUID) == "" {
		out.SessionUUID = overlay.SessionUUID
	}
	if strings.TrimSpace(out.AuthUsername) == "" {
		out.AuthUsername = overlay.AuthUsername
	}
	if strings.TrimSpace(out.AuthPassword) == "" {
		out.AuthPassword = overlay.AuthPassword
	}
	if strings.TrimSpace(out.AuthLine) == "" {
		out.AuthLine = overlay.AuthLine
	}
	if len(overlay.AuthCredentialGroups) > 0 {
		out.AuthCredentialGroups = mergeCredentialGroups(out.AuthCredentialGroups, overlay.AuthCredentialGroups)
	}
	mergeSSHFields(&out, overlay)
	if strings.TrimSpace(out.Phase4Mode) == "" {
		out.Phase4Mode = overlay.Phase4Mode
	}
	ensureDefaultCredentialGroup(&out)
	syncLegacyAuthFieldsFromGroups(&out)
	if out.TargetRaw != "" {
		out.TargetRaw = NormalizeTargetString(out.TargetRaw)
	}
	return &out
}

func pipelineIntentHeuristic(userText string, parsed *ParsedUserInput) bool {
	if parsed != nil && strings.TrimSpace(parsed.CodePath) != "" {
		return true
	}
	if parsed != nil && SSHRemoteSourceConfigured(parsed) {
		return true
	}
	if strings.TrimSpace(guessAbsolutePath(userText)) != "" {
		return true
	}
	lower := strings.ToLower(userText)
	for _, k := range []string{
		"code path:", "code path：", "target:", "target：", "靶机:", "靶机：",
		"remote_code", "remote code", "ssh_host", "ssh host", "ssh:", "源码",
		"全流程", "六阶段", "ssa api", "syntaxflow", "攻击面发现",
		"漏洞验证", "http 验证", "discovery_report",
	} {
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

func qaIntentHeuristic(userText string, parsed *ParsedUserInput) bool {
	if pipelineIntentHeuristic(userText, parsed) {
		return false
	}
	for _, k := range []string{
		"仅问答", "不要扫描", "不跑全流程", "纯讨论", "不用跑代码", "不要跑流水线",
	} {
		if strings.Contains(userText, k) {
			return true
		}
	}
	if parsed != nil && strings.TrimSpace(parsed.CodePath) == "" && strings.TrimSpace(guessAbsolutePath(userText)) == "" {
		for _, k := range []string{"什么是", "如何理解", "解释一下", "复盘一下", "帮我理解"} {
			if strings.Contains(userText, k) {
				return true
			}
		}
	}
	return false
}

// ClassifySsaDiscoveryRoute 返回 SsaDiscoveryModeFullPipeline 或 SsaDiscoveryModeQAReview；仅在启发式不确定时调用 LiteForge（action 名 ssa_discovery_route）。
func ClassifySsaDiscoveryRoute(ctx context.Context, r aicommon.AIInvokeRuntime, userText string, parsed *ParsedUserInput) (mode string, usedLLM bool) {
	if qaIntentHeuristic(userText, parsed) {
		return SsaDiscoveryModeQAReview, false
	}
	if pipelineIntentHeuristic(userText, parsed) {
		return SsaDiscoveryModeFullPipeline, false
	}
	if r == nil {
		return SsaDiscoveryModeQAReview, false
	}
	prompt := utils.MustRenderTemplate(`你是模式路由器。判断用户是要「全流程 SSA API 发现扫描（需要本地代码目录 + 可选靶机）」，还是「仅代码审计问答/方法论/复盘讨论」（不要求跑扫描流水线）。

仅输出下列之一作为 route：
- full_pipeline ：用户希望或需要跑分析/扫描/发现/验证流水线，或提供了代码路径意图
- qa_review ：概念解释、方法论、面试题式问答、对报告文字的讨论等，且未表达要启动本地代码扫描

用户输入：
<|USER_INPUT|>
{{ .UserText }}
<|END|>

若不确定，倾向于 qa_review（避免误启长时间流水线）。`, map[string]any{"UserText": userText})

	act, lerr := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"ssa_discovery_route",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringParam("route",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("full_pipeline 或 qa_review")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("一句话分类依据")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("ssa_route", "reason"),
	)
	if lerr != nil {
		log.Warnf("ssa_api_discovery: ssa_discovery_route LiteForge failed: %v, default qa_review", lerr)
		return SsaDiscoveryModeQAReview, false
	}
	route := strings.TrimSpace(strings.ToLower(act.GetString("route")))
	reason := act.GetString("reason")
	log.Infof("ssa_api_discovery: route=%s reason=%s (llm)", route, reason)
	switch route {
	case "full_pipeline", "pipeline", "scan", "full":
		return SsaDiscoveryModeFullPipeline, true
	default:
		return SsaDiscoveryModeQAReview, true
	}
}

// TryMergeParsedFromLatestSession 在同 workDir / 会话库下用最近一次成功会话补全空的 CodePath/Target。
func TryMergeParsedFromLatestSession(r aicommon.AIInvokeRuntime, workDir string, parsed *ParsedUserInput) {
	if r == nil || parsed == nil || strings.TrimSpace(workDir) == "" {
		return
	}
	if strings.TrimSpace(parsed.CodePath) != "" && strings.TrimSpace(parsed.TargetRaw) != "" {
		return
	}
	cfg := store.SessionDBConfig{WorkDir: workDir}
	if dsn := strings.TrimSpace(parsed.SessionDBDSN); dsn != "" {
		cfg.Dialect = "postgres"
		cfg.DSN = dsn
	}
	db, err := store.OpenSessionDBFromConfig(cfg)
	if err != nil {
		return
	}
	defer func() { _ = closeGorm(db) }()
	repo := store.NewRepository(db)
	prev, perr := repo.GetLatestSession()
	if perr != nil || prev == nil || !prev.CodePathOK || strings.TrimSpace(prev.CodeRootPath) == "" {
		return
	}
	if strings.TrimSpace(parsed.CodePath) == "" {
		parsed.CodePath = prev.CodeRootPath
		if strings.TrimSpace(parsed.TargetRaw) == "" {
			parsed.TargetRaw = prev.TargetRaw
		}
		r.AddToTimeline("[ssa_discovery]", "Preflight: merged Code path / Target from latest session uuid="+prev.UUID)
	} else if strings.TrimSpace(parsed.TargetRaw) == "" {
		parsed.TargetRaw = prev.TargetRaw
		r.AddToTimeline("[ssa_discovery]", "Preflight: merged Target from latest session uuid="+prev.UUID)
	}
}

// TryMergeParsedFromLatestSQLite 保留旧名；行为同 TryMergeParsedFromLatestSession。
func TryMergeParsedFromLatestSQLite(r aicommon.AIInvokeRuntime, workDir string, parsed *ParsedUserInput) {
	TryMergeParsedFromLatestSession(r, workDir, parsed)
}

// EnrichParsedForFullPipeline 在同 workDir 下做 SQLite 回补，并用 LiteForge 泛化提取 code path / target / language / auth 等字段。
func EnrichParsedForFullPipeline(ctx context.Context, r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, workDir string, parsed *ParsedUserInput) (*ParsedUserInput, error) {
	if parsed == nil {
		return nil, utils.Error("nil parsed")
	}
	cur := parsed
	TryMergeParsedFromLatestSQLite(r, workDir, cur)
	if r == nil || task == nil {
		return cur, nil
	}
	return EnrichParsedWithAIExtract(ctx, r, task.GetUserInput(), cur)
}
