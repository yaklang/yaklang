package model

import (
	"fmt"
	"strings"
)

// scan_coverage.go 提供 Phase2 类别扫描覆盖状态的结构化披露：
// - BuildCategoryCoverageTable: Go 层渲染覆盖表（注入报告 prompt + 附录兜底共用）
// - AppendMissingCoverageAppendix: 报告写完后，若存在未完成类别则强制追加机器
//   准确的覆盖附录（不依赖 AI 自觉披露）
//
// 关键词: ScanObservation.Status, 覆盖范围, 报告披露, 附录兜底

// IncompleteScanObservations 返回所有非 completed 的 observation（partial/not_run
// 及旧数据中 Status 为空的条目——旧快照恢复的场景按未知处理，保守视为未完成）。
func IncompleteScanObservations(state *AuditState) []*ScanObservation {
	if state == nil {
		return nil
	}
	var out []*ScanObservation
	for _, obs := range state.GetScanObservations() {
		if obs == nil {
			continue
		}
		if obs.Status != ScanStatusCompleted {
			out = append(out, obs)
		}
	}
	return out
}

// formatScanStatusLabel 把 Status 翻译成报告用中文标签。
func formatScanStatusLabel(status string) string {
	switch status {
	case ScanStatusCompleted:
		return "已完成"
	case ScanStatusPartial:
		return "部分完成（中断后系统收尾）"
	case ScanStatusNotRun:
		return "未运行"
	default:
		return "未知（状态缺失）"
	}
}

// BuildCategoryCoverageTable 渲染全部类别扫描覆盖状态表（Markdown 表格）。
// includeAll=false 时只列未完成类别（附录用）；true 时列全部（prompt 注入用）。
func BuildCategoryCoverageTable(state *AuditState, includeAll bool) string {
	if state == nil {
		return ""
	}
	observations := state.GetScanObservations()
	if len(observations) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("| 类别 | 状态 | 已审文件/目标文件 | 停止原因 |\n")
	b.WriteString("|------|------|------------------|----------|\n")
	for _, obs := range observations {
		if obs == nil {
			continue
		}
		if !includeAll && obs.Status == ScanStatusCompleted {
			continue
		}
		name := obs.CategoryName
		if name == "" {
			name = obs.CategoryID
		}
		coverage := ""
		if obs.TargetFiles > 0 {
			coverage = fmt.Sprintf("%d/%d", obs.AuditedFiles, obs.TargetFiles)
		} else {
			coverage = "-"
		}
		stop := obs.StopReason
		if stop == "" {
			stop = "-"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | `%s` |\n", name, formatScanStatusLabel(obs.Status), coverage, stop))
	}
	return b.String()
}

// AppendMissingCoverageAppendix 在报告末尾追加"Phase2 类别扫描覆盖状态"附录。
// 仅当存在 Status != completed 的 observation 时追加（机器准确，不依赖 AI 自觉）。
func AppendMissingCoverageAppendix(report string, state *AuditState) (string, bool) {
	incomplete := IncompleteScanObservations(state)
	if len(incomplete) == 0 {
		return report, false
	}
	table := BuildCategoryCoverageTable(state, false)
	var b strings.Builder
	b.WriteString(strings.TrimRight(report, "\n"))
	b.WriteString("\n\n---\n\n## 附录：Phase2 类别扫描覆盖状态（系统自动补录）\n\n")
	b.WriteString(fmt.Sprintf("> 以下 %d 个类别的扫描未完整完成（被中断后由系统收尾或从未运行），", len(incomplete)))
	b.WriteString("本附录由系统在 Go 层根据扫描状态机器准确地生成，用于如实披露审计覆盖范围。\n\n")
	b.WriteString(table)
	b.WriteString("\n> 上述类别的覆盖缺口意味着对应风险类别的审计结论可能不完整，")
	b.WriteString("建议结合修复二恢复续扫机制重跑审计，或在解读报告时将这些类别视为低置信度。\n")
	return b.String(), true
}