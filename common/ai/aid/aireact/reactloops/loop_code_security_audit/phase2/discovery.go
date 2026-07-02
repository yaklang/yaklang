// Package loop_code_security_audit — phase2_discovery.go
//
// Tracks fast_context discovery candidates and enforces the phase-A contract:
// every candidate must be spot-read (read_file) and locked (lock_target_files)
// before lock_target_files(done=true) enters phase B.
package phase2

import (
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
)

const (
	// phase2MaxDiscoveryCandidates caps fast_context paths registered per category (breadth-first).
	phase2MaxDiscoveryCandidates = 24
	// phase2MinLockedForDiscoveryRelax allows done=true while auto-including remaining candidates.
	phase2MinLockedForDiscoveryRelax = 1
	// phase2MinAddFindingConfidence: Phase2 prefers recall; Phase3 will verify/discard.
	phase2MinAddFindingConfidence = 4
)

// AddDiscoveryCandidates merges paths from fast_context (or future discovery tools)
// into the per-category pending pool. Returns how many new paths were added.
func (s *ScanState) AddDiscoveryCandidates(paths []string) (added int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.DiscoveryCandidates == nil {
		s.DiscoveryCandidates = make(map[string]bool)
	}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" || s.DiscoveryCandidates[p] {
			continue
		}
		s.DiscoveryCandidates[p] = true
		s.DiscoveryCandidateOrder = append(s.DiscoveryCandidateOrder, p)
		added++
	}
	return added
}

// DiscoveryCandidateCount returns total discovery paths accumulated in phase A.
func (s *ScanState) DiscoveryCandidateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.DiscoveryCandidates)
}

// IsDiscoveryCandidate reports whether path came from a discovery tool (e.g. fast_context).
func (s *ScanState) IsDiscoveryCandidate(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.DiscoveryCandidates[path]
}

// MarkSpotChecked records a phase-A read_file on a candidate path.
func (s *ScanState) MarkSpotChecked(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SpotCheckedCandidates == nil {
		s.SpotCheckedCandidates = make(map[string]bool)
	}
	s.SpotCheckedCandidates[path] = true
}

// IsSpotChecked reports whether path was spot-read in phase A.
func (s *ScanState) IsSpotChecked(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SpotCheckedCandidates[path]
}

// UnresolvedDiscovery returns discovery candidates not yet in the locked target list.
func (s *ScanState) UnresolvedDiscovery() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, p := range s.DiscoveryCandidateOrder {
		if s.DiscoveryCandidates[p] && !s.TargetFileSet[p] {
			out = append(out, p)
		}
	}
	return out
}

// UncheckedDiscovery returns discovery candidates not yet spot-read in phase A.
func (s *ScanState) UncheckedDiscovery() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, p := range s.DiscoveryCandidateOrder {
		if s.DiscoveryCandidates[p] && !s.SpotCheckedCandidates[p] {
			out = append(out, p)
		}
	}
	return out
}

// AllDiscoveryCandidates returns ordered discovery paths (for UI / reactive data).
func (s *ScanState) AllDiscoveryCandidates() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.DiscoveryCandidateOrder))
	for _, p := range s.DiscoveryCandidateOrder {
		if s.DiscoveryCandidates[p] {
			out = append(out, p)
		}
	}
	return out
}

// validatePhaseALockTargetFiles enforces discovery coverage before phase B.
// Breadth-first: spot-read is recommended but not required to lock; done=true auto-includes remaining candidates.
func validatePhaseALockTargetFiles(scan *ScanState, files []string, done bool) (bool, string) {
	if !done {
		return true, ""
	}

	unresolved := scan.UnresolvedDiscovery()
	if len(unresolved) == 0 {
		return true, ""
	}

	if scan.TargetFileCount() >= phase2MinLockedForDiscoveryRelax {
		autoLocked, _ := scan.PrepareDiscoveryGateForPhaseB()
		unresolved = scan.UnresolvedDiscovery()
		if len(unresolved) == 0 {
			return true, fmt.Sprintf(
				"[系统] 广度优先：已自动纳入剩余 %d 个 fast_context 候选（含未抽查），进入阶段B。",
				autoLocked,
			)
		}
	}
	return false, formatDiscoveryGateBlockedFeedback(unresolved)
}

// PrepareDiscoveryGateForPhaseB auto-locks all remaining discovery candidates so phase B
// can audit broadly. Never-read paths are included (breadth over precision; Phase3 verifies).
func (s *ScanState) PrepareDiscoveryGateForPhaseB() (autoLocked, skipped int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range s.DiscoveryCandidateOrder {
		if !s.DiscoveryCandidates[p] || s.TargetFileSet[p] {
			continue
		}
		s.TargetFileSet[p] = true
		s.TargetFiles = append(s.TargetFiles, p)
		autoLocked++
	}
	return autoLocked, skipped
}

func formatDiscoveryGateBlockedFeedback(unresolved []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf(
		"[错误] fast_context 尚有 %d 个候选未纳入目标列表，禁止 lock_target_files(done=true) 进入阶段B。\n",
		len(unresolved),
	))
	b.WriteString("**广度优先**：建议 read_file 抽查后 lock；也可批量 lock_target_files(done=false) 直接纳入多个候选。\n")
	b.WriteString("或 lock_target_files(done=true)：系统将**自动纳入**剩余候选后进入阶段B（宁可多审，Phase3 会验证）。\n")
	b.WriteString("未纳入的候选：\n")
	b.WriteString(formatPathListForFeedback(unresolved, 30))
	if len(unresolved) > 0 {
		first := unresolved[0]
		b.WriteString(fmt.Sprintf(
			"\n请立即：read_file(file=%q, offset=1, lines=80) → lock_target_files(target_files=[%q], done=false)",
			first, first,
		))
	}
	return b.String()
}

func formatPendingDiscoveryForReactive(scan *ScanState) (list string, warning string) {
	unresolved := scan.UnresolvedDiscovery()
	unchecked := scan.UncheckedDiscovery()
	if len(unresolved) == 0 && len(unchecked) == 0 {
		return "", ""
	}

	var b strings.Builder
	all := scan.AllDiscoveryCandidates()
	for i, p := range all {
		status := "待 read + lock"
		if scan.IsSpotChecked(p) && !scan.IsTargetFile(p) {
			status = "已 read，待 lock"
		} else if scan.IsTargetFile(p) {
			status = "已纳入"
		}
		b.WriteString(fmt.Sprintf("  %d. %s [%s]\n", i+1, p, status))
	}
	list = b.String()

	if len(unresolved) > 0 {
		warning = fmt.Sprintf("尚有 %d 个 fast_context 候选未纳入；可批量 lock 或 done=true 自动纳入", len(unresolved))
	} else if len(unchecked) > 0 {
		warning = fmt.Sprintf("尚有 %d 个候选未 read_file 抽查", len(unchecked))
	}
	return list, warning
}

// emitPhase2DiscoveryGateBlocked surfaces a blocked done=true attempt to the UI stream.
func emitPhase2DiscoveryGateBlocked(loop *reactloops.ReActLoop, category model.VulnCategory, unresolved []string) {
	if loop == nil || len(unresolved) == 0 {
		return
	}
	lines := fmt.Sprintf(
		"[门禁] 阶段A [%s]：%d 个 fast_context 候选未纳入 / Discovery gate: %d candidates not locked\n%s",
		category.Name, len(unresolved), len(unresolved),
		formatPathListForFeedback(unresolved, 12),
	)
	reactloops.EmitActionLog(loop, util.ScanNodeID, lines)
	reactloops.EmitStatus(loop, fmt.Sprintf("阶段A：待纳入 %d 个候选 / Phase A: %d candidates pending", len(unresolved), len(unresolved)))
	emitPhase2Structured(loop, "code_audit_scan_discovery_gate", map[string]any{
		"category_id":   category.ID,
		"category_name": category.Name,
		"unresolved":    unresolved,
		"count":         len(unresolved),
	})
}
