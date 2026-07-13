package loop_yaklangcode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const (
	yakRunOutputMaxBytes     = 8 * 1024
	defaultYakRunSelfTestSec = 30

	configYakRunSelfTestDisabled = "yaklang_auto_run_self_test_disabled"
	configYakRunSelfTestTimeout  = "yaklang_run_self_test_timeout_sec"

	loopVarYakRunOK           = "yak_run_ok"
	loopVarYakRunOutput       = "yak_run_output"
	loopVarYakRunLastFeedback = "yak_run_last_feedback"
)

// YakRunResult captures stdout/logs from a YAK_MAIN self-test execution.
type YakRunResult struct {
	Output    string
	Truncated bool
}

// RunYakSelfTest executes code with YAK_MAIN=true in an isolated yak subprocess (same as Yakit UI default).
// Mock CLI flags are injected so cli.check() passes without killing the grpc parent process.
func RunYakSelfTest(ctx context.Context, code, absPath string, timeoutSec int) (YakRunResult, error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return YakRunResult{}, utils.Error("yak self-test: code is empty")
	}
	if timeoutSec <= 0 {
		timeoutSec = defaultYakRunSelfTestSec
	}
	if absPath == "" {
		absPath = "yaklang_self_test.yak"
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	mockArgs := buildSelfTestCLIArgs(code)
	log.Infof("yaklang self-test mock cli args: %v", mockArgs)

	if _, binErr := resolveYakEngineBinary(); binErr == nil {
		return runYakSelfTestSubprocess(runCtx, code, absPath, mockArgs)
	}
	return runYakSelfTestInProcess(runCtx, code, absPath, mockArgs)
}

func runYakSelfTestSubprocess(ctx context.Context, code, absPath string, mockArgs []string) (YakRunResult, error) {
	yakBin, err := resolveYakEngineBinary()
	if err != nil {
		return YakRunResult{}, err
	}

	scriptPath, cleanup, err := writeSelfTestScriptFile(code, absPath)
	if err != nil {
		return YakRunResult{}, err
	}
	defer cleanup()

	cmdArgs := append([]string{scriptPath}, mockArgs...)
	cmd := exec.CommandContext(ctx, yakBin, cmdArgs...)
	cmd.Env = os.Environ()
	if home := strings.TrimSpace(os.Getenv("YAKIT_HOME")); home != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("YAKIT_HOME=%s", home))
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Infof("yaklang self-test subprocess: binary=%s args=%v", yakBin, cmdArgs)
	runErr := cmd.Run()

	var combined strings.Builder
	if stdout.Len() > 0 {
		combined.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if combined.Len() > 0 {
			combined.WriteByte('\n')
		}
		combined.WriteString(stderr.String())
	}
	out, truncated := truncateYakRunOutput(combined.String())
	result := YakRunResult{Output: out, Truncated: truncated}

	if runErr != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			return result, utils.Errorf("yak self-test exited with code %d", exitErr.ExitCode())
		}
		return result, utils.Errorf("yak self-test subprocess failed: %v", runErr)
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, nil
}

// runYakSelfTestInProcess executes in the current process (test fallback when yak binary unavailable).
// Uses panic-based cli.check callback and injected mock CLI args — never os.Exit.
func runYakSelfTestInProcess(ctx context.Context, code, absPath string, mockArgs []string) (YakRunResult, error) {
	var buf bytes.Buffer
	yakitClient := yaklib.NewVirtualYakitClient(func(result *ypb.ExecResult) error {
		if ret := yaklib.ConvertExecResultIntoAIToolCallStdoutLog(result); ret != "" {
			buf.WriteString(ret)
			buf.WriteByte('\n')
		}
		return nil
	})

	prevArgs := cli.DefaultCliApp.GetArgs()
	cli.DefaultCliApp.SetCliCheckCallback(func() {
		panic("cli check fail")
	})
	cli.DefaultCliApp.SetArgs(append([]string{}, mockArgs...))
	defer func() {
		cli.DefaultCliApp.SetCliCheckCallback(nil)
		cli.DefaultCliApp.SetArgs(prevArgs)
	}()

	engine := yak.NewYakitVirtualClientScriptEngine(yakitClient)
	err := engine.ExecuteMainWithContext(ctx, code, absPath)
	out, truncated := truncateYakRunOutput(buf.String())
	result := YakRunResult{Output: out, Truncated: truncated}
	if err != nil {
		return result, err
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, nil
}

func writeSelfTestScriptFile(code, absPath string) (scriptPath string, cleanup func(), err error) {
	code = strings.TrimSpace(code)
	if code == "" {
		return "", func() {}, utils.Error("yak self-test: code is empty")
	}

	if p := strings.TrimSpace(absPath); p != "" && utils.IsFile(p) {
		if existing, readErr := os.ReadFile(p); readErr == nil && string(existing) == code {
			return p, func() {}, nil
		}
	}

	dir := os.TempDir()
	if p := strings.TrimSpace(absPath); p != "" {
		if d := filepath.Dir(p); d != "" && d != "." {
			dir = d
		}
	}
	f, err := os.CreateTemp(dir, "yaklang-selftest-*.yak")
	if err != nil {
		return "", func() {}, utils.Errorf("create self-test script temp file: %v", err)
	}
	if _, err = f.WriteString(code); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", func() {}, utils.Errorf("write self-test script: %v", err)
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", func() {}, utils.Errorf("close self-test script: %v", err)
	}
	path := f.Name()
	return path, func() { _ = os.Remove(path) }, nil
}

// FormatRunFailureForAI builds AI-facing feedback when self-test fails.
func FormatRunFailureForAI(result YakRunResult, err error) string {
	var b strings.Builder
	b.WriteString("YAK_MAIN 自测运行失败。请根据下面的运行输出/panic 信息用 modify_code 修复（禁止 write_code 重置）。\n\n")
	if err != nil {
		b.WriteString("--- runtime error ---\n")
		b.WriteString(strings.TrimSpace(err.Error()))
		b.WriteString("\n")
	}
	if strings.TrimSpace(result.Output) != "" {
		b.WriteString("--- execution log ---\n")
		b.WriteString(result.Output)
		if result.Truncated {
			b.WriteString("\n...(output truncated)")
		}
		b.WriteString("\n")
	}
	if err == nil && strings.TrimSpace(result.Output) == "" {
		b.WriteString("（无额外日志；检查 assert 失败或 silent panic）\n")
	}
	return strings.TrimSpace(b.String())
}

func truncateYakRunOutput(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) <= yakRunOutputMaxBytes {
		return s, false
	}
	return s[:yakRunOutputMaxBytes], true
}

func resolveYakRunAbsPath(editorFilePath, filename string) string {
	if p := strings.TrimSpace(editorFilePath); p != "" {
		return p
	}
	if p := strings.TrimSpace(filename); p != "" {
		return p
	}
	return "yaklang_self_test.yak"
}

func yakRunSelfTestTimeoutSec(config aicommonGetter) int {
	if config == nil {
		return defaultYakRunSelfTestSec
	}
	return config.GetConfigInt(configYakRunSelfTestTimeout, defaultYakRunSelfTestSec)
}

func yakRunSelfTestDisabled(config aicommonGetter) bool {
	if config == nil {
		return false
	}
	return config.GetConfigBool(configYakRunSelfTestDisabled, false)
}

// aicommonGetter is the minimal config surface for run tuning.
type aicommonGetter interface {
	GetConfigBool(key string, defaults ...bool) bool
	GetConfigInt(key string, defaults ...int) int
}

func logYakRunSelfTestSkip(reason string) {
	log.Infof("skip yaklang YAK_MAIN self-test: %s", reason)
}

func FormatMissingSelfTestFeedback(policy YakScriptRunPolicy) string {
	var b strings.Builder
	b.WriteString("语法已通过，但脚本类型需要 YAK_MAIN 自测块才能自动运行验证。\n\n")
	b.WriteString("--- 检测到的脚本类型 ---\n")
	b.WriteString(string(policy.Kind))
	b.WriteString("\n\n")
	b.WriteString("--- 要求 ---\n")
	b.WriteString("在文件末尾追加 func runSelfTest(){...} 与 if YAK_MAIN { runSelfTest() }（或 native 插件的分流写法）。禁止 write_code 重置；用 modify_code 追加。\n\n")
	if policy.HintForAI != "" {
		b.WriteString("--- 自测 mock 指引 ---\n")
		b.WriteString(policy.HintForAI)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

// FormatRunSkippedStatus builds a short UI status when auto-run is intentionally skipped.
func FormatRunSkippedStatus(policy YakScriptRunPolicy) string {
	if policy.SkipReason != "" {
		return "跳过自测: " + policy.SkipReason + " / Skipped self-test"
	}
	return "跳过自测（无 YAK_MAIN）/ Skipped self-test (no YAK_MAIN)"
}
