//go:build !selfcontained

package compiler

import (
	"os/exec"
	"strings"
)

// cacheToolKeyPart writes the legacy clang/llc/opt/go-tool/runtime-archive key
// components (probed from the host toolchain via subprocess) into the cache
// hash. The self-contained build uses the embedded-runtime manifest instead
// (no host probing, no subprocess).
func cacheToolKeyPart(cfg *CompileConfig, write func(string)) {
	needClang := !cfg.EmitLLVM && !cfg.EmitAsm && !cfg.CompileOnly
	needLLC := cfg.EmitAsm || cfg.CompileOnly
	if needClang {
		write("clang=" + llvmToolVersionKey("clang"))
	}
	if needLLC {
		write("llc=" + llvmToolVersionKey("llc"))
	}
	if strings.TrimSpace(cfg.LLVMPluginPath) != "" || strings.TrimSpace(cfg.LLVMPack) != "" {
		write("opt=" + llvmToolVersionKey("opt"))
	}
	if cfg.StdlibCompile {
		write("goTool=" + goToolVersionKey())
	} else if strings.TrimSpace(cfg.RuntimeArchive) != "" {
		write("runtimeArchive=" + strings.TrimSpace(cfg.RuntimeArchive))
	}
}

func llvmToolVersionKey(tool string) string {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "<empty>"
	}
	if v, ok := toolVersionMemo.Load("llvm:" + tool); ok {
		return v.(string)
	}

	path, err := findLLVMTool(tool)
	if err != nil {
		toolVersionMemo.Store("llvm:"+tool, "missing:"+err.Error())
		return "missing:" + err.Error()
	}
	out, err := exec.Command(path, "--version").CombinedOutput()
	if err != nil {
		v := "error:" + err.Error()
		toolVersionMemo.Store("llvm:"+tool, v)
		return v
	}
	line := strings.TrimSpace(string(out))
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		line = strings.TrimSpace(line[:idx])
	}
	v := path + " " + line
	toolVersionMemo.Store("llvm:"+tool, v)
	return v
}

func goToolVersionKey() string {
	if v, ok := toolVersionMemo.Load("go"); ok {
		return v.(string)
	}
	goPath, err := exec.LookPath("go")
	if err != nil {
		toolVersionMemo.Store("go", "missing:"+err.Error())
		return "missing:" + err.Error()
	}
	out, err := exec.Command(goPath, "version").CombinedOutput()
	if err != nil {
		toolVersionMemo.Store("go", "error:"+err.Error())
		return "error:" + err.Error()
	}
	v := strings.TrimSpace(string(out))
	toolVersionMemo.Store("go", v)
	return v
}