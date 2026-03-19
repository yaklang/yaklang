package loop_code_security_audit

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

//go:embed prompts/phase1_recon_instruction.txt
var phase1ReconInstruction string

//go:embed prompts/phase1_output_example.txt
var phase1OutputExample string

const phase1ReactiveDataTpl = `## 当前探索状态
<|RECON_STATUS_{{ .Nonce }}|>
[路径规范] 所有工具调用必须使用用户指定的项目绝对路径，禁止使用相对路径
探索阶段: {{ .Phase }}
已用工具调用次数: {{ .IterationCount }} / 20（超过12次请立即调用 complete_recon）
{{ if .TechStack }}已识别技术栈: {{ .TechStack }}{{ end }}
{{ if .EntryPoints }}已识别入口点摘要: {{ .EntryPoints }}{{ end }}
{{ if .FeedbackMessages }}
### 上步操作反馈
{{ .FeedbackMessages }}
{{ end }}
<|RECON_STATUS_END_{{ .Nonce }}|>
{{ if .ShouldFinish }}
[严重警告] 已执行 {{ .IterationCount }} 次操作！必须立即调用 complete_recon，这是唯一的退出方式！
{{ end }}
[重要] complete_recon 是本 loop 退出的唯一合法方式。每次工具调用后必须继续执行，直到调用 complete_recon 为止。不调用 complete_recon 则 loop 不会结束，后续审计阶段无法启动。`

// buildPhase1ReconLoop 构建 Phase 1 探索 Loop。
// 使用 Yak 文件系统工具（tree/read_file/grep/find_file）。
func buildPhase1ReconLoop(r aicommon.AIInvokeRuntime, state *AuditState, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
	preset := []reactloops.ReActLoopOption{
		reactloops.WithMaxIterations(20),
		reactloops.WithAllowRAG(false),
		reactloops.WithAllowAIForge(false),
		reactloops.WithAllowPlanAndExec(false),
		reactloops.WithAllowToolCall(false),
		reactloops.WithAllowUserInteract(false),
		reactloops.WithEnableSelfReflection(false),
		reactloops.WithSameActionTypeSpinThreshold(4),
		reactloops.WithSameLogicSpinThreshold(3),
		reactloops.WithMaxConsecutiveSpinWarnings(2),
		reactloops.WithActionFilter(func(action *reactloops.LoopAction) bool {
			return action.ActionType != "load_capability"
		}),

		reactloops.WithPersistentContextProvider(func(loop *reactloops.ReActLoop, nonce string) (string, error) {
			return utils.RenderTemplate(phase1ReconInstruction, map[string]any{
				"Nonce": nonce,
			})
		}),
		reactloops.WithReflectionOutputExample(phase1OutputExample),

		reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
			iterCount := loop.GetCurrentIterationIndex()
			return utils.RenderTemplate(phase1ReactiveDataTpl, map[string]any{
				"Nonce":            nonce,
				"Phase":            string(state.GetPhase()),
				"TechStack":        state.TechStack,
				"EntryPoints":      state.EntryPoints,
				"FeedbackMessages": feedbacker.String(),
				"IterationCount":   iterCount,
				"ShouldFinish":     iterCount >= 12,
			})
		}),

		reactloops.WithInitTask(func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
			log.Infof("[CodeAudit/Phase1] Recon loop started")
			op.Continue()
		}),

		// 文件系统工具（Yak 脚本提供）
		buildFSAction(r, "tree"),
		buildFSAction(r, "read_file"),
		buildFSAction(r, "grep"),
		buildFSAction(r, "find_file"),
	}

	// complete_recon: AI 探索完成后调用
	preset = append(preset, reactloops.WithRegisterLoopAction(
		"complete_recon",
		"完成项目探索，提交项目路径、技术栈摘要、入口点摘要、认证机制摘要以及侦察报告大纲（章节列表）。系统将自动启动报告生成器按大纲写入详细侦察笔记文件。",
		[]aitool.ToolOption{
			aitool.WithStringParam("project_path",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("用户指定的项目绝对路径，如 '/home/user/myproject'")),
			aitool.WithStringParam("project_name",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("项目名称（可选），默认取 project_path 的最后一段目录名")),
			aitool.WithStringParam("tech_stack",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("技术栈摘要（一行），如 'PHP 7.4, Laravel 8, MySQL 5.7, Redis'")),
			aitool.WithStringParam("entry_points",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("入口点摘要（一行），如 'routes/web.php, app/Http/Controllers/, /api/v1/'")),
			aitool.WithStringParam("auth_mechanism",
				aitool.WithParam_Required(false),
				aitool.WithParam_Description("认证机制摘要（一行），如 'JWT, AuthMiddleware@Auth.php, 公开路由: /login /register'")),
			aitool.WithStringParam("outline",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("侦察报告大纲，列出报告将包含的章节标题（Markdown ## 格式列表）。系统会据此生成详细报告。示例：'## 目录结构\n## 路由列表\n## 数据访问模式\n## 认证中间件\n## 用户输入点\n## 潜在危险点'")),
		},
		nil,
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			projectPath := action.GetString("project_path")
			projectName := action.GetString("project_name")
			techStack := action.GetString("tech_stack")
			entryPoints := action.GetString("entry_points")
			authMechanism := action.GetString("auth_mechanism")
			outline := action.GetString("outline")

			// 由 AI 回填项目路径；project_name 为空时取路径末段
			if projectName == "" && projectPath != "" {
				projectName = filepath.Base(projectPath)
			}
			state.SetProjectInfo(projectPath, projectName)
			state.SetReconResult(techStack, entryPoints, authMechanism)
			if outline != "" {
				state.SetReconOutline(outline)
			}

			r.AddToTimeline("[RECON_COMPLETE]",
				fmt.Sprintf("Phase 1 探索完成\n项目路径: %s\n技术栈: %s\n入口点: %s\n认证: %s\n报告大纲:\n%s",
					projectPath, techStack, entryPoints, authMechanism, outline))
			log.Infof("[CodeAudit/Phase1] Recon complete. Tech: %s", techStack)

			// 启动 report_generating 子 loop，按大纲写入详细侦察笔记
			reconFilePath := buildReconFilePath(state)
			if reconFilePath != "" && outline != "" {
				if err := os.MkdirAll(filepath.Dir(reconFilePath), 0o755); err != nil {
					log.Warnf("[CodeAudit/Phase1] Failed to create audit dir: %v", err)
				} else {
					// 提前注册路径：只要目录创建成功就注册，不依赖子 loop 执行结果
					// 这样即使报告生成 loop 部分失败，Phase2/3 仍能找到文件路径尝试读取
					_ = os.WriteFile(reconFilePath, []byte(""), 0o644)
					state.SetReconFilePath(reconFilePath)
					log.Infof("[CodeAudit/Phase1] Recon notes path registered: %s", reconFilePath)

					writePrompt := buildReconReportWritePrompt(state, outline, reconFilePath)
					reportLoop, err := reactloops.CreateLoopByName(
						schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
						r,
						reactloops.WithMaxIterations(25),
						reactloops.WithAllowUserInteract(false),
						// 传递输出文件路径和任务描述
						reactloops.WithInitTask(func(innerLoop *reactloops.ReActLoop, task aicommon.AIStatefulTask, innerOp *reactloops.InitTaskOperator) {
							innerLoop.Set("report_filename", reconFilePath)
							innerLoop.Set("full_report_code", "")
							innerLoop.Set("user_requirements", writePrompt)
							innerLoop.Set("available_files", "（项目文件由 Phase1 探索已完成，无需重新读取）")
							innerLoop.Set("available_knowledge_bases", "")
							innerLoop.Set("collected_references", "")
							innerLoop.Set("is_modify_mode", "false")
							innerOp.Continue()
						}),
					)
					if err != nil {
						log.Warnf("[CodeAudit/Phase1] Failed to create report_generating loop: %v", err)
					} else {
						subTask := aicommon.NewSubTaskBase(loop.GetCurrentTask(), "phase1-recon-report", writePrompt, true)
						if err := reportLoop.ExecuteWithExistedTask(subTask); err != nil {
							log.Warnf("[CodeAudit/Phase1] Recon report loop returned error: %v", err)
						} else {
							log.Infof("[CodeAudit/Phase1] Recon notes written to: %s", reconFilePath)
						}
					}
				}
			}

			op.Feedback("探索完成，侦察报告已生成，进入扫描阶段。")
			op.Exit()
		},
	))

	preset = append(preset, opts...)
	return reactloops.NewReActLoop("code_audit_phase1_recon", r, preset...)
}

// buildReconFilePath 生成侦察笔记文件路径，写入 AI workdir 下的 audit 目录
func buildReconFilePath(state *AuditState) string {
	dir := auditDir(state)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "recon_notes.md")
}

// buildReconReportWritePrompt 构造传给 report_generating loop 的写作任务描述
// outline 是 Phase1 AI 提交的章节列表，report_generating 将按此大纲填写详细内容
func buildReconReportWritePrompt(state *AuditState, outline, outputPath string) string {
	return fmt.Sprintf(`请根据以下大纲，为代码安全审计项目撰写详细的侦察笔记报告。

## 项目信息
- **项目名称**: %s
- **项目路径**: %s
- **技术栈**: %s
- **入口点**: %s
- **认证机制**: %s

## 报告大纲（请严格按此结构展开）

%s

## 写作要求

1. 不要重新读取项目文件
2. 尽可能具体：列出文件路径、行号、函数名、路由路径等精确信息
3. 对每个用户输入点，说明其参数名称、传入方式（GET/POST/Cookie等）
4. 对数据访问模式，说明是否使用预处理语句，是否存在字符串拼接风险
5. 输出文件: %s

报告使用 Markdown 格式，直接用 write_section 写入完整报告，不需要分批。`,
		state.ProjectName,
		state.ProjectPath,
		state.TechStack,
		state.EntryPoints,
		state.AuthMechanism,
		outline,
		outputPath,
	)
}
