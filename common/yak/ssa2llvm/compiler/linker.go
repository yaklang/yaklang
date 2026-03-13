package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

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
		args = append(args,
			runtimeArchive,
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
