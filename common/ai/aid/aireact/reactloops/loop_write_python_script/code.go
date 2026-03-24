package loop_write_python_script

import (
	"bytes"
	_ "embed"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loopinfra"
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
		schema.AI_REACT_LOOP_NAME_WRITE_PYTHON_SCRIPT,
		func(r aicommon.AIInvokeRuntime, opts ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error) {
			modSuite := loopinfra.NewSingleFileModificationSuiteFactory(
				r,
				loopinfra.WithLoopVarsPrefix("python"),
				loopinfra.WithActionSuffix("script"),
				loopinfra.WithAITagConfig("GEN_PYTHON_SCRIPT", "python_script_code", "python-script", aicommon.TypeCodePython),
				loopinfra.WithFileExtension(".py"),
				loopinfra.WithExitAfterWrite(false),
				loopinfra.WithFileChanged(pythonLintCheck),
				loopinfra.WithEventType("python_script_editor"),
			)

			preset := []reactloops.ReActLoopOption{
				reactloops.WithAllowRAG(true),
				reactloops.WithAllowToolCall(true),
				reactloops.WithInitTask(buildInitTask(r)),
				reactloops.WithMaxIterations(int(r.GetConfig().GetMaxIterationCount())),
				reactloops.WithAllowUserInteract(r.GetConfig().GetAllowUserInteraction()),
				modSuite.GetAITagOption(),
				reactloops.WithPersistentInstruction(instruction),
				reactloops.WithReflectionOutputExample(outputExample),
				reactloops.WithReactiveDataBuilder(func(loop *reactloops.ReActLoop, feedbacker *bytes.Buffer, nonce string) (string, error) {
					pythonCode := loop.Get(modSuite.GetFullCodeVariableName())
					codeWithLine := utils.PrefixLinesWithLineNumbers(pythonCode)
					filename := loop.Get(modSuite.GetFilenameVariableName())
					pythonCommand := loop.Get("python_command")
					pythonVersion := loop.Get("python_version")
					pkgManager := loop.Get("pkg_manager")
					depsChecked := loop.Get("deps_checked") == "true"
					depsInstalled := loop.Get("deps_installed") == "true"
					lintOk := loop.Get(modSuite.GetLintStatusVariableName()) == "true"

					feedbacks := feedbacker.String()
					feedbacks = strings.TrimSpace(feedbacks)
					renderMap := map[string]any{
						"Code":                      pythonCode,
						"CurrentCodeWithLineNumber": codeWithLine,
						"Nonce":                     nonce,
						"FeedbackMessages":          feedbacks,
						"Filename":                  filename,
						"PythonCommand":             pythonCommand,
						"PythonVersion":             pythonVersion,
						"PkgManager":                pkgManager,
						"DepsChecked":               depsChecked,
						"DepsInstalled":             depsInstalled,
						"LintOk":                    lintOk,
					}
					return utils.RenderTemplate(reactiveData, renderMap)
				}),
				checkAndInstallDependencies(r),
			}
			preset = append(preset, modSuite.GetActions()...)
			preset = append(preset, opts...)
			return reactloops.NewReActLoop(schema.AI_REACT_LOOP_NAME_WRITE_PYTHON_SCRIPT, r, preset...)
		},
		reactloops.WithLoopDescription("Enter focused mode for Python script generation. Creates production-quality Python scripts with CLI entry points, dependency management, and syntax validation."),
		reactloops.WithLoopDescriptionZh("Python 脚本生成模式：用于编写或修改生产可用的 Python 脚本，支持 CLI 入口、依赖检查与语法校验。"),
		reactloops.WithLoopUsagePrompt("Use when user requests to write or modify Python scripts. Provides specialized tools: write_script, modify_script, insert_script, delete_script, check_and_install_dependencies. Use bash tool to execute scripts and install dependencies."),
		reactloops.WithLoopOutputExample(`
* When user requests to write a Python script:
  {"@action": "write_python_script", "human_readable_thought": "I need to write a Python script with CLI entry point and proper dependency management"}
`),

		reactloops.WithVerboseName("Python Script Builder"),
		reactloops.WithVerboseNameZh("Python 脚本生成"),
	)
	if err != nil {
		log.Errorf("register reactloop: %v failed", schema.AI_REACT_LOOP_NAME_WRITE_PYTHON_SCRIPT)
	}
}
