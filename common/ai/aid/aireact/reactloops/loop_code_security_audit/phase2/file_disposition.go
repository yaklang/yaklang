// Package loop_code_security_audit — phase2_file_disposition.go
//
// Enforces that every locked target file has an explicit attribution before the
// category scan can complete: either add_finding (disposition=finding) or
// mark_file_done(disposition=not_vul).
package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"path/filepath"
	"strings"
)

const (
	// FileDispositionFinding means the file has at least one add_finding for this category.
	FileDispositionFinding = "finding"
	// FileDispositionNotVul means the file was audited and has no vulnerability in this category.
	FileDispositionNotVul = "not_vul"
)

// NoteFinding records that a target file has an add_finding for the current category scan.
func (s *ScanState) NoteFinding(absPath string) {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FilesWithFinding == nil {
		s.FilesWithFinding = make(map[string]bool)
	}
	s.FilesWithFinding[absPath] = true
}

// HasFindingNoted reports whether add_finding was recorded for the target path.
func (s *ScanState) HasFindingNoted(absPath string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.FilesWithFinding[absPath]
}

// GetFileDisposition returns the disposition recorded at mark_file_done (empty if unset).
func (s *ScanState) GetFileDisposition(absPath string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.FileDisposition[absPath]
}

// MarkFileDoneWithDisposition marks audit complete and records file attribution.
func (s *ScanState) MarkFileDoneWithDisposition(filePath, disposition string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.FileDisposition == nil {
		s.FileDisposition = make(map[string]string)
	}
	s.FileDisposition[filePath] = disposition
	s.AuditedFiles[filePath] = true
	remaining := 0
	for _, f := range s.TargetFiles {
		if !s.AuditedFiles[f] {
			remaining++
		}
	}
	return remaining
}

func normalizeFileDisposition(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case FileDispositionFinding, "with_finding", "vulnerable", "vuln":
		return FileDispositionFinding
	case FileDispositionNotVul, "not_vulnerable", "safe", "no_vuln":
		return FileDispositionNotVul
	default:
		return ""
	}
}

func resolveTargetAbsPath(projectRoot string, scan *ScanState, findingRelPath string) string {
	if scan == nil || findingRelPath == "" {
		return ""
	}
	rel := filepath.ToSlash(strings.TrimPrefix(findingRelPath, "./"))
	for _, target := range scan.CollectedTargetFiles() {
		if pathMatchesTarget(projectRoot, target, rel) {
			return target
		}
	}
	return ""
}

func pathMatchesTarget(projectRoot, absPath, relPath string) bool {
	absNorm := filepath.ToSlash(absPath)
	relNorm := filepath.ToSlash(strings.TrimPrefix(relPath, "./"))
	if relNorm == "" {
		return false
	}
	if projectRoot != "" {
		if r, err := filepath.Rel(projectRoot, absPath); err == nil {
			r = filepath.ToSlash(strings.TrimPrefix(r, "./"))
			if r == relNorm {
				return true
			}
		}
	}
	return strings.HasSuffix(absNorm, "/"+relNorm) || absNorm == relNorm
}

func fileHasCategoryFinding(state *model.AuditState, scan *ScanState, categoryID, absPath, projectRoot string) bool {
	if scan != nil && scan.HasFindingNoted(absPath) {
		return true
	}
	return hasFindingForAbsPath(state, categoryID, absPath, projectRoot)
}

func validateMarkFileDoneDisposition(
	scan *ScanState,
	state *model.AuditState,
	categoryID, filePath, projectRoot, disposition string,
) (bool, string) {
	disp := normalizeFileDisposition(disposition)
	if disp == "" {
		return false, formatMarkFileDoneDispositionRequiredFeedback(filePath)
	}
	hasFinding := fileHasCategoryFinding(state, scan, categoryID, filePath, projectRoot)
	switch disp {
	case FileDispositionFinding:
		if !hasFinding {
			return false, formatMarkFileDoneFindingRequiredFeedback(filePath)
		}
	case FileDispositionNotVul:
		if hasFinding {
			return false, formatMarkFileDoneNotVulConflictFeedback(filePath)
		}
	default:
		return false, formatMarkFileDoneDispositionRequiredFeedback(filePath)
	}
	return true, ""
}

// validateAllTargetsAttributed checks every locked file has finding or not_vul attribution.
func validateAllTargetsAttributed(scan *ScanState, state *model.AuditState, categoryID, projectRoot string) (bool, string) {
	if scan == nil {
		return true, ""
	}
	targets := scan.CollectedTargetFiles()
	if len(targets) == 0 {
		return false, "[错误] 目标文件列表为空，无法 complete_scan。"
	}

	var issues []string
	for _, f := range targets {
		if !scan.IsFileAudited(f) {
			if fileHasCategoryFinding(state, scan, categoryID, f, projectRoot) {
				issues = append(issues, fmt.Sprintf("  - %s → 已 add_finding，须 mark_file_done(disposition=%q)", f, FileDispositionFinding))
			} else {
				issues = append(issues, fmt.Sprintf("  - %s → 未归属：add_finding 或 mark_file_done(disposition=%q, audit_summary=...)", f, FileDispositionNotVul))
			}
			continue
		}
		disp := scan.GetFileDisposition(f)
		switch disp {
		case FileDispositionFinding:
			if !fileHasCategoryFinding(state, scan, categoryID, f, projectRoot) {
				issues = append(issues, fmt.Sprintf("  - %s → disposition=finding 但无 add_finding 记录", f))
			}
		case FileDispositionNotVul:
			if fileHasCategoryFinding(state, scan, categoryID, f, projectRoot) {
				issues = append(issues, fmt.Sprintf("  - %s → disposition=not_vul 但与 add_finding 冲突", f))
			}
		default:
			issues = append(issues, fmt.Sprintf("  - %s → 已 mark 但缺少归属 disposition", f))
		}
	}
	if len(issues) == 0 {
		return true, ""
	}
	var b strings.Builder
	b.WriteString("[错误] 以下目标文件尚未完成归属（每个 lock 的文件必须：add_finding + mark(disposition=finding)，或 mark(disposition=not_vul)）：\n")
	b.WriteString(strings.Join(issues, "\n"))
	b.WriteString("\n\n全部文件归属完成后才能 complete_scan。")
	return false, b.String()
}

func formatMarkFileDoneDispositionRequiredFeedback(filePath string) string {
	return fmt.Sprintf(
		"[错误] mark_file_done 缺少必填参数 disposition。\n"+
			"每个 lock 的文件必须有明确归属：\n"+
			"  - 有漏洞：先 add_finding，再 mark_file_done(file_path=%q, disposition=%q, audit_summary=...)\n"+
			"  - 无漏洞：mark_file_done(file_path=%q, disposition=%q, audit_summary=\"本类别无漏洞/已防护\")",
		filePath, FileDispositionFinding, filePath, FileDispositionNotVul,
	)
}

func formatMarkFileDoneFindingRequiredFeedback(filePath string) string {
	return fmt.Sprintf(
		"[错误] disposition=%q 但 %q 尚无 add_finding。\n"+
			"请先调用 add_finding(..., file=相对项目根路径)，再 mark_file_done(disposition=%q)。\n"+
			"若确认无本类别漏洞，请改用 disposition=%q。",
		FileDispositionFinding, filePath, FileDispositionFinding, FileDispositionNotVul,
	)
}

func formatMarkFileDoneNotVulConflictFeedback(filePath string) string {
	return fmt.Sprintf(
		"[错误] disposition=%q 与已有 add_finding 冲突（%q）。\n"+
			"该文件已提交 finding，请使用 mark_file_done(disposition=%q)。",
		FileDispositionNotVul, filePath, FileDispositionFinding,
	)
}

func formatRemainingFileAttributionHint(
	scan *ScanState,
	state *model.AuditState,
	categoryID, filePath, projectRoot string,
) string {
	if fileHasCategoryFinding(state, scan, categoryID, filePath, projectRoot) {
		return " [已 add_finding → 须 mark disposition=finding]"
	}
	return " [待归属 → add_finding 或 mark disposition=not_vul]"
}
