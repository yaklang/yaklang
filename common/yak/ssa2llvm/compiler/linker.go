package compiler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/yaklang/yaklang/common/utils"
)

// CompileLLVMToBinary compiles an LLVM IR file to a native executable, linking with the Yak runtime.
func CompileLLVMToBinary(llFile, binFile string) error {
	// Find clang
	clangPath, err := findLLVMTool("clang")
	if err != nil {
		return err
	}

	// Determine runtime path
	// We check standard locations relative to the project root
	runtimeDir := "common/yak/ssa2llvm/runtime"
	if _, err := os.Stat(runtimeDir); os.IsNotExist(err) {
		// Try finding it relative to current working directory
		if _, err := os.Stat("runtime/libyak.a"); err == nil {
			runtimeDir = "runtime"
		} else if _, err := os.Stat("../runtime/libyak.a"); err == nil {
			runtimeDir = "../runtime"
		} else {
			cwd, _ := os.Getwd()
			return utils.Errorf("runtime library not found in %s/common/yak/ssa2llvm/runtime or runtime", cwd)
		}
	}

	absRuntimeDir, _ := filepath.Abs(runtimeDir)

	// Build command: clang <llFile> -o <binFile> -L<runtimeDir> -lyak
	args := []string{
		llFile,
		"-o", binFile,
		"-L" + absRuntimeDir,
		"-lyak",
		// "-v", // Debug linking
	}

	cmd := exec.Command(clangPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("clang linking failed: %v\n%s", err, output)
	}

	return nil
}

func CompileLLVMToAsm(llFile, asmFile string) error {
	llcPath, err := findLLVMTool("llc")
	if err != nil {
		return err
	}

	cmd := exec.Command(llcPath, llFile, "-o", asmFile)
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
