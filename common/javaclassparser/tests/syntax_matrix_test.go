package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/javasyntax"
)

// This file builds a reproducible, javac-backed syntax-coverage matrix for the
// decompiler. Real .java sources live under corpus/{classic,modern}; they are
// compiled at test time (classic at --release 8, modern at --release 17), every
// resulting .class is decompiled and classified, and a per-category matrix is
// printed (and optionally written to COV_OUT for the benchmark report).
//
// The whole suite is skipped when javac is unavailable, so it never breaks a JDK
// less CI environment; the embedded-class battery in syntax_coverage_test.go
// remains the always-on core gate.

// classification of a single class decompilation outcome.
const (
	covOK     = "ok"     // decompiled, no stub, re-parses as valid Java
	covStub   = "stub"   // decompiled but at least one member degraded to a stub
	covSyntax = "syntax" // decompiled without stub, but output fails to re-parse
	covError  = "error"  // Decompile returned an error
	covPanic  = "panic"  // Decompile panicked
)

type covResult struct {
	status string
	detail string
}

// classifyDecompile decompiles raw and reports the coverage status plus a short detail.
func classifyDecompile(raw []byte) (res covResult) {
	defer func() {
		if r := recover(); r != nil {
			res = covResult{status: covPanic, detail: fmt.Sprint(r)}
		}
	}()
	out, err := javaclassparser.Decompile(raw)
	if err != nil {
		return covResult{status: covError, detail: firstLine(err.Error())}
	}
	if strings.Contains(out, javaclassparser.DecompileStubMarker) {
		return covResult{status: covStub, detail: stubReason(out)}
	}
	if vErr := javasyntax.Validate(out); vErr != nil {
		return covResult{status: covSyntax, detail: firstLine(vErr.Error())}
	}
	return covResult{status: covOK}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:120]
	}
	return strings.TrimSpace(s)
}

func stubReason(out string) string {
	for _, ln := range strings.Split(out, "\n") {
		if strings.Contains(ln, javaclassparser.DecompileStubMarker) {
			return firstLine(strings.TrimSpace(ln))
		}
	}
	return ""
}

// compileCorpus compiles every .java under srcDir into a fresh temp dir using the
// given --release level and returns the temp dir holding the .class files.
func compileCorpus(t *testing.T, javac, srcDir, release string) string {
	t.Helper()
	srcs, err := filepath.Glob(filepath.Join(srcDir, "*.java"))
	if err != nil || len(srcs) == 0 {
		t.Fatalf("no sources under %s: %v", srcDir, err)
	}
	outDir := t.TempDir()
	args := []string{"--release", release, "-d", outDir}
	args = append(args, srcs...)
	cmd := exec.Command(javac, args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("javac failed for %s (release %s): %v\n%s", srcDir, release, err, stderr.String())
	}
	return outDir
}

// outerName returns the top-level class name for a .class file (the part before '$').
func outerName(classFile string) string {
	base := strings.TrimSuffix(filepath.Base(classFile), ".class")
	if i := strings.IndexByte(base, '$'); i >= 0 {
		base = base[:i]
	}
	return base
}

// rankStatus orders statuses worst-first so a category's roll-up reflects its worst class.
func rankStatus(s string) int {
	switch s {
	case covPanic:
		return 4
	case covError:
		return 3
	case covSyntax:
		return 2
	case covStub:
		return 1
	default:
		return 0
	}
}

type groupResult struct {
	group   string
	status  string // worst status across the group's classes
	detail  string
	classes int
}

// decompileTree walks a compiled tree and rolls results up per top-level class.
func decompileTree(t *testing.T, dir string) map[string]*groupResult {
	t.Helper()
	groups := map[string]*groupResult{}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".class") {
			return nil
		}
		raw, rErr := os.ReadFile(p)
		if rErr != nil {
			return nil
		}
		g := outerName(p)
		res := classifyDecompile(raw)
		gr, ok := groups[g]
		if !ok {
			gr = &groupResult{group: g, status: covOK}
			groups[g] = gr
		}
		gr.classes++
		if rankStatus(res.status) > rankStatus(gr.status) {
			gr.status = res.status
			gr.detail = res.detail
		}
		return nil
	})
	return groups
}

func renderMatrix(title string, groups map[string]*groupResult) (string, map[string]int) {
	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)
	counts := map[string]int{}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("==== %s ====\n", title))
	for _, n := range names {
		g := groups[n]
		counts[g.status]++
		line := fmt.Sprintf("  %-7s %-22s classes=%d", strings.ToUpper(g.status), n, g.classes)
		if g.detail != "" {
			line += "  | " + g.detail
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString(fmt.Sprintf("  -- totals: ok=%d stub=%d syntax=%d error=%d panic=%d (groups=%d)\n",
		counts[covOK], counts[covStub], counts[covSyntax], counts[covError], counts[covPanic], len(names)))
	return sb.String(), counts
}

// TestSyntaxCoverageMatrix is the Phase-1 coverage probe: it compiles the corpus,
// decompiles every class and prints a per-category status matrix. It gates on the
// hard-failure classes (panic / decompile-error / invalid-syntax output) for the
// classic corpus, which represent real decompiler defects; honest stubs are tracked
// but not gated. The modern corpus is reported only (newer syntax is exploratory).
func TestSyntaxCoverageMatrix(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found; skipping reproducible syntax-coverage matrix")
	}

	classicDir := compileCorpus(t, javac, "corpus/classic", "8")
	modernDir := compileCorpus(t, javac, "corpus/modern", "17")

	classic := decompileTree(t, classicDir)
	modern := decompileTree(t, modernDir)

	classicReport, classicCounts := renderMatrix("CLASSIC corpus (Java 8 bytecode)", classic)
	modernReport, modernCounts := renderMatrix("MODERN corpus (Java 17 bytecode)", modern)

	t.Log("\n" + classicReport)
	t.Log("\n" + modernReport)

	if out := os.Getenv("COV_OUT"); out != "" {
		body := classicReport + "\n" + modernReport
		if wErr := os.WriteFile(out, []byte(body), 0o644); wErr != nil {
			t.Logf("write COV_OUT failed: %v", wErr)
		} else {
			t.Logf("coverage matrix written to %s", out)
		}
	}

	// Gate: classic corpus must not produce hard failures (panic / error / invalid syntax).
	var hard []string
	for name, g := range classic {
		switch g.status {
		case covPanic, covError, covSyntax:
			hard = append(hard, fmt.Sprintf("%s=%s (%s)", name, g.status, g.detail))
		}
	}
	sort.Strings(hard)
	if len(hard) > 0 {
		t.Errorf("classic corpus has %d hard-failing categories (must be ok/stub):\n  %s",
			len(hard), strings.Join(hard, "\n  "))
	}
	_ = classicCounts
	_ = modernCounts
}
