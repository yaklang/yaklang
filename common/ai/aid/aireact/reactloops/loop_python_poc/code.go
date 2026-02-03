package loop_python_poc

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
		schema.AI_REACT_LOOP_NAME_PYTHON_POC,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			// Create preset options
			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				reactloops.WithAITagFieldWithAINodeId("GEN_PYTHON_POC", "python_poc_code", "python-poc", aicommon.TypeCodePython),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					pythonCode := loop.Get("full_code")
					codeWithLine := utils.PrefixLinesWithLineNumbers(pythonCode)
					filename := loop.Get("filename")
					envChecked := loop.Get("env_checked") == "true"
					pythonAvailable := loop.Get("python_available") == "true"
					pythonCommand := loop.Get("python_command")
					pythonVersion := loop.Get("python_version")

					feedbacks := feedbacker.String()
					renderMap := map[string]any{
						"Code":                      pythonCode,
						"CurrentCodeWithLineNumber": codeWithLine,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacks,
						"Filename":                  filename,
						"EnvChecked":                envChecked,
						"PythonAvailable":           pythonAvailable,
						"PythonCommand":             pythonCommand,
						"PythonVersion":             pythonVersion,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				checkPythonEnv(r),
				writePythonPOC(r),
				modifyPythonPOC(r),
				verifySyntax(r),
			}
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_PYTHON_POC, r, preset...)
		},
		// Register metadata for better AI understanding
		reactloops.WithLoopDescription("Enter focused mode for Python POC code generation. Used to create security vulnerability proof-of-concept scripts. AI should use bash tool to check Python environment and syntax."),
		reactloops.WithLoopUsagePrompt("Use when user requests to generate Python POC code for security vulnerabilities. Provides specialized tools: write_python_poc, modify_python_poc. AI should use bash tool to verify Python syntax after code generation."),
		reactloops.WithLoopOutputExample(`
* When user requests to generate Python POC code:
  {"@action": "python_poc", "human_readable_thought": "I need to generate Python POC code with proper syntax and security testing logic"}
`),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_PYTHON_POC)
	}
}

// buildInitTask creates the initialization task handler
func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		log.Infof("[*] React: Python PoC loop initialized, waiting for AI to generate code")
		// Default: Continue with normal loop execution
		operator.Continue()
	}
}
