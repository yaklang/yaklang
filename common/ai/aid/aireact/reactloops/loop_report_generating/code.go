package loop_report_generating

import (
	"bytes"
	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
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

			// 创建预设选项
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithAITagFieldWithAINodeId("GEN_REPORT", "report_content", "report-content", "text/markdown"),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					reportContent := loop.Get("full_report")
					reportWithLine := utils.PrefixLinesWithLineNumbers(reportContent)
					userRequirements := loop.Get("user_requirements")
					collectedReferences := loop.Get("collected_references")
					availableFiles := loop.Get("available_files")
					availableKBs := loop.Get("available_knowledge_bases")

					feedbacks := feedbacker.String()

					renderMap := map[string]any{
						"UserRequirements":            userRequirements,
						"Filename":                    loop.Get("filename"),
						"CurrentReportWithLineNumber": reportWithLine,
						"CollectedReferences":         collectedReferences,
						"AvailableFiles":              availableFiles,
						"AvailableKnowledgeBases":     availableKBs,
						"FeedbackMessages":            feedbacks,
						"Nonce":                       nonce,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// 注册查资料 actions
				readReferenceFileAction(r),
				grepReferenceAction(r),
				searchKnowledgeAction(r),
				// 注册写作 actions
				writeReportAction(r),
				modifySectionAction(r),
				insertSectionAction(r),
				deleteSectionAction(r),
			}
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
