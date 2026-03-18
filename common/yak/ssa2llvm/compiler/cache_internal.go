package compiler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
)

const cachedCompileVersion = "ssa2llvm-cache-v1"

var toolVersionMemo sync.Map // map[string]string

func cachedWorkKeyFromConfig(cfg *CompileConfig) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("compute work key failed: nil config")
	}

	var code []byte
	if strings.TrimSpace(cfg.SourceCode) != "" {
		code = []byte(cfg.SourceCode)
	} else {
		srcPath := strings.TrimSpace(cfg.SourceFile)
		if srcPath == "" {
			return "", fmt.Errorf("compute work key failed: empty source file")
		}
		b, err := os.ReadFile(srcPath)
		if err != nil {
			return "", fmt.Errorf("compute work key failed: read source file: %w", err)
		}
		code = b
	}

	obf := append([]string{}, cfg.Obfuscators...)
	sort.Strings(obf)

	h := sha256.New()
	write := func(s string) {
		_, _ = io.WriteString(h, s)
		_, _ = io.WriteString(h, "\n")
	}

	write(cachedCompileVersion)
	write("compiler=" + buildInfoKey())
	write("goos=" + runtime.GOOS)
	write("goarch=" + runtime.GOARCH)
	write("go=" + runtime.Version())
	write("lang=" + strings.TrimSpace(cfg.Language))
	write("entry=" + strings.TrimSpace(cfg.EntryFunctionName))
	write(fmt.Sprintf("emitLLVM=%t", cfg.EmitLLVM))
	write(fmt.Sprintf("emitAsm=%t", cfg.EmitAsm))
	write(fmt.Sprintf("compileOnly=%t", cfg.CompileOnly))
	write(fmt.Sprintf("printIR=%t", cfg.PrintIR))
	write(fmt.Sprintf("printEntryResult=%t", cfg.PrintEntryResult))
	write(fmt.Sprintf("skipRuntimeLink=%t", cfg.SkipRuntimeLink))
	write(fmt.Sprintf("stdlibCompile=%t", cfg.StdlibCompile))
	needClang := !cfg.EmitLLVM && !cfg.EmitAsm && !cfg.CompileOnly
	needLLC := cfg.EmitAsm || cfg.CompileOnly
	if needClang {
		write("clang=" + llvmToolVersionKey("clang"))
	}
	if needLLC {
		write("llc=" + llvmToolVersionKey("llc"))
	}
	if cfg.StdlibCompile {
		write("goTool=" + goToolVersionKey())
		if h, ok, err := embed.EmbeddedRuntimeSourceHash(); ok {
			if err != nil {
				write("embeddedRuntimeSourceHash=error:" + err.Error())
			} else {
				write("embeddedRuntimeSourceHash=" + h)
			}
		} else {
			write("embeddedRuntimeSourceHash=<none>")
		}
	} else {
		if h, ok, err := embed.EmbeddedRuntimeHash(); ok {
			if err != nil {
				write("embeddedRuntimeHash=error:" + err.Error())
			} else {
				write("embeddedRuntimeHash=" + h)
			}
		} else {
			write("embeddedRuntimeHash=<none>")
		}
	}
	write("obf=" + strings.Join(obf, ","))
	if strings.TrimSpace(cfg.RuntimeArchive) != "" {
		write("runtimeArchive=" + strings.TrimSpace(cfg.RuntimeArchive))
	}
	if len(cfg.ExtraLinkArgs) > 0 {
		write("extraLinkArgs=" + strings.Join(cfg.ExtraLinkArgs, "\x00"))
	}
	_, _ = h.Write(code)

	return hex.EncodeToString(h.Sum(nil)), nil
}

func buildInfoKey() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok || bi == nil {
		return "<no-build-info>"
	}
	rev := ""
	modified := ""
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			modified = s.Value
		}
	}
	// Keep stable and short; include revision when available to bust caches on upgrades.
	if strings.TrimSpace(rev) == "" {
		return bi.GoVersion + " " + bi.Main.Version
	}
	return bi.GoVersion + " " + rev + " modified=" + modified
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

func cachedWorkDirFromKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) > 32 {
		key = key[:32]
	}
	if key == "" {
		key = "unknown"
	}
	return filepath.Join(os.TempDir(), "yakssa-compile-"+key)
}

func cachedArtifactPath(workDir string, cfg *CompileConfig) string {
	switch {
	case cfg != nil && cfg.EmitLLVM:
		return filepath.Join(workDir, "cache.ll")
	case cfg != nil && cfg.EmitAsm:
		return filepath.Join(workDir, "cache.s")
	case cfg != nil && cfg.CompileOnly:
		return filepath.Join(workDir, "cache.o")
	default:
		if runtime.GOOS == "windows" {
			return filepath.Join(workDir, "cache.exe")
		}
		return filepath.Join(workDir, "cache.bin")
	}
}
