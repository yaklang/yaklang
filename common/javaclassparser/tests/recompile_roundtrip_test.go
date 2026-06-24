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
// Multi-class groups (nested/inner/anonymous/local classes) are decompiled per
// `.class` into separate `$`-named compilation units and recompiled together, so
// inner-class reconstruction (synthetic access$NNN bridges, this$0 captures, val$
// fields, `new Outer$Inner(...)` references) is exercised by the same oracle.
// Stubbed outputs are reported as skipped (a stub throws by design and is not
// meant to recompile).

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

// recompileUnits writes each decompiled compilation unit to <base>.java in a fresh dir and
// compiles them together, returning javac stderr (empty on success) and whether it compiled.
// Compiling the whole group together is required for inner/nested classes, which reference
// each other (Outer$Inner, synthetic access$NNN bridges, this$0 captures).
func recompileUnits(t *testing.T, javac string, units map[string]string) (string, bool) {
	t.Helper()
	dir := t.TempDir()
	files := make([]string, 0, len(units))
	for base, src := range units {
		jf := filepath.Join(dir, base+".java")
		if err := os.WriteFile(jf, []byte(src), 0o644); err != nil {
			t.Fatalf("write decompiled java: %v", err)
		}
		files = append(files, jf)
	}
	outDir := filepath.Join(dir, "out")
	_ = os.MkdirAll(outDir, 0o755)
	args := append([]string{"-J-Duser.language=en", "-J-Duser.country=US",
		"-nowarn", "-Xlint:none", "--release", rcRelease, "-d", outDir}, files...)
	cmd := exec.Command(javac, args...)
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
		// Decompile every class of the group. Nested/inner/anonymous/local classes are
		// emitted as separate `$`-named compilation units; compiling them together is the
		// real round-trip oracle for inner-class reconstruction (synthetic accessors,
		// this$0 captures, val$ fields, etc.). Single-class groups have exactly one unit.
		units := map[string]string{}
		var decErr error
		var stubbed bool
		var stubDetail string
		var combined strings.Builder
		for _, f := range files {
			raw, rErr := os.ReadFile(f)
			if rErr != nil {
				decErr = rErr
				break
			}
			out, dErr := safeDecompileHarness(raw)
			if dErr != nil {
				decErr = dErr
				break
			}
			if strings.Contains(out, javaclassparser.DecompileStubMarker) {
				stubbed = true
				stubDetail = stubReason(out)
			}
			base := strings.TrimSuffix(filepath.Base(f), ".class")
			units[base] = out
			combined.WriteString("\n// ===== " + base + " =====\n" + out + "\n")
		}
		if decErr != nil {
			results = append(results, rcResult{name, rcDecErr, firstLine(decErr.Error())})
			continue
		}
		if stubbed {
			results = append(results, rcResult{name, rcStub, stubDetail})
			continue
		}
		detail := ""
		if len(files) > 1 {
			detail = fmt.Sprintf("%d classes", len(files))
		}
		stderr, ok := recompileUnits(t, javac, units)
		if ok {
			results = append(results, rcResult{name, rcOK, detail})
		} else {
			results = append(results, rcResult{name, rcFail, firstJavacError(stderr)})
			if os.Getenv("RC_VERBOSE") != "" {
				t.Logf("\n######## RECOMPILE FAIL: %s\n--- decompiled ---\n%s\n--- javac ---\n%s", name, combined.String(), stderr)
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
// The gate has grown as correctness bugs were fixed: multianewarray dimensions (Arrays),
// synchronized monitor temp (Concurrency), field-init type inference (Initializers),
// boolean-on-int operators and literal suffixes (Literals), scope-aware local renaming for
// nested catch parameters (TryWithResources), and full inner/nested-class reconstruction:
// synthetic accessors and this$0/val$ captures (InnerClasses), interface default methods
// (Inheritance), @interface annotation types (Annotations), and enum synthetic suppression
// with constant arguments (Enums).
//
// Categories still failing the roundtrip and tracked for follow-up: Generics (slot split by
// type widening leaves a block-scoped var read out of scope), Lambdas (captured-variable
// naming), Loops (do/while lowering emits javac-unreachable code), Operators (short-circuit
// boolean recovery). Exceptions remains a stub (try/catch/finally CFG with multiple
// successors).
func recompileGateBaseline() []string {
	return []string{
		"Annotations",
		"Arrays",
		"CastsInstanceof",
		"Concurrency",
		"ControlFlow",
		"Enums",
		"Inheritance",
		"Initializers",
		"InnerClasses",
		"Literals",
		"Strings",
		"Switches",
		"TryWithResources",
	}
}
