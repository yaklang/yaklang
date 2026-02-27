package tests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func init() {
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()
}

func check(t *testing.T, code string, expected interface{}) {
	t.Helper()
	checkEx(t, code, "yak", expected)
}

func checkEx(t *testing.T, code string, language string, expected interface{}) {
	t.Helper()

	result, err := compiler.RunViaJIT(
		compiler.WithRunSourceCode(code),
		compiler.WithRunLanguage(language),
	)
	if err != nil {
		t.Fatalf("JIT execution failed: %v", err)
	}
	compareResult(t, expected, result)

	want, ok := expectedToInt64(expected)
	if !ok {
		t.Fatalf("Unsupported expected value type for binary check: %T", expected)
	}
	binaryRet, output := runBinaryReturnValue(t, code, "check", language)
	if binaryRet != want {
		t.Fatalf("Binary return mismatch. Expected: %d, Got: %d, Output: %q", want, binaryRet, output)
	}
}

func checkPrint(t *testing.T, code string, expectedVals ...int64) {
	t.Helper()

	teardown := SetupJITHook()
	_, err := compiler.RunViaJIT(
		compiler.WithRunSourceCode(code),
		compiler.WithRunLanguage("yak"),
		compiler.WithRunFunction("check"),
		compiler.WithRunExternalHooks(map[string]unsafe.Pointer{
			"yak_internal_print_int": getHookAddr(),
			"yak_internal_malloc":    getMallocHookAddr(),
		}),
	)
	vals := teardown()
	if err != nil {
		t.Fatalf("JIT execution failed: %v", err)
	}

	if len(vals) != len(expectedVals) {
		t.Errorf("JIT hook mismatch. Expected %d calls, got %d. Got: %v, Expected: %v",
			len(expectedVals), len(vals), vals, expectedVals)
	} else {
		for i, v := range vals {
			if v != expectedVals[i] {
				t.Errorf("JIT hook mismatch at index %d. Expected %d, got %d", i, expectedVals[i], v)
			}
		}
	}

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

func withRuntimeCode(goCode string) runBinaryOption {
	return func(cfg *runBinaryConfig) error {
		tmpDir, err := os.MkdirTemp("", "ssa2llvm-gohook-*")
		if err != nil {
			return fmt.Errorf("failed to create hook temp dir: %w", err)
		}
		cfg.cleanup = append(cfg.cleanup, func() {
			_ = os.RemoveAll(tmpDir)
		})

		source, exportedNames, err := prepareGoHookCode(goCode)
		if err != nil {
			return err
		}

		goFile := tmpDir + "/hook.go"
		archiveFile := tmpDir + "/hook.a"
		headerFile := tmpDir + "/hook.h"

		if err := os.WriteFile(goFile, []byte(source), 0o644); err != nil {
			return fmt.Errorf("failed to write hook go file: %w", err)
		}

		cmd := exec.Command("go", "build", "-buildmode=c-archive", "-o", archiveFile, goFile)
		cmd.Env = append([]string{}, os.Environ()...)
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to build go hook archive: %v\n%s", err, output)
		}

		headerBytes, err := os.ReadFile(headerFile)
		if err != nil {
			return fmt.Errorf("failed to read generated hook header: %w", err)
		}
		bindings, err := parseExternBindingsFromHeader(string(headerBytes), exportedNames)
		if err != nil {
			return err
		}

		cfg.compileOpts = append(cfg.compileOpts,
			compiler.WithCompileExternBindings(bindings),
			compiler.WithCompileSkipRuntimeLink(true),
			compiler.WithCompileExtraLinkArgs(archiveFile),
		)

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

	if err := compiler.CompileToExecutable(options...); err != nil {
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
	_, output := runBinaryExitCodeWithEnv(
		t,
		code,
		entry,
		nil,
		withCompileLanguage(language),
		withCompilePrintEntryResult(true),
	)
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

func parseGoExportNames(goCode string) []string {
	re := regexp.MustCompile(`(?m)^\s*//export\s+([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	matches := re.FindAllStringSubmatch(goCode, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		out = append(out, m[1])
	}
	return out
}

func normalizeGoHookCode(goCode string) string {
	src := strings.TrimSpace(goCode)
	if !strings.Contains(src, "package ") {
		src = "package main\n\n" + src
	}
	if !strings.Contains(src, `import "C"`) {
		inject := "/*\n#include <stdint.h>\n*/\nimport \"C\"\n\n"
		src = injectAfterPackage(src, inject)
	}
	mainRe := regexp.MustCompile(`(?m)^\s*func\s+main\s*\(\s*\)\s*\{`)
	if !mainRe.MatchString(src) {
		src += "\n\nfunc main() {}\n"
	}
	return src + "\n"
}

func prepareGoHookCode(goCode string) (string, []string, error) {
	src := normalizeGoHookCode(goCode)
	exported := parseGoExportNames(src)
	if len(exported) > 0 {
		return src, exported, nil
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "hook.go", src, parser.ParseComments)
	if err != nil {
		return "", nil, fmt.Errorf("failed to parse go hook code: %w", err)
	}

	type fnAtLine struct {
		name string
		line int
	}
	funcs := make([]fnAtLine, 0)
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Recv != nil {
			continue
		}
		name := fd.Name.Name
		if name == "main" || name == "init" {
			continue
		}
		line := fset.Position(fd.Pos()).Line
		if line > 0 {
			funcs = append(funcs, fnAtLine{name: name, line: line})
		}
	}
	if len(funcs) == 0 {
		return "", nil, fmt.Errorf("hook goCode has no top-level functions to export")
	}

	sort.Slice(funcs, func(i, j int) bool {
		return funcs[i].line < funcs[j].line
	})

	lineToExports := make(map[int][]string)
	seen := make(map[string]struct{}, len(funcs))
	for _, f := range funcs {
		lineToExports[f.line] = append(lineToExports[f.line], f.name)
		if _, ok := seen[f.name]; !ok {
			exported = append(exported, f.name)
			seen[f.name] = struct{}{}
		}
	}

	lines := strings.Split(src, "\n")
	var out strings.Builder
	for i, line := range lines {
		n := i + 1
		if names, ok := lineToExports[n]; ok {
			for _, name := range names {
				out.WriteString("//export ")
				out.WriteString(name)
				out.WriteString("\n")
			}
		}
		out.WriteString(line)
		if i != len(lines)-1 {
			out.WriteString("\n")
		}
	}

	return out.String(), exported, nil
}

func injectAfterPackage(src string, inject string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "package ") {
			head := strings.Join(lines[:i+1], "\n")
			tail := strings.Join(lines[i+1:], "\n")
			if strings.TrimSpace(tail) == "" {
				return head + "\n\n" + inject
			}
			return head + "\n\n" + inject + tail
		}
	}
	return "package main\n\n" + inject + src
}

func parseExternBindingsFromHeader(header string, names []string) (map[string]compiler.ExternBinding, error) {
	re := regexp.MustCompile(`(?m)^extern\s+(.+?)\s+([A-Za-z_][A-Za-z0-9_]*)\((.*)\);$`)
	matches := re.FindAllStringSubmatch(header, -1)
	prototypes := make(map[string]struct {
		ret    string
		params string
	}, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		prototypes[m[2]] = struct {
			ret    string
			params string
		}{
			ret:    strings.TrimSpace(m[1]),
			params: strings.TrimSpace(m[3]),
		}
	}

	bindings := make(map[string]compiler.ExternBinding, len(names))
	for _, name := range names {
		p, ok := prototypes[name]
		if !ok {
			return nil, fmt.Errorf("exported symbol %q not found in generated header", name)
		}
		paramTypes := parseCParamTypes(p.params)
		bindings[name] = compiler.ExternBinding{
			Symbol: name,
			Params: paramTypes,
			Return: parseCTypeToLLVMExternType(p.ret),
		}
	}
	return bindings, nil
}

func parseCParamTypes(params string) []compiler.LLVMExternType {
	params = strings.TrimSpace(params)
	if params == "" || params == "void" {
		return nil
	}
	parts := strings.Split(params, ",")
	out := make([]compiler.LLVMExternType, 0, len(parts))
	for _, p := range parts {
		out = append(out, parseCTypeToLLVMExternType(trimParamName(p)))
	}
	return out
}

func trimParamName(param string) string {
	s := strings.TrimSpace(param)
	fields := strings.Fields(s)
	if len(fields) >= 2 {
		last := fields[len(fields)-1]
		if !strings.Contains(last, "*") {
			return strings.Join(fields[:len(fields)-1], " ")
		}
	}
	return s
}

func parseCTypeToLLVMExternType(t string) compiler.LLVMExternType {
	typ := strings.TrimSpace(t)
	if strings.HasPrefix(typ, "const ") {
		typ = strings.TrimSpace(strings.TrimPrefix(typ, "const "))
	}
	if typ == "void" {
		return compiler.ExternTypeVoid
	}
	if strings.Contains(typ, "*") {
		return compiler.ExternTypePtr
	}
	return compiler.ExternTypeI64
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
