package loop_python_poc

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
)

var checkPythonEnv = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"check_python_env",
		"MUST be called FIRST before generating any Python code. Use this action to report the result of your Python environment check. You should have already used the 'bash' tool to run commands like 'python3 --version' or 'python --version' to check if Python is available.",
		[]aitool.ToolOption{
			aitool.WithBoolParam("python_available",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Whether Python is available in the environment. Set to true if your bash command returned a valid Python version (e.g., 'Python 3.x.x'), false otherwise.")),
			aitool.WithStringParam("python_command",
				aitool.WithParam_Description("The Python command that works (e.g., 'python3' or 'python'). Only set if python_available is true.")),
			aitool.WithStringParam("python_version",
				aitool.WithParam_Description("The Python version string returned by the command (e.g., 'Python 3.11.0'). Only set if python_available is true.")),
			aitool.WithStringParam("check_result",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("Description of what commands you ran and what the output was.")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			pythonAvailable := action.GetBool("python_available")
			pythonCommand := action.GetString("python_command")
			pythonVersion := action.GetString("python_version")
			checkResult := action.GetString("check_result")

			// Store environment status in loop variables
			if pythonAvailable {
				loop.Set("python_available", "true")
				loop.Set("python_command", pythonCommand)
				loop.Set("python_version", pythonVersion)
				log.Infof("Python environment check: available, command=%s, version=%s", pythonCommand, pythonVersion)
				r.AddToTimeline("python_env_check", "Python 环境可用: "+pythonCommand+" ("+pythonVersion+")")
				operator.Feedback("Python 环境检查完成。Python 可用: " + pythonCommand + " (" + pythonVersion + ")\n你现在可以使用 write_python_poc 生成代码。")
			} else {
				loop.Set("python_available", "false")
				loop.Set("python_command", "")
				loop.Set("python_version", "")
				log.Warnf("Python environment check: NOT available. Check result: %s", checkResult)
				r.AddToTimeline("python_env_check", "Python 环境不可用: "+checkResult)
				operator.Feedback("⚠️ Python 环境不可用。\n检查结果: " + checkResult + "\n\n你仍然可以生成 Python POC 代码，但无法进行语法检查。生成的代码将包含注释说明未验证语法。")
			}

			// Mark that environment check has been done
			loop.Set("env_checked", "true")
		},
	)
}

// isPythonVersionOutput checks if the output looks like a valid Python version
func isPythonVersionOutput(output string) bool {
	output = strings.ToLower(strings.TrimSpace(output))
	return strings.HasPrefix(output, "python 3") || strings.HasPrefix(output, "python 2")
}
