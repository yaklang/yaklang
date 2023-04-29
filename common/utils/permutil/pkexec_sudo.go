package permutil

import (
	"context"
	"fmt"
	"os/exec"
	"yaklang/common/log"
	"yaklang/common/utils"
	"runtime"
	"strconv"
	"strings"
)

func LinuxPKExecSudo(cmd string, opt ...SudoOption) error {
	switch runtime.GOOS {
	case "linux":
	default:
		return utils.Error("not a linux system")
	}

	config := NewDefaultSudoConfig()
	for _, i := range opt {
		i(config)
	}

	_, err := exec.LookPath("bash")
	if err != nil {
		return utils.Errorf("cannot found bash: %v", err)
	}

	_, err = exec.LookPath("pkexec")
	if err != nil {
		return utils.Errorf("pkexec not found: %s", err)
	}
	/* pkexec --user root  */

	var lines []string

	/* check cwd */
	cwd := config.Workdir
	if !utils.IsDir(cwd) && cwd != "" {
		return utils.Errorf("workdir is not existed: %s", cwd)
	}
	if cwd != "" {
		lines = append(lines, fmt.Sprintf("cd %v", strconv.Quote(cwd)))
	}

	// checking env
	env := config.Environments
	if env != nil && len(env) > 0 {
		for k, v := range env {
			if !utils.MatchAllOfRegexp(k, `\w[\w\d]+`) {
				log.Errorf("invalid env key: %v   value: %v", k, v)
				continue
			}
			lines = append(lines, fmt.Sprintf(`export %v="%v"`, k, strings.Trim(strconv.Quote(v), `"`)))
		}
	}

	// pkexec
	lines = append(lines, fmt.Sprintf(`pkexec --disable-internal-agent bash -c %v`, strconv.Quote(cmd)))

	ctx := config.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)
	proc := exec.CommandContext(ctx, "bash", "-c", strings.Join(lines, " && "))
	if config.Stdout != nil {
		proc.Stdout = config.Stdout
	}
	if config.Stderr != nil {
		proc.Stderr = config.Stderr
	}
	_ = proc.Run()
	cancel()

	if config.ExitCodeHandler != nil && proc.ProcessState != nil {
		config.ExitCodeHandler(proc.ProcessState.ExitCode())
	}
	return nil
}
