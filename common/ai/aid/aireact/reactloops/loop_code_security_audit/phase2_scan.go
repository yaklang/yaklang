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
}

func newScanState() *ScanState {
	return &ScanState{
		Phase:         ScanPhaseSearch,
		TargetFileSet: make(map[string]bool),
		AuditedFiles:  make(map[string]bool),
	}
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

// ─────────────────────────────────────────────────────────────────────
// Reactive data 模板
// ─────────────────────────────────────────────────────────────────────

const phase2ReactiveDataTpl = `## 当前扫描任务
<|SCAN_TASK_{{ .Nonce }}|>
**漏洞类别**: {{ .CategoryName }} ({{ .CategoryID }})
**技术栈**: {{ .TechStack }}
**入口点摘要**: {{ .EntryPoints }}

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
目标：根据下方 **Sink 语义提示** 和已知技术栈，自主决定 grep 关键词，**使用 output-mode="files_with_matches" 模式**获取文件列表，每次搜索后立即调用 lock_target_files 追加命中文件。可多次 grep + lock_target_files 累积文件；收集完毕后调用 lock_target_files(done=true) 切换到阶段B。
**重要**：阶段A的 grep 必须指定 output-mode="files_with_matches"，避免因 limit 限制遗漏大量文件。

**已累积目标文件**: {{ .CollectedFileCount }} 个

**Sink 语义提示**（根据实际技术栈自主选择合适的 grep 关键词，示例仅供参考）:
{{ .SinkHints }}
{{ else }}
### 阶段B：逐文件审计
目标：对每个目标文件依次调用 read_file(file=<绝对路径>) 审计，发现漏洞调用 add_finding，完成后调用 mark_file_done。所有文件完成后调用 complete_scan。
注意：read_file 的路径参数名是 file（不是 path），必须使用文件完整绝对路径。

**审计进度**: {{ .AuditDone }} / {{ .AuditTotal }} 个文件已完成

**待审计文件**:
{{ .RemainingFilesList }}
{{ end }}

{{ if .FeedbackMessages }}
---
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|SCAN_TASK_END_{{ .Nonce }}|>

**当前迭代**: {{ .IterationCount }} | **本轮 Finding 数**: {{ .FindingsCount }}

{{ if not .IsSearchPhase }}[终止规则] complete_scan 是阶段B退出的唯一合法方式。每个目标文件审计完后必须调用 mark_file_done，所有文件完成后立即调用 complete_scan。不调用 complete_scan 则 loop 不会结束。{{ end }}`

// buildSingleCategoryScanLoop 构建针对单一漏洞类别的扫描 Loop（两阶段：grep→逐文件审计）
func buildSingleCategoryScanLoop(r aicommon.AIInvokeRuntime, state *AuditState, category VulnCategory) (*reactloops.ReActLoop, error) {
	scan := newScanState()

	maxIter := math.MaxInt32

	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(maxIter),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(true),
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
				"Nonce":       nonce,
				"ReconFile":   state.GetReconFilePath(),
				"TechStack":   state.TechStack,
				"EntryPoints": state.EntryPoints,
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
			reactivePrompt, err := utils.RenderTemplate(phase2ReactiveDataTpl, map[string]any{
				"Nonce":               nonce,
				"CategoryID":          category.ID,
				"CategoryName":        category.Name,
				"TechStack":           state.TechStack,
				"EntryPoints":         state.EntryPoints,
				"ReconOutline":        state.GetReconOutline(),
				"ReconFileHint":       reconFileHint,
				"PrevFindingsSummary": buildPrevFindingsSummary(state, category.ID),
				"PhaseLabel":          phaseLabel,
				"IsSearchPhase":       isSearchPhase,
				"SinkHints":           sinkHintsText,
				"CollectedFileCount":  scan.TargetFileCount(),
				"AuditDone":           auditDone,
				"AuditTotal":          auditTotal,
				"RemainingFilesList":  remainingFilesSB.String(),
				"FeedbackMessages":    feedbacker.String(),
				"FindingsCount":       len(state.GetFindings()),
				"IterationCount":      iterCount,
			})
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
					op.Feedback("[错误] 已处于审计阶段（阶段B），无法继续追加文件。")
					return
				}
				scan.mu.Unlock()

				files := action.GetStringSlice("target_files")
				done := action.GetBool("done")
				reason := action.GetString("reason")

				added, total := scan.AddTargetFiles(files)

				logMsg := fmt.Sprintf("[Phase2/%s] 追加目标文件 +%d 个（累计 %d 个）", category.ID, added, total)
				if reason != "" {
					logMsg += "，理由: " + reason
				}
				r.AddToTimeline("[ADD_TARGET_FILES]", logMsg)
				log.Infof("[CodeAudit/Phase2/%s] Added %d files (total %d)", category.ID, added, total)

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

	loopName := fmt.Sprintf("code_audit_scan_%s", category.ID)
	return reactloops.NewReActLoop(loopName, r, preset...)
}

// ─────────────────────────────────────────────────────────────────────
// Phase 2 编排层
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

// buildPhase2AllCategoriesLoop 构建 Phase 2 编排 Loop
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

			log.Infof("[CodeAudit/Phase2] Starting serial scan of %d categories", len(finalCategories))
			r.AddToTimeline("[PHASE2_START]",
				fmt.Sprintf("Phase 2 开始：将串行扫描 %d 个漏洞类别，各类别 agent 间 timeline 隔离。", len(finalCategories)))

			timeline := getTimeline(r)

			for i, category := range finalCategories {
				log.Infof("[CodeAudit/Phase2] [%d/%d] Starting category: %s (%s)",
					i+1, len(finalCategories), category.Name, category.ID)

				var checkpoint int64
				if timeline != nil {
					checkpoint = timeline.GetMaxID()
				}

				catLoop, err := buildSingleCategoryScanLoop(r, state, category)
				if err != nil {
					log.Errorf("[CodeAudit/Phase2] Failed to build loop for category '%s': %v", category.ID, err)
					continue
				}

				obsCountBefore := len(state.GetScanObservations())
				catSubTask := aicommon.NewSubTaskBase(task, fmt.Sprintf("%s-scan-%s", task.GetId(), category.ID), task.GetUserInput(), true)
				if err := catLoop.ExecuteWithExistedTask(catSubTask); err != nil {
					log.Warnf("[CodeAudit/Phase2] Category '%s' loop error: %v", category.ID, err)
				}

				// 完成性检查：complete_scan 未被调用时（ScanObservation 未新增）记录警告
				if len(state.GetScanObservations()) == obsCountBefore {
					log.Warnf("[CodeAudit/Phase2] Category '%s' ended without calling complete_scan. "+
						"AI may have exited the loop prematurely.", category.ID)
					r.AddToTimeline("[PHASE2_CAT_INCOMPLETE]",
						fmt.Sprintf("警告：类别 '%s' 扫描未调用 complete_scan 就结束了，可能未完整审计。", category.ID))
				}

				if timeline != nil {
					truncated := countIDsAfter(timeline, checkpoint)
					timeline.TruncateAfter(checkpoint)
					log.Infof("[CodeAudit/Phase2] Category '%s' done. Timeline rolled back: removed %d entries",
						category.ID, truncated)
				}

				log.Infof("[CodeAudit/Phase2] [%d/%d] Category '%s' complete. Total findings so far: %d",
					i+1, len(finalCategories), category.ID, len(state.GetFindings()))
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

	var defaultDesc strings.Builder
	for _, c := range DefaultVulnCategories {
		defaultDesc.WriteString(fmt.Sprintf("- %s（id: %s）\n", c.Name, c.ID))
	}

	userInput := task.GetUserInput()
	if userInput == "" {
		userInput = "（用户未提供额外说明）"
	}

	prompt := fmt.Sprintf(planPromptTemplate, defaultDesc.String(), userInput)

	ctx := task.GetContext()
	action, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
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

	if err != nil {
		log.Warnf("[CodeAudit/Phase2] Plan AI call failed: %v, falling back to default categories", err)
		return DefaultVulnCategories
	}

	selectedIDs := action.GetStringSlice("selected_category_ids")
	if len(selectedIDs) == 0 {
		log.Warnf("[CodeAudit/Phase2] Plan returned empty selected_category_ids, using all defaults")
		selectedIDs = make([]string, 0, len(DefaultVulnCategories))
		for _, c := range DefaultVulnCategories {
			selectedIDs = append(selectedIDs, c.ID)
		}
	}

	var result []VulnCategory
	selectedSet := make(map[string]bool, len(selectedIDs))
	for _, id := range selectedIDs {
		selectedSet[id] = true
	}
	for _, c := range DefaultVulnCategories {
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
				result = append(result, VulnCategory{
					ID:   e.ID,
					Name: e.Name,
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

// buildPrevFindingsSummary 返回当前 category 之前已发现的 findings 摘要
func buildPrevFindingsSummary(state *AuditState, currentCategoryID string) string {
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
