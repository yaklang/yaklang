package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	loop_fast_context "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_fast_context"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

func buildFastContextIsolatedAction(
	r aicommon.AIInvokeRuntime,
	state *model.AuditState,
	category model.VulnCategory,
	scan *ScanState,
) reactloops.ReActLoopOption {
	// fast_context is discovery-only (parallel files_with_matches). Phase B blocks it here;
	// phase-B trace uses single grep in content mode (phase2_grep_guard.go).
	return reactloops.WithOverrideLoopAction(&reactloops.LoopAction{
		ActionType: schema.AI_REACT_LOOP_NAME_FAST_CONTEXT,
		Description: "Run isolated FastContext sub-loop: parallel files_with_matches / find_file search for this vulnerability category. " +
			"Returns a candidate file index (not auto-locked). In phase A: read_file spot-check candidates, then lock_target_files; repeat or enter phase B.",
		Options: []aitool.ToolOption{
			aitool.WithStringParam("query",
				aitool.WithParam_Description("What to locate; defaults to category name + sink hints")),
			aitool.WithStringParam("reference_material",
				aitool.WithParam_Description("Optional override; default is lean context + optional-read catalog (recon_report path). Sub-agent reads files on demand via require_tool.")),
		},
		ActionHandler: func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			scan.mu.Lock()
			inSearch := scan.Phase == ScanPhaseSearch
			scan.mu.Unlock()
			if !inSearch {
				op.Feedback("[错误] fast_context 仅可在阶段A（关键词搜索）使用。")
				op.Continue()
				return
			}

			query := strings.TrimSpace(action.GetString("query"))
			if query == "" {
				query = BuildFastContextQuery(category)
			}
			refMaterial := strings.TrimSpace(action.GetString("reference_material"))
			if refMaterial == "" {
				refMaterial = BuildFastContextReferenceMaterial(state, category)
			}

			attempt := scan.BumpFastContextAttempt()
			workDir := strings.TrimSpace(state.ProjectPath)
			if workDir == "" {
				if cfg := r.GetConfig(); cfg != nil {
					workDir = cfg.GetOrCreateWorkDir()
				}
			}

			log.Infof("[CodeAudit/Phase2/%s] fast_context start query_len=%d", category.ID, len(query))
			r.AddToTimeline("[FAST_CONTEXT_START]",
				fmt.Sprintf("[Phase2/%s] 启动 FastContext 并行搜索（子 loop timeline 隔离）", category.ID))

			result := loop_fast_context.RunFastContextSearch(r, loop.GetCurrentTask(), loop_fast_context.SearchInput{
				Query:             query,
				WorkDir:           workDir,
				ReferenceMaterial: refMaterial,
			})
			if result.Error != nil {
				op.Feedback(fmt.Sprintf("[fast_context 错误] %v\n可改用手动 grep / grep_files_batch 补充。", result.Error))
				op.Continue()
				return
			}

			paths := loop_fast_context.FilterAuditCandidatePaths(result.FilePaths)
			paths = loop_fast_context.PrioritizeAuditCandidatePaths(paths, phase2MaxDiscoveryCandidates)
			added := scan.AddDiscoveryCandidates(paths)

			quality := EvaluateDiscoveryQuality(category, len(paths), attempt)
			scan.setLastDiscoveryQuality(quality.Level)

			log.Infof("[CodeAudit/Phase2/%s] fast_context registered %d new discovery candidates (total %d)",
				category.ID, added, scan.DiscoveryCandidateCount())
			reactloops.EmitStatus(loop, fmt.Sprintf("阶段A：处理 %d 个 fast_context 候选 / Phase A: %d candidates to review", len(paths), len(paths)))
			emitPhase2FastContextResult(loop, category, paths, result.Markdown)

			if result.Markdown != "" {
				r.AddToTimeline("[FAST_CONTEXT_SUMMARY]", utils.ShrinkString(result.Markdown, 2048))
			}
			r.AddToTimeline("[FAST_CONTEXT_RESULT]",
				fmt.Sprintf("[Phase2/%s] FastContext 完成：索引 %d 个候选文件（未自动纳入目标，待 read + lock）",
					category.ID, len(paths)))

			feedback := result.Markdown
			if feedback == "" && result.Report != nil {
				feedback = result.Report.FormatUserMarkdown()
			}
			feedback += fmt.Sprintf(
				"\n\n---\n**候选文件 %d 个**（广度优先：建议 read 后 lock，也可批量 lock 或 done=true 自动纳入全部）：\n%s\n"+
					"**推荐下一步**：\n"+
					"1. （可选）`read_file` 抽查高优先级候选\n"+
					"2. `lock_target_files(target_files=[...], done=false)` 批量纳入，或 `done=true` 自动纳入剩余候选进阶段B\n"+
					"3. 阶段B 对可疑数据流 `add_finding`（confidence≥4），Phase3 会验证\n"+
					"（当前已锁定目标：%d 个；未纳入候选：%d 个；fast_context 第 %d 次）",
				len(paths), formatFastContextCandidatePaths(paths), scan.TargetFileCount(), len(scan.UnresolvedDiscovery()), attempt,
			)
			feedback += FormatDeepDiscoveryGuidance(category, quality)
			op.Feedback(feedback)
			op.Continue()
		},
	})
}

func formatFastContextCandidatePaths(paths []string) string {
	if len(paths) == 0 {
		return "  （无候选文件）\n"
	}
	const maxShow = 40
	var b strings.Builder
	for i, p := range paths {
		if i >= maxShow {
			b.WriteString(fmt.Sprintf("  ... 另有 %d 个文件未列出\n", len(paths)-maxShow))
			break
		}
		b.WriteString(fmt.Sprintf("  %d. %s\n", i+1, p))
	}
	return b.String()
}
