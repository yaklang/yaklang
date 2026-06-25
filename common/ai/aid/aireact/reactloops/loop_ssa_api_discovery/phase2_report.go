package loop_ssa_api_discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/store"
	"github.com/yaklang/yaklang/common/log"
)

// finalizePhase1DiscoveryArtifacts exports discovery_snapshot.json and writes discovery_report.md
// programmatically at Phase1 end (no ReAct report loop).
func finalizePhase1DiscoveryArtifacts(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) error {
	if rt == nil || rt.Repo == nil || rt.Session == nil {
		return fmt.Errorf("nil runtime")
	}
	snapPath, err := ExportDiscoverySnapshotJSON(rt)
	if err != nil {
		return err
	}
	dir := filepath.Join(rt.WorkDir, store.SubDirName())
	reportPath := filepath.Join(dir, "discovery_report.md")
	body := writeDiscoveryReportProgrammatic(rt, snapPath)
	if err := os.WriteFile(reportPath, []byte(body), 0o644); err != nil {
		return err
	}
	if pl != nil {
		pl.SetDiscoveryReportPath(reportPath)
	}
	if r != nil {
		if s := strings.TrimSpace(body); s != "" {
			r.EmitResultAfterStream(s)
		}
		r.AddToTimeline("[ssa_phase1]", fmt.Sprintf("discovery artifacts: snapshot=%s report=%s", snapPath, reportPath))
	}
	return nil
}

// ensurePhase1DiscoveryArtifacts regenerates snapshot/report when resuming past Phase1 without rerunning it.
func ensurePhase1DiscoveryArtifacts(r aicommon.AIInvokeRuntime, rt *Runtime, pl *PipelineState) error {
	if rt == nil {
		return fmt.Errorf("nil runtime")
	}
	reportPath := filepath.Join(rt.WorkDir, store.SubDirName(), "discovery_report.md")
	snapPath := store.DiscoverySnapshotPath(rt.WorkDir)
	if err := fileExistsMinBytes(snapPath, 32); err == nil {
		if err2 := fileExistsMinBytes(reportPath, minDiscoveryReportBytes); err2 == nil {
			if pl != nil {
				pl.SetDiscoveryReportPath(reportPath)
			}
			return nil
		}
	}
	log.Infof("ssa_api_discovery: regenerating discovery artifacts (resume checkpoint)")
	return finalizePhase1DiscoveryArtifacts(r, rt, pl)
}

func writeDiscoveryReportProgrammatic(rt *Runtime, snapPath string) string {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return "# API 与架构分析（自动生成）\n\n（无会话数据）\n"
	}
	s := rt.Session
	total, verified, _ := rt.Repo.CountVerifiedHttpApis(s.ID)
	eps, _ := rt.Repo.ListHttpEndpoints(s.ID)
	vha, _ := rt.Repo.ListVerifiedHttpApis(s.ID)

	var b strings.Builder
	fmt.Fprintf(&b, "# [阶段 2/%d] %s 完成报告\n\n", pipelineSummaryTotalPhases, phase2ReportHeadingTitle)
	b.WriteString("## 执行摘要\n\n")
	fmt.Fprintf(&b, "- Session: `%s`\n", s.UUID)
	fmt.Fprintf(&b, "- 代码根: `%s`\n", s.CodeRootPath)
	fmt.Fprintf(&b, "- 靶机: `%s`（可达=%v）\n", s.TargetRaw, s.TargetReachable)
	fmt.Fprintf(&b, "- SSA 编译: %s\n", map[bool]string{true: "ok", false: "failed"}[s.SSACompileOK])
	fmt.Fprintf(&b, "- **verified_http_apis**: verified=true **%d** 条，记录总数 **%d** 条（含 rejected）\n", verified, total)
	fmt.Fprintf(&b, "- **http_endpoints** 候选: **%d** 条\n", len(eps))
	b.WriteString("- 说明: 本报告由 Phase1 结束时程序化生成；HTTP 确认真源为 DB 表 `verified_http_apis`。\n\n")

	if tech, err := loadTechArchitectureRecord(rt.WorkDir); err == nil && tech != nil {
		b.WriteString("## 架构概览\n\n")
		if tech.SystemSummary != "" {
			fmt.Fprintf(&b, "%s\n\n", tech.SystemSummary)
		}
		if len(tech.ModuleLayout.Modules) > 0 {
			b.WriteString("| 模块 | 角色 |\n|------|------|\n")
			for _, m := range tech.ModuleLayout.Modules {
				fmt.Fprintf(&b, "| %s | %s |\n", m.Name, m.Role)
			}
			b.WriteString("\n")
		}
	}

	b.WriteString("## HTTP 攻击面（verified=true）\n\n")
	if verified == 0 {
		b.WriteString("（无 verified=true 记录；详见 Phase1 工件与 rejected 条目。）\n\n")
	} else {
		b.WriteString("| Method | Path Pattern | Source | Verdict |\n|--------|--------------|--------|----------|\n")
		shown := 0
		for _, row := range vha {
			if !row.Verified {
				continue
			}
			verdict := strings.TrimSpace(row.VerdictReason)
			if verdict == "" {
				verdict = row.Source
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s | %s |\n",
				row.Method, row.PathPattern, row.Source, strings.ReplaceAll(verdict, "|", "/"))
			shown++
			if shown >= 120 {
				fmt.Fprintf(&b, "\n*（另有 %d 条 verified=true 未列出）*\n\n", verified-shown)
				break
			}
		}
		if shown > 0 && shown < verified {
			b.WriteString("\n")
		}
	}

	rejected := total - verified
	if rejected > 0 {
		fmt.Fprintf(&b, "## 未确认 / 已拒绝（verified=false）\n\n共 **%d** 条（如 `not_probed`、鉴权阻塞等）。完整列表见 `discovery_snapshot.json` 与 Phase1 报告。\n\n", rejected)
	}

	b.WriteString("## 数据引用\n\n")
	fmt.Fprintf(&b, "- 快照: `%s`\n", snapPath)
	fmt.Fprintf(&b, "- Phase1 报告: `%s`\n", store.Phase1DiscoveryReportPath(rt.WorkDir))
	b.WriteString("- 后续: Phase3 SyntaxFlow 静态扫描 → Phase4 动态验证 → Phase5 总报告\n")

	out := b.String()
	if len(strings.TrimSpace(out)) < minDiscoveryReportBytes {
		return generateDiscoveryReportFallback(rt, snapPath)
	}
	return out
}

func generateDiscoveryReportFallback(rt *Runtime, snapPath string) string {
	if rt == nil || rt.Session == nil || rt.Repo == nil {
		return "# API 与架构分析（自动生成兜底）\n\n（无会话数据）\n"
	}
	s := rt.Session
	total, verified, _ := rt.Repo.CountVerifiedHttpApis(s.ID)
	eps, _ := rt.Repo.ListHttpEndpoints(s.ID)
	return fmt.Sprintf(`# [阶段 2/%d] API 与架构分析 完成报告

## 阶段概览
本报告由编排器自动产出（程序化摘要）。

## 执行摘要
- Session: %s
- 代码根: %s
- 靶机: %s（可达=%v）
- SSA: %s
- verified_http_apis verified=true: %d / 记录总数 %d
- http_endpoints 候选: %d

## 数据引用
- 快照: %s
- HTTP 确认真源: verified_http_apis（discovery_read_session_data）

> 完整分析见 workdir/%s/discovery_report.md（本文件）与 Phase1 工件。
`, pipelineSummaryTotalPhases, s.UUID, s.CodeRootPath, s.TargetRaw, s.TargetReachable,
		map[bool]string{true: "ok", false: "failed"}[s.SSACompileOK],
		verified, total, len(eps), snapPath, store.SubDirName())
}
