// Package loop_code_security_audit — phase2_grep_guard.go
//
// Phase-B tool policy for grep: allow **trace** (content-mode, scoped) but block **discovery**
// (files_with_matches, find_file, tree, fast_context).
//
// Design background
// -----------------
// Phase A uses grep in files_with_matches mode to expand the lock list. Phase B must audit
// locked targets and build add_finding.data_flow (often requires symbol/caller search).
// A blanket phase-B grep ban hurts audit quality; unlimited grep causes agents to never
// mark_file_done. This file implements the middle ground.
//
// Hook pipeline (registered in buildSingleCategoryScanLoop)
// ---------------------------------------------------------
// invoke_toolcall.go runs, per tool invocation while this category loop is active:
//  1. ToolInvokeGuard  — veto before execution (discovery tools, grep mode/path/budget)
//  2. ToolInvokeParamsMutator — rewrite params (read_file clamp, grep content/limit) + bump counters
//
// Guard runs before mutator. Grep budget is checked in the guard using the pre-bump count;
// the mutator bumps after the guard allows the call.
//
// See also: phase2_guards.go (read_file spin guards), phase2_read_file_guard.go,
// prompts/phase2_scan_instruction.txt.
package phase2

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// phase2MaxPhaseBTraceGrepsPerFile caps content-mode greps charged to one target file
	// before mark_file_done. Cross-file greps (project root) still charge the primary remaining file.
	phase2MaxPhaseBTraceGrepsPerFile = 8

	// phase2MaxPhaseBTraceGrepLimit bounds grep result lines in phase B to keep feedback small.
	phase2MaxPhaseBTraceGrepLimit = 50
)

// phase2DiscoveryOnlyTools are blocked entirely in phase B (not grep — trace grep is separate).
var phase2DiscoveryOnlyTools = map[string]bool{
	"find_file": true,
	"tree":      true,
}

// isDiscoveryGrepOutputMode reports grep modes that enumerate new files rather than show
// matching lines for call-chain tracing.
func isDiscoveryGrepOutputMode(mode string) bool {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "files_with_matches", "files-with-matches", "count":
		return true
	default:
		return false
	}
}

// grepParamOutputMode reads output-mode from invoke params (hyphen or underscore key).
func grepParamOutputMode(params aitool.InvokeParams) string {
	if params == nil {
		return ""
	}
	if mode := strings.TrimSpace(utils.InterfaceToString(params["output-mode"])); mode != "" {
		return mode
	}
	return strings.TrimSpace(utils.InterfaceToString(params["output_mode"]))
}

// isPhaseBGrepPathAllowed restricts trace grep to paths that cannot expand the audit scope
// arbitrarily: project root, a locked target file, or a directory on the path to a target.
func isPhaseBGrepPathAllowed(grepPath string, scan *ScanState, projectPath string) bool {
	grepPath = filepath.Clean(strings.TrimSpace(grepPath))
	if grepPath == "" {
		return false
	}
	if projectPath != "" {
		cleanProject := filepath.Clean(strings.TrimSpace(projectPath))
		if grepPath == cleanProject {
			return true
		}
	}
	for _, target := range scan.CollectedTargetFiles() {
		target = filepath.Clean(target)
		if grepPath == target {
			return true
		}
		// grepPath is an ancestor directory of a locked target.
		if strings.HasPrefix(target, grepPath+string(filepath.Separator)) {
			return true
		}
		if filepath.Dir(target) == grepPath {
			return true
		}
	}
	return false
}

// resolvePhaseBGrepBudgetFile picks which locked target file is charged for a trace grep.
// Direct greps on a remaining target file charge that file; project-wide greps charge the
// first remaining file (the one the agent should be auditing now).
func resolvePhaseBGrepBudgetFile(scan *ScanState, grepPath string) string {
	remaining := scan.RemainingFiles()
	if len(remaining) == 0 {
		return ""
	}
	grepPath = filepath.Clean(strings.TrimSpace(grepPath))
	for _, target := range remaining {
		cleanTarget := filepath.Clean(target)
		if grepPath == cleanTarget {
			return target
		}
	}
	return remaining[0]
}

// buildPhase2PhaseBDiscoveryToolGuard blocks find_file/tree in phase B.
// fast_context is blocked in its action handler; grep is handled by buildPhase2PhaseBGrepGuard.
func buildPhase2PhaseBDiscoveryToolGuard(scan *ScanState) reactloops.ToolInvokeGuard {
	return func(toolName string, _ aitool.InvokeParams) (bool, string) {
		if scan.CurrentPhase() != ScanPhaseAudit {
			return true, ""
		}
		if !phase2DiscoveryOnlyTools[toolName] {
			return true, ""
		}
		remaining := scan.RemainingFiles()
		next := ""
		if len(remaining) > 0 {
			next = remaining[0]
		}
		return false, fmt.Sprintf(
			"[错误] 当前处于阶段B（逐文件审计），禁止调用 %s 等 discovery 类工具。\n"+
				"阶段B 允许：read_file、**content 模式 trace grep**（在目标文件/父目录/项目根内溯源）、read_recon_notes、add_finding、mark_file_done。\n"+
				"待审计文件（%d 个）：\n%s\n"+
				"请从第一个待审文件开始：%s",
			toolName,
			len(remaining),
			formatPathListForFeedback(remaining, 20),
			next,
		)
	}
}

// buildPhase2PhaseBGrepGuard enforces trace-only grep in phase B: content mode, scoped path,
// per-target budget. Phase A is unaffected (returns allow immediately).
func buildPhase2PhaseBGrepGuard(scan *ScanState, projectPath string) reactloops.ToolInvokeGuard {
	return func(toolName string, params aitool.InvokeParams) (bool, string) {
		if toolName != "grep" || scan.CurrentPhase() != ScanPhaseAudit {
			return true, ""
		}
		if len(params) == 0 {
			return true, ""
		}

		if isDiscoveryGrepOutputMode(grepParamOutputMode(params)) {
			return false, "[错误] 阶段B 禁止 discovery 模式 grep（output-mode=files_with_matches/count）。" +
				"溯源请使用 **content 模式**（默认或显式 output-mode=content），在目标文件、其父目录或项目根内搜索符号/调用链。"
		}

		path := strings.TrimSpace(utils.InterfaceToString(params["path"]))
		if !isPhaseBGrepPathAllowed(path, scan, projectPath) {
			return false, fmt.Sprintf(
				"[错误] 阶段B trace grep 的 path 必须落在当前目标文件、其祖先目录或项目根内。\n"+
					"path=%q 不在允许范围。待审计文件：\n%s",
				path,
				formatPathListForFeedback(scan.RemainingFiles(), 20),
			)
		}

		budgetFile := resolvePhaseBGrepBudgetFile(scan, path)
		if budgetFile != "" && scan.PhaseBGrepCount(budgetFile) >= phase2MaxPhaseBTraceGrepsPerFile {
			return false, formatPhaseBGrepBudgetFeedback(budgetFile, scan)
		}
		return true, ""
	}
}

// buildPhase2PhaseBGrepParamsMutator normalizes phase-B grep params and bumps trace counters.
// Must run after guards pass (registered as a separate WithToolInvokeParamsMutator).
func buildPhase2PhaseBGrepParamsMutator(scan *ScanState) reactloops.ToolInvokeParamsMutator {
	return func(toolName string, params aitool.InvokeParams) aitool.InvokeParams {
		if toolName != "grep" || scan.CurrentPhase() != ScanPhaseAudit || len(params) == 0 {
			return params
		}

		out := make(aitool.InvokeParams, len(params))
		for k, v := range params {
			out[k] = v
		}

		mode := strings.ToLower(grepParamOutputMode(out))
		if mode == "" || mode == "auto" || isDiscoveryGrepOutputMode(mode) {
			out["output-mode"] = "content"
			delete(out, "output_mode")
		}

		limit := int(utils.InterfaceToInt(out["limit"]))
		if limit <= 0 || limit > phase2MaxPhaseBTraceGrepLimit {
			out["limit"] = phase2MaxPhaseBTraceGrepLimit
		}

		path := strings.TrimSpace(utils.InterfaceToString(out["path"]))
		if budgetFile := resolvePhaseBGrepBudgetFile(scan, path); budgetFile != "" {
			scan.BumpPhaseBGrep(budgetFile)
		}
		return out
	}
}

func formatPhaseBGrepBudgetFeedback(file string, scan *ScanState) string {
	return fmt.Sprintf(
		"[错误] 阶段B 已对当前文件 %q 执行 %d 次 trace grep（上限 %d）。\n"+
			"请基于已有 grep/read 结果完成 data_flow 分析，然后 **mark_file_done**。\n"+
			"若仍需跨文件证据，优先 read_recon_notes 或 read_file 相关路径。\n"+
			"当前待审计文件：\n%s",
		file,
		scan.PhaseBGrepCount(file),
		phase2MaxPhaseBTraceGrepsPerFile,
		formatPathListForFeedback(scan.RemainingFiles(), 20),
	)
}
