package phase2

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/emit"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/model"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_code_security_audit/internal/util"
	"math"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
		"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/scan_instruction.txt
var phase2ScanInstruction string

//go:embed prompts/output_example.txt
var phase2OutputExample string

// ─────────────────────────────────────────────────────────────────────
// 两阶段扫描状态（极简）
//
// ScanState is per-category, per-loop-instance. model.AuditState (state.go) is global across
// Phase 1–4. Progress in phase B is driven only by mark_file_done on TargetFiles.
// ─────────────────────────────────────────────────────────────────────

// ScanPhase 标识单次 category 扫描的内部阶段
type ScanPhase string

const (
	ScanPhaseSearch ScanPhase = "search" // 阶段A：discovery（fast_context/grep files_with_matches → lock）
	ScanPhaseAudit  ScanPhase = "audit"  // 阶段B：逐文件审计（read/trace-grep → mark → complete_scan）
)

// ScanState 单次 category 扫描的轻量状态。
// Counters in PhaseBReadCounts / PhaseBGrepCounts are cleared in mark_file_done;
// PhaseASpotReadsSinceLock resets on every lock_target_files call.
type ScanState struct {
	mu sync.Mutex

	Phase         ScanPhase
	TargetFiles   []string        // 阶段B的目标文件（累积追加，去重）
	TargetFileSet map[string]bool // 去重用
	AuditedFiles  map[string]bool // 已完成审计的文件

	// PhaseASpotReadsSinceLock 阶段A 自上次 lock 以来的 read_file 抽查次数（用于提示及时 lock）
	PhaseASpotReadsSinceLock int

	// PhaseBReadCounts 阶段B 每个目标文件的 read_file 次数（未 mark 前累计，用于防打转）
	PhaseBReadCounts map[string]int

	// PhaseBGrepCounts 阶段B 每个目标文件的 trace grep 次数（content 模式溯源，未 mark 前累计）
	PhaseBGrepCounts map[string]int

	// DiscoveryCandidates / DiscoveryCandidateOrder: paths from fast_context that must be
	// spot-read and locked before lock_target_files(done=true).
	DiscoveryCandidates     map[string]bool
	DiscoveryCandidateOrder []string
	SpotCheckedCandidates   map[string]bool

	// FilesWithFinding: target abs paths with add_finding in this category scan.
	FilesWithFinding map[string]bool
	// FileDisposition: per-target attribution at mark_file_done (finding | not_vul).
	FileDisposition map[string]string

	// FastContextAttempts counts fast_context invocations in this category (for deep-discovery escalation).
	FastContextAttempts int
	// LastDiscoveryQuality records the latest discovery quality level (good|weak|empty).
	LastDiscoveryQuality string
}

func newScanState() *ScanState {
	return &ScanState{
		Phase:            ScanPhaseSearch,
		TargetFileSet:    make(map[string]bool),
		AuditedFiles:     make(map[string]bool),
		PhaseBReadCounts: make(map[string]int),
		PhaseBGrepCounts: make(map[string]int),
		FilesWithFinding: make(map[string]bool),
		FileDisposition:  make(map[string]string),
	}
}

// ResetPhaseASpotReads clears the phase-A read counter (call after lock_target_files).
func (s *ScanState) ResetPhaseASpotReads() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PhaseASpotReadsSinceLock = 0
}

// BumpPhaseASpotReads increments phase-A spot read counter.
func (s *ScanState) BumpPhaseASpotReads() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PhaseASpotReadsSinceLock++
	return s.PhaseASpotReadsSinceLock
}

// PhaseASpotReadCount returns reads since last lock in phase A.
func (s *ScanState) PhaseASpotReadCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PhaseASpotReadsSinceLock
}

// BumpPhaseBRead increments phase-B read counter for a target file; returns new count.
func (s *ScanState) BumpPhaseBRead(file string) int {
	if file == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PhaseBReadCounts[file]++
	return s.PhaseBReadCounts[file]
}

// PhaseBReadCount returns phase-B read count for a file.
func (s *ScanState) PhaseBReadCount(file string) int {
	if file == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PhaseBReadCounts[file]
}

// ClearPhaseBReads clears phase-B read counter for a file (call after mark_file_done).
func (s *ScanState) ClearPhaseBReads(file string) {
	if file == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.PhaseBReadCounts, file)
}

// BumpPhaseBGrep increments phase-B trace grep counter for a target file.
func (s *ScanState) BumpPhaseBGrep(file string) int {
	if file == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PhaseBGrepCounts[file]++
	return s.PhaseBGrepCounts[file]
}

// PhaseBGrepCount returns phase-B trace grep count for a file.
func (s *ScanState) PhaseBGrepCount(file string) int {
	if file == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PhaseBGrepCounts[file]
}

// ClearPhaseBGreps clears phase-B trace grep counter for a file (call after mark_file_done).
func (s *ScanState) ClearPhaseBGreps(file string) {
	if file == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.PhaseBGrepCounts, file)
}

// AddTargetFiles 追加目标文件（去重），返回新增数量。不切换阶段。
func (s *ScanState) AddTargetFiles(files []string) (added int, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range files {
		if f != "" && !s.TargetFileSet[f] {
			s.TargetFileSet[f] = true
			s.TargetFiles = append(s.TargetFiles, f)
			added++
		}
	}
	return added, len(s.TargetFiles)
}

// CommitToAudit 切换到审计阶段（阶段B），返回最终目标文件列表
func (s *ScanState) CommitToAudit() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Phase = ScanPhaseAudit
	result := make([]string, len(s.TargetFiles))
	copy(result, s.TargetFiles)
	return result
}

// TargetFileCount 返回当前已收集的目标文件数
func (s *ScanState) TargetFileCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.TargetFiles)
}

// CollectedTargetFiles 返回当前已纳入的目标文件列表（阶段A/B 均可）
func (s *ScanState) CollectedTargetFiles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]string, len(s.TargetFiles))
	copy(result, s.TargetFiles)
	return result
}

// MarkFileDone marks a file audited without recording disposition (internal/tests/auto-finalize only).
func (s *ScanState) MarkFileDone(filePath string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AuditedFiles[filePath] = true
	remaining := 0
	for _, f := range s.TargetFiles {
		if !s.AuditedFiles[f] {
			remaining++
		}
	}
	return remaining
}

// Progress 返回（已完成数，总数），仅统计 TargetFiles 中已 mark 的文件
func (s *ScanState) Progress() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	done := 0
	for _, f := range s.TargetFiles {
		if s.AuditedFiles[f] {
			done++
		}
	}
	return done, len(s.TargetFiles)
}

// CurrentPhase returns the active scan phase.
func (s *ScanState) CurrentPhase() ScanPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Phase
}

// IsTargetFile reports whether path is in the locked target list.
func (s *ScanState) IsTargetFile(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.TargetFileSet[path]
}

// IsFileAudited reports whether path was marked done (only meaningful for target files).
func (s *ScanState) IsFileAudited(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.AuditedFiles[path]
}

// RemainingFiles 返回尚未审计的文件列表
func (s *ScanState) RemainingFiles() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []string
	for _, f := range s.TargetFiles {
		if !s.AuditedFiles[f] {
			result = append(result, f)
		}
	}
	return result
}

// AllDone 是否所有目标文件都已完成
func (s *ScanState) AllDone() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.TargetFiles) == 0 {
		return false
	}
	for _, f := range s.TargetFiles {
		if !s.AuditedFiles[f] {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────
// Reactive data 模板
// ─────────────────────────────────────────────────────────────────────

const phase2ReactiveDataTpl = `## 当前扫描任务
<|SCAN_TASK_{{ .Nonce }}|>
**漏洞类别**: {{ .CategoryName }} ({{ .CategoryID }})
**技术栈**: {{ .TechStack }}
**入口点摘要**: {{ .EntryPoints }}

{{ if .HasSelectionFocus }}
**[优先] 用户选中片段**（必须先覆盖文件 {{ .FocusFilePath }}）:
{{ .Selection }}
{{ else if .HasOpenFileFocus }}
**[优先] 前端打开文件**: {{ .FocusFilePath }}
{{ end }}

> [路径规则] 所有文件路径参数必须使用用户指定的项目绝对路径，禁止使用相对路径。
{{ if .ReconOutline }}
**项目背景报告章节大纲**（包含路由列表、数据访问模式、认证机制等）:
{{ .ReconOutline }}
{{ else if .ReconFileHint }}
**项目背景报告**（包含路由列表、数据访问模式、认证机制等）: {{ .ReconFileHint }}（调用 read_recon_notes 读取全文）
{{ end }}
{{ if .PrevFindingsSummary }}
---
### 前序类别已发现的 Findings（仅供参考，避免重复提交）
{{ .PrevFindingsSummary }}
{{ end }}

---
## 当前阶段：{{ .PhaseLabel }}

{{ if .IsSearchPhase }}
### 阶段A：关键词搜索
目标：根据 **Sink 语义提示** 发现候选文件。

**阶段A read_file 限制**：仅抽查，必须 offset + lines（≤80）；禁止全文件读。违反时运行时会自动截断。

**首选 fast_context**：子 loop 内并行 grep；参考材料为**可选目录**（子 loop 内按需 require_tool + read_file 打开 recon_report）。若发现质量偏弱，父 loop 应先 **read_recon_notes 深度调研** 再重写 query 重试 fast_context。

{{ if .DiscoveryQualityWarning }}
**发现质量提醒**: {{ .DiscoveryQualityWarning }}
{{ end }}

**阶段A推荐循环（广度优先）**：fast_context →（可选 read 抽查）→ lock_target_files 批量纳入 → 可继续 fast_context 扩搜 → done=true 进阶段B。

{{ if ge .PhaseASpotReads 1 }}
**阶段A 已抽查 read_file**: {{ .PhaseASpotReads }} 次（自上次 lock 起）
{{ end }}
{{ if ge .PhaseASpotReads 2 }}
**请尽快 lock**：已连续抽查 {{ .PhaseASpotReads }} 次 read_file 仍未 lock。下一 action 应是 lock_target_files(done=false) 纳入已读候选，不要连读多个文件不 lock。
{{ end }}
{{ if .PendingDiscoveryList }}
**fast_context 候选处理进度**（建议全部纳入；done=true 可自动纳入剩余）:
{{ .PendingDiscoveryList }}
{{ end }}
{{ if .DiscoveryGateWarning }}
**候选门禁**: {{ .DiscoveryGateWarning }}
{{ end }}

**已累积目标文件**: {{ .CollectedFileCount }} 个
{{ if .CollectedFilesList }}
**已纳入目标文件列表**:
{{ .CollectedFilesList }}
{{ else if eq .CollectedFileCount 0 }}
（尚未 lock 任何文件；fast_context/grep 候选需经 read_file 判断后调用 lock_target_files 写入）
{{ end }}

**Sink 语义提示**（根据实际技术栈自主选择合适的 grep 关键词，示例仅供参考）:
{{ .SinkHints }}
{{ else }}
### 阶段B：逐文件审计
目标：对**已 lock 的目标文件**逐文件审计。**一次只处理一个文件**：read_file →（可选 trace grep）→ **可疑则 add_finding** → **mark_file_done(disposition=...)**。

**文件归属（代码强制）**：每个 lock 的文件必须有且仅有一种归属：
- finding：已 add_finding → mark_file_done(disposition="finding")
- not_vul：本类别无漏洞 → mark_file_done(disposition="not_vul", audit_summary="...")
- complete_scan 会校验全部目标文件均已归属，否则拒绝并列出待处理文件

**阶段B 允许 trace grep（content 模式）**：在目标文件、其父目录或项目根内搜索符号/调用链，用于 data_flow 上溯；**禁止** files_with_matches / find_file / tree / fast_context。

**审计进度**: {{ .AuditDone }} / {{ .AuditTotal }} 个目标文件已完成 mark_file_done

**待审计文件**（须全部 mark_file_done 后才能 complete_scan）:
{{ .RemainingFilesList }}
{{ if .PhaseBReadWarning }}
**阶段B read 提醒**: {{ .PhaseBReadWarning }}
{{ end }}
{{ if .PhaseBGrepWarning }}
**阶段B trace grep 提醒**: {{ .PhaseBGrepWarning }}
{{ end }}
{{ if eq .AuditRemaining 1 }}
**仅剩 1 个文件**：read 后**立即** mark_file_done，然后 complete_scan。
{{ end }}
{{ end }}

{{ if .FeedbackMessages }}
---
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|SCAN_TASK_END_{{ .Nonce }}|>

**当前迭代**: {{ .IterationCount }} | **本轮 Finding 数**: {{ .FindingsCount }}

{{ if not .IsSearchPhase }}[终止规则] complete_scan 仅在**全部**目标文件均已 mark_file_done 后才会被接受。每个文件：read_file → mark_file_done；不可用 next_movements 跳过 mark。{{ end }}`

// buildSingleCategoryScanLoop 构建针对单一漏洞类别的扫描 Loop（两阶段：discovery → 逐文件审计）。
//
// Tool policy hooks (order matters for grep budget: guard before mutator bump):
//   - read_file mutator: phase-A line clamp + read counters
//   - grep mutator: phase-B content mode + grep counters (phase2_grep_guard.go)
//   - discovery guard: block find_file/tree in phase B
//   - trace grep guard: scoped content grep in phase B
//   - spot-read / read-spin guards: phase2_guards.go
func buildSingleCategoryScanLoop(r aicommon.AIInvokeRuntime, state *model.AuditState, category model.VulnCategory, categoryIndex, categoryTotal int, initialScan *ScanState, artifacts *categoryArtifactStore) (*reactloops.ReActLoop, *ScanState, error) {
	scan := initialScan
	if scan == nil {
		scan = newScanState()
	}
	scanCompleted := false

	maxIter := math.MaxInt32

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithSameActionTypeSpinThreshold(len(category.SinkHints)*2 + 5),
		reactloops.WithSameLogicSpinThreshold(3),
		reactloops.WithMaxConsecutiveSpinWarnings(4),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),

		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			vars := map[string]any{
				"Nonce":       nonce,
				"ReconFile":   state.GetReconFilePath(),
				"TechStack":   state.TechStack,
				"EntryPoints": state.EntryPoints,
			}
			for k, v := range reactloops.FocusPromptVars(state.GetFocusFilePath(), state.GetSelection()) {
				vars[k] = v
			}
			return utils.RenderTemplate(phase2ScanInstruction, vars)
		}),
		reactloops.WithReflectionOutputExample(phase2OutputExample),

		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			iterCount := loop.GetCurrentIterationIndex()
			reconFileHint := state.GetReconFilePath()

			scan.mu.Lock()
			phase := scan.Phase
			scan.mu.Unlock()

			isSearchPhase := phase == ScanPhaseSearch
			phaseLabel := "阶段A：关键词搜索"
			if !isSearchPhase {
				phaseLabel = "阶段B：逐文件审计"
			}

			// SinkHints（阶段A显示，替代硬编码关键词列表）
			sinkHintsText := category.RenderSinkHints()

			// 待审计 / 已纳入文件列表
			auditDone, auditTotal := scan.Progress()
			remaining := scan.RemainingFiles()
			var remainingFilesSB strings.Builder
			for i, f := range remaining {
				attrib := formatRemainingFileAttributionHint(scan, state, category.ID, f, state.ProjectPath)
				readCnt := scan.PhaseBReadCount(f)
				grepCnt := scan.PhaseBGrepCount(f)
				switch {
				case readCnt > 0 && grepCnt > 0:
					remainingFilesSB.WriteString(fmt.Sprintf("  %d. %s（已 read %d 次、trace grep %d 次，须 mark）%s\n", i+1, f, readCnt, grepCnt, attrib))
				case readCnt > 0:
					remainingFilesSB.WriteString(fmt.Sprintf("  %d. %s（已 read %d 次，须 mark）%s\n", i+1, f, readCnt, attrib))
				case grepCnt > 0:
					remainingFilesSB.WriteString(fmt.Sprintf("  %d. %s（已 trace grep %d 次，须 mark）%s\n", i+1, f, grepCnt, attrib))
				default:
					remainingFilesSB.WriteString(fmt.Sprintf("  %d. %s%s\n", i+1, f, attrib))
				}
			}
			if len(remaining) == 0 && !isSearchPhase {
				remainingFilesSB.WriteString("  （全部文件已审计完毕，请调用 complete_scan）\n")
			}
			phaseBReadWarning := ""
			phaseBGrepWarning := ""
			if !isSearchPhase && len(remaining) > 0 {
				first := remaining[0]
				if cnt := scan.PhaseBReadCount(first); cnt >= 1 {
					phaseBReadWarning = fmt.Sprintf("%q 已 read %d 次，下一 action 必须是 mark_file_done（不可继续 read）", first, cnt)
				}
				if cnt := scan.PhaseBGrepCount(first); cnt >= 3 {
					phaseBGrepWarning = fmt.Sprintf("%q 已 trace grep %d 次（上限 %d），请尽快完成 data_flow 并 mark_file_done",
						first, cnt, phase2MaxPhaseBTraceGrepsPerFile)
				}
			}
			auditRemaining := 0
			if !isSearchPhase {
				auditRemaining = len(remaining)
			}
			var collectedFilesSB strings.Builder
			if isSearchPhase {
				collected := scan.CollectedTargetFiles()
				const maxShow = 50
				for i, f := range collected {
					if i >= maxShow {
						collectedFilesSB.WriteString(fmt.Sprintf("  ... 另有 %d 个文件未列出\n", len(collected)-maxShow))
						break
					}
					collectedFilesSB.WriteString(fmt.Sprintf("  %d. %s\n", i+1, f))
				}
			}
			pendingDiscoveryList, discoveryGateWarning := formatPendingDiscoveryForReactive(scan)
			discoveryQualityWarning := FormatDiscoveryQualityWarningForReactive(category, scan)
			reactivePrompt, err := utils.RenderTemplate(phase2ReactiveDataTpl, mergePhase2ReactiveVars(map[string]any{
				"Nonce":                   nonce,
				"CategoryID":              category.ID,
				"CategoryName":            category.Name,
				"TechStack":               state.TechStack,
				"EntryPoints":             state.EntryPoints,
				"ReconOutline":            state.GetReconOutline(),
				"ReconFileHint":           reconFileHint,
				"PrevFindingsSummary":     buildPrevFindingsSummary(state, category.ID),
				"PhaseLabel":              phaseLabel,
				"IsSearchPhase":           isSearchPhase,
				"SinkHints":               sinkHintsText,
				"CollectedFileCount":      scan.TargetFileCount(),
				"CollectedFilesList":      collectedFilesSB.String(),
				"PendingDiscoveryList":    pendingDiscoveryList,
				"DiscoveryGateWarning":    discoveryGateWarning,
				"DiscoveryQualityWarning": discoveryQualityWarning,
				"AuditDone":               auditDone,
				"AuditTotal":              auditTotal,
				"RemainingFilesList":      remainingFilesSB.String(),
				"PhaseBReadWarning":       phaseBReadWarning,
				"PhaseBGrepWarning":       phaseBGrepWarning,
				"AuditRemaining":          auditRemaining,
				"FeedbackMessages":        feedbacker.String(),
				"FindingsCount":           len(state.GetFindings()),
				"IterationCount":          iterCount,
				"PhaseASpotReads":         scan.PhaseASpotReadCount(),
			}, state))
			return reactivePrompt, err
		}),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			if focusPath := reactloops.ResolveFocusFilePath(state.GetFocusFilePath(), state.GetSelection()); focusPath != "" {
				added, total := scan.AddTargetFiles([]string{focusPath})
				log.Infof("[CodeAudit/Phase2/%s] Pre-seeded focus file: %s (added=%d total=%d)", category.ID, focusPath, added, total)
				if state.GetSelection() != nil {
					r.AddToTimeline("[CODE_AUDIT_FOCUS]", fmt.Sprintf("Phase2/%s 选中片段优先: %s", category.ID, focusPath))
				} else {
					r.AddToTimeline("[CODE_AUDIT_FOCUS]", fmt.Sprintf("Phase2/%s 优先审计文件: %s", category.ID, focusPath))
				}
			}
			phase := scan.CurrentPhase()
			if categoryTotal > 0 {
				if phase == ScanPhaseAudit {
					emitPhase2CategoryBanner(loop, category, categoryIndex, categoryTotal, "阶段B：逐文件审计 / Phase B: audit (resumed)")
				} else {
					emitPhase2CategoryBanner(loop, category, categoryIndex, categoryTotal, "阶段A：关键词搜索 / Phase A: search")
				}
			} else {
				reactloops.EmitStatus(loop, fmt.Sprintf("扫描类别 %s / Scanning category %s", category.Name, category.ID))
			}
			log.Infof("[CodeAudit/Phase2] Category '%s' scan started (phase=%s, targets=%d)", category.ID, phase, scan.TargetFileCount())
			op.Continue()
		}),

		// Runtime tool policy for this category loop (wired in invoke_toolcall.go).
		reactloops.WithToolInvokeParamsMutator(buildPhase2ReadFileParamsMutator(r, scan, category)),
		reactloops.WithToolInvokeParamsMutator(buildPhase2PhaseBGrepParamsMutator(scan)),
		reactloops.WithToolInvokeGuard(buildPhase2PhaseBDiscoveryToolGuard(scan)),
		reactloops.WithToolInvokeGuard(buildPhase2PhaseBGrepGuard(scan, state.ProjectPath)),
		reactloops.WithToolInvokeGuard(buildPhase2PhaseASpotReadGuard(scan)),
		reactloops.WithToolInvokeGuard(buildPhase2PhaseBReadSpinGuard(scan)),

		reactloops.WithOnPostIteraction(func(loop *reactloops.ReActLoop, iteration int, task aicommon.AIStatefulTask, isDone bool, reason any, op *reactloops.OnPostIterationOperator) {
			if !isDone || scanCompleted {
				return
			}
			finalizeCategoryScanOnLoopEnd(loop, r, state, scan, category, reason, artifacts)
		}),

		reactloops.WithActionFactoryFromLoop(schema.AI_REACT_LOOP_NAME_FAST_CONTEXT),
		buildFastContextAction(r, state, category, scan),

		// ─── lock_target_files：每次 grep 后调用追加文件，done=true 时切换阶段B ───
		reactloops.WithRegisterLoopAction(
			"lock_target_files",
			"阶段A：将筛选后的候选文件追加到审计目标列表。fast_context/grep 返回路径后，先用 read_file 抽查判断，再调用本 action 写入。done=false（默认）仅追加、可继续 fast_context/grep 扩搜；done=true 切换到阶段B。",
			[]aitool.ToolOption{
				aitool.WithStringArrayParam("target_files",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("本次确认纳入的候选文件绝对路径列表（fast_context/grep 命中后经 read_file 筛选；排除测试/vendor）")),
				aitool.WithBoolParam("done",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("false（默认）=仅追加、可继续 fast_context/grep；true=停止收集并切换到阶段B")),
				aitool.WithStringParam("reason",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("筛选理由（可选）")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				scan.mu.Lock()
				if scan.Phase != ScanPhaseSearch {
					scan.mu.Unlock()
					op.Feedback("[错误] 已处于审计阶段（阶段B），无法继续追加文件。")
					return
				}
				scan.mu.Unlock()

				files := action.GetStringSlice("target_files")
				done := action.GetBool("done")
				reason := action.GetString("reason")

				if ok, gateMsg := validatePhaseALockTargetFiles(scan, files, done); !ok {
					if done {
						emitPhase2DiscoveryGateBlocked(loop, category, scan.UnresolvedDiscovery())
					}
					op.Feedback(gateMsg)
					return
				}

				added, total := scan.AddTargetFiles(files)
				scan.ResetPhaseASpotReads()

				logMsg := fmt.Sprintf("[Phase2/%s] 追加目标文件 +%d 个（累计 %d 个）", category.ID, added, total)
				if reason != "" {
					logMsg += "，理由: " + reason
				}
				r.AddToTimeline("[ADD_TARGET_FILES]", logMsg)
				log.Infof("[CodeAudit/Phase2/%s] Added %d files (total %d)", category.ID, added, total)

				if !done {
					emitPhase2LockTargetFiles(loop, category, added, total, false, reason, files)
					unresolved := scan.UnresolvedDiscovery()
					feedback := fmt.Sprintf("已追加 %d 个文件到目标列表（去重后累计 %d 个）。", added, total)
					if len(unresolved) > 0 {
						feedback += fmt.Sprintf("\n**尚有 %d 个 fast_context 候选未纳入**，请继续 read_file → lock_target_files(done=false)。全部纳入后才能 done=true。", len(unresolved))
						feedback += "\n未纳入：\n" + formatPathListForFeedback(unresolved, 15)
					} else if scan.DiscoveryCandidateCount() > 0 {
						feedback += "\n全部 fast_context 候选已纳入，可 lock_target_files(done=true) 进入阶段B。"
					} else {
						feedback += "\n可继续 fast_context / grep 扩搜，或用 read_file 抽查更多候选；满意后 lock_target_files(done=true) 进入审计阶段。"
					}
					op.Feedback(feedback)
					return
				}

				if total == 0 {
					op.Feedback("[警告] 目标文件列表为空，无法进入阶段B。请继续 fast_context / grep 扩搜并用 read_file 抽查后 lock_target_files(done=false) 纳入候选。")
					return
				}

				// done=true：切换到阶段B
				locked := scan.CommitToAudit()

				if categoryTotal > 0 {
					emitPhase2CategoryBanner(loop, category, categoryIndex, categoryTotal, "阶段B：逐文件审计 / Phase B: audit")
				}

				var fileList strings.Builder
				for i, f := range locked {
					fileList.WriteString(fmt.Sprintf("  %d. %s\n", i+1, f))
				}
				r.AddToTimeline("[COMMIT_AUDIT]", fmt.Sprintf("[Phase2/%s] 进入审计阶段，共 %d 个目标文件\n%s",
					category.ID, len(locked), fileList.String()))
				log.Infof("[CodeAudit/Phase2/%s] Committed %d files to audit", category.ID, len(locked))

				emitPhase2LockTargetFiles(loop, category, added, total, true, reason, files)

				op.Feedback(fmt.Sprintf("目标收集完成，共 %d 个文件，正式进入审计阶段（阶段B）。\n\n文件列表：\n%s\n请依次对每个文件：read_file 深读 →（可选 content 模式 grep 溯源调用链）→ add_finding → mark_file_done。",
					len(locked), fileList.String()))
			},
		),

		// ─── mark_file_done：阶段B，标记文件审计完成 ───
		reactloops.WithRegisterLoopAction(
			"mark_file_done",
			"[阶段B] 标记文件审计完成。必填 disposition：finding（须已 add_finding）或 not_vul（本类别无漏洞）。全部文件归属完成后才能 complete_scan。",
			[]aitool.ToolOption{
				aitool.WithStringParam("file_path",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("刚完成审计的文件绝对路径（与 read_file 的 file 参数一致）")),
				aitool.WithStringParam("disposition",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("文件归属（必填）：finding=已 add_finding；not_vul=本类别无漏洞")),
				aitool.WithStringParam("audit_summary",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("本文件审计摘要")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				scan.mu.Lock()
				if scan.Phase != ScanPhaseAudit {
					scan.mu.Unlock()
					op.Feedback("[错误] 当前处于搜索阶段（阶段A），mark_file_done 只能在审计阶段使用。")
					return
				}
				scan.mu.Unlock()

				filePath := action.GetString("file_path")
				disposition := action.GetString("disposition")
				auditSummary := action.GetString("audit_summary")
				if auditSummary == "" {
					auditSummary = "（无摘要）"
				}

				if !scan.IsTargetFile(filePath) {
					op.Feedback(formatMarkFileDoneNotTargetFeedback(filePath, scan))
					return
				}
				if ok, msg := validateMarkFileDoneDisposition(scan, state, category.ID, filePath, state.ProjectPath, disposition); !ok {
					op.Feedback(msg)
					return
				}
				if scan.IsFileAudited(filePath) {
					op.Feedback(formatMarkFileDoneAlreadyDoneFeedback(filePath, scan))
					return
				}

				disp := normalizeFileDisposition(disposition)
				remaining := scan.MarkFileDoneWithDisposition(filePath, disp)
				scan.ClearPhaseBReads(filePath)
				scan.ClearPhaseBGreps(filePath)
				done, total := scan.Progress()

				r.AddToTimeline("[FILE_DONE]", fmt.Sprintf("[Phase2/%s] 审计完成: %s（%d/%d）\n%s",
					category.ID, filePath, done, total, auditSummary))
				log.Infof("[CodeAudit/Phase2/%s] File done: %s (%d/%d)", category.ID, filePath, done, total)

				emitPhase2MarkFileDone(loop, category, filePath, done, total, remaining, auditSummary)

				if remaining == 0 {
					op.Feedback(fmt.Sprintf("文件 %s 审计完成（%d/%d）。\n所有目标文件已审计完毕！请调用 complete_scan 结束本类别扫描。", filePath, done, total))
				} else {
					nextFiles := scan.RemainingFiles()
					next := ""
					if len(nextFiles) > 0 {
						next = nextFiles[0]
					}
					op.Feedback(fmt.Sprintf("文件 %s 审计完成（%d/%d）。还有 %d 个文件待审计。\n下一个文件：%s\n请继续使用 read_file 读取并审计该文件。",
						filePath, done, total, remaining, next))
				}
			},
		),

		// ─── add_finding：提交确认漏洞 ───
		reactloops.WithRegisterLoopAction(
			"add_finding",
			"[阶段B] 提交一个已确认的结构化漏洞 finding。",
			[]aitool.ToolOption{
				aitool.WithStringParam("module",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("所属模块或功能区域")),
				aitool.WithStringParam("file",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("相对于项目根目录的文件路径（如 includes/admin.inc.php；与 read_file 的绝对路径不同）")),
				aitool.WithIntegerParam("line",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("Sink 函数所在行号")),
				aitool.WithStringParam("severity",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("严重程度：HIGH / MEDIUM / LOW")),
				aitool.WithStringParam("title",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("漏洞简短标题")),
				aitool.WithStringParam("description",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("漏洞描述：函数、参数、危险操作")),
				aitool.WithStringParam("data_flow",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("数据流路径：HTTP入口[路由] → 处理函数[文件:行] → Sink函数[文件:行]")),
				aitool.WithStringParam("exploit_scenario",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("攻击场景：具体 payload 示例")),
				aitool.WithStringParam("recommendation",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("修复建议，提供代码示例")),
				aitool.WithIntegerParam("confidence",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("置信度 1-10（< 6 将被拒绝）")),
			},
			func(l *reactloops.ReActLoop, action *aicommon.Action) error {
				if action.GetInt("confidence") < phase2MinAddFindingConfidence {
					return fmt.Errorf("confidence %d < %d，置信度不足；若存在可疑数据流请用 confidence=%d 提交，由 Phase3 验证",
						action.GetInt("confidence"), phase2MinAddFindingConfidence, phase2MinAddFindingConfidence)
				}
				return nil
			},
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				scan.mu.Lock()
				if scan.Phase != ScanPhaseAudit {
					scan.mu.Unlock()
					op.Feedback("[错误] 当前处于阶段A（搜索收集），add_finding 只能在阶段B（逐文件审计）使用。请先 lock_target_files(done=true) 进入阶段B。")
					return
				}
				scan.mu.Unlock()

				f := &model.Finding{
					Module:          action.GetString("module"),
					File:            action.GetString("file"),
					Line:            int(action.GetInt("line")),
					Severity:        strings.ToUpper(action.GetString("severity")),
					Category:        category.ID,
					Title:           action.GetString("title"),
					Description:     action.GetString("description"),
					DataFlow:        action.GetString("data_flow"),
					ExploitScenario: action.GetString("exploit_scenario"),
					Recommendation:  action.GetString("recommendation"),
					Confidence:      int(action.GetInt("confidence")),
				}
				if f.Confidence > 10 {
					f.Confidence = 10
				}
				state.AddFinding(f)

				if abs := resolveTargetAbsPath(state.ProjectPath, scan, f.File); abs != "" {
					scan.NoteFinding(abs)
				}

				loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "code_audit_finding", map[string]any{
					"finding": f,
					"total":   len(state.GetFindings()),
				})
				emit.Phase2FindingAdded(loop, category, f, len(state.GetFindings()))

				log.Infof("[CodeAudit/Phase2] Finding added: %s - %s (%s:%d)", f.ID, f.Category, f.File, f.Line)

				op.Feedback(fmt.Sprintf("Finding %s 已记录（%s, %s:%d, %s）。继续审计当前文件，完成后调用 mark_file_done。",
					f.ID, f.Category, f.File, f.Line, f.Title))
			},
		),

		reactloops.WithRegisterLoopAction(
			"read_recon_notes",
			"读取项目背景报告（包含路由列表、中间件链、数据库访问模式、认证机制等）。需要了解路由映射或权限控制时优先调用，比重新 grep 代码更高效。",
			nil,
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				content, err := state.GetReconFileContent()
				if err != nil {
					op.Feedback(fmt.Sprintf("无法读取项目背景报告: %v\n请直接使用 grep/read_file 查找所需信息。", err))
					return
				}
				summary, _ := reactloops.SpillLongContent(loop, "recon_notes", content)
				r.AddToTimeline("read_recon_notes", fmt.Sprintf("[Phase2/%s] 读取项目背景报告 (%d 字节)", category.ID, len(content)))
				op.Feedback(fmt.Sprintf("=== 项目背景报告 (%d bytes) ===\n\n%s", len(content), summary))
			},
		),

		// ─── complete_scan：结束本类别扫描 ───
		reactloops.WithRegisterLoopAction(
			"complete_scan",
			"完成本类别扫描（所有文件审计完毕后调用）。",
			[]aitool.ToolOption{
				aitool.WithStringParam("coverage_summary",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("覆盖总结：搜索了哪些关键词、审计了哪些文件、发现情况")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				scan.mu.Lock()
				inSearch := scan.Phase == ScanPhaseSearch
				scan.mu.Unlock()
				if inSearch {
					op.Feedback("[错误] 当前处于阶段A（搜索收集）。请先 lock_target_files(done=true) 进入阶段B并完成逐文件审计，再调用 complete_scan。")
					op.Continue()
					return
				}

				if !scan.AllDone() {
					op.Feedback(formatCompleteScanBlockedFeedback(scan, state, category.ID, state.ProjectPath))
					op.Continue()
					return
				}
				if ok, msg := validateAllTargetsAttributed(scan, state, category.ID, state.ProjectPath); !ok {
					op.Feedback(msg)
					op.Continue()
					return
				}

				scanCompleted = true
				coverageSummary := action.GetString("coverage_summary")
				coverageSpill, _ := reactloops.SpillLongContent(loop, "scan_coverage_"+category.ID, coverageSummary)

				obs := &model.ScanObservation{
					CategoryID:      category.ID,
					CategoryName:    category.Name,
					StopReason:      "files_all_audited",
					CoverageSummary: coverageSummary,
				}
				state.AddScanObservation(obs)

				catFindingCount := 0
				for _, finding := range state.GetFindings() {
					if finding.Category == category.ID {
						catFindingCount++
					}
				}
				emit.Phase2CategoryScanComplete(loop, category, catFindingCount, coverageSummary)

				r.AddToTimeline("[SCAN_COMPLETE]", fmt.Sprintf("[Phase2/%s] 扫描完成\n%s", category.ID, coverageSummary))
				log.Infof("[CodeAudit/Phase2] Category '%s' complete", category.ID)

				auditDirPath := util.AuditDir(state)
				if mkErr := os.MkdirAll(auditDirPath, 0o755); mkErr == nil {
					persistCategoryObservation(artifacts, auditDirPath, category.ID, obs)
				}

				op.Feedback(fmt.Sprintf("类别 [%s] 扫描完成。\n%s", category.Name, coverageSpill))
				op.Exit()
			},
		),
	}

	preset = append(preset, buildPhase2WhitelistFSToolOptions(r)...)

	loopName := fmt.Sprintf("code_audit_scan_%s", category.ID)
	loop, err := reactloops.NewReActLoop(loopName, r, preset...)
	return loop, scan, err
}

// ─────────────────────────────────────────────────────────────────────
// Phase 2 编排层
//
// BuildAllCategoriesLoop is a non-interactive orchestrator: it plans categories,
// runs buildSingleCategoryScanLoop via forked sub-agents, then hands off to Phase 3.
// ─────────────────────────────────────────────────────────────────────

const planPromptTemplate = `你是代码安全审计专家。现在需要确定本次审计的漏洞扫描计划。

## 默认扫描类别（8 个）

%s

## 用户需求

%s

## 任务

1. 从默认类别中选择本次审计应该包含的类别（通常全选，除非用户明确说要跳过某类）
2. 如果用户提到了默认类别之外的特殊漏洞关注点，为每个额外关注点生成一个扫描类别

对于 extra_categories 中的每项，格式如下：
- id：类别标识（小写字母+下划线）
- name：中文名称
- sink_patterns：grep 关键词列表（逗号分隔）
- instruction：针对该类别的扫描指南（2-3句话）

如果用户没有提到额外的漏洞类型，extra_categories 返回空字符串 ""。
`

// BuildAllCategoriesLoop 构建 Phase 2 编排 Loop
func BuildAllCategoriesLoop(r aicommon.AIInvokeRuntime, state *model.AuditState, overrideCategories []model.VulnCategory, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(math.MaxInt32),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(false),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			finalCategories := overrideCategories
			if len(finalCategories) == 0 {
				finalCategories = planScanCategories(r, task, state)
			}
			presentAuditVulnerabilityTypes(r, loop, task, finalCategories)
			emit.Phase2ScanPlan(loop, finalCategories)

			runAllCategoryScans(r, loop, task, state, finalCategories)

			allFindings := state.GetFindings()
			state.SetPhase(model.AuditPhaseVerify)

			r.AddToTimeline("[PHASE2_COMPLETE]",
				fmt.Sprintf("Phase 2 扫描完成。共扫描 %d 个漏洞类别，发现 %d 个疑似漏洞。", len(finalCategories), len(allFindings)))
			emit.Phase2AllCategoriesDone(loop, len(finalCategories), len(allFindings))

			log.Infof("[CodeAudit/Phase2] All categories done. Total findings: %d", len(allFindings))
			op.Done()
		}),
	}

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_phase2_orchestrator", r, preset...)
}

// planScanCategories 调用 AI 生成本次审计的扫描类别列表（LiteForge 在 fork 子 timeline 上运行，避免污染父 timeline）。
func planScanCategories(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *model.AuditState) []model.VulnCategory {
	var defaultDesc strings.Builder
	for _, c := range model.DefaultVulnCategories {
		defaultDesc.WriteString(fmt.Sprintf("- %s（id: %s）\n", c.Name, c.ID))
	}

	userInput := task.GetUserInput()
	if userInput == "" {
		userInput = "（用户未提供额外说明）"
	}

	prompt := fmt.Sprintf(planPromptTemplate, defaultDesc.String(), userInput)

	var action *aicommon.Action
	planErr := reactloops.RunForkInvokerCallback(r, task, reactloops.SubAgentJob{
		Identifier: "scan-plan",
		TaskName:   "Determine code audit vulnerability scan categories",
		Goal:       "Determine code audit vulnerability scan categories",
	}, func(childInvoker aicommon.AIInvokeRuntime, childTask aicommon.AIStatefulTask) error {
		var err error
		action, err = childInvoker.InvokeSpeedPriorityLiteForge(
			childTask.GetContext(),
			"scan_plan",
			prompt,
			[]aitool.ToolOption{
				aitool.WithStringArrayParam("selected_category_ids",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("从默认 8 个类别中选择的 ID 列表，通常全选"),
				),
				aitool.WithStringParam("extra_categories_json",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description(`用户额外要求的漏洞类别，JSON 数组字符串：
[{"id":"custom_category","name":"类别名称","sink_patterns":"keyword1,keyword2","instruction":"扫描指南"}]
如果没有额外要求，传入空字符串 ""`),
				),
			},
		)
		return err
	})
	if planErr != nil {
		log.Warnf("[CodeAudit/Phase2] Plan AI call failed: %v, falling back to default categories", planErr)
		return model.DefaultVulnCategories
	}

	if action == nil {
		log.Warnf("[CodeAudit/Phase2] Plan returned nil action, using all defaults")
		return model.DefaultVulnCategories
	}

	selectedIDs := action.GetStringSlice("selected_category_ids")
	if len(selectedIDs) == 0 {
		log.Warnf("[CodeAudit/Phase2] Plan returned empty selected_category_ids, using all defaults")
		selectedIDs = make([]string, 0, len(model.DefaultVulnCategories))
		for _, c := range model.DefaultVulnCategories {
			selectedIDs = append(selectedIDs, c.ID)
		}
	}

	var result []model.VulnCategory
	selectedSet := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selectedSet[id] = true
	}
	for _, c := range model.DefaultVulnCategories {
		if selectedSet[c.ID] {
			result = append(result, c)
		}
	}

	extraRaw := action.GetString("extra_categories_json")
	if extraRaw != "" && extraRaw != "[]" {
		var extras []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			SinkPatterns string `json:"sink_patterns"`
			Instruction  string `json:"instruction"`
		}
		if err := json.Unmarshal([]byte(extraRaw), &extras); err == nil {
			for _, e := range extras {
				if e.ID == "" || e.Name == "" {
					continue
				}
				result = append(result, model.VulnCategory{
					ID:   e.ID,
					Name: e.Name,
					SinkHints: []model.SinkHint{
						{
							Name:        "用户自定义Sink",
							Description: "用户指定的漏洞扫描关注点",
							Examples:    strings.Split(e.SinkPatterns, ","),
						},
					},
					Instruction: e.Instruction,
				})
				log.Infof("[CodeAudit/Phase2] Extra category added: %s (%s)", e.Name, e.ID)
			}
		}
	}

	if len(result) == 0 {
		return model.DefaultVulnCategories
	}

	var planSummary strings.Builder
	planSummary.WriteString(fmt.Sprintf("扫描计划确定：共 %d 个类别\n", len(result)))
	for i, c := range result {
		planSummary.WriteString(fmt.Sprintf("  %d. %s（%s）\n", i+1, c.Name, c.ID))
	}
	r.AddToTimeline("[SCAN_PLAN]", planSummary.String())
	log.Infof("[CodeAudit/Phase2] Scan plan: %s", planSummary.String())
	return result
}

// buildPrevFindingsSummary 返回当前 category 之前已发现的 findings 摘要
func buildPrevFindingsSummary(state *model.AuditState, currentCategoryID string) string {
	findings := state.GetFindings()
	if len(findings) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, f := range findings {
		if f.Category == currentCategoryID {
			continue
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s（%s）: %s:%d - %s\n",
			f.Severity, f.ID, f.Category, f.File, f.Line, f.Title))
	}
	return sb.String()
}

func mergePhase2ReactiveVars(base map[string]any, state *model.AuditState) map[string]any {
	for k, v := range reactloops.FocusPromptVars(state.GetFocusFilePath(), state.GetSelection()) {
		base[k] = v
	}
	return base
}
