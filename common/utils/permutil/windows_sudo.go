package permutil

import (
	"context"
	"fmt"
	"github.com/hpcloud/tail"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"yaklang/common/log"
	"yaklang/common/utils"
)

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
	os.RemoveAll(batName)

	stdoutFile := filepath.Join(tempFileDir, "stdout-"+token+".txt")
	stderrFile := filepath.Join(tempFileDir, "stderr-"+token+".txt")
	exitCodeFile := filepath.Join(tempFileDir, "exitcode-"+token+".txt")
	defer func() {
		os.RemoveAll(stdoutFile)
		os.RemoveAll(stderrFile)
		os.RemoveAll(exitCodeFile)
		os.RemoveAll(batName)
	}()

	var combiled []string
	combiled = append(combiled, "@echo off")
	if config.Workdir != "" {
		if !utils.IsDir(config.Workdir) {
			return utils.Errorf("workdir: %s is not valid", config.Workdir)
		}
		combiled = append(combiled, fmt.Sprintf("cd %v", strconv.Quote(config.Workdir)))
	}

	env := config.Environments
	if env != nil && len(env) > 0 {
		for k, v := range env {
			if !utils.MatchAllOfRegexp(k, `\w[\w\d]+`) {
				log.Errorf("invalid env key: %v   value: %v", k, v)
				continue
			}
			combiled = append(combiled, fmt.Sprintf(`set %v=%v`, k, strings.Trim(strconv.Quote(v), `"`)))
		}
	}
	combiled = append(combiled, "")
	combiled = append(combiled, fmt.Sprintf("call :sub > %v 2> %v", strconv.Quote(stdoutFile), strconv.Quote(stderrFile)))
	combiled = append(combiled, `exit /b`)
	combiled = append(combiled, "")
	combiled = append(combiled, ":sub")
	combiled = append(combiled, cmd)
	combiled = append(combiled, "echo %errorlevel% > "+exitCodeFile)
	combiled = append(combiled, "exit /b")

	fp, err := os.OpenFile(batName, os.O_RDWR|os.O_CREATE, os.ModePerm)
	if err != nil {
		return utils.Errorf("create sudo....bat failed: %s", err)
	}
	fp.Write([]byte(strings.Join(combiled, "\n")))
	fp.Close()

	ctx := config.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithCancel(ctx)

	/*
		Stdout Err Handler
	*/
	stdout := config.Stdout
	stderr := config.Stderr

	wg := new(sync.WaitGroup)
	wg.Add(2)
	go func() {
		defer func() {
			wg.Done()
		}()

		if stdout == nil {
			return
		}

		t, err := tail.TailFile(stdoutFile, tail.Config{Follow: true})
		if err != nil {
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case l, ok := <-t.Lines:
				if !ok {
					return
				}
				if l.Text != "" {
					stdout.Write([]byte(l.Text))
					stderr.Write([]byte{'\n'})
				}
			}
		}
	}()

	go func() {
		defer func() {
			wg.Done()
		}()

		if stderr == nil {
			return
		}
		t, err := tail.TailFile(stderrFile, tail.Config{
			Follow: true,
		})
		if err != nil {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case l, ok := <-t.Lines:
				if !ok {
					return
				}
				if l.Text != "" {
					stderr.Write([]byte(l.Text))
					stderr.Write([]byte{'\n'})
				}
			}
		}
	}()

	proc := exec.CommandContext(ctx, "powershell.exe", "start-process", "-verb", "runas",
		"-windowstyle", "hidden", batName)
	_ = proc.Run()

	go func() {
		// do not exit immediately, give some duration for trigger output
		time.Sleep(500 * time.Millisecond)
		cancel()
	}()
	wg.Wait()

	statusCode := config.ExitCodeHandler
	if statusCode == nil {
		return nil
	}

	raw, _ := ioutil.ReadFile(exitCodeFile)
	if raw != nil {
		i, _ := strconv.Atoi(string(raw))
		statusCode(i)
		return nil
	}
	statusCode(0)
	return nil
}
