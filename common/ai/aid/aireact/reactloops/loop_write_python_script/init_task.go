package loop_write_python_script

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

func buildInitTask(r aicommon.AIInvokeRuntime) func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
	return func(loop *reactloops.ReActLoop, task aicommon.AIStatefulTask, operator *reactloops.InitTaskOperator) {
		log.Infof("[*] React: write_python_script loop initialized, detecting environment")

		pythonCmd, pythonVersion := detectPython()
		if pythonCmd != "" {
			loop.Set("python_command", pythonCmd)
			loop.Set("python_version", pythonVersion)
			r.AddToTimeline("python_env", fmt.Sprintf("Python available: %s (%s)", pythonCmd, pythonVersion))
			log.Infof("detected python: %s (%s)", pythonCmd, pythonVersion)
		} else {
			log.Warnf("python not found in PATH")
			r.AddToTimeline("python_env", "Python NOT found in PATH")
		}

		pkgManager, pkgVersion := detectPackageManager()
		if pkgManager != "" {
			loop.Set("pkg_manager", pkgManager)
			loop.Set("pkg_manager_version", pkgVersion)
			r.AddToTimeline("pkg_manager", fmt.Sprintf("Package manager available: %s (%s)", pkgManager, pkgVersion))
			log.Infof("detected package manager: %s (%s)", pkgManager, pkgVersion)
		} else {
			log.Warnf("no package manager (uv/pip) found in PATH")
			r.AddToTimeline("pkg_manager", "No package manager (uv/pip) found")
		}

		ruffPath, _ := exec.LookPath("ruff")
		if ruffPath != "" {
			loop.Set("ruff_available", "true")
			log.Infof("ruff linter available at: %s", ruffPath)
		}

		filename := r.EmitFileArtifactWithExt("python_script", ".py", "")
		loop.Set("python_filename", filename)
		r.GetConfig().GetEmitter().EmitPinFilename(filename)
		log.Infof("created python script artifact: %s", filename)

		operator.Continue()
	}
}

func detectPython() (string, string) {
	for _, cmd := range []string{"python3", "python"} {
		output, err := exec.Command(cmd, "--version").CombinedOutput()
		if err != nil {
			continue
		}
		version := strings.TrimSpace(string(output))
		if strings.HasPrefix(strings.ToLower(version), "python") {
			return cmd, version
		}
	}
	return "", ""
}

func detectPackageManager() (string, string) {
	// uv first
	if output, err := exec.Command("uv", "--version").CombinedOutput(); err == nil {
		version := strings.TrimSpace(string(output))
		return "uv", version
	}

	// pip fallback
	for _, cmd := range []string{"pip3", "pip"} {
		output, err := exec.Command(cmd, "--version").CombinedOutput()
		if err != nil {
			continue
		}
		version := strings.TrimSpace(string(output))
		if strings.Contains(strings.ToLower(version), "pip") {
			return cmd, version
		}
	}
	return "", ""
}
