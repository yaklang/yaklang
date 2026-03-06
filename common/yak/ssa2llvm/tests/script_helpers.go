package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// RepoRoot walks up from the current working directory until it finds go.mod.
func RepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}

	for i := 0; i < 15; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}

	t.Fatalf("failed to locate repo root (go.mod)")
	return ""
}

// EnsureRuntimeArchive builds runtime/libyak.a when missing or stale.
func EnsureRuntimeArchive(t *testing.T, repoRoot string) {
	t.Helper()

	archiveFile := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "runtime", "libyak.a")
	runtimeSrcDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "runtime", "runtime_go")

	archiveInfo, err := os.Stat(archiveFile)
	if err == nil {
		rebuild := false
		if walkErr := filepath.WalkDir(runtimeSrcDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(path))
			switch ext {
			case ".go", ".c", ".h":
			default:
				return nil
			}
			info, statErr := d.Info()
			if statErr != nil {
				return statErr
			}
			if info.ModTime().After(archiveInfo.ModTime()) {
				rebuild = true
				return filepath.SkipAll
			}
			return nil
		}); walkErr != nil {
			t.Fatalf("failed to inspect runtime sources: %v", walkErr)
		}
		if !rebuild {
			return
		}
	}

	cmd := exec.Command("go", "build",
		"-buildmode=c-archive",
		"-o", archiveFile,
		"./common/yak/ssa2llvm/runtime/runtime_go",
	)
	cmd.Dir = repoRoot
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, "CGO_ENABLED=1")

	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build runtime archive: %v\n%s", err, output)
	}

	if _, err := os.Stat(archiveFile); err != nil {
		t.Fatalf("runtime archive not found at %s: %v", archiveFile, err)
	}
}

// ListYakScripts returns absolute paths of *.yak scripts under the given directory.
func ListYakScripts(t *testing.T, scriptDir string) []string {
	t.Helper()

	entries, err := os.ReadDir(scriptDir)
	if err != nil {
		t.Fatalf("failed to read script directory: %v", err)
	}

	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.ToLower(filepath.Ext(name)) != ".yak" {
			continue
		}
		out = append(out, filepath.Join(scriptDir, name))
	}
	sort.Strings(out)
	return out
}

// RunYakScriptFile compiles and runs a yak script with the provided env, returning stdout+stderr.
func RunYakScriptFile(t *testing.T, scriptPath string, env map[string]string) string {
	t.Helper()

	scriptAbs, err := filepath.Abs(scriptPath)
	if err != nil {
		t.Fatalf("filepath.Abs(%q) failed: %v", scriptPath, err)
	}
	code, err := os.ReadFile(scriptAbs)
	if err != nil {
		t.Fatalf("failed to read script %s: %v", scriptAbs, err)
	}

	repoRoot := RepoRoot(t)
	EnsureRuntimeArchive(t, repoRoot)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd failed: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("os.Chdir(%q) failed: %v", repoRoot, err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	out := runBinaryWithEnv(t, string(code), "", env)
	if strings.TrimSpace(out) == "" {
		return out
	}
	return out
}

func formatEnvMap(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s=%q", k, env[k]))
	}
	return b.String()
}
