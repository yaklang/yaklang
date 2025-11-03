package loop_java_decompiler

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

//go:embed prompts/output_example.txt
var outputExample string

//go:embed prompts/reactive_data.txt
var reactiveData string

func init() {
	err := reactloops.RegisterLoopFactory(
		schema.AI_REACT_LOOP_NAME_JAVA_DECOMPILER,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(false),
				reactloops.WithAllowToolCall(false),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithAITagFieldWithAINodeId("JAVA_CODE", "java_code", "re-act-loop-answer-payload"),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					currentFile := loop.Get("current_file")
					currentFileContent := loop.Get("current_file_content")
					currentFileWithLineNumber := ""
					if currentFileContent != "" {
						currentFileWithLineNumber = utils.PrefixLinesWithLineNumbers(currentFileContent)
					}

					workingDir := loop.Get("working_directory")
					currentTask := loop.Get("current_task")
					totalFiles := loop.GetInt("total_files")
					filesWithIssues := loop.GetInt("files_with_issues")
					fixedFiles := loop.GetInt("fixed_files")

					feedbacks := feedbacker.String()

					renderMap := map[string]any{
						"WorkingDirectory":          workingDir,
						"CurrentTask":               currentTask,
						"CurrentFile":               currentFile,
						"CurrentFileWithLineNumber": currentFileWithLineNumber,
						"FeedbackMessages":          feedbacks,
						"TotalFiles":                totalFiles,
						"FilesWithIssues":           filesWithIssues,
						"FixedFiles":                fixedFiles,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				// Register actions
				decompileJarAction(r),
				listFilesAction(r),
				readJavaFileAction(r),
				rewriteJavaFileAction(r),
				checkJavaSyntaxAction(r),
				compareFilesAction(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_JAVA_DECOMPILER, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("进入 Java 反编译专家模式：自动反编译 JAR 包，分析导出的 Java 源码，发现并修复语法及编译错误，支持单文件/批量增量修复，适合 JAR 二进制分析和源码重构。"),
		reactloops.WithLoopUsagePrompt(`当用户输入一般为 JAR 文件修复导出需求，如：
- "请帮我反编译 /tmp/xxx.jar 并输出至 ./xxx"
- "请检查导出的 Java 文件，并一并修复编译错误"
调用本流程，将启用如下专用工具：decompile_jar（反编译 JAR）、list_files（枚举文件）、read_java_file（查看源码）、rewrite_java_file（修正/重写源码）、check_syntax（检测语法错误）、compare_with_backup（对比备份版本）。human_readable_thought 字段会详细描述用户意图与 jar 路径，辅助 AI 做出针对性决策。`),
		reactloops.WithLoopOutputExample(`
* 用户请求反编译 jar 并自动修复导出的 Java 代码。例如：
  {"@action": "java_decompiler", "human_readable_thought": "请将 /tmp/xxx.jar 反编译输出到 ./xxx，并修复所有导出的 Java 文件中的语法和常见反编译问题"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed: %v", schema.AI_REACT_LOOP_NAME_JAVA_DECOMPILER, err)
	}
}

// buildInitTask creates the initial task for the Java decompiler loop
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask) error {
		// Get user input from task
		userQuery := task.GetUserInput()

		// Initialize loop context
		loop.Set("current_task", userQuery)
		loop.Set("total_files", 0)
		loop.Set("files_with_issues", 0)
		loop.Set("fixed_files", 0)     // Legacy counter
		loop.Set("rewritten_files", 0) // New counter for rewrite action

		r.AddToTimeline("task_initialized", "Java decompiler task initialized: "+userQuery)
		return nil
	}
}
