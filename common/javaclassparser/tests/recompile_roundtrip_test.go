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
)

// This file implements the strongest correctness oracle for the decompiler: a
// recompile roundtrip. For every self-contained, single-class corpus category we
//   1. compile the original .java with javac          (source -> bytecode)
//   2. decompile the resulting .class                 (bytecode -> Java)
//   3. recompile the decompiled Java with javac       (Java -> bytecode)
//
// Step 3 is a far stricter check than ANTLR re-parsing: javac rejects type errors,
// malformed array dimensions, illegal cast precedence and other defects that the
// lenient decompiler grammar happily accepts. A category that survives the
// roundtrip is strong evidence the decompiled source is correct Java.
//
// Only single-class groups are eligible: the decompiler does not inline nested
// classes (it emits `new Outer$Inner(...)` references), so multi-class groups
// cannot be recompiled in isolation. Those remain covered by the parse-based
// coverage matrix. Stubbed outputs are reported as skipped (a stub throws by
// design and is not meant to recompile).

const (
	rcOK      = "recompile-ok"
	rcFail    = "recompile-fail"
	rcStub    = "skip-stub"
	rcDecErr  = "decompile-error"
	rcMulti   = "skip-multiclass"
	rcRelease = "8"
)

type rcResult struct {
	group  string
	status string
	detail string
}

// listClassGroups groups compiled .class files by their top-level class name.
func listClassGroups(dir string) map[string][]string {
	groups := map[string][]string{}
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".class") {
			return nil
		}
		groups[outerName(p)] = append(groups[outerName(p)], p)
		return nil
	})
	return groups
}

// recompileDecompiled writes src to <name>.java in a fresh dir and runs javac on it,
// returning the javac stderr (empty on success) and whether compilation succeeded.
func recompileDecompiled(t *testing.T, javac, className, src string) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	// javac requires the file name to match the public top-level class name.
	javaFile := filepath.Join(dir, className+".java")
	if err := os.WriteFile(javaFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write decompiled java: %v", err)
	}
	outDir := filepath.Join(dir, "out")
	_ = os.MkdirAll(outDir, 0o755)
	cmd := exec.Command(javac, "-J-Duser.language=en", "-J-Duser.country=US",
		"-nowarn", "-Xlint:none", "--release", rcRelease, "-d", outDir, javaFile)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stderr.String(), err == nil
}

// TestRecompileRoundtrip is the Phase-2 correctness oracle. It gates on a curated
// allow-list of categories that must survive the roundtrip; categories not on the
// list are reported (so newly-fixed ones are visible) but do not fail the build.
func TestRecompileRoundtrip(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found; skipping recompile roundtrip oracle")
	}

	classicDir := compileCorpus(t, javac, "corpus/classic", "8")
	groups := listClassGroups(classicDir)

	// Categories proven to survive the recompile roundtrip today. Adding a category
	// here turns it into a hard correctness gate. This list grows as bugs are fixed.
	mustRecompile := map[string]bool{}
	for _, n := range recompileGateBaseline() {
		mustRecompile[n] = true
	}

	names := make([]string, 0, len(groups))
	for n := range groups {
		names = append(names, n)
	}
	sort.Strings(names)

	var results []rcResult
	for _, name := range names {
		files := groups[name]
		if len(files) != 1 {
			results = append(results, rcResult{name, rcMulti, fmt.Sprintf("%d classes", len(files))})
			continue
		}
		raw, rErr := os.ReadFile(files[0])
		if rErr != nil {
			continue
		}
		out, dErr := safeDecompileHarness(raw)
		if dErr != nil {
			results = append(results, rcResult{name, rcDecErr, firstLine(dErr.Error())})
			continue
		}
		if strings.Contains(out, javaclassparser.DecompileStubMarker) {
			results = append(results, rcResult{name, rcStub, stubReason(out)})
			continue
		}
		stderr, ok := recompileDecompiled(t, javac, name, out)
		if ok {
			results = append(results, rcResult{name, rcOK, ""})
		} else {
			results = append(results, rcResult{name, rcFail, firstJavacError(stderr)})
			if os.Getenv("RC_VERBOSE") != "" {
				t.Logf("\n######## RECOMPILE FAIL: %s\n--- decompiled ---\n%s\n--- javac ---\n%s", name, out, stderr)
			}
		}
	}

	// Render report.
	var sb strings.Builder
	counts := map[string]int{}
	sb.WriteString("==== RECOMPILE ROUNDTRIP (classic, single-class) ====\n")
	for _, r := range results {
		counts[r.status]++
		line := fmt.Sprintf("  %-16s %-20s", r.status, r.group)
		if r.detail != "" {
			line += " | " + r.detail
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString(fmt.Sprintf("  -- totals: ok=%d fail=%d stub=%d dec-err=%d multiclass=%d\n",
		counts[rcOK], counts[rcFail], counts[rcStub], counts[rcDecErr], counts[rcMulti]))
	report := sb.String()
	t.Log("\n" + report)
	if outPath := os.Getenv("RC_OUT"); outPath != "" {
		_ = os.WriteFile(outPath, []byte(report), 0o644)
	}

	// Gate: every category on the baseline allow-list must still recompile.
	byName := map[string]rcResult{}
	for _, r := range results {
		byName[r.group] = r
	}
	for name := range mustRecompile {
		r, ok := byName[name]
		if !ok {
			t.Errorf("gated category %q missing from corpus", name)
			continue
		}
		if r.status != rcOK {
			t.Errorf("gated category %q regressed: status=%s detail=%s", name, r.status, r.detail)
		}
	}
}

// firstJavacError extracts the first error line from javac stderr (locale-agnostic).
func firstJavacError(stderr string) string {
	for _, ln := range strings.Split(stderr, "\n") {
		if strings.Contains(ln, "error:") || strings.Contains(ln, "错误:") {
			return firstLine(strings.TrimSpace(ln))
		}
	}
	return firstLine(stderr)
}

// recompileGateBaseline lists the categories that currently survive the recompile
// roundtrip and therefore act as hard regression gates. Populated empirically; this
// list grows as decompiler correctness bugs are fixed.
//
// CastsInstanceof joined the gate once the checkcast precedence bug was fixed
// (((T)x).m() instead of (T)(x).m()). Categories still failing the roundtrip and
// tracked for follow-up: Arrays (multi-dim allocation), Concurrency (synchronized
// monitor temp), Generics/Lambdas (loop-var scope, lambda param naming), Initializers
// (field type inference), Literals (long/boolean boxing literal suffix), Loops
// (do/while lowering emits javac-unreachable code), Operators (boolean-on-int ops),
// TryWithResources (resource var scope).
func recompileGateBaseline() []string {
	return []string{
		"CastsInstanceof",
		"ControlFlow",
		"Strings",
		"Switches",
	}
}
