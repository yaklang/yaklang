package privileged

import (
	"fmt"
	"strconv"
)

func buildWindowsUACBatchLines(cmd, stdoutTarget, stderrTarget, exitCodeFile string, writeExitCode bool, deleteSelf bool) []string {
	var batLines []string
	batLines = append(batLines, "@echo off")
	batLines = append(batLines, "")
	batLines = append(batLines, fmt.Sprintf("call :sub > %s 2> %s", stdoutTarget, stderrTarget))
	batLines = append(batLines, "exit /b")
	batLines = append(batLines, "")
	batLines = append(batLines, ":sub")
	batLines = append(batLines, cmd)
	batLines = append(batLines, `set "yak_exit_code=%errorlevel%"`)
	if writeExitCode {
		batLines = append(batLines, "echo %yak_exit_code% > "+strconv.Quote(exitCodeFile))
	}
	if deleteSelf {
		batLines = append(batLines, `del /f /q "%~f0" >nul 2>nul`)
	}
	batLines = append(batLines, "exit /b %yak_exit_code%")
	return batLines
}
