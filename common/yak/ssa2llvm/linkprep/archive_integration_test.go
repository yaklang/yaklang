package linkprep

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPrepareForLink_integration rewrites a tiny archive whose only global is
// yak_internal_malloc. Requires clang, ar, nm, and llvm-objcopy/objcopy on PATH.
func TestPrepareForLink_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping archive integration test in -short mode")
	}
	cc := firstLookPath([]string{"clang", "gcc"})
	arTool := findAr()
	nmTool := findNm()
	objcopy := findObjcopy()
	if cc == "" || arTool == "" || nmTool == "" || objcopy == "" {
		t.Skip("need clang/gcc, ar/llvm-ar, nm/llvm-nm, and objcopy/llvm-objcopy on PATH")
	}

	td := t.TempDir()
	src := filepath.Join(td, "one.c")
	require.NoError(t, os.WriteFile(src, []byte("void yak_internal_malloc(void) {}\n"), 0o644))

	obj := filepath.Join(td, "one.o")
	run(t, td, cc, "-c", "-o", obj, src)

	arc := filepath.Join(td, "libtest.a")
	run(t, td, arTool, "rcs", arc, obj)

	newName := "rt_0123456789abcdef"
	manifest := map[string]string{"yak_internal_malloc": newName}
	outs, cleanup, err := PrepareForLink(PrepareInput{
		Archives: []string{arc},
		Manifest: manifest,
		WorkDir:  td,
		Trace:    false,
	})
	require.NoError(t, err)
	t.Cleanup(cleanup)
	require.Len(t, outs, 1)

	outNm := runOutput(t, td, nmTool, "-g", "--defined-only", outs[0])
	require.Contains(t, outNm, newName)
	require.False(t, strings.Contains(outNm, "yak_internal_malloc"), "symbol should be renamed in nm output:\n%s", outNm)
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", name, args, out)
}

func runOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", name, args, out)
	return string(out)
}

func firstLookPath(names []string) string {
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	return ""
}
