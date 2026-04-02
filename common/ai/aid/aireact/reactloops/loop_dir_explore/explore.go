package loop_dir_explore

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/explore_instruction.txt
var exploreInstruction string

//go:embed prompts/explore_output_example.txt
var exploreOutputExample string

const exploreReactiveDataTpl = `## 当前探索状态
<|EXPLORE_STATUS_{{ .Nonce }}|>
[路径规范] 所有工具调用必须使用绝对路径，禁止使用相对路径
目标目录: {{ .TargetPath }}
探索进度: 已执行 {{ .IterationCount }} 次操作
{{ if .NoteFiles }}已写出探索文件（{{ .NoteFileCount }} / 建议至少 3 个）:
{{ .NoteFiles }}{{ else }}尚未写出任何探索文件（建议至少写出 3 个：dir_structure.md / entry_points.md / tech_stack.md）{{ end }}
{{ if .FeedbackMessages }}
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|EXPLORE_STATUS_END_{{ .Nonce }}|>
{{ if .NeedMoreFiles }}
[文件数量不足] 已写出 {{ .NoteFileCount }} 个文件，建议至少写出以下类别后再调用 complete_explore：
  - 目录结构文件（dir_structure.md）
  - 入口点文件（entry_points.md）
  - 技术栈与依赖文件（tech_stack.md）
{{ end }}

[重要] complete_explore 是本 loop 退出的唯一合法方式。调用前请确保已用 write_file 写出足够的探索文件（建议 3 个以上）。`

// ExploreState 保存探索 loop 的共享状态
type ExploreState struct {
	// 要探索的目标路径（必须，由 InitTask 从用户输入提取）
	TargetPath string

	// 已写出的探索文件路径列表
	noteFiles []string

	// complete_explore 提交的摘要信息
	ProjectName    string
	TechStack      string
	EntryPoints    string
	ModulesSummary string

	// 最终报告文件路径
	ReportFilePath string
}

func newExploreState() *ExploreState {
	return &ExploreState{}
}

func (s *ExploreState) addNoteFile(path string) {
	for _, f := range s.noteFiles {
		if f == path {
			return
		}
	}
	s.noteFiles = append(s.noteFiles, path)
}

func (s *ExploreState) getNoteFiles() []string {
	result := make([]string, len(s.noteFiles))
	copy(result, s.noteFiles)
	return result
}

// extractTargetPath 使用 LiteForge 从用户输入中提取目标目录路径。
func extractTargetPath(ctx context.Context, r aicommon.AIInvokeRuntime, userInput string) (string, error) {
	promptTpl := `分析用户的请求，提取需要探索的目标目录路径。

## 用户输入
<|USER_INPUT_{{ .Nonce }}|>
{{ .UserInput }}
<|USER_INPUT_END_{{ .Nonce }}|>

## 提取规则
1. 找到用户希望 AI 探索/分析的目录的绝对路径
2. 路径通常是一个本地文件系统路径，例如 "/home/user/myproject" 或 "/Users/me/code/app"
3. 如果用户提到多个路径，选择最主要/最明确的那个
4. 如果没有找到任何目录路径，返回空字符串
5. 输出路径必须是绝对路径（以 / 或 驱动器字母 开头）

请返回目标路径。`

	rendered, err := utils.RenderTemplate(promptTpl, map[string]any{
		"Nonce":     utils.RandStringBytes(4),
		"UserInput": userInput,
	})
	if err != nil {
		return "", utils.Wrap(err, "render extract-path prompt")
	}

	result, err := r.InvokeSpeedPriorityLiteForge(
		ctx,
		"extract-explore-target-path",
		rendered,
		[]aitool.ToolOption{
			aitool.WithStringParam("target_path",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("需要探索的目标目录绝对路径，如果没有找到则返回空字符串")),
			aitool.WithStringParam("reason",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("简要说明从用户输入中识别到的路径依据")),
		},
		aicommon.WithGeneralConfigStreamableFieldWithNodeId("intent", "reason"),
	)
	if err != nil {
		return "", utils.Wrap(err, "LiteForge extract target path")
	}

	path := strings.TrimSpace(result.GetString("target_path"))
	reason := result.GetString("reason")
	log.Infof("[DirExplore] extracted target path: %q (reason: %s)", path, reason)
	return path, nil
}

// BuildDirExploreLoop 构建目录探索 Loop。
//
// 必须从用户输入中提取到有效的目标目录路径，否则 loop 会 fail 并给出原因。
// 探索笔记文件默认写入 aispace workdir/explore/ 目录（通过 EmitFileArtifactWithExt 机制）；
// 若用户通过 loop var "output_report_path" 指定了输出路径，则最终报告写入该路径。
func BuildDirExploreLoop(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	state := newExploreState()

	preset := []reactloops.ReActLoopOption{
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(false),
		reactloops.WithSameActionTypeSpinThreshold(3),
		reactloops.WithSameLogicSpinThreshold(3),
		reactloops.WithMaxConsecutiveSpinWarnings(2),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),

		// InitTask：提取目标路径，失败则拒绝启动
		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			userInput := task.GetUserInput()
			log.Infof("[DirExplore] Init: extracting target path from user input")

			// 尝试从 loop var 中直接获取路径（调用方可提前注入）
			targetPath := strings.TrimSpace(loop.Get("target_path"))

			// 如果没有预注入路径，则用 LiteForge 从用户输入中提取
			if targetPath == "" {
				extracted, err := extractTargetPath(task.GetContext(), r, userInput)
				if err != nil {
					log.Warnf("[DirExplore] LiteForge extraction failed: %v, will require path from AI", err)
				} else {
					targetPath = extracted
				}
			}

			// 路径为空 → fail
			if targetPath == "" {
				op.Failed(utils.Error(
					"[DirExplore] 无法从用户输入中提取目标目录路径。\n" +
						"请在请求中明确指定需要探索的目录绝对路径，例如：\n" +
						"  '请探索 /home/user/myproject 目录'\n" +
						"  '帮我分析 /Users/me/code/webapp 项目的结构'\n",
				))
				return
			}

			// 路径不存在 → fail
			if _, err := os.Stat(targetPath); err != nil {
				op.Failed(utils.Errorf(
					"[DirExplore] 目标目录不存在或无法访问: %s\n错误: %v\n请确认路径正确。",
					targetPath, err,
				))
				return
			}

			state.TargetPath = targetPath
			log.Infof("[DirExplore] Target path confirmed: %s", targetPath)
			r.AddToTimeline("[EXPLORE_START]", fmt.Sprintf("开始目录探索，目标路径: %s", targetPath))
			op.Continue()
		}),

		// PersistentContextProvider：每轮注入探索指令，动态替换工作目录提示
		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			// 探索笔记的推荐写入目录：使用 aispace 机制生成，通过 loop var 缓存
			exploreWorkDir := getOrCreateExploreWorkDir(r, loop)
			return utils.RenderTemplate(exploreInstruction, map[string]any{
				"Nonce":          nonce,
				"TargetPath":     state.TargetPath,
				"ExploreWorkDir": exploreWorkDir,
			})
		}),
		reactloops.WithReflectionOutputExample(exploreOutputExample),

		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			iterCount := loop.GetCurrentIterationIndex()
			noteFileList := state.getNoteFiles()
			noteFiles := ""
			for _, f := range noteFileList {
				noteFiles += "  - " + f + "\n"
			}
			exploreWorkDir := getOrCreateExploreWorkDir(r, loop)
			hasFiles := len(noteFileList) > 0
			needMoreFiles := hasFiles && len(noteFileList) < 3 && iterCount >= 10
			return utils.RenderTemplate(exploreReactiveDataTpl, map[string]any{
				"Nonce":            nonce,
				"TargetPath":       state.TargetPath,
				"IterationCount":   iterCount,
				"NoteFiles":        noteFiles,
				"NoteFileCount":    len(noteFileList),
				"ExploreWorkDir":   exploreWorkDir,
				"FeedbackMessages": feedbacker.String(),
				"NeedMoreFiles":    needMoreFiles,
			})
		}),
	}

	// 注册文件系统工具
	preset = append(preset, buildFSToolAction(r, "tree", nil))
	preset = append(preset, buildFSToolAction(r, "read_file", nil))
	preset = append(preset, buildFSToolAction(r, "grep", nil))
	preset = append(preset, buildFSToolAction(r, "find_file", nil))
	preset = append(preset, buildFSToolAction(r, "write_file", func(action *aicommon.Action) {
		filePath := action.GetString("file")
		if filePath != "" {
			state.addNoteFile(filePath)
			log.Infof("[DirExplore] Explore note written: %s", filePath)
		}
	}))

	// complete_explore：AI 探索完成后调用
	preset = append(preset, reactloops.WithRegisterLoopAction(
		"complete_explore",
		"完成目录探索，提交技术栈摘要、入口点摘要、核心模块概览。调用前必须已通过 write_file 写出探索文件。系统将把这些文件作为参考材料传给报告生成器。",
		[]aitool.ToolOption{
			aitool.WithStringParam("project_name",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("项目名称（可选），默认取目标路径的最后一段目录名")),
			aitool.WithStringParam("tech_stack",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("技术栈摘要（一行），如 'Go 1.21, gRPC, SQLite' 或 'PHP 7.4, Laravel 8, MySQL 5.7'")),
			aitool.WithStringParam("entry_points",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("所有入口点摘要，多入口点用逗号或分号分隔，如 'cmd/yak/main.go (Yak解释器), cmd/server/main.go (gRPC服务端)'")),
			aitool.WithStringParam("modules_summary",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("核心模块职责概览（一行），如 'common/yak (引擎), common/ai (AI框架), common/yakgrpc (gRPC层)'")),
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action) error {
			noteFiles := state.getNoteFiles()
			if len(noteFiles) == 0 {
				return utils.Error(
					"[complete_explore 被拒绝] 你尚未通过 write_file 写出任何探索文件。" +
						"请先将已探索到的信息（目录结构/入口点/技术栈/模块概览等）写入文件，再调用 complete_explore。" +
						"推荐写出至少 3 个探索文件，例如：dir_structure.md, entry_points.md, tech_stack.md",
				)
			}
			hasDirStructure := false
			for _, f := range noteFiles {
				if strings.HasSuffix(f, "/dir_structure.md") || strings.HasSuffix(f, "\\dir_structure.md") {
					hasDirStructure = true
					break
				}
			}
			if !hasDirStructure {
				return utils.Error(
					"[complete_explore 被拒绝] 缺少 dir_structure.md 文件。" +
						"dir_structure.md 包含完整目录树，是最终报告的核心素材，必须在调用 complete_explore 之前写出。" +
						"请先调用 tree 获取完整目录结构，然后用 write_file 写入 dir_structure.md，再重试 complete_explore。",
				)
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			projectName := action.GetString("project_name")
			techStack := action.GetString("tech_stack")
			entryPoints := action.GetString("entry_points")
			modulesSummary := action.GetString("modules_summary")

			if projectName == "" && state.TargetPath != "" {
				projectName = filepath.Base(state.TargetPath)
			}

			state.ProjectName = projectName
			state.TechStack = techStack
			state.EntryPoints = entryPoints
			state.ModulesSummary = modulesSummary

			// 将结果写入 loop vars，便于外部调用方在 loop 结束后读取
			loop.Set("result_project_name", projectName)
			loop.Set("result_tech_stack", techStack)
			loop.Set("result_entry_points", entryPoints)
			loop.Set("result_modules_summary", modulesSummary)
			loop.Set("result_target_path", state.TargetPath)

			noteFiles := state.getNoteFiles()
			if len(noteFiles) > 0 {
				loop.Set("result_note_files", strings.Join(noteFiles, "\n"))
			}

			r.AddToTimeline("[EXPLORE_COMPLETE]",
				fmt.Sprintf("目录探索完成\n目标路径: %s\n技术栈: %s\n入口点: %s\n模块: %s\n探索文件(%d个): %v",
					state.TargetPath, techStack, entryPoints, modulesSummary, len(noteFiles), noteFiles))
			log.Infof("[DirExplore] Explore complete. Tech: %s, note files: %v", techStack, noteFiles)

			// 确定报告输出路径：
			//   1. 若调用方通过 loop var "output_report_path" 指定了路径，使用该路径
			//   2. 否则通过 EmitFileArtifactWithExt 写到 aispace（默认行为）
			reportPath := strings.TrimSpace(loop.Get("output_report_path"))
			if reportPath == "" {
				// 使用 aispace 默认行为
				name := projectName
				if name == "" {
					name = "explore"
				}
				reportPath = r.EmitFileArtifactWithExt(name+"_explore_report", ".md", "")
			} else {
				// 用户指定路径：确保父目录存在，并创建空文件以便 report_generating 写入
				if err := os.MkdirAll(filepath.Dir(reportPath), 0o755); err != nil {
					log.Warnf("[DirExplore] Failed to create report dir: %v", err)
				} else if err := os.WriteFile(reportPath, []byte(""), 0o644); err != nil {
					log.Warnf("[DirExplore] Failed to create report file: %v", err)
				}
				_, _ = r.GetConfig().GetEmitter().EmitPinFilename(reportPath)
			}

			if reportPath != "" {
				writePrompt := buildExploreReportPrompt(state, reportPath, noteFiles)
				if err := generateExploreReport(r, loop, writePrompt, reportPath, noteFiles, state); err != nil {
					log.Warnf("[DirExplore] Failed to generate explore report: %v", err)
				} else {
					state.ReportFilePath = reportPath
					loop.Set("result_report_path", reportPath)
					log.Infof("[DirExplore] Explore report written to: %s", reportPath)
				}
			}

			op.Feedback(fmt.Sprintf("探索完成，报告已生成: %s", reportPath))
			op.Exit()
		},
	))

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("dir_explore", r, preset...)
}

// getOrCreateExploreWorkDir 返回探索笔记的写出目录（即 aispace workdir）。
// 直接通过 r.GetConfig().GetOrCreateWorkDir() 获取，首次调用时懒创建并 pin 目录，
// 然后缓存到 loop var 供后续复用。
func getOrCreateExploreWorkDir(r aicommon.AIInvokeRuntime, loop *reactloops.ReActLoop) string {
	if cached := loop.Get("explore_work_dir"); cached != "" {
		return cached
	}
	dir := r.GetConfig().GetOrCreateWorkDir()
	if dir != "" {
		loop.Set("explore_work_dir", dir)
	}
	return dir
}

// buildFSToolAction 直接用 WithRegisterLoopAction 注册文件系统工具。
// 在 handler 里手动调用 invoker.ExecuteToolRequiredAndCallWithoutRequired，
// 执行完后直接 op.Continue()，不经过 ConvertAIToolToLoopAction，
// 因此完全绕开了 VerifyUserSatisfaction 退出逻辑。
// onAction 是可选的执行后回调，用于记录副作用（如 write_file 记录路径）。
func buildFSToolAction(r aicommon.AIInvokeRuntime, toolName string, onAction func(action *aicommon.Action)) reactloops.ReActLoopOption {
	toolMgr := r.GetConfig().GetAiToolManager()
	if toolMgr == nil {
		log.Warnf("[DirExplore] tool manager not available, skip %q action", toolName)
		return func(r *reactloops.ReActLoop) {}
	}
	tool, err := toolMgr.GetToolByName(toolName)
	if err != nil || tool == nil {
		log.Warnf("[DirExplore] tool %q not found: %v", toolName, err)
		return func(r *reactloops.ReActLoop) {}
	}

	return reactloops.WithRegisterLoopAction(
		toolName,
		tool.GetDescription(),
		tool.BuildParamsOptions(),
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			invoker := loop.GetInvoker()
			ctx := loop.GetConfig().GetContext()
			if task := loop.GetCurrentTask(); task != nil && !utils.IsNil(task.GetContext()) {
				ctx = task.GetContext()
			}

			params := action.GetParams()
			result, _, err := invoker.ExecuteToolRequiredAndCallWithoutRequired(ctx, toolName, params)
			if err != nil {
				log.Warnf("[DirExplore] tool %q failed: %v", toolName, err)
				op.Feedback(fmt.Sprintf("[工具执行失败] %s: %v，请尝试其他方法。", toolName, err))
				op.Continue()
				return
			}

			content := ""
			if result != nil {
				content = utils.InterfaceToString(result.Data)
			}
			invoker.AddToTimeline(fmt.Sprintf("[%s]", toolName),
				utils.ShrinkString(content, 2048))

			// 对 grep 工具做额外的空结果检测：
			// 如果输出只含 [info] 行而无任何匹配行，说明搜索范围太大或模式有误，
			// 给出明确提示引导 AI 缩小范围重试。
			if toolName == "grep" {
				if isGrepEmptyResult(content) {
					searchPath := action.GetString("path")
					pattern := action.GetString("pattern")
					hint := fmt.Sprintf(
						"[grep 结果为空] 在路径 %q 中未找到模式 %q 的匹配。可能原因：\n"+
							"  1. 搜索范围太大导致超时（仓库根目录不适合直接 grep）\n"+
							"  2. 文件扩展名未过滤，扫描了大量无关文件\n"+
							"建议措施：\n"+
							"  - 用 find_file 先定位包含入口的文件，再对具体文件/小目录 grep\n"+
							"  - 添加 include-ext 参数（如 include-ext=\".go\" 或 \".java\"）\n"+
							"  - 缩小 path 范围（如改为 %q/cmd 或 %q/src）\n"+
							"  - 检查 pattern 是否正确（如 Go 应用 \"func main()\" 而不是 \"main()\"）",
						searchPath, pattern, searchPath, searchPath,
					)
					log.Infof("[DirExplore] grep returned empty results for path=%q pattern=%q", searchPath, pattern)
					invoker.AddToTimeline("[GREP_EMPTY_RESULT]", hint)
					op.Feedback(hint)
					op.Continue()
					return
				}
			}

			op.Feedback(fmt.Sprintf("[%s 完成] 输出 %d 字节", toolName, len(content)))
			op.Continue()

			if onAction != nil {
				onAction(action)
			}
		},
	)
}

// isGrepEmptyResult 判断 grep 工具的输出是否为空结果（只有 [info] 头信息，无实际匹配行）。
// grep 工具有匹配时每行格式为 "filepath:lineNo: content"，无匹配时只输出 [info] 行。
// 通过 JSON 结构解析 stdout 字段来判断。
func isGrepEmptyResult(content string) bool {
	if content == "" {
		return true
	}
	// content 是 JSON 格式 {"stdout": "..."}，解析其中的 stdout
	var parsed map[string]any
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		// 非 JSON，直接检查字符串
		return isGrepStdoutEmpty(content)
	}
	stdout, _ := parsed["stdout"].(string)
	return isGrepStdoutEmpty(stdout)
}

// isGrepStdoutEmpty 检查 grep stdout 是否只有 info 行（即没有实际匹配结果）。
func isGrepStdoutEmpty(stdout string) bool {
	if stdout == "" {
		return true
	}
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 实际匹配行不以 [info] 或 [warn] 或 [error] 开头
		if !strings.HasPrefix(line, "[info]") &&
			!strings.HasPrefix(line, "[warn]") &&
			!strings.HasPrefix(line, "[error]") &&
			!strings.HasPrefix(line, "[debug]") {
			return false // 有非 info 行，说明有匹配结果
		}
	}
	return true // 全是 info/warn/error 行，视为空结果
}

// buildAvailableFilesHint 构造报告生成器可见的参考文件列表提示
func buildAvailableFilesHint(files []string) string {
	if len(files) == 0 {
		return "（探索未写出任何文件）"
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### 探索文件（共 %d 个，必须全部读取后再开始写报告）\n", len(files)))
	for i, f := range files {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, f))
	}
	sb.WriteString("\n> [强制] 在调用 write_section 之前，必须对以上每一个文件都调用 read_reference_file。\n")
	return sb.String()
}

// buildExploreReportPrompt 构造传给 report_generating loop 的写作任务描述
func buildExploreReportPrompt(state *ExploreState, outputPath string, noteFiles []string) string {
	fileHint := "（探索未写出任何文件，请根据下方项目信息直接生成报告框架）"
	mustReadAll := ""
	if len(noteFiles) > 0 {
		lines := make([]string, 0, len(noteFiles))
		for _, f := range noteFiles {
			lines = append(lines, fmt.Sprintf("- %s", f))
		}
		fileHint = fmt.Sprintf("以下 **%d 个**探索文件已就绪，必须全部读取：\n%s",
			len(noteFiles), strings.Join(lines, "\n"))
		mustReadAll = fmt.Sprintf(`
## [强制约束] 必须读完所有 %d 个探索文件

在调用 write_section 之前，你必须对上面列出的每一个文件都调用一次 read_reference_file。
- 禁止在读完全部文件之前开始写作
- 读取顺序不限，但必须全部读完
- 如果某个文件读取失败，跳过并继续读其余文件
- 读完所有文件后，将全部内容整合后一次性写入报告
`, len(noteFiles))
	}

	return fmt.Sprintf(`请汇总目录探索文件，生成结构化的项目探索报告。

## 项目信息
- **项目名称**: %s
- **目标路径**: %s
- **技术栈**: %s
- **入口点**: %s
- **核心模块**: %s

## 参考文件

%s
%s
## 写作要求

1. **[必须]** 先用 read_reference_file 逐一读取上方列出的**所有**探索文件，不得遗漏
2. 读完所有文件后，按以下结构整合内容写入报告：
   - 项目概览（一句话描述项目用途）
   - 目录结构树：**必须原样保留** dir_structure.md 中的完整目录树，一个目录都不能省略。禁止用 "..." 或 "[多种xxx]" 省略任何目录
   - 技术栈与关键依赖
   - 所有入口点列表（每个入口点单独一节，说明功能、启动方式）
   - 核心模块职责概览（每个顶层包/模块一句话描述）
   - 关键配置文件路径
3. **目录树完整性是硬性要求**：如果 dir_structure.md 包含 700 行目录树，报告中也必须有 700 行。不允许精简、归纳或省略
4. 输出文件: %s

报告使用 Markdown 格式，用 write_section 写入。`,
		state.ProjectName,
		state.TargetPath,
		state.TechStack,
		state.EntryPoints,
		state.ModulesSummary,
		fileHint,
		mustReadAll,
		outputPath,
	)
}
