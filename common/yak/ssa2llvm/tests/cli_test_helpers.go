package tests

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	ssa2llvmCLIOnce sync.Once
	ssa2llvmCLIPath string
	ssa2llvmCLIErr  error
)

type processResult struct {
	ExitCode int
	Output   string
}

// buildSSA2LLVMCLI builds the real shipping CLI once for the current test run.
// Acceptance tests should exercise this binary instead of calling compiler helpers directly.
func buildSSA2LLVMCLI(t *testing.T) string {
	t.Helper()

	repoRoot := RepoRoot(t)
	ssa2llvmCLIOnce.Do(func() {
		buildDir, err := os.MkdirTemp("", "ssa2llvm-cli-*")
		if err != nil {
			ssa2llvmCLIErr = fmt.Errorf("create cli build dir failed: %w", err)
			return
		}

		name := "ssa2llvm"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		ssa2llvmCLIPath = filepath.Join(buildDir, name)

		cmd := exec.Command("go", "build", "-o", ssa2llvmCLIPath, "./common/yak/ssa2llvm/cmd/ssa2llvm")
		cmd.Dir = repoRoot
		cmd.Env = append([]string{}, os.Environ()...)
		cmd.Env = append(cmd.Env, "CGO_ENABLED=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			ssa2llvmCLIErr = fmt.Errorf("build ssa2llvm cli failed: %v\n%s", err, output)
			return
		}
	})

	if ssa2llvmCLIErr != nil {
		t.Fatalf("%v", ssa2llvmCLIErr)
	}
	return ssa2llvmCLIPath
}

func runProcess(t *testing.T, bin string, env map[string]string, args ...string) processResult {
	return runProcessInDir(t, "", bin, env, args...)
}

func runProcessInDir(t *testing.T, dir, bin string, env map[string]string, args ...string) processResult {
	t.Helper()

	cmd := exec.Command(bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	output, err := cmd.CombinedOutput()
	if err == nil {
		return processResult{ExitCode: 0, Output: string(output)}
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return processResult{ExitCode: exitErr.ExitCode(), Output: string(output)}
	}
	t.Fatalf("process %q failed: %v\nOutput: %s", bin, err, output)
	return processResult{}
}

func runSSA2LLVMCLI(t *testing.T, args ...string) processResult {
	return runSSA2LLVMCLIInDir(t, "", args...)
}

func runSSA2LLVMCLIInDir(t *testing.T, dir string, args ...string) processResult {
	t.Helper()

	if len(args) == 0 {
		t.Fatal("runSSA2LLVMCLI requires args")
	}
	// Real CLI compile/run tests should match ordinary user flows, which expect
	// a usable runtime archive. Force a rebuild only when the compiler package
	// or the runtime tree changed since the last CLI invocation in this process;
	// during a stable run the cached work dir (yakssa-compile-*) is reused,
	// avoiding the ~10x cost of -a on every compile while still busting stale
	// caches after source changes. Callers that explicitly pass -a keep their
	// force semantics.
	switch args[0] {
	case "compile", "run":
		ensureRuntimeArchiveOnce(t)
		if dir != "" {
			prepareRuntimeArchiveForDir(t, dir)
		}
		if !containsArg(args, "-a") && cliForceRebuild.needsForce(RepoRoot(t), ssa2llvmSourceFingerprint(t)) {
			t.Logf("ssa2llvm: force rebuild (-a) enabled, compiler/runtime fingerprint changed")
			args = insertBeforeArgSeparator(args, "-a")
		}
	}

	cliPath := buildSSA2LLVMCLI(t)
	yakitHome := filepath.Join(t.TempDir(), ".db")
	return runProcessInDir(t, dir, cliPath, map[string]string{
		"YAKIT_HOME": yakitHome,
	}, args...)
}

func insertBeforeArgSeparator(args []string, item string) []string {
	for i, arg := range args {
		if arg != "--" {
			continue
		}
		out := make([]string, 0, len(args)+1)
		out = append(out, args[:i]...)
		out = append(out, item)
		out = append(out, args[i:]...)
		return out
	}
	return append(args, item)
}

// forceRebuildTracker persists the last source fingerprint under /tmp so the
// decision survives across test processes. The first invocation (no recorded
// fingerprint) always forces a rebuild, then a rebuild is forced again only
// when the compiler/runtime fingerprint changes. This keeps the anti-stale
// semantics while letting stable runs reuse the cached work dir.
type forceRebuildTracker struct {
	mu              sync.Mutex
	fingerprintPath string
}

func (tr *forceRebuildTracker) needsForce(repoRoot, fingerprint string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.fingerprintPath == "" {
		// Keep one fingerprint per worktree so concurrent checkouts never
		// invalidate each other's cached work dirs.
		rootHash := sha256.Sum256([]byte(repoRoot))
		tr.fingerprintPath = filepath.Join(
			os.TempDir(),
			fmt.Sprintf("yakssa-compile-fingerprint-%s", hex.EncodeToString(rootHash[:8])),
		)
	}
	last, err := os.ReadFile(tr.fingerprintPath)
	if err == nil && string(last) == fingerprint {
		return false
	}
	// Atomic write: temp file + rename, so concurrent processes never observe
	// a partially written fingerprint.
	tmp := tr.fingerprintPath + ".tmp"
	if writeErr := os.WriteFile(tmp, []byte(fingerprint), 0o644); writeErr != nil {
		return true
	}
	if renameErr := os.Rename(tmp, tr.fingerprintPath); renameErr != nil {
		_ = os.Remove(tmp)
		return true
	}
	return true
}

var cliForceRebuild forceRebuildTracker

// ssa2llvmSourceFingerprint hashes the size and mtime of every source file in
// the compiler package and every file under the ssa2llvm runtime tree (source
// files plus generated artifacts such as libyak.a and libyak.linkflags).
// Content is intentionally not read: mtime+size is the requested cheap signal
// and these trees only change when the toolchain itself is rebuilt.
func ssa2llvmSourceFingerprint(t *testing.T) string {
	t.Helper()
	repoRoot := RepoRoot(t)
	h := sha256.New()
	compilerDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "compiler")
	runtimeDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "runtime")
	for _, dir := range []string{compilerDir, runtimeDir} {
		_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if dir == compilerDir && filepath.Ext(path) != ".go" {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(repoRoot, path)
			if err != nil {
				return err
			}
			_, _ = io.WriteString(h, rel)
			_, _ = fmt.Fprintf(h, "\x00%d\x00%d\n", info.Size(), info.ModTime().UnixNano())
			return nil
		})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func writeYakSourceFile(t *testing.T, code string) string {
	t.Helper()

	src := filepath.Join(t.TempDir(), "input.yak")
	if err := os.WriteFile(src, []byte(code), 0o644); err != nil {
		t.Fatalf("write source file failed: %v", err)
	}
	return src
}

func prepareRuntimeArchiveForDir(t *testing.T, dir string) {
	t.Helper()

	repoRoot := RepoRoot(t)
	srcRuntimeDir := filepath.Join(repoRoot, "common", "yak", "ssa2llvm", "runtime")
	dstRuntimeDir := filepath.Join(dir, "common", "yak", "ssa2llvm", "runtime")
	if _, err := os.Stat(dstRuntimeDir); err == nil {
		return
	}

	requireDir := filepath.Dir(dstRuntimeDir)
	if err := os.MkdirAll(requireDir, 0o755); err != nil {
		t.Fatalf("prepare runtime archive dir failed: %v", err)
	}

	if err := os.Symlink(srcRuntimeDir, dstRuntimeDir); err == nil {
		return
	}

	if err := os.MkdirAll(dstRuntimeDir, 0o755); err != nil {
		t.Fatalf("prepare mirrored runtime dir failed: %v", err)
	}

	srcArchive := filepath.Join(srcRuntimeDir, "libyak.a")
	dstArchive := filepath.Join(dstRuntimeDir, "libyak.a")
	src, err := os.Open(srcArchive)
	if err != nil {
		t.Fatalf("open runtime archive failed: %v", err)
	}
	defer src.Close()

	dst, err := os.Create(dstArchive)
	if err != nil {
		t.Fatalf("create mirrored runtime archive failed: %v", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("copy mirrored runtime archive failed: %v", err)
	}

	srcRuntimeGo := filepath.Join(srcRuntimeDir, "runtime_go")
	dstRuntimeGo := filepath.Join(dstRuntimeDir, "runtime_go")
	if err := os.Symlink(srcRuntimeGo, dstRuntimeGo); err != nil && !os.IsExist(err) {
		t.Fatalf("mirror runtime_go dir failed: %v", err)
	}

	srcLinkFlags := filepath.Join(srcRuntimeDir, "libyak.linkflags")
	if _, err := os.Stat(srcLinkFlags); err == nil {
		dstLinkFlags := filepath.Join(dstRuntimeDir, "libyak.linkflags")
		data, readErr := os.ReadFile(srcLinkFlags)
		if readErr != nil {
			t.Fatalf("read runtime link flags failed: %v", readErr)
		}
		if writeErr := os.WriteFile(dstLinkFlags, data, 0o644); writeErr != nil {
			t.Fatalf("write runtime link flags failed: %v", writeErr)
		}
	}
}
