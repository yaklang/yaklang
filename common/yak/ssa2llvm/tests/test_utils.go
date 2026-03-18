package tests

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

var ensureRuntimeOnce sync.Once

func ensureRuntimeArchiveOnce(t *testing.T) {
	t.Helper()
	ensureRuntimeOnce.Do(func() {
		repoRoot := RepoRoot(t)
		EnsureRuntimeArchive(t, repoRoot)
	})
}

func check(t *testing.T, code string, expected interface{}) {
	t.Helper()
	checkEx(t, code, "yak", expected)
}

func checkEx(t *testing.T, code string, language string, expected interface{}) {
	t.Helper()
	checkVerify(t, code, language)
	checkBinaryEx(t, code, "check", language, expected)
}

func checkIntegrated(t *testing.T, code string, expected interface{}) {
	t.Helper()
	checkVerify(t, code, "yak")
	checkBinaryEx(t, code, "check", "yak", expected)
}

func checkVerify(t *testing.T, code string, language string) {
	t.Helper()

	tmpIR, err := os.CreateTemp("", "ssa2llvm-verify-*.ll")
	if err != nil {
		t.Fatalf("Failed to create verify temp file: %v", err)
	}
	_ = tmpIR.Close()
	defer os.Remove(tmpIR.Name())

	if _, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceCode(code),
		compiler.WithCompileLanguage(language),
		compiler.WithCompileEmitLLVM(true),
		compiler.WithCompileOutputFile(tmpIR.Name()),
	); err != nil {
		t.Fatalf("Compile/verify failed: %v", err)
	}
}

func checkBinaryEx(t *testing.T, code string, entry string, language string, expected interface{}) {
	t.Helper()
	want, ok := expectedToInt64(expected)
	if !ok {
		t.Fatalf("Unsupported expected value type for binary check: %T", expected)
	}
	binaryRet, output := runBinaryReturnValue(t, code, entry, language)
	if binaryRet != want {
		t.Fatalf("Binary return mismatch. Expected: %d, Got: %d, Output: %q", want, binaryRet, output)
	}
}

func checkBinaryExWithOptions(t *testing.T, code string, entry string, language string, expected interface{}, options ...runBinaryOption) {
	t.Helper()
	want, ok := expectedToInt64(expected)
	if !ok {
		t.Fatalf("Unsupported expected value type for binary check: %T", expected)
	}
	binaryRet, output := runBinaryReturnValueWithOptions(t, code, entry, language, options...)
	if binaryRet != want {
		t.Fatalf("Binary return mismatch. Expected: %d, Got: %d, Output: %q", want, binaryRet, output)
	}
}

func checkPrint(t *testing.T, code string, expectedVals ...int64) {
	t.Helper()
	checkPrintBinary(t, code, expectedVals...)
}

func checkPrintBinary(t *testing.T, code string, expectedVals ...int64) {
	t.Helper()
	output := runBinaryWithEnv(t, code, "check", nil)

	var expectedBuffer strings.Builder
	for _, v := range expectedVals {
		expectedBuffer.WriteString(fmt.Sprintf("%d\n", v))
	}
	expectedStr := expectedBuffer.String()
	if output != expectedStr {
		t.Errorf("Binary output mismatch. Expected '%s', got '%s'", expectedStr, output)
	}
}

type runBinaryConfig struct {
	env         map[string]string
	compileOpts []compiler.CompileOption
	cleanup     []func()
}

type runBinaryOption func(*runBinaryConfig) error

func withEnvMap(env map[string]string) runBinaryOption {
	return func(cfg *runBinaryConfig) error {
		if len(env) == 0 {
			return nil
		}
		for k, v := range env {
			cfg.env[k] = v
		}
		return nil
	}
}

func withCompileLanguage(language string) runBinaryOption {
	return func(cfg *runBinaryConfig) error {
		if language != "" {
			cfg.compileOpts = append(cfg.compileOpts, compiler.WithCompileLanguage(language))
		}
		return nil
	}
}

func withCompilePrintEntryResult(enabled bool) runBinaryOption {
	return func(cfg *runBinaryConfig) error {
		cfg.compileOpts = append(cfg.compileOpts, compiler.WithCompilePrintEntryResult(enabled))
		return nil
	}
}

func withCompileObfuscators(names ...string) runBinaryOption {
	return func(cfg *runBinaryConfig) error {
		cfg.compileOpts = append(cfg.compileOpts, compiler.WithCompileObfuscators(names...))
		return nil
	}
}

func runBinary(t *testing.T, code string, entry string) string {
	return runBinaryWithEnv(t, code, entry, nil)
}

func runBinaryWithEnv(t *testing.T, code string, entry string, env map[string]string, options ...runBinaryOption) string {
	allOpts := make([]runBinaryOption, 0, len(options)+1)
	allOpts = append(allOpts, withEnvMap(env))
	allOpts = append(allOpts, options...)

	t.Helper()
	cfg := newRunBinaryConfig(t, allOpts...)
	tmpBin, cleanup := compileBinary(t, code, entry, cfg)
	defer cleanup()

	cmd := exec.Command(tmpBin)
	if len(cfg.env) > 0 {
		cmd.Env = append([]string{}, os.Environ()...)
		for k, v := range cfg.env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Binary execution failed: %v\nOutput: %s", err, output)
	}

	return string(output)
}

func newRunBinaryConfig(t *testing.T, options ...runBinaryOption) *runBinaryConfig {
	t.Helper()

	cfg := &runBinaryConfig{
		env: make(map[string]string),
	}
	for _, opt := range options {
		if opt == nil {
			continue
		}
		if err := opt(cfg); err != nil {
			t.Fatalf("Failed to apply run option: %v", err)
		}
	}
	return cfg
}

func compileBinary(t *testing.T, code string, entry string, cfg *runBinaryConfig) (string, func()) {
	t.Helper()

	ensureRuntimeArchiveOnce(t)

	cleanup := func() {
		for i := len(cfg.cleanup) - 1; i >= 0; i-- {
			cfg.cleanup[i]()
		}
	}

	tmpFile, err := os.CreateTemp("", "test_run_*.yak")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(code); err != nil {
		cleanup()
		t.Fatalf("Failed to write code: %v", err)
	}
	tmpFile.Close()

	tmpBin := tmpFile.Name() + ".bin"
	cfg.cleanup = append(cfg.cleanup, func() {
		_ = os.Remove(tmpFile.Name())
		_ = os.Remove(tmpBin)
	})

	options := make([]compiler.CompileOption, 0, 8+len(cfg.compileOpts))
	options = append(options,
		compiler.WithCompileSourceFile(tmpFile.Name()),
		compiler.WithCompileOutputFile(tmpBin),
		compiler.WithCompileLanguage("yak"),
		compiler.WithCompileEntryFunction(entry),
	)
	options = append(options, cfg.compileOpts...)

	if _, err := compiler.CompileToExecutable(options...); err != nil {
		cleanup()
		t.Fatalf("Binary compilation failed: %v", err)
	}

	return tmpBin, cleanup
}

func runBinaryExitCodeWithEnv(t *testing.T, code string, entry string, env map[string]string, options ...runBinaryOption) (int, string) {
	allOpts := make([]runBinaryOption, 0, len(options)+1)
	allOpts = append(allOpts, withEnvMap(env))
	allOpts = append(allOpts, options...)

	t.Helper()
	cfg := newRunBinaryConfig(t, allOpts...)
	tmpBin, cleanup := compileBinary(t, code, entry, cfg)
	defer cleanup()

	cmd := exec.Command(tmpBin)
	if len(cfg.env) > 0 {
		cmd.Env = append([]string{}, os.Environ()...)
		for k, v := range cfg.env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), string(output)
	}
	t.Fatalf("Binary execution failed: %v\nOutput: %s", err, output)
	return -1, string(output)
}

func runBinaryReturnValue(t *testing.T, code string, entry string, language string) (int64, string) {
	t.Helper()
	return runBinaryReturnValueWithOptions(t, code, entry, language)
}

func runBinaryReturnValueWithOptions(t *testing.T, code string, entry string, language string, options ...runBinaryOption) (int64, string) {
	t.Helper()
	allOpts := make([]runBinaryOption, 0, len(options)+2)
	allOpts = append(allOpts,
		withCompileLanguage(language),
		withCompilePrintEntryResult(true),
	)
	allOpts = append(allOpts, options...)

	_, output := runBinaryExitCodeWithEnv(t, code, entry, nil, allOpts...)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var v int64
		if _, err := fmt.Sscanf(line, "%d", &v); err == nil {
			return v, output
		}
	}
	t.Fatalf("Failed to parse binary return value from output: %q", output)
	return 0, output
}

func checkRunBinary(t *testing.T, code string, entry string, env map[string]string, required []string, options ...runBinaryOption) string {
	t.Helper()
	output := runBinaryWithEnv(t, code, entry, env, options...)
	for _, want := range required {
		require.Contains(t, output, want)
	}
	return output
}

func expectedToInt64(expected interface{}) (int64, bool) {
	switch expect := expected.(type) {
	case int:
		return int64(expect), true
	case int64:
		return expect, true
	default:
		return 0, false
	}
}

func compareResult(t *testing.T, expected interface{}, result int64) {
	t.Helper()
	switch expect := expected.(type) {
	case int:
		if result != int64(expect) {
			t.Errorf("Result check failed. Expected: %d, Got: %d", expect, result)
		}
	case int64:
		if result != expect {
			t.Errorf("Result check failed. Expected: %d, Got: %d", expect, result)
		}
	case string:
		if fmt.Sprintf("%d", result) != expect {
			t.Errorf("Result check failed. Expected: %s, Got: %d", expect, result)
		}
	default:
		t.Fatalf("Unsupported expected value type: %T", expected)
	}
}
