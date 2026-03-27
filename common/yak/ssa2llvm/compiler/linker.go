package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

var runtimeLinkArgsCache sync.Map

// CompileLLVMToBinary compiles an LLVM IR file to a native executable.
// When linkRuntime is true, it links against the default yak runtime archive.
func CompileLLVMToBinary(llFile, binFile string, linkRuntime bool, runtimeArchiveOverride string, extraArgs ...string) error {
	clangPath, err := findLLVMTool("clang")
	if err != nil {
		return err
	}

	args := []string{llFile}
	if len(extraArgs) > 0 {
		args = append(args, extraArgs...)
	}

	if linkRuntime {
		runtimeArchive := runtimeArchiveOverride
		if runtimeArchive == "" {
			runtimeArchive, err = findRuntimeArchive()
			if err != nil {
				return err
			}
		}
		runtimeLinkArgs, err := resolveRuntimeLinkArgs(runtimeArchive)
		if err != nil {
			return err
		}
		args = append(args,
			runtimeArchive,
		)
		args = appendUniqueLinkArgs(args, runtimeLinkArgs...)
		args = appendUniqueLinkArgs(args,
			"-lgc",
			"-lm",
			"-lpthread",
			"-ldl",
		)
	}
	args = append(args, "-o", binFile)

	cmd := exec.Command(clangPath, args...)
	trace.PrintCmd(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("clang linking failed: %v\n%s", err, output)
	}

	return nil
}

func findRuntimeArchive() (string, error) {
	candidates := []string{
		"common/yak/ssa2llvm/runtime/libyak.a",
		"runtime/libyak.a",
		"../runtime/libyak.a",
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			absPath, absErr := filepath.Abs(candidate)
			if absErr != nil {
				return candidate, nil
			}
			return absPath, nil
		}
	}

	cwd, _ := os.Getwd()
	return "", utils.Errorf("runtime library not found: expected libyak.a under %s/common/yak/ssa2llvm/runtime, runtime, or ../runtime; run ./common/yak/ssa2llvm/scripts/build_runtime_go.sh first", cwd)
}

func resolveRuntimeLinkArgs(runtimeArchive string) ([]string, error) {
	runtimeArchive = strings.TrimSpace(runtimeArchive)
	if runtimeArchive == "" {
		return nil, nil
	}
	if cached, ok := runtimeLinkArgsCache.Load(runtimeArchive); ok {
		return append([]string{}, cached.([]string)...), nil
	}

	if flags, ok, err := readRuntimeLinkArgsFile(runtimeArchive); err != nil {
		return nil, err
	} else if ok {
		runtimeLinkArgsCache.Store(runtimeArchive, append([]string{}, flags...))
		return flags, nil
	}

	sourceDir, ok := runtimeSourceDirForArchive(runtimeArchive)
	if !ok {
		return nil, nil
	}

	flags, err := collectRuntimeLinkArgs(sourceDir)
	if err != nil {
		return nil, err
	}
	runtimeLinkArgsCache.Store(runtimeArchive, append([]string{}, flags...))
	return flags, nil
}

func readRuntimeLinkArgsFile(runtimeArchive string) ([]string, bool, error) {
	path := runtimeLinkArgsFile(runtimeArchive)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, utils.Errorf("read runtime link args failed: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	args := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		flag := strings.TrimSpace(line)
		if flag == "" {
			continue
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		args = append(args, flag)
	}
	return args, true, nil
}

func runtimeLinkArgsFile(runtimeArchive string) string {
	return filepath.Join(filepath.Dir(runtimeArchive), "libyak.linkflags")
}

func runtimeSourceDirForArchive(runtimeArchive string) (string, bool) {
	dir := filepath.Dir(runtimeArchive)
	candidates := []string{
		filepath.Join(dir, "runtime_go"),
		filepath.Join(dir, "ssa2llvm-stdlib-src", "common", "yak", "ssa2llvm", "runtime", "runtime_go"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

func collectRuntimeLinkArgs(sourceDir string) ([]string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	cmd := exec.Command(goPath, "list", "-deps", "-f", "{{if .CgoLDFLAGS}}{{range .CgoLDFLAGS}}{{printf \"%s\\n\" .}}{{end}}{{end}}", ".")
	cmd.Dir = sourceDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
		"GOWORK=off",
	)
	trace.PrintCmd(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, utils.Errorf("collect runtime link args failed: %v\n%s", err, output)
	}

	lines := strings.Split(string(output), "\n")
	args := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		flag := strings.TrimSpace(line)
		if flag == "" {
			continue
		}
		if _, ok := seen[flag]; ok {
			continue
		}
		seen[flag] = struct{}{}
		args = append(args, flag)
	}
	return args, nil
}

func appendUniqueLinkArgs(dst []string, args ...string) []string {
	seen := make(map[string]struct{}, len(dst)+len(args))
	for _, arg := range dst {
		seen[arg] = struct{}{}
	}
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			continue
		}
		if _, ok := seen[arg]; ok {
			continue
		}
		seen[arg] = struct{}{}
		dst = append(dst, arg)
	}
	return dst
}

func CompileLLVMToAsm(llFile, asmFile string) error {
	llcPath, err := findLLVMTool("llc")
	if err != nil {
		return err
	}

	cmd := exec.Command(llcPath, llFile, "-o", asmFile)
	trace.PrintCmd(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("llc failed: %v\n%s", err, output)
	}
	return nil
}

func CompileLLVMToObject(llFile, objFile string) error {
	llcPath, err := findLLVMTool("llc")
	if err != nil {
		return err
	}

	cmd := exec.Command(llcPath, "-filetype=obj", llFile, "-o", objFile)
	trace.PrintCmd(cmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("llc failed: %v\n%s", err, output)
	}
	return nil
}

func findLLVMTool(tool string) (string, error) {
	paths := []string{
		tool,
		"/opt/homebrew/opt/llvm/bin/" + tool,
		"/usr/local/opt/llvm/bin/" + tool,
		"/usr/bin/" + tool,
	}

	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return p, nil
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("%s not found, please install LLVM", tool)
}
