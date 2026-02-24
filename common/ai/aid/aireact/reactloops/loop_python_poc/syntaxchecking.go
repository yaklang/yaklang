package loop_python_poc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/log"
)

func pythonSyntaxCheck(content string, op *reactloops.LoopActionHandlerOperator) (string, bool) {
	if strings.TrimSpace(content) == "" {
		return "", false
	}

	tmpDir, err := os.MkdirTemp("", "python_syntax_check_*")
	if err != nil {
		log.Warnf("failed to create temp dir for python syntax check: %v", err)
		return "", false
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "check.py")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		log.Warnf("failed to write temp file for python syntax check: %v", err)
		return "", false
	}

	pythonCmd := findPythonCommand()
	if pythonCmd == "" {
		log.Warnf("python command not found, skip syntax check")
		return "", false
	}

	var errBuf bytes.Buffer
	cmd := exec.Command(pythonCmd, "-m", "py_compile", tmpFile)
	cmd.Stderr = &errBuf
	pyCompileErr := cmd.Run()

	if pyCompileErr != nil {
		rawErr := errBuf.String()
		formattedErr := formatPyCompileError(rawErr, tmpFile)
		if formattedErr != "" {
			return formattedErr, true
		}
		return fmt.Sprintf("Python syntax check failed:\n%s", rawErr), true
	}

	ruffResult := runRuffCheck(tmpFile)
	if ruffResult != "" {
		return ruffResult, false
	}

	return "", false
}

func findPythonCommand() string {
	for _, cmd := range []string{"python3", "python"} {
		if _, err := exec.LookPath(cmd); err == nil {
			return cmd
		}
	}
	return ""
}

func runRuffCheck(filename string) string {
	ruffPath, err := exec.LookPath("ruff")
	if err != nil {
		return ""
	}

	var outBuf bytes.Buffer
	cmd := exec.Command(ruffPath, "check", "--no-fix", filename)
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf
	_ = cmd.Run()

	output := strings.TrimSpace(outBuf.String())
	if output == "" || strings.Contains(output, "All checks passed") {
		return ""
	}
	return fmt.Sprintf("[ruff] Code style warnings (non-blocking):\n%s", output)
}

func formatPyCompileError(rawErr string, tmpFile string) string {
	if rawErr == "" {
		return ""
	}

	rawErr = strings.ReplaceAll(rawErr, tmpFile, "<generated_script>")

	var result bytes.Buffer
	result.WriteString("Python Syntax Error:\n")

	lines := strings.Split(rawErr, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		result.WriteString("  ")
		result.WriteString(trimmed)
		result.WriteString("\n")
	}

	return strings.TrimRight(result.String(), "\n")
}
