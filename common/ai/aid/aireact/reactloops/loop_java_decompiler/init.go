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
				reactloops.WithAITagField("JAVA_CODE", "java_code"),
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
