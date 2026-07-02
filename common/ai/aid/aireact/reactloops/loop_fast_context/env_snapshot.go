package loop_fast_context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

// WorkEnvSnapshot is injected into the FastContext system prompt each run.
type WorkEnvSnapshot struct {
	OSKind     string
	ShellName  string
	WorkDir    string
	DirListing string
}

func buildWorkEnvSnapshot(invoker aicommon.AIInvokeRuntime, workDir string) WorkEnvSnapshot {
	snap := WorkEnvSnapshot{
		OSKind:    runtime.GOOS,
		ShellName: os.Getenv("SHELL"),
		WorkDir:   resolveWorkDir(invoker, workDir),
	}
	if snap.ShellName == "" {
		snap.ShellName = "unknown"
	}
	snap.DirListing = listWorkDir(snap.WorkDir)
	return snap
}

func resolveWorkDir(invoker aicommon.AIInvokeRuntime, preferred string) string {
	if wd := strings.TrimSpace(preferred); wd != "" {
		if abs, err := filepath.Abs(wd); err == nil {
			return abs
		}
		return filepath.Clean(wd)
	}
	if invoker != nil {
		if cfg := invoker.GetConfig(); cfg != nil {
			if dir := cfg.GetOrCreateWorkDir(); dir != "" {
				if abs, err := filepath.Abs(dir); err == nil {
					return abs
				}
				return filepath.Clean(dir)
			}
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

func listWorkDir(workDir string) string {
	if workDir == "" {
		return "(workdir empty)"
	}
	// Prefer `ls -la` as specified by the paper guide; fall back to Go walk on failure.
	if out, err := exec.Command("ls", "-la", workDir).CombinedOutput(); err == nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return text
		}
	}
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return fmt.Sprintf("(cannot list %s: %v)", workDir, err)
	}
	var lines []string
	for _, e := range entries {
		lines = append(lines, e.Name())
	}
	if len(lines) == 0 {
		return "(empty directory)"
	}
	return strings.Join(lines, "\n")
}
