package permutil

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func buildWindowsSudoBatchLines(cmd, stdoutTarget, stderrTarget, exitCodeFile string, writeExitCode bool, deleteSelf bool, workdir string, env map[string]string) []string {
	var lines []string
	lines = append(lines, "@echo off")
	if workdir != "" {
		lines = append(lines, fmt.Sprintf("cd %v", strconv.Quote(workdir)))
	}

	if env != nil && len(env) > 0 {
		for k, v := range env {
			if !utils.MatchAllOfRegexp(k, `\w[\w\d]+`) {
				log.Errorf("invalid env key: %v   value: %v", k, v)
				continue
			}
			lines = append(lines, fmt.Sprintf(`set %v=%v`, k, strings.Trim(strconv.Quote(v), `"`)))
		}
	}

	lines = append(lines, "")
	lines = append(lines, fmt.Sprintf("call :sub > %s 2> %s", stdoutTarget, stderrTarget))
	lines = append(lines, "exit /b")
	lines = append(lines, "")
	lines = append(lines, ":sub")
	lines = append(lines, cmd)
	lines = append(lines, `set "yak_exit_code=%errorlevel%"`)
	if writeExitCode {
		lines = append(lines, "echo %yak_exit_code% > "+strconv.Quote(exitCodeFile))
	}
	if deleteSelf {
		lines = append(lines, `del /f /q "%~f0" >nul 2>nul`)
	}
	lines = append(lines, "exit /b %yak_exit_code%")
	return lines
}

func startWindowsSudoProcess(ctx context.Context, batName string, waitForExit bool) (*exec.Cmd, error) {
	args := []string{
		"-NoProfile",
		"-NonInteractive",
		"Start-Process",
		"-FilePath", batName,
		"-Verb", "RunAs",
		"-WindowStyle", "Hidden",
		"-ErrorAction", "Stop",
	}
	if waitForExit {
		args = append(args, "-Wait")
	}

	proc := exec.CommandContext(ctx, "powershell.exe", args...)
	if err := proc.Start(); err != nil {
		return nil, err
	}
	return proc, nil
}

func readWindowsSudoTempFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return raw, nil
}

func cleanupWindowsSudoTempFile(path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Warnf("failed to remove temporary Windows sudo file %s: %v", path, err)
	}
}

func WindowsSudo(cmd string, opts ...SudoOption) error {
	/**
	 .bat

	cd %CWD%
	set KEY=VALUE
	set KEY=VALUE

	cmd
	*/

	if runtime.GOOS != "windows" {
		return utils.Error("windows sudo only for windows")
	}

	config := NewDefaultSudoConfig()
	for _, i := range opts {
		i(config)
	}

	/** powershell.exe start-process -verb runas -windowstyle hidden {temp}.bat */
	tempFileDir := os.TempDir()
	token := utils.RandStringBytes(20)
	batName := filepath.Join(tempFileDir, fmt.Sprintf("windows-uac-prompt-%v.bat", token))
	//batName := filepath.Join(tempFileDir, "windows-uac-prompt.bat")
	cleanupWindowsSudoTempFile(batName)

	waitForExit := config.Stdout != nil || config.Stderr != nil || config.ExitCodeHandler != nil
	writeExitCode := config.ExitCodeHandler != nil
	stdoutTarget := "NUL"
	stderrTarget := "NUL"

	var stdoutFile string
	var stderrFile string
	var exitCodeFile string
	if waitForExit {
		if config.Stdout != nil {
			stdoutFile = filepath.Join(tempFileDir, "stdout-"+token+".txt")
			stdoutTarget = strconv.Quote(stdoutFile)
		}
		if config.Stderr != nil {
			stderrFile = filepath.Join(tempFileDir, "stderr-"+token+".txt")
			stderrTarget = strconv.Quote(stderrFile)
		}
		if writeExitCode {
			exitCodeFile = filepath.Join(tempFileDir, "exitcode-"+token+".txt")
		}
	}
	cleanupBatchOnReturn := true
	defer func() {
		cleanupWindowsSudoTempFile(stdoutFile)
		cleanupWindowsSudoTempFile(stderrFile)
		cleanupWindowsSudoTempFile(exitCodeFile)
		if cleanupBatchOnReturn {
			cleanupWindowsSudoTempFile(batName)
		}
	}()

	if config.Workdir != "" {
		if !utils.IsDir(config.Workdir) {
			return utils.Errorf("workdir: %s is not valid", config.Workdir)
		}
	}

	compiled := buildWindowsSudoBatchLines(
		cmd,
		stdoutTarget,
		stderrTarget,
		exitCodeFile,
		writeExitCode,
		!waitForExit,
		config.Workdir,
		config.Environments,
	)

	fp, err := os.OpenFile(batName, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return utils.Errorf("create sudo....bat failed: %s", err)
	}
	_, err = fp.Write([]byte(strings.Join(compiled, "\n")))
	fp.Close()
	if err != nil {
		return utils.Errorf("write sudo....bat failed: %s", err)
	}

	ctx := config.Ctx
	if ctx == nil {
		ctx = context.Background()
	}

	proc, err := startWindowsSudoProcess(ctx, batName, waitForExit)
	if err != nil {
		return utils.Wrapf(err, "failed to start Windows sudo prompt")
	}

	if waitErr := proc.Wait(); waitErr != nil {
		return utils.Wrapf(waitErr, "failed to complete Windows sudo prompt")
	}

	if !waitForExit {
		cleanupBatchOnReturn = false
		time.Sleep(500 * time.Millisecond)
		return nil
	}

	if config.Stdout != nil {
		raw, err := readWindowsSudoTempFile(stdoutFile)
		if err != nil {
			return utils.Wrapf(err, "failed to read stdout temp file")
		}
		if len(raw) > 0 {
			if _, err := config.Stdout.Write(raw); err != nil {
				return utils.Wrapf(err, "failed to write stdout output")
			}
		}
	}

	if config.Stderr != nil {
		raw, err := readWindowsSudoTempFile(stderrFile)
		if err != nil {
			return utils.Wrapf(err, "failed to read stderr temp file")
		}
		if len(raw) > 0 {
			if _, err := config.Stderr.Write(raw); err != nil {
				return utils.Wrapf(err, "failed to write stderr output")
			}
		}
	}

	statusCode := config.ExitCodeHandler
	if statusCode == nil {
		return nil
	}

	raw, err := readWindowsSudoTempFile(exitCodeFile)
	if err != nil {
		return utils.Wrapf(err, "failed to read exit code temp file")
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return utils.Errorf("windows sudo finished without exit code")
	}

	i, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	statusCode(i)
	return nil
}
