package loop_report_generating

import (
	"bytes"
	_ "embed"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	// 默认展示的字符数限制，约 30K 字符
	DefaultReportShowSize = 30000
	// 默认起始行
	DefaultOffsetLine = 1
)

//go:embed prompts/persistent_instruction.txt
var instruction string

//go:embed prompts/reflection_output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_REPORT_GENERATING,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			log.Infof("initializing report_generating loop")

			// 创建单文件修改工厂（复用 loopinfra 基础设施）
			modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
				r,
				loopinfra.WithLoopVarsPrefix("report"),
				loopinfra.WithActionSuffix("section"),
				loopinfra.WithAITagConfig("GEN_REPORT", "report_content", "report-content", "text/markdown"),
				loopinfra.WithFileExtension(".md"),
				loopinfra.WithExitAfterWrite(false),
			)

			// 创建预设选项
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				modSuite.GetAITagOption(),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					reportContent := loop.Get(modSuite.GetFullCodeVariableName())

					// 获取分页参数
					offsetLine, _ := strconv.Atoi(loop.Get("offset_line"))
					if offsetLine < 1 {
						offsetLine = DefaultOffsetLine
					}
					showSize, _ := strconv.Atoi(loop.Get("report_show_size"))
					if showSize <= 0 {
						showSize = DefaultReportShowSize
					}

					// 计算分页展示的报告内容
					lines := strings.Split(reportContent, "\n")
					totalLines := len(lines)
					totalSize := len(reportContent)

					// 确保 offsetLine 不超过总行数
					if offsetLine > totalLines {
						offsetLine = totalLines
					}

					// 从 offsetLine 开始截取，直到达到 showSize 或文件结束
					var visibleLines []string
					var currentSize int
					endLine := offsetLine - 1 // 转为 0-based index

					for i := offsetLine - 1; i < totalLines && currentSize < showSize; i++ {
						lineContent := lines[i]
						lineSize := len(lineContent) + 1 // +1 for newline
						if currentSize+lineSize > showSize && len(visibleLines) > 0 {
							break
						}
						visibleLines = append(visibleLines, lineContent)
						currentSize += lineSize
						endLine = i + 1 // 转回 1-based
					}

					// 为可见行添加行号
					var reportWithLine string
					if len(visibleLines) > 0 {
						// 使用带偏移的行号
						var numberedLines []string
						for i, line := range visibleLines {
							lineNum := offsetLine + i
							numberedLines = append(numberedLines, strconv.Itoa(lineNum)+"|"+line)
						}
						reportWithLine = strings.Join(numberedLines, "\n")
					}

					// 构建分页信息
					hasMore := endLine < totalLines
					hasPrev := offsetLine > 1

					userRequirements := loop.Get("user_requirements")
					collectedReferences := loop.Get("collected_references")
					availableFiles := loop.Get("available_files")
					availableKBs := loop.Get("available_knowledge_bases")

					feedbacks := feedbacker.String()

					renderMap := map[string]any{
						"UserRequirements":            userRequirements,
						"Filename":                    loop.Get(modSuite.GetFilenameVariableName()),
						"CurrentReportWithLineNumber": reportWithLine,
						"CollectedReferences":         collectedReferences,
						"AvailableFiles":              availableFiles,
						"AvailableKnowledgeBases":     availableKBs,
						"FeedbackMessages":            feedbacks,
						"Nonce":                       nonce,
						// 分页相关信息
						"OffsetLine":     offsetLine,
						"EndLine":        endLine,
						"TotalLines":     totalLines,
						"TotalSize":      totalSize,
						"VisibleSize":    currentSize,
						"ShowSize":       showSize,
						"HasMoreContent": hasMore,
						"HasPrevContent": hasPrev,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// 注册查资料 actions
				readReferenceFileAction(r),
				grepReferenceAction(r),
				searchKnowledgeAction(r),
				// 注册视图控制 actions
				changeOffsetLineAction(r),
			}
			// 添加工厂生成的文件修改 actions (write_section, modify_section, insert_section, delete_section)
			preset = append(preset, modSuite.GetActions()...)
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_REPORT_GENERATING, r, preset...)
		},
		// 注册元数据，帮助 AI 理解这个 loop 的用途
		reactloops.WithLoopDescription("报告生成模式：AI 一边查阅资料一边撰写调查报告/分析文章，支持分批编写和修改"),
		reactloops.WithLoopUsagePrompt(`当用户需要生成调查报告、分析文章、技术文档等长文本时使用。
AI会先收集资料（读取文件、搜索知识库），然后分批撰写报告，并根据需要进行修改和优化。`),
		reactloops.WithLoopOutputExample(`
* 当需要生成报告或分析文章时：
  {"@action": "report_generating", "human_readable_thought": "用户需要生成一份调查报告，我将收集资料并分批撰写"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_REPORT_GENERATING, err)
	}
}
