package embed

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

// BuildRuntimeArchiveFromSourceTree builds libyak.a from an extracted yaklang source tree.
// srcDir must be a module root containing go.mod/go.sum and the repo-relative paths
// like "common/yak/ssa2llvm/runtime/runtime_go".
//
// It returns:
// - archivePath: path to the generated libyak.a under buildDir
// - gcLibDir: directory containing libgc.a (used for clang -L... to satisfy -lgc)
func BuildRuntimeArchiveFromSourceTree(buildDir, srcDir string) (archivePath string, gcLibDir string, err error) {
	buildDir = strings.TrimSpace(buildDir)
	srcDir = strings.TrimSpace(srcDir)
	if buildDir == "" {
		return "", "", fmt.Errorf("build runtime archive failed: empty buildDir")
	}
	if srcDir == "" {
		return "", "", fmt.Errorf("build runtime archive failed: empty srcDir")
	}

	gcLibDir = filepath.Join(srcDir, "common", "yak", "ssa2llvm", "runtime", "runtime_go", "libs")
	if _, statErr := os.Stat(filepath.Join(gcLibDir, "libgc.a")); statErr != nil {
		return "", "", fmt.Errorf("build runtime archive failed: libgc.a not found under %s", gcLibDir)
	}

	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	archivePath = filepath.Join(buildDir, "libyak.a")
	cmd := exec.Command(goPath, "build", "-buildmode=c-archive", "-o", archivePath, "./common/yak/ssa2llvm/runtime/runtime_go")
	cmd.Dir = srcDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
		"GOWORK=off",
	)
	trace.PrintCmd(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("build runtime archive failed: %v\n%s", err, out)
	}

	if _, err := os.Stat(archivePath); err != nil {
		return "", "", fmt.Errorf("build runtime archive failed: output libyak.a missing: %v", err)
	}
	if _, err := writeRuntimeLinkArgsFile(buildDir, srcDir); err != nil {
		return "", "", err
	}
	return archivePath, gcLibDir, nil
}

// BuildRuntimeArchiveFromEmbeddedSource extracts the embedded source archive into buildDir
// and then builds libyak.a from it. The caller is expected to pass "-L<gcLibDir>" to clang.
func BuildRuntimeArchiveFromEmbeddedSource(buildDir string) (archivePath string, gcLibDir string, err error) {
	srcDir := filepath.Join(buildDir, "ssa2llvm-stdlib-src")
	if _, err := ExtractRuntimeSourceToDir(srcDir); err != nil {
		return "", "", err
	}
	return BuildRuntimeArchiveFromSourceTree(buildDir, srcDir)
}

func writeRuntimeLinkArgsFile(buildDir, srcDir string) (string, error) {
	goPath, err := exec.LookPath("go")
	if err != nil {
		return "", fmt.Errorf("go toolchain not found in PATH: %w", err)
	}

	runtimeDir := filepath.Join(srcDir, "common", "yak", "ssa2llvm", "runtime", "runtime_go")
	cmd := exec.Command(goPath, "list", "-deps", "-f", "{{if .CgoLDFLAGS}}{{range .CgoLDFLAGS}}{{printf \"%s\\n\" .}}{{end}}{{end}}", ".")
	cmd.Dir = runtimeDir
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
		"GOWORK=off",
	)
	trace.PrintCmd(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("build runtime archive failed: collect runtime link args: %v\n%s", err, out)
	}

	linkFlagsPath := filepath.Join(buildDir, "libyak.linkflags")
	if err := os.WriteFile(linkFlagsPath, out, 0o644); err != nil {
		return "", fmt.Errorf("build runtime archive failed: write %s: %v", linkFlagsPath, err)
	}
	return linkFlagsPath, nil
}
