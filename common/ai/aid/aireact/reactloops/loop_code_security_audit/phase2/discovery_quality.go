package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"strings"
)

// DiscoveryQuality summarizes whether a fast_context run is worth trusting for Phase A.
type DiscoveryQuality struct {
	Level   string // good | weak | empty
	Reason  string
	Attempt int
}

// EvaluateDiscoveryQuality scores a discovery result for follow-up decisions.
func EvaluateDiscoveryQuality(category model.VulnCategory, candidateCount, attempt int) DiscoveryQuality {
	q := DiscoveryQuality{Attempt: attempt}
	switch {
	case candidateCount == 0:
		q.Level = "empty"
		q.Reason = "fast_context 未返回任何候选文件"
	case candidateCount < 3 && isFlowCentricCategory(category.ID):
		q.Level = "weak"
		q.Reason = fmt.Sprintf("仅 %d 个候选，对「多跳数据流」类别可能不足", candidateCount)
	case candidateCount < 2:
		q.Level = "weak"
		q.Reason = fmt.Sprintf("仅 %d 个候选，覆盖可能不足", candidateCount)
	default:
		q.Level = "good"
		q.Reason = fmt.Sprintf("返回 %d 个候选", candidateCount)
	}
	return q
}

// isFlowCentricCategory marks categories that typically need source+sink pairing.
func isFlowCentricCategory(categoryID string) bool {
	switch categoryID {
	case "xss_injection", "path_traversal", "auth_bypass", "xxe_ssrf":
		return true
	default:
		return false
	}
}

// FormatDeepDiscoveryGuidance tells the parent Phase2 agent how to recover from weak discovery.
func FormatDeepDiscoveryGuidance(category model.VulnCategory, quality DiscoveryQuality) string {
	if quality.Level == "good" {
		return ""
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n\n---\n**发现质量: %s** — %s\n", quality.Level, quality.Reason))
	b.WriteString("**建议深度调研后再试**（父 loop Phase A，非 fast_context 内长篇思考）：\n")
	b.WriteString("1. 调用 `read_recon_notes()` 获取路由/模块/框架细节\n")
	b.WriteString("2. 根据技术栈与模块目录，重写 `fast_context` 的 `query`（明确 Source 与 Sink 两类 pattern）\n")
	if isFlowCentricCategory(category.ID) {
		b.WriteString("3. 本类别为**数据流型**：query 中应分别描述「用户输入来源」与「渲染/输出 Sink」，必要时两轮 fast_context\n")
	} else {
		b.WriteString("3. 再次调用 `fast_context`，在 query/reference_material 中附上 recon 关键摘录\n")
	}
	b.WriteString("4. 仍不足时可手动 `grep` / `find_file` 扩搜，再 `lock_target_files`\n")
	if quality.Attempt >= 2 {
		b.WriteString("\n已多次 fast_context 仍偏弱：优先手动 grep 关键目录，避免空转。\n")
	}
	return b.String()
}

// BuildFastContextQuery builds a structured default query (parent may override).
func BuildFastContextQuery(category model.VulnCategory) string {
	if isFlowCentricCategory(category.ID) {
		return fmt.Sprintf(
			"【%s / %s】数据流型发现：分别搜索 (1) 用户可控输入来源 (2) 未编码输出/渲染 Sink；"+
				"结合技术栈推导多组 grep pattern；必要时 read_file 打开 recon_report 确认模块目录后再搜。",
			category.Name, category.ID,
		)
	}
	return fmt.Sprintf(
		"【%s / %s】定位所有典型 Sink 与相关数据流文件；覆盖框架变体；宁多勿少。",
		category.Name, category.ID,
	)
}

// BumpFastContextAttempt increments per-category fast_context call count on scan state.
func (s *ScanState) BumpFastContextAttempt() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FastContextAttempts++
	return s.FastContextAttempts
}

// FastContextAttemptCount returns how many times fast_context was invoked in this category scan.
func (s *ScanState) FastContextAttemptCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.FastContextAttempts
}

// LastDiscoveryQualityLevel returns the last discovery quality level string.
func (s *ScanState) LastDiscoveryQualityLevel() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.LastDiscoveryQuality
}

func (s *ScanState) setLastDiscoveryQuality(level string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LastDiscoveryQuality = level
}

// FormatDiscoveryQualityWarningForReactive surfaces weak/empty discovery to Phase2 reactive data.
func FormatDiscoveryQualityWarningForReactive(category model.VulnCategory, scan *ScanState) string {
	if scan == nil {
		return ""
	}
	level := scan.LastDiscoveryQualityLevel()
	if level == "" || level == "good" {
		return ""
	}
	attempt := scan.FastContextAttemptCount()
	q := DiscoveryQuality{Level: level, Attempt: attempt, Reason: "见上步 fast_context 反馈"}
	return strings.TrimSpace(FormatDeepDiscoveryGuidance(category, q))
}
