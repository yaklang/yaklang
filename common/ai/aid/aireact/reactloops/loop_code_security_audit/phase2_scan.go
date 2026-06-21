package loop_code_security_audit

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/sync/errgroup"
)

//go:embed prompts/phase2_scan_instruction.txt
var phase2ScanInstruction string

//go:embed prompts/phase2_output_example.txt
var phase2OutputExample string

// ─────────────────────────────────────────────────────────────────────
// 两阶段扫描状态（极简）
// ─────────────────────────────────────────────────────────────────────

// ScanPhase 标识单次 category 扫描的内部阶段
type ScanPhase string

const (
	ScanPhaseSearch ScanPhase = "search" // 阶段A：grep 收集目标文件
	ScanPhaseAudit  ScanPhase = "audit"  // 阶段B：逐文件审计
)

// ScanState 单次 category 扫描的轻量状态
type ScanState struct {
	mu sync.Mutex

	Phase         ScanPhase
	TargetFiles   []string        // 阶段B的目标文件（累积追加，去重）
	TargetFileSet map[string]bool // 去重用
	AuditedFiles  map[string]bool // 已完成审计的文件
	SearchCount   int             // 阶段A的搜索次数
	AuditCount    int             // 阶段B的审计次数
	FindingCount  int             // 当前类别发现的 finding 数
	MaxFiles      int             // 最大目标文件数限制
}

func newScanState() *ScanState {
	return &ScanState{
		Phase:         ScanPhaseSearch,
		TargetFileSet: make(map[string]bool),
		AuditedFiles:  make(map[string]bool),
		MaxFiles:      3, // 每个类别最多扫描 3 个文件
	}
}

// AddTargetFiles 追加目标文件（去重），返回新增数量。不切换阶段。
// 当文件数达到 MaxFiles 上限时停止追加。
func (s *ScanState) AddTargetFiles(files []string) (added int, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range files {
		if f == "" || s.TargetFileSet[f] {
			continue
		}
		if len(s.TargetFiles) >= s.MaxFiles {
			break // 达到上限，停止追加
		}
		s.TargetFileSet[f] = true
		s.TargetFiles = append(s.TargetFiles, f)
		added++
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

// MarkFileDone 标记一个文件审计完成，返回剩余文件数
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

// Progress 返回（已完成数，总数）
func (s *ScanState) Progress() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.AuditedFiles), len(s.TargetFiles)
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

// IncrSearch 增加搜索计数
func (s *ScanState) IncrSearch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SearchCount++
}

// IncrAudit 增加审计计数
func (s *ScanState) IncrAudit() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AuditCount++
}

// IncrFinding 增加 finding 计数
func (s *ScanState) IncrFinding() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.FindingCount++
}

// ShouldEarlyExit 检查是否应该提前退出扫描
// 规则：
// - 阶段A：搜索超过 2 次且没有找到任何文件 → 提前退出
// - 阶段B：审计超过 2 个文件且没有发现任何 finding → 提前退出
func (s *ScanState) ShouldEarlyExit() (bool, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Phase == ScanPhaseSearch {
		if s.SearchCount >= 2 && len(s.TargetFiles) == 0 {
			return true, fmt.Sprintf("搜索 %d 次未找到相关文件，本类别可能不适用", s.SearchCount)
		}
	} else if s.Phase == ScanPhaseAudit {
		if s.AuditCount >= 2 && s.FindingCount == 0 {
			return true, fmt.Sprintf("已审计 %d 个文件未发现漏洞，本类别可能安全", s.AuditCount)
		}
	}
	return false, ""
}

// GetStats 返回当前扫描统计
func (s *ScanState) GetStats() (searchCount, auditCount, findingCount int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.SearchCount, s.AuditCount, s.FindingCount
}

// ─────────────────────────────────────────────────────────────────────
// Reactive data 模板
// ─────────────────────────────────────────────────────────────────────

const phase2ReactiveDataTpl = `## 当前扫描任务
<|SCAN_TASK_{{ .Nonce }}|>
**漏洞类别**: {{ .CategoryName }} ({{ .CategoryID }})
**技术栈**: {{ .TechStack }}

> [路径规则] 所有文件路径参数必须使用用户指定的项目绝对路径，禁止使用相对路径。
{{ if .ReconFileHint }}
**项目背景报告**: {{ .ReconFileHint }}（调用 read_recon_notes 读取）
{{ end }}
{{ if .PrevFindingsSummary }}
---
### 前序类别 Findings（避免重复）
{{ .PrevFindingsSummary }}
{{ end }}

---
## 当前阶段：{{ .PhaseLabel }}

{{ if .IsSearchPhase }}
### 阶段A：关键词搜索
目标：根据 Sink 语义提示自主决定 grep 关键词，使用 output-mode="files_with_matches" 获取文件列表，每次搜索后调用 lock_target_files 追加文件。收集完毕后调用 lock_target_files(done=true) 切换到阶段B。

**已累积目标文件**: {{ .CollectedFileCount }} 个 | **搜索次数**: {{ .SearchCount }}

**Sink 语义提示**:
{{ .SinkHints }}
{{ else }}
### 阶段B：逐文件审计
目标：对每个目标文件调用 read_file(file=<绝对路径>) 审计，发现漏洞调用 add_finding，完成后调用 mark_file_done。所有文件完成后调用 complete_scan。

**审计进度**: {{ .AuditDone }} / {{ .AuditTotal }} | **Findings**: {{ .FindingsCount }}

**待审计文件**:
{{ .RemainingFilesList }}
{{ end }}

{{ if .EarlyExitWarning }}
---
⚠️ **[效率提示]** {{ .EarlyExitWarning }} 如果确认本类别不适用，调用 complete_scan 跳过。
{{ end }}

{{ if .FeedbackMessages }}
---
### 反馈
{{ .FeedbackMessages }}
{{ end }}
<|SCAN_TASK_END_{{ .Nonce }}|>

**迭代**: {{ .IterationCount }} / 10 | **Findings**: {{ .FindingsCount }}

{{ if not .IsSearchPhase }}[终止规则] 所有文件审计完后调用 complete_scan。迭代超过 10 次时 loop 自动结束。{{ end }}`

// buildSingleCategoryScanLoop 构建针对单一漏洞类别的扫描 Loop（两阶段：grep→逐文件审计）
func buildSingleCategoryScanLoop(r aicommon.AIInvokeRuntime, state *AuditState, category VulnCategory) (*reactloops.ReActLoop, error) {
	scan := newScanState()

	maxIter := 10 // 每个类别最多 10 次迭代，避免无限扫描

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(true),
		reactloops.WithSameActionTypeSpinThreshold(len(category.SinkHints)*2 + 5),
		reactloops.WithSameLogicSpinThreshold(3),
		reactloops.WithMaxConsecutiveSpinWarnings(2),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),

		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			return utils.RenderTemplate(phase2ScanInstruction, map[string]any{
				"Nonce":        nonce,
				"ReconFile":    state.GetReconFilePath(),
				"TechStack":    state.TechStack,
				"EntryPoints":  state.EntryPoints,
				"PreAnalysis":  state.GetPreAnalysisPrompt(),
				"SFScanResult": state.GetSFScanSummaryPrompt(),
			})
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

			// 待审计文件列表（阶段B显示）
			auditDone, auditTotal := scan.Progress()
			remaining := scan.RemainingFiles()
			var remainingFilesSB strings.Builder
			for i, f := range remaining {
				remainingFilesSB.WriteString(fmt.Sprintf("  %d. %s\n", i+1, f))
			}
			if len(remaining) == 0 && !isSearchPhase {
				remainingFilesSB.WriteString("  （全部文件已审计完毕，请调用 complete_scan）\n")
			}

			// Early exit 检查
			earlyExit, earlyExitMsg := scan.ShouldEarlyExit()

			searchCount, _, _ := scan.GetStats()
			reactivePrompt, err := utils.RenderTemplate(phase2ReactiveDataTpl, map[string]any{
				"Nonce":               nonce,
				"CategoryID":          category.ID,
				"CategoryName":        category.Name,
				"TechStack":           state.TechStack,
				"ReconFileHint":       reconFileHint,
				"PrevFindingsSummary": buildPrevFindingsSummary(state, category.ID),
				"PhaseLabel":          phaseLabel,
				"IsSearchPhase":       isSearchPhase,
				"SinkHints":           sinkHintsText,
				"CollectedFileCount":  scan.TargetFileCount(),
				"SearchCount":         searchCount,
				"AuditDone":           auditDone,
				"AuditTotal":          auditTotal,
				"FindingsCount":       len(state.GetFindings()),
				"RemainingFilesList":  remainingFilesSB.String(),
				"FeedbackMessages":    feedbacker.String(),
				"IterationCount":      iterCount,
				"EarlyExitWarning":    earlyExitMsg,
			})

			// 如果应该 early exit，注入提示
			if earlyExit {
				log.Infof("[CodeAudit/Phase2/%s] Early exit suggested: %s", category.ID, earlyExitMsg)
			}

			return reactivePrompt, err
		}),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			log.Infof("[CodeAudit/Phase2] Category '%s' scan started", category.ID)
			op.Continue()
		}),

		// ─── lock_target_files：每次 grep 后调用追加文件，done=true 时切换阶段B ───
		reactloops.WithRegisterLoopAction(
			"lock_target_files",
			"每次 grep 后调用，将命中文件追加到审计目标列表。done=false（默认）时只追加、不切换阶段，可继续 grep 更多关键词；done=true 时正式切换到阶段B开始逐文件审计。",
			[]aitool.ToolOption{
				aitool.WithStringArrayParam("target_files",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("本次 grep 命中的文件绝对路径列表（去掉测试文件/vendor 等明显无关文件）")),
				aitool.WithBoolParam("done",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("是否完成收集、正式进入审计阶段。false（默认）=继续追加可再 grep；true=停止收集切换到阶段B")),
				aitool.WithStringParam("reason",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("筛选理由（可选）")),
			},
			nil,
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				scan.mu.Lock()
				if scan.Phase != ScanPhaseSearch {
					scan.mu.Unlock()
					op.Feedback("[提示] 目标文件已锁定并进入审计阶段（阶段B），无需再调用 lock_target_files。请直接使用 read_file 审计待审计文件。")
					return
				}
				scan.mu.Unlock()

				files := action.GetStringSlice("target_files")
				done := action.GetBool("done")
				reason := action.GetString("reason")

				scan.IncrSearch() // 增加搜索计数
				added, total := scan.AddTargetFiles(files)

				logMsg := fmt.Sprintf("[Phase2/%s] 追加目标文件 +%d 个（累计 %d 个）", category.ID, added, total)
				if reason != "" {
					logMsg += "，理由: " + reason
				}
				r.AddToTimeline("[ADD_TARGET_FILES]", logMsg)
				log.Infof("[CodeAudit/Phase2/%s] Added %d files (total %d)", category.ID, added, total)

				// 检查是否达到文件数上限
				if total >= scan.MaxFiles && !done {
					// 自动切换到审计阶段
					done = true
					logMsg += fmt.Sprintf("（已达到上限 %d 个文件，自动切换到审计阶段）", scan.MaxFiles)
					r.AddToTimeline("[MAX_FILES_REACHED]", fmt.Sprintf("[Phase2/%s] 已达到文件数上限 %d，自动切换到审计阶段", category.ID, scan.MaxFiles))
				}

				if !done {
					// 仅追加，继续搜索阶段
					op.Feedback(fmt.Sprintf("已追加 %d 个文件到目标列表（去重后累计 %d 个）。\n如需继续搜索其他关键词，请继续 grep；搜索完毕后调用 lock_target_files(done=true) 进入审计阶段。",
						added, total))
					return
				}

				// done=true：切换到阶段B
				locked := scan.CommitToAudit()
				if len(locked) == 0 {
					op.Feedback("[警告] 目标文件列表为空（所有 grep 均未命中）。\n请调用 complete_scan 结束本类别扫描。")
					return
				}

				var fileList strings.Builder
				for i, f := range locked {
					fileList.WriteString(fmt.Sprintf("  %d. %s\n", i+1, f))
				}
				r.AddToTimeline("[COMMIT_AUDIT]", fmt.Sprintf("[Phase2/%s] 进入审计阶段，共 %d 个目标文件\n%s",
					category.ID, len(locked), fileList.String()))
				log.Infof("[CodeAudit/Phase2/%s] Committed %d files to audit", category.ID, len(locked))

				op.Feedback(fmt.Sprintf("目标收集完成，共 %d 个文件，正式进入审计阶段（阶段B）。\n\n文件列表：\n%s\n请依次对每个文件使用 read_file 读取代码，发现漏洞调用 add_finding，完成后调用 mark_file_done。",
					len(locked), fileList.String()))
			},
		),

		// ─── mark_file_done：阶段B，标记文件审计完成 ───
		reactloops.WithRegisterLoopAction(
			"mark_file_done",
			"[阶段B] 标记当前文件审计完成（无论是否发现漏洞都必须调用）。所有文件完成后系统提示调用 complete_scan。",
			[]aitool.ToolOption{
				aitool.WithStringParam("file_path",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("刚完成审计的文件路径")),
				aitool.WithStringParam("audit_summary",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("本文件审计摘要：发现了什么/没发现什么")),
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
				auditSummary := action.GetString("audit_summary")
				if auditSummary == "" {
					auditSummary = "（无摘要）"
				}

				scan.IncrAudit() // 增加审计计数
				remaining := scan.MarkFileDone(filePath)
				done, total := scan.Progress()

				r.AddToTimeline("[FILE_DONE]", fmt.Sprintf("[Phase2/%s] 审计完成: %s（%d/%d）\n%s",
					category.ID, filePath, done, total, auditSummary))
				log.Infof("[CodeAudit/Phase2/%s] File done: %s (%d/%d)", category.ID, filePath, done, total)

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
			"提交一个已确认的结构化漏洞 finding。",
			[]aitool.ToolOption{
				aitool.WithStringParam("module",
					aitool.WithParam_Required(false),
					aitool.WithParam_Description("所属模块或功能区域")),
				aitool.WithStringParam("file",
					aitool.WithParam_Required(true),
					aitool.WithParam_Description("相对于项目根目录的文件路径")),
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
				if action.GetInt("confidence") < 6 {
					return fmt.Errorf("confidence %d < 6，置信度不足，请继续分析或跳过", action.GetInt("confidence"))
				}
				return nil
			},
			func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
				f := &Finding{
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
				scan.IncrFinding() // 增加 finding 计数

				loop.GetEmitter().EmitJSON(schema.EVENT_TYPE_STRUCTURED, "code_audit_finding", map[string]any{
					"finding": f,
					"total":   len(state.GetFindings()),
				})

				jsonBytes, _ := json.MarshalIndent(f, "", "  ")
				log.Infof("[CodeAudit/Phase2] Finding added: %s - %s (%s:%d)", f.ID, f.Category, f.File, f.Line)

				// 实时落盘：全量 + 当前类别独立文件
				auditDirPath := auditDir(state)
				if mkErr := os.MkdirAll(auditDirPath, 0o755); mkErr == nil {
					allFindingsFile := filepath.Join(auditDirPath, "scan_findings.json")
					if err := state.PersistFindings(allFindingsFile); err != nil {
						log.Warnf("[CodeAudit/Phase2] Persist findings failed: %v", err)
					}
					var catFindings []*Finding
					for _, finding := range state.GetFindings() {
						if finding.Category == category.ID {
							catFindings = append(catFindings, finding)
						}
					}
					catFile := filepath.Join(auditDirPath, fmt.Sprintf("findings_%s.json", category.ID))
					if data, err := json.MarshalIndent(catFindings, "", "  "); err == nil {
						if err := os.WriteFile(catFile, data, 0o644); err != nil {
							log.Warnf("[CodeAudit/Phase2] Persist category findings failed: %v", err)
						}
					}
				}

				op.Feedback(fmt.Sprintf("Finding %s 已记录（%s, %s:%d）。\n```json\n%s\n```\n继续审计当前文件，完成后调用 mark_file_done。",
					f.ID, f.Category, f.File, f.Line, string(jsonBytes)))
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
				r.AddToTimeline("read_recon_notes", fmt.Sprintf("[Phase2/%s] 读取项目背景报告 (%d 字节)", category.ID, len(content)))
				op.Feedback("=== 项目背景报告 ===\n\n" + content)
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
				coverageSummary := action.GetString("coverage_summary")

				obs := &ScanObservation{
					CategoryID:      category.ID,
					CategoryName:    category.Name,
					StopReason:      "files_all_audited",
					CoverageSummary: coverageSummary,
				}
				state.AddScanObservation(obs)

				r.AddToTimeline("[SCAN_COMPLETE]", fmt.Sprintf("[Phase2/%s] 扫描完成\n%s", category.ID, coverageSummary))
				log.Infof("[CodeAudit/Phase2] Category '%s' complete", category.ID)

				// 落盘
				auditDirPath := auditDir(state)
				if mkErr := os.MkdirAll(auditDirPath, 0o755); mkErr == nil {
					obsFile := filepath.Join(auditDirPath, "scan_observations.md")
					if err := state.PersistScanObservations(obsFile); err != nil {
						log.Warnf("[CodeAudit/Phase2] Persist scan_observations failed: %v", err)
					}
					var catFindings []*Finding
					for _, f := range state.GetFindings() {
						if f.Category == category.ID {
							catFindings = append(catFindings, f)
						}
					}
					catFile := filepath.Join(auditDirPath, fmt.Sprintf("findings_%s.json", category.ID))
					if data, err := json.MarshalIndent(catFindings, "", "  "); err == nil {
						if err := os.WriteFile(catFile, data, 0o644); err != nil {
							log.Warnf("[CodeAudit/Phase2] Write category findings file failed: %v", err)
						} else {
							log.Infof("[CodeAudit/Phase2] Category '%s' findings (%d) persisted: %s",
								category.ID, len(catFindings), catFile)
						}
					}
				}

				op.Feedback(fmt.Sprintf("类别 [%s] 扫描完成。%s", category.Name, coverageSummary))
				op.Exit()
			},
		),
	}

	preset = append(preset,
		buildPhase2FSToolAction(r, category, scan, "grep"),
		buildPhase2FSToolAction(r, category, scan, "read_file"),
		buildPhase2FSToolAction(r, category, scan, "find_file"),
	)

	loopName := fmt.Sprintf("code_audit_scan_%s", category.ID)
	return reactloops.NewReActLoop(loopName, r, preset...)
}

func buildPhase2FSToolAction(r aicommon.AIInvokeRuntime, category VulnCategory, scan *ScanState, toolName string) reactloops.ReActLoopOption {
	toolMgr := r.GetConfig().GetAiToolManager()
	if toolMgr == nil {
		log.Warnf("[CodeAudit/Phase2/%s] tool manager not available, skip %q action", category.ID, toolName)
		return func(r *reactloops.ReActLoop) {}
	}
	tool, err := toolMgr.GetToolByName(toolName)
	if err != nil || tool == nil {
		log.Warnf("[CodeAudit/Phase2/%s] tool %q not found: %v", category.ID, toolName, err)
		return func(r *reactloops.ReActLoop) {}
	}

	return reactloops.WithRegisterLoopAction(
		toolName,
		tool.GetDescription(),
		tool.BuildParamsOptions(),
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			if toolName == "grep" {
				scan.mu.Lock()
				phase := scan.Phase
				searchCount := scan.SearchCount
				targetCount := len(scan.TargetFiles)
				scan.mu.Unlock()
				if phase == ScanPhaseSearch && searchCount >= 2 && targetCount == 0 {
					op.Feedback(fmt.Sprintf(
						"[搜索预算已用尽] %s 已执行 %d 次 grep 仍未锁定目标文件。禁止继续 grep，请调用 complete_scan 结束本类别，避免空转。",
						category.ID, searchCount))
					return
				}
			}

			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			if task := loop.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			params := action.GetParams()
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
			if err != nil {
				log.Warnf("[CodeAudit/Phase2/%s] tool %q failed: %v", category.ID, toolName, err)
				op.Feedback(fmt.Sprintf("[工具执行失败] %s: %v，请调整参数或结束本类别。", toolName, err))
				return
			}

			content := ""
			if result != nil {
				content = utils.InterfaceToString(result.Data)
			}
			invoker.AddToTimeline(fmt.Sprintf("[%s]", toolName), utils.ShrinkString(content, 2048))

			if toolName == "grep" {
				scan.mu.Lock()
				phase := scan.Phase
				scan.mu.Unlock()
				if phase == ScanPhaseSearch {
					scan.IncrSearch()
					files := extractGrepMatchedFiles(content)
					added, total := scan.AddTargetFiles(files)
					searchCount, _, _ := scan.GetStats()
					if total > 0 {
						locked := scan.CommitToAudit()
						var fileList strings.Builder
						for i, f := range locked {
							fileList.WriteString(fmt.Sprintf("  %d. %s\n", i+1, f))
						}
						r.AddToTimeline("[COMMIT_AUDIT]",
							fmt.Sprintf("[Phase2/%s] grep 自动锁定 %d 个目标文件并进入审计阶段\n%s",
								category.ID, len(locked), fileList.String()))
						op.Feedback(fmt.Sprintf(
							"[%s 完成] 输出 %d 字节。grep 命中 %d 个新文件（累计 %d 个），系统已自动进入审计阶段。\n\n目标文件：\n%s\n请直接 read_file 审计第一个文件；发现漏洞调用 add_finding，无漏洞调用 mark_file_done。",
							toolName, len(content), added, total, fileList.String()))
						return
					}
					if searchCount >= 2 {
						op.Feedback(fmt.Sprintf(
							"[%s 完成] 输出 %d 字节。已搜索 %d 次但没有命中目标文件；请调用 complete_scan 结束本类别，不要继续 grep。",
							toolName, len(content), searchCount))
						return
					}
				}
			}

			op.Feedback(fmt.Sprintf("[%s 完成] 输出 %d 字节\n%s", toolName, len(content), utils.ShrinkString(content, 6000)))
		},
	)
}

func extractGrepMatchedFiles(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	stdout := content
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err == nil {
		if s, ok := parsed["stdout"].(string); ok {
			stdout = s
		}
	}
	seen := make(map[string]struct{})
	var files []string
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" ||
			strings.HasPrefix(line, "[info]") ||
			strings.HasPrefix(line, "[warn]") ||
			strings.HasPrefix(line, "[error]") ||
			strings.HasPrefix(line, "[debug]") {
			continue
		}
		if idx := strings.Index(line, "/"); idx >= 0 {
			line = line[idx:]
		}
		if idx := strings.Index(line, " ("); idx > 0 {
			line = line[:idx]
		}
		if idx := strings.Index(line, ":"); idx > 0 {
			line = line[:idx]
		}
		line = strings.Trim(line, "`'\" ")
		if !filepath.IsAbs(line) || !isPhase2SourceFile(line) {
			continue
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		files = append(files, line)
	}
	return files
}

func isPhase2SourceFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go", ".java", ".kt", ".scala", ".py", ".js", ".ts", ".jsx", ".tsx", ".php", ".rb", ".rs", ".c", ".cc", ".cpp", ".cxx", ".h", ".cs":
		return true
	default:
		return false
	}
}

// ─────────────────────────────────────────────────────────────────────
// Phase 2 编排层
// ─────────────────────────────────────────────────────────────────────

const planPromptTemplate = `你是代码安全审计专家。现在需要确定本次审计的漏洞扫描计划。

## 项目侦察结果

%s

## 可选漏洞类别（共 %d 个 CWE 类别）

%s

## 用户需求

%s

## 任务

**核心目标**：根据项目侦察结果，选择与项目实际功能最相关的漏洞类别。**宁可少选，不要多选无关类别**。

### 裁剪原则（严格遵守）

1. **语言相关裁剪**：
   - Go/Java/Python 项目：移除 C/C++ 内存安全类别（cwe_120, cwe_190, cwe_416, cwe_476）
   - 非 Web 项目：移除 Web 特定类别（cwe_79, cwe_352, cwe_434, cwe_601）

2. **功能相关裁剪**（基于项目侦察结果）：
   - 如果项目不涉及数据库操作：移除 cwe_89（SQL注入）
   - 如果项目不涉及 HTTP 服务端：移除 cwe_918（SSRF）、cwe_611（XXE）
   - 如果项目不涉及用户认证：移除 cwe_287（认证绕过）、cwe_862（授权缺失）
   - 如果项目不涉及文件操作：移除 cwe_22（路径遍历）
   - 如果项目不涉及加密：移除 cwe_327、cwe_321

3. **用户需求优先**：
   - 如果用户明确提到某些漏洞类型，确保包含相关类别
   - 如果用户没有提到特定漏洞类型，只选择与项目功能最相关的类别

4. **最少选择原则**：
   - 如果项目功能简单（如 CLI 工具、脚本），只选择 1-3 个最相关的类别
   - 如果项目是 Web 应用，选择 5-8 个相关类别
   - **不要超过 8 个类别**

### 输出格式

selected_category_ids: 从 CWE 类别库中选择的 ID 列表
extra_categories_json: 如果用户提到库中没有的漏洞类型，格式为 JSON 数组字符串
`

// buildPhase2AllCategoriesLoop 构建 Phase 2 编排 Loop（支持并发扫描）
func buildPhase2AllCategoriesLoop(r aicommon.AIInvokeRuntime, state *AuditState, overrideCategories []VulnCategory, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
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

			log.Infof("[CodeAudit/Phase2] Starting concurrent scan of %d categories", len(finalCategories))
			r.AddToTimeline("[PHASE2_START]",
				fmt.Sprintf("Phase 2 开始：将并发扫描 %d 个漏洞类别。", len(finalCategories)))

			// 并发执行各类别扫描
			var eg errgroup.Group
			// 限制并发数为 4，避免过多并发导致资源竞争
			eg.SetLimit(4)

			for _, category := range finalCategories {
				cat := category // 捕获循环变量
				eg.Go(func() error {
					log.Infof("[CodeAudit/Phase2] Starting category: %s (%s)", cat.Name, cat.ID)

					catLoop, err := buildSingleCategoryScanLoop(r, state, cat)
					if err != nil {
						log.Errorf("[CodeAudit/Phase2] Failed to build loop for category '%s': %v", cat.ID, err)
						return nil // 不中断其他类别扫描
					}

					obsCountBefore := len(state.GetScanObservations())
					catSubTask := aicommon.NewSubTaskBase(task, fmt.Sprintf("%s-scan-%s", task.GetId(), cat.ID), task.GetUserInput(), true)
					if err := catLoop.ExecuteWithExistedTask(catSubTask); err != nil {
						log.Warnf("[CodeAudit/Phase2] Category '%s' loop error: %v", cat.ID, err)
					}

					if len(state.GetScanObservations()) == obsCountBefore {
						log.Warnf("[CodeAudit/Phase2] Category '%s' ended without calling complete_scan.", cat.ID)
					}

					log.Infof("[CodeAudit/Phase2] Category '%s' complete. Findings: %d",
						cat.ID, len(state.GetFindings()))
					return nil
				})
			}

			// 等待所有类别扫描完成
			if err := eg.Wait(); err != nil {
				log.Warnf("[CodeAudit/Phase2] Some category scans failed: %v", err)
			}

			allFindings := state.GetFindings()
			state.SetPhase(AuditPhaseVerify)

			r.AddToTimeline("[PHASE2_COMPLETE]",
				fmt.Sprintf("Phase 2 扫描完成。共扫描 %d 个漏洞类别，发现 %d 个疑似漏洞。", len(finalCategories), len(allFindings)))

			log.Infof("[CodeAudit/Phase2] All categories done. Total findings: %d", len(allFindings))
			op.Done()
		}),
	}

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_phase2_orchestrator", r, preset...)
}

// planScanCategories 调用 AI 生成本次审计的扫描类别列表
func planScanCategories(r aicommon.AIInvokeRuntime, task aicommon.AIStatefulTask, state *AuditState) []VulnCategory {
	timeline := getTimeline(r)
	var planCheckpoint int64
	if timeline != nil {
		planCheckpoint = timeline.GetMaxID()
	}
	defer func() {
		if timeline != nil {
			timeline.TruncateAfter(planCheckpoint)
		}
	}()

	// 检查用户是否在 prompt 中指定了扫描类别
	userInput := task.GetUserInput()
	if specified := extractSpecifiedCategories(userInput); len(specified) > 0 {
		log.Infof("[CodeAudit/Phase2] User specified categories: %v", specified)
		r.AddToTimeline("[SCAN_PLAN]",
			fmt.Sprintf("使用用户指定的 %d 个类别: %v", len(specified), specified))
		return specified
	}

	// 检查用户是否在 prompt 中指定了跳过的类别
	skipIDs := extractSkippedCategories(userInput)

	// 构建类别描述（包含标签和语言信息）
	var categoryDesc strings.Builder
	for _, c := range CWECategoryLibrary {
		langInfo := "所有语言"
		if c.LangRe != "" && c.LangRe != "any" {
			langInfo = c.LangRe
		}
		categoryDesc.WriteString(fmt.Sprintf("- %s（id: %s, tag: %s, 语言: %s）\n", c.Name, c.ID, c.Tag, langInfo))
	}

	// 构建项目侦察结果摘要（优先使用结构化预分析数据）
	var reconSummary strings.Builder

	// 结构化预分析数据（Phase 0 产出）
	if preAnalysis := state.PreAnalysis; preAnalysis != nil {
		reconSummary.WriteString(fmt.Sprintf("- **主语言**: %s", preAnalysis.Language))
		if preAnalysis.LanguageVer != "" {
			reconSummary.WriteString(fmt.Sprintf(" %s", preAnalysis.LanguageVer))
		}
		reconSummary.WriteString("\n")

		if len(preAnalysis.HTTPFrameworks) > 0 {
			reconSummary.WriteString(fmt.Sprintf("- **HTTP 框架**: %s\n", strings.Join(uniqueStrings(preAnalysis.HTTPFrameworks), ", ")))
		}
		if len(preAnalysis.DBLibs) > 0 {
			reconSummary.WriteString(fmt.Sprintf("- **数据库库**: %s\n", strings.Join(uniqueStrings(preAnalysis.DBLibs), ", ")))
		}
		if len(preAnalysis.AuthLibs) > 0 {
			reconSummary.WriteString(fmt.Sprintf("- **认证库**: %s\n", strings.Join(uniqueStrings(preAnalysis.AuthLibs), ", ")))
		}
		if len(preAnalysis.ExecLibs) > 0 {
			reconSummary.WriteString(fmt.Sprintf("- **命令执行库**: %s\n", strings.Join(uniqueStrings(preAnalysis.ExecLibs), ", ")))
		}
		if len(preAnalysis.CryptoLibs) > 0 {
			reconSummary.WriteString(fmt.Sprintf("- **加密库**: %s\n", strings.Join(uniqueStrings(preAnalysis.CryptoLibs), ", ")))
		}
		if len(preAnalysis.TemplateLibs) > 0 {
			reconSummary.WriteString(fmt.Sprintf("- **模板引擎**: %s\n", strings.Join(uniqueStrings(preAnalysis.TemplateLibs), ", ")))
		}

		reconSummary.WriteString(fmt.Sprintf("- **项目规模**: %d 文件, ~%d 行\n",
			preAnalysis.ProjectScale.TotalFiles, preAnalysis.ProjectScale.TotalLines))

		if len(preAnalysis.EntryPoints) > 0 {
			reconSummary.WriteString("- **入口点**:\n")
			for _, ep := range preAnalysis.EntryPoints {
				reconSummary.WriteString(fmt.Sprintf("  - `%s:%d` (%s)\n", filepath.Base(ep.File), ep.Line, ep.Type))
			}
		}
	}

	// 回退：使用 LLM 侦察的文本摘要
	if reconSummary.Len() == 0 {
		reconSummary.WriteString(fmt.Sprintf("- **技术栈**: %s\n", state.TechStack))
		reconSummary.WriteString(fmt.Sprintf("- **入口点**: %s\n", state.EntryPoints))
		if state.AuthMechanism != "" {
			reconSummary.WriteString(fmt.Sprintf("- **认证机制**: %s\n", state.AuthMechanism))
		}
		if state.ReconOutline != "" {
			reconSummary.WriteString(fmt.Sprintf("- **项目结构大纲**: %s\n", state.ReconOutline))
		}
	}

	if reconSummary.Len() == 0 {
		reconSummary.WriteString("（侦察结果未知，请根据用户输入推断）")
	}

	if userInput == "" {
		userInput = "（用户未提供额外说明）"
	}

	prompt := fmt.Sprintf(planPromptTemplate, reconSummary.String(), len(CWECategoryLibrary), categoryDesc.String(), userInput)

	ctx := task.GetContext()
	action, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"scan_plan",
		prompt,
		[]aitool.ToolOption{
			aitool.WithStringArrayParam("selected_category_ids",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("从 CWE 类别库中选择的 ID 列表，根据技术栈裁剪不适用的类别"),
			),
			aitool.WithStringParam("extra_categories_json",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description(`用户额外要求的漏洞类别，JSON 数组字符串：
[{"id":"custom_category","name":"类别名称","tag":"分类标签","sink_patterns":"keyword1,keyword2","instruction":"扫描指南"}]
如果没有额外要求，传入空字符串 ""`),
			),
		},
	)

	if err != nil {
		log.Warnf("[CodeAudit/Phase2] Plan AI call failed: %v, falling back to default categories", err)
		return DefaultVulnCategories
	}

	selectedIDs := action.GetStringSlice("selected_category_ids")
	if len(selectedIDs) == 0 {
		log.Warnf("[CodeAudit/Phase2] Plan returned empty selected_category_ids, using all defaults")
		selectedIDs = make([]string, 0, len(CWECategoryLibrary))
		for _, c := range CWECategoryLibrary {
			selectedIDs = append(selectedIDs, c.ID)
		}
	}

	// 过滤掉用户指定跳过的类别
	if len(skipIDs) > 0 {
		skipSet := make(map[string]bool, len(skipIDs))
		for _, id := range skipIDs {
			skipSet[id] = true
		}
		var filtered []string
		for _, id := range selectedIDs {
			if !skipSet[id] {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) < len(selectedIDs) {
			log.Infof("[CodeAudit/Phase2] Skipped %d categories: %v", len(selectedIDs)-len(filtered), skipIDs)
			selectedIDs = filtered
		}
	}

	var result []VulnCategory
	selectedSet := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selectedSet[id] = true
	}
	for _, c := range CWECategoryLibrary {
		if selectedSet[c.ID] {
			result = append(result, c)
		}
	}

	extraRaw := action.GetString("extra_categories_json")
	if extraRaw != "" && extraRaw != "[]" {
		var extras []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Tag          string `json:"tag"`
			SinkPatterns string `json:"sink_patterns"`
			Instruction  string `json:"instruction"`
		}
		if err := json.Unmarshal([]byte(extraRaw), &extras); err == nil {
			for _, e := range extras {
				if e.ID == "" || e.Name == "" {
					continue
				}
				result = append(result, VulnCategory{
					ID:   e.ID,
					Name: e.Name,
					Tag:  e.Tag,
					SinkHints: []SinkHint{
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
		return DefaultVulnCategories
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

// getTimeline 从 AIInvokeRuntime 中提取 *aicommon.Timeline
func getTimeline(r aicommon.AIInvokeRuntime) *aicommon.Timeline {
	cfg := r.GetConfig()
	if cfg == nil {
		return nil
	}
	c, ok := cfg.(*aicommon.Config)
	if !ok {
		return nil
	}
	return c.Timeline
}

// countIDsAfter 返回 timeline 中 ID > checkpoint 的条目数量
func countIDsAfter(timeline *aicommon.Timeline, checkpoint int64) int {
	count := 0
	for _, id := range timeline.GetTimelineItemIDs() {
		if id > checkpoint {
			count++
		}
	}
	return count
}

// buildPrevFindingsSummary 返回当前 category 之前已发现的 findings 摘要（最多显示 10 个）
func buildPrevFindingsSummary(state *AuditState, currentCategoryID string) string {
	findings := state.GetFindings()
	if len(findings) == 0 {
		return ""
	}
	var sb strings.Builder
	count := 0
	for _, f := range findings {
		if f.Category == currentCategoryID {
			continue
		}
		sb.WriteString(fmt.Sprintf("- [%s] %s: %s:%d\n", f.Severity, f.ID, f.File, f.Line))
		count++
		if count >= 10 {
			sb.WriteString(fmt.Sprintf("... 还有 %d 个 finding\n", len(findings)-count))
			break
		}
	}
	return sb.String()
}

// extractSpecifiedCategories 从用户输入中提取指定的扫描类别。
// 如果用户在 prompt 中包含 "[指定扫描类别]" 标记，则解析后面的类别 ID 列表。
func extractSpecifiedCategories(userInput string) []VulnCategory {
	if !strings.Contains(userInput, "[指定扫描类别]") {
		return nil
	}

	// 提取 "只扫描以下类别，不要选择其他类别：" 后面的内容
	prefix := "只扫描以下类别，不要选择其他类别："
	idx := strings.Index(userInput, prefix)
	if idx < 0 {
		return nil
	}
	categoriesStr := userInput[idx+len(prefix):]
	// 截取到下一个换行或字符串结束
	if nlIdx := strings.Index(categoriesStr, "\n"); nlIdx >= 0 {
		categoriesStr = categoriesStr[:nlIdx]
	}
	categoriesStr = strings.TrimSpace(categoriesStr)

	// 解析逗号分隔的类别 ID
	ids := strings.Split(categoriesStr, ",")
	var result []VulnCategory
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		// 在 CWE 库中查找
		for _, c := range CWECategoryLibrary {
			if c.ID == id {
				result = append(result, c)
				break
			}
		}
		// 也在 DefaultVulnCategories 中查找（兼容旧 ID）
		if len(result) == 0 || result[len(result)-1].ID != id {
			for _, c := range DefaultVulnCategories {
				if c.ID == id {
					result = append(result, c)
					break
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// extractSkippedCategories 从用户输入中提取要跳过的类别 ID。
// 如果用户在 prompt 中包含 "[跳过类别]" 标记，则解析后面的类别 ID 列表。
func extractSkippedCategories(userInput string) []string {
	if !strings.Contains(userInput, "[跳过类别]") {
		return nil
	}

	prefix := "以下类别不需要扫描，请在选择时排除："
	idx := strings.Index(userInput, prefix)
	if idx < 0 {
		return nil
	}
	categoriesStr := userInput[idx+len(prefix):]
	if nlIdx := strings.Index(categoriesStr, "\n"); nlIdx >= 0 {
		categoriesStr = categoriesStr[:nlIdx]
	}
	categoriesStr = strings.TrimSpace(categoriesStr)

	ids := strings.Split(categoriesStr, ",")
	var result []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			result = append(result, id)
		}
	}
	return result
}
