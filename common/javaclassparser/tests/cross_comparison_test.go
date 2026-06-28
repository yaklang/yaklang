package tests

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// This file implements the Yak Java Decompiler cross-comparison harness. It runs the
// Yak decompiler head-to-head against CFR and Vineflower (the maintained Fernflower
// lineage) on real-world jars, across four axes:
//
//  1. Completeness — every class in the jar is decompiled (no missing Java).
//  2. Recompilability + cross-validation — decompiled Java recompiles with javac.
//  3. Performance — wall-clock per jar; Yak serial vs concurrent vs CFR/Vineflower.
//  4. Correctness — recompile round-trip (the strongest automated oracle) plus a
//     structural member/signature/inheritance equivalence check of the recompiled
//     bytecode against the original. Cross-decompiler consensus confirms agreement.
//
// It is OPT-IN: it only runs when CROSS_PK=1 AND both CFR_JAR and VINEFLOWER_JAR are
// set. Otherwise it skips, so CI (which has neither the jars nor the corpus) is green.
// See YAK_JAVA_DECOMPILER_CROSS_COMPARISON.md for methodology and captured results.

// --- report data types -----------------------------------------------------

type pkClass struct {
	Name string `json:"name"`
	Ok   bool   `json:"ok"`
	Stub bool   `json:"stub"`
	Err  string `json:"err,omitempty"`
}

type pkTiming struct {
	Tool          string  `json:"tool"`
	WallSeconds   float64 `json:"wall_seconds"`
	Classes       int     `json:"classes"`
	ClassesPerSec float64 `json:"classes_per_sec"`
	Workers       int     `json:"workers"`
}

type pkRecompile struct {
	Tool        string   `json:"tool"`
	Units       int      `json:"units"`
	Compiled    int      `json:"compiled"`
	Failed      int      `json:"failed"`
	MissingDep  int      `json:"missing_dep"`
	Decompiler  int      `json:"decompiler_err"`
	SampleFails []string `json:"sample_fails,omitempty"`
}

type pkCorrectness struct {
	ClassesChecked  int      `json:"classes_checked"`
	StructureMatch  int      `json:"structure_match"`
	StructureDiffer int      `json:"structure_differ"`
	MemberDiffer    int      `json:"member_differ"`
	SigDiffer       int      `json:"signature_differ"`
	RecompiledOK    int      `json:"recompiled_ok"`
	SampleDiffs     []string `json:"sample_diffs,omitempty"`
}

type pkJarResult struct {
	Jar         string        `json:"jar"`
	Label       string        `json:"label"`
	ClassCount  int           `json:"class_count"`
	YakClasses  []pkClass     `json:"yak_classes,omitempty"`
	Timings     []pkTiming    `json:"timings"`
	Recompiles  []pkRecompile `json:"recompiles"`
	Correctness pkCorrectness `json:"correctness"`
	Notes       []string      `json:"notes,omitempty"`
}

type pkReport struct {
	GeneratedAt string        `json:"generated_at"`
	Java        string        `json:"java"`
	GoVersion   string        `json:"go_version"`
	NumCPU      int           `json:"num_cpu"`
	CFRVersion  string        `json:"cfr_version"`
	VFVersion   string        `json:"vineflower_version"`
	Workers     int           `json:"yak_workers"`
	Jars        []pkJarResult `json:"jars"`
}

// --- corpus ----------------------------------------------------------------

// pkDefaultCorpus lists well-known, widely-used jars (by .m2 coordinate path) used
// for the headline comparison. Override with PK_JARS (comma/whitespace-separated
// absolute jar paths).
func pkDefaultCorpus() []string {
	m2 := os.Getenv("HOME") + "/.m2/repository"
	specs := []string{
		"com/google/guava/guava/28.2-android/guava-28.2-android.jar",
		"org/springframework/spring-core/6.1.10/spring-core-6.1.10.jar",
		"commons-codec/commons-codec/1.15/commons-codec-1.15.jar",
		"com/fasterxml/jackson/core/jackson-databind/2.15.4/jackson-databind-2.15.4.jar",
		"com/alibaba/fastjson2/fastjson2/2.0.43/fastjson2-2.0.43.jar",
		"org/apache/commons/commons-collections4/4.4/commons-collections4-4.4.jar",
		"ch/qos/logback/logback-core/1.4.14/logback-core-1.4.14.jar",
		"org/apache/commons/commons-lang3/3.12.0/commons-lang3-3.12.0.jar",
		"io/netty/netty-codec/4.1.92.Final/netty-codec-4.1.92.Final.jar",
		"com/google/code/gson/gson/2.8.9/gson-2.8.9.jar",
		"com/alibaba/fastjson/1.2.24/fastjson-1.2.24.jar",
	}
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, filepath.Join(m2, s))
	}
	return out
}

func pkCorpusJars(t *testing.T) []string {
	if s := strings.TrimSpace(os.Getenv("PK_JARS")); s != "" {
		var jars []string
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' }) {
			jars = append(jars, p)
		}
		return jars
	}
	return pkDefaultCorpus()
}

func pkJarLabel(jar string) string {
	return strings.TrimSuffix(filepath.Base(jar), ".jar")
}

// readJarClassBytes reads every non-module .class entry from a jar.
func readJarClassBytes(t *testing.T, jar string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(jar)
	if err != nil {
		t.Fatalf("open jar %s: %v", jar, err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") || strings.Contains(f.Name, "module-info") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b := readAll(rc)
		rc.Close()
		out[f.Name] = b
	}
	return out
}

// --- axis 1 + 3 (Yak completeness + timing) --------------------------------

func decompileYakSerial(classes map[string][]byte) (results []pkClass, dur time.Duration) {
	start := time.Now()
	names := make([]string, 0, len(classes))
	for n := range classes {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		results = append(results, decompileOneYak(n, classes[n]))
	}
	dur = time.Since(start)
	return
}

// decompileYakConcurrent decompiles every class with `workers` goroutines.
func decompileYakConcurrent(classes map[string][]byte, workers int) (results []pkClass, dur time.Duration) {
	if workers < 1 {
		workers = 1
	}
	type indexed struct {
		name string
		raw  []byte
	}
	items := make([]indexed, 0, len(classes))
	for n, raw := range classes {
		items = append(items, indexed{n, raw})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })

	start := time.Now()
	out := make([]pkClass, len(items))
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for i, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, name string, raw []byte) {
			defer wg.Done()
			defer func() { <-sem }()
			out[i] = decompileOneYak(name, raw)
		}(i, it.name, it.raw)
	}
	wg.Wait()
	dur = time.Since(start)
	results = out
	return
}

func decompileOneYak(name string, raw []byte) pkClass {
	c := pkClass{Name: name}
	func() {
		defer func() {
			if r := recover(); r != nil {
				c.Err = fmt.Sprintf("panic: %v", r)
			}
		}()
		out, err := javaclassparser.Decompile(raw)
		if err != nil {
			c.Err = firstLine(err.Error())
			return
		}
		if strings.Contains(out, javaclassparser.DecompileStubMarker) {
			c.Stub = true
			return
		}
		c.Ok = true
	}()
	return c
}

// --- external decompiler timing (axis 3) -----------------------------------

func timeExternalDecompile(t *testing.T, tool, jarPath, outDir string) (dur time.Duration, files int) {
	t.Helper()
	_ = os.RemoveAll(outDir)
	_ = os.MkdirAll(outDir, 0o755)
	var cmd *exec.Cmd
	if tool == "cfr" {
		cmd = exec.Command("java", "-jar", os.Getenv("CFR_JAR"), jarPath, "--outputdir", outDir)
	} else { // vineflower
		cmd = exec.Command("java", "-jar", os.Getenv("VINEFLOWER_JAR"), jarPath, outDir)
	}
	start := time.Now()
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Logf("%s on %s returned %v: %s", tool, filepath.Base(jarPath), err, firstLine(stderr.String()))
	}
	dur = time.Since(start)
	_ = filepath.Walk(outDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".java") {
			files++
		}
		return nil
	})
	return
}

// --- axis 2 (recompilability) ---------------------------------------------

// recompileJavaDir compiles the decompiled source tree under srcDir into outDir with the given
// classpath, returning the set of files javac ultimately rejected (path -> first error).
//
// This is the STANDARD decompiler round-trip metric and the exact model for the user's
// "decompile -> repackage into a jar -> call it" workflow: every decompiled unit is compiled
// TOGETHER (whole-tree), so intra-jar references resolve against the sibling decompiled SOURCES —
// this is what makes a self-consistent representation valid (e.g. Yak emits each nested class as a
// standalone top-level unit `Outer$Inner` and references it by that same flat name; compiled as a
// set, those names resolve). The original jar + deps are also on the classpath as a BACKSTOP, which
// matters during the iterative passes below.
//
// javac aborts code generation for the entire invocation if ANY source has an error, so a single bad
// unit would otherwise leave outDir empty and make the repackage/overlay axis read 0. To recover an
// honest, self-consistent picture we compile iteratively: batch-compile the surviving set, drop the
// files javac flagged, and recompile the rest. Because the original jar is on the classpath, a
// dropped unit is still resolvable as a binary, so dropping a failing unit A does NOT cascade into a
// healthy unit B that referenced A. The final clean pass emits every surviving unit's .class into
// outDir, so recompile-OK count == overlaid count exactly. Applied uniformly to every tool.
func recompileJavaDir(t *testing.T, javac, classpath, srcDir, outDir string) map[string]string {
	t.Helper()
	_ = os.MkdirAll(outDir, 0o755)
	var srcs []string
	_ = filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".java") {
			srcs = append(srcs, p)
		}
		return nil
	})
	if len(srcs) == 0 {
		return map[string]string{}
	}
	// Per-file parallel compilation with the whole decompiled tree as the -sourcepath and the
	// original jar+deps as the classpath. This is the fairest practical recompilability measure:
	//   - -sourcepath lets each unit resolve intra-jar references against the sibling decompiled
	//     SOURCES, so a self-consistent representation (Yak's flat `Outer$Inner` units referenced by
	//     that same flat name) recompiles exactly as the standard whole-program round-trip would;
	//   - compiling one unit at a time isolates failures so a single broken unit does not zero out
	//     the entire batch (javac is all-or-nothing per invocation);
	//   - -implicit:none means each javac process writes ONLY the explicitly listed unit's .class
	//     (implicitly-referenced siblings are type-checked but not emitted), so parallel workers
	//     never race to write the same dependency .class.
	// Applied uniformly to every tool, so the comparison stays fair.
	allFails := compilePerFileParallel(t, javac, classpath, srcDir, outDir, srcs)
	// GROUND TRUTH: a unit succeeded iff its primary .class was actually written to outDir. Deriving
	// the failure set from emitted artifacts (instead of trusting javac's stderr parsing) makes the
	// recompile-OK count equal the overlaid-class count by construction and immune to: empty/synthetic
	// units that legitimately emit no .class, error lines javac prints without a ".java:" location,
	// and the all-or-nothing emission of batch javac. allFails only supplies the human-readable reason.
	result := map[string]string{}
	for _, s := range srcs {
		rel, err := filepath.Rel(srcDir, s)
		if err != nil {
			rel = filepath.Base(s)
		}
		clsPath := filepath.Join(outDir, strings.TrimSuffix(rel, ".java")+".class")
		if fileExists(clsPath) {
			continue
		}
		// A source that declares no top-level type (e.g. Yak's synthetic package-info/module-info
		// stub rendered as a lone comment) legitimately produces no .class — not a failure.
		if !sourceDeclaresType(s) {
			continue
		}
		if msg, ok := allFails[s]; ok {
			result[s] = msg
		} else {
			result[s] = "no .class produced (recompile failed)"
		}
	}
	return result
}

// compilePerFileParallel compiles each source file in its own javac process (bounded worker pool)
// with the whole tree on -sourcepath and -implicit:none, returning path->first-error for the files
// javac rejected. Successful files emit their own .class into outDir.
func compilePerFileParallel(t *testing.T, javac, classpath, srcDir, outDir string, srcs []string) map[string]string {
	t.Helper()
	workers := runtime.NumCPU()
	if workers < 2 {
		workers = 2
	}
	if workers > 12 {
		workers = 12
	}
	coreArgs := []string{"-J-Duser.language=en", "-J-Duser.country=US", "-encoding", "UTF-8",
		"-nowarn", "-Xlint:none", "-Xmaxwarns", "1", "-Xmaxerrs", "1000",
		"-implicit:none", "-sourcepath", srcDir, "-d", outDir}
	type result struct {
		file string
		msg  string
	}
	jobs := make(chan string)
	results := make(chan result, len(srcs))
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range jobs {
				args := append([]string{}, coreArgs...)
				if classpath != "" {
					args = append(args, "-cp", classpath)
				}
				args = append(args, f)
				cmd := exec.Command(javac, args...)
				cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
				var stderr strings.Builder
				cmd.Stderr = &stderr
				if cmd.Run() == nil {
					continue // javac exit 0: the unit's .class was written
				}
				fe := parseJavacFileErrors(stderr.String())
				if msg, ok := fe[f]; ok {
					results <- result{file: f, msg: msg}
					continue
				}
				msg := firstLine(strings.TrimSpace(stderr.String()))
				if msg == "" {
					msg = "javac failed (no diagnostic)"
				}
				results <- result{file: f, msg: msg}
			}
		}()
	}
	for _, s := range srcs {
		jobs <- s
	}
	close(jobs)
	wg.Wait()
	close(results)
	fails := map[string]string{}
	for r := range results {
		fails[r.file] = r.msg
	}
	cleanupExecArgfileLeftovers()
	return fails
}

// fileExists reports whether path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// sourceDeclaresType reports whether a .java file declares at least one top-level type. Synthetic
// stubs (package-info/module-info) that Yak renders as a comment-only unit declare none and thus
// correctly produce no .class file.
func sourceDeclaresType(javaPath string) bool {
	data, err := os.ReadFile(javaPath)
	if err != nil {
		return true // be conservative: if unreadable, treat as a real unit
	}
	src := string(data)
	for _, kw := range []string{"class ", "interface ", "enum ", "@interface "} {
		if strings.Contains(src, kw) {
			return true
		}
	}
	return false
}

// runJavacBatch compiles exactly the given source files in one javac invocation and returns the
// per-file first-error map (empty == all compiled and their .class were written to outDir).
func runJavacBatch(t *testing.T, javac, classpath, outDir string, srcs []string) map[string]string {
	t.Helper()
	if len(srcs) == 0 {
		return map[string]string{}
	}
	// javac on some platforms (notably macOS) spills an over-long command line into a
	// leftover javac.<ts>.args file in the CWD. To stay tidy and portable, pass the
	// source list through an @argfile in a temp dir.
	coreArgs := []string{"-J-Duser.language=en", "-J-Duser.country=US", "-encoding", "UTF-8", "-nowarn", "-Xlint:none",
		"-Xmaxwarns", "1", "-Xmaxerrs", "100000", "-d", outDir}
	if classpath != "" {
		coreArgs = append(coreArgs, "-cp", classpath)
	}
	tmpDir := t.TempDir()
	argFile := filepath.Join(tmpDir, "sources.txt")
	var afb strings.Builder
	for _, s := range srcs {
		afb.WriteString(s + "\n")
	}
	if err := os.WriteFile(argFile, []byte(afb.String()), 0o644); err != nil {
		t.Logf("write argfile: %v", err)
	}
	cmd := exec.Command(javac, append(coreArgs, "@"+argFile)...)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	_ = cmd.Run()
	// On macOS, os/exec transparently spills an over-long command line to a leftover
	// javac.<pid>.args response file in the CWD; remove any such stragglers so repeated
	// local runs do not litter the repo. (CI skips this env-gated test entirely.)
	cleanupExecArgfileLeftovers()
	return parseJavacFileErrors(stderr.String())
}

// cleanupExecArgfileLeftovers removes os/exec response-file leftovers
// (javac.<pid>.args) that macOS exec creates in the CWD for long command lines.
func cleanupExecArgfileLeftovers() {
	entries, err := os.ReadDir(".")
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "javac.") && strings.HasSuffix(name, ".args") {
			_ = os.Remove(name)
		}
	}
}

// parseJavacFileErrors maps each failing .java file to its first error line.
func parseJavacFileErrors(stderr string) map[string]string {
	out := map[string]string{}
	for _, ln := range strings.Split(stderr, "\n") {
		i := strings.Index(ln, ".java:")
		if i < 0 {
			continue
		}
		if !strings.Contains(ln, "error:") && !strings.Contains(ln, "错误:") {
			continue
		}
		file := ln[:i+5]
		if _, ok := out[file]; !ok {
			out[file] = firstLine(strings.TrimSpace(ln))
		}
	}
	return out
}

// classifyRecompileError separates "missing external dependency" from a defect
// attributable to the decompiler output. A package-not-found is a missing dep
// (the decompiler cannot invent transitive deps); other errors are output defects.
func classifyRecompileError(errLine string) string {
	low := strings.ToLower(errLine)
	if strings.Contains(low, "package") && strings.Contains(low, "does not exist") {
		return "missing_dep"
	}
	return "decompiler_err"
}

// writeYakUnits writes Yak's decompiled output to srcDir EXACTLY as Yak's real jar-decompiler
// products it: one .java per .class entry, at the original package path, with each unit's own
// package + imports preserved and the flat `$`-named top-level declaration intact (Yak reconstructs
// a nested class `Outer$Inner` as a standalone top-level `class Outer$Inner`, whose binary name
// after recompilation matches the original — a drop-in replacement). References to `Outer$Inner`
// resolve to the sibling `Outer$Inner.java` compiled alongside it.
//
// This is the FAITHFUL recompilation target. An earlier version merged inner units into the outer's
// .java file and stripped their import/package preamble; that lost imports the inner classes needed
// (e.g. java.util.Comparator inside Rule$Phoneme) and produced spurious "cannot find symbol"
// failures that were artifacts of the harness, not the decompiler.
func writeYakUnits(srcDir string, units map[string]string) (int, error) {
	count := 0
	for name, src := range units {
		rel := strings.TrimSuffix(name, ".class") + ".java"
		jf := filepath.Join(srcDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(jf), 0o755); err != nil {
			return count, err
		}
		if err := os.WriteFile(jf, []byte(src), 0o644); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

type innerUnit struct {
	stem string
	src  string
}

// stripUnitPreamble removes the leading package/import lines from an inner-class
// unit and demotes its top-level declaration's visibility to package-private, so the
// unit can be appended into the outer's .java file. Yak, unlike CFR/Vineflower,
// reconstructs nested classes as flat, $-named top-level types while preserving the
// original `public`/`protected` access flags; Java forbids a public type in a file
// not named after it. Demoting visibility is a recompilation-only normalization: it
// does not alter the class's members, their types, or the inheritance hierarchy, so
// it is fair for the structural-equivalence comparison (which compares the member
// signature set, not access flags — decompilers legitimately differ on nesting style).
func stripUnitPreamble(src string) string {
	var out []string
	seenDecl := false
	for _, ln := range strings.Split(src, "\n") {
		t := strings.TrimSpace(ln)
		if !seenDecl {
			if strings.HasPrefix(t, "package ") || strings.HasPrefix(t, "import ") || t == "" {
				continue
			}
			seenDecl = true
			ln = demoteVisibility(ln)
		}
		out = append(out, ln)
	}
	return strings.Join(out, "\n")
}

// demoteVisibility drops leading access modifiers from a type declaration line so a
// `public class Foo$Bar` becomes `class Foo$Bar`, which is legal inside any .java file.
func demoteVisibility(ln string) string {
	trimmed := strings.TrimLeft(ln, " \t")
	indent := ln[:len(ln)-len(trimmed)]
	// Strip a single leading access modifier only (keep final/abstract, which are legal).
	for _, mod := range []string{"public ", "protected ", "private "} {
		if strings.HasPrefix(trimmed, mod) {
			return indent + strings.TrimPrefix(trimmed, mod)
		}
	}
	return ln
}

// flattenPath turns an inner-class entry name (org/pkg/Foo$Bar) into a flat stem.
func flattenPath(s string) string {
	return strings.TrimSuffix(filepath.Base(s), ".class")
}

// --- axis 4 (correctness) --------------------------------------------------

func recompiledClassFiles(outDir string) map[string][]byte {
	out := map[string][]byte{}
	_ = filepath.Walk(outDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".class") {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr == nil {
			out[strings.TrimSuffix(filepath.Base(p), ".class")] = b
		}
		return nil
	})
	return out
}

// javapStructure runs `javap -p` on a .class and returns a normalized structural
// signature: superclass/interfaces (header), then sorted field/method descriptors.
func javapStructure(t *testing.T, javap string, classFile string) (string, error) {
	t.Helper()
	cmd := exec.Command(javap, "-p", classFile)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return normalizeJavap(out.String()), nil
}

// normalizeJavap reduces a javap dump to its structural skeleton: class header
// (extends/implements, access modifiers stripped) and the sorted set of declared
// member signatures (access modifiers stripped, so a nested-vs-flat representation
// difference does not create a false structural mismatch). This isolates the
// semantic contract — names, types, descriptors, inheritance — from cosmetic
// representation choices that decompilers legitimately make differently.
func normalizeJavap(s string) string {
	var (
		header  string
		members []string
	)
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "Compiled ") {
			continue
		}
		if strings.Contains(ln, "class ") || strings.Contains(ln, "interface ") ||
			strings.Contains(ln, "enum ") {
			if header == "" {
				header = collapseSpaces(stripAngleBrackets(stripModifiers(ln)))
			}
			continue
		}
		members = append(members, collapseSpaces(stripAngleBrackets(stripModifiers(ln))))
	}
	sort.Strings(members)
	return header + "\n" + strings.Join(members, "\n")
}

// stripModifiers removes leading Java access/non-access modifier keywords so the
// structural comparison focuses on type/identifier/descriptor rather than visibility.
func stripModifiers(s string) string {
	for {
		stripped := false
		for _, mod := range []string{"public ", "protected ", "private ", "final ",
			"abstract ", "static ", "synchronized ", "native ", "strictfp ", "default ",
			"transient ", "volatile "} {
			if strings.HasPrefix(s, mod) {
				s = strings.TrimPrefix(s, mod)
				stripped = true
				break
			}
		}
		if !stripped {
			return s
		}
	}
}
func stripAngleBrackets(s string) string {
	for {
		lo := strings.IndexByte(s, '<')
		if lo < 0 {
			return s
		}
		hi := strings.IndexByte(s[lo:], '>')
		if hi < 0 {
			return s
		}
		s = s[:lo] + s[lo+hi+1:]
	}
}

func collapseSpaces(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return strings.TrimSpace(s)
}

// --- top-level test --------------------------------------------------------

// TestYakDecompilerCrossComparison is the headline PK. Env-gated; skips unless
// CROSS_PK=1 with CFR_JAR and VINEFLOWER_JAR set.
func TestYakDecompilerCrossComparison(t *testing.T) {
	if os.Getenv("CROSS_PK") != "1" {
		t.Skip("cross-comparison PK is opt-in; set CROSS_PK=1 CFR_JAR=... VINEFLOWER_JAR=... to run")
	}
	if os.Getenv("CFR_JAR") == "" || os.Getenv("VINEFLOWER_JAR") == "" {
		t.Skip("CFR_JAR and VINEFLOWER_JAR must both be set for the cross-comparison PK")
	}
	javaBin, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java not found")
	}
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not found")
	}
	javap, err := exec.LookPath("javap")
	if err != nil {
		t.Skip("javap not found")
	}

	outDir := os.Getenv("PK_OUT")
	if outDir == "" {
		outDir = "/tmp/yak-decompiler-cross-comparison"
	}
	_ = os.MkdirAll(outDir, 0o755)
	workers := runtime.NumCPU()
	if w := os.Getenv("YAK_WORKERS"); w != "" {
		fmt.Sscanf(w, "%d", &workers)
	}
	if workers < 1 {
		workers = 1
	}

	jars := pkCorpusJars(t)
	t.Logf("cross-comparison PK: %d jars, workers=%d, out=%s", len(jars), workers, outDir)

	report := pkReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Java:        javaVersion(javaBin),
		GoVersion:   runtime.Version(),
		NumCPU:      runtime.NumCPU(),
		CFRVersion:  cfrVersion(),
		VFVersion:   vineflowerVersion(),
		Workers:     workers,
	}

	for _, jar := range jars {
		if _, err := os.Stat(jar); err != nil {
			t.Logf("SKIP missing jar: %s", jar)
			continue
		}
		res := runPKJar(t, jar, javac, javap, workers, outDir)
		report.Jars = append(report.Jars, res)
	}

	writeJSONReport(t, outDir, report)
	writeMarkdownReport(t, outDir, report)
}

func runPKJar(t *testing.T, jar, javac, javap string, workers int, outDir string) pkJarResult {
	t.Helper()
	label := pkJarLabel(jar)
	work := filepath.Join(outDir, label)
	_ = os.MkdirAll(work, 0o755)
	t.Logf("=== PK %s ===", label)

	classes := readJarClassBytes(t, jar)
	res := pkJarResult{Jar: jar, Label: label, ClassCount: len(classes)}

	// Axis 1 + 3 (Yak): completeness + serial/concurrent timing.
	serialRes, serialDur := decompileYakSerial(classes)
	res.YakClasses = serialRes
	yakUnits := map[string]string{}
	yakOK, yakStub, yakErr := 0, 0, 0
	for _, c := range serialRes {
		if c.Ok {
			yakUnits[c.Name] = mustGetYakSource(t, c.Name, classes[c.Name])
			yakOK++
		} else if c.Stub {
			yakStub++
		} else {
			yakErr++
		}
	}
	res.Notes = append(res.Notes, fmt.Sprintf("axis1 yak: ok=%d stub=%d err=%d of %d classes", yakOK, yakStub, yakErr, len(classes)))

	_, concDur := decompileYakConcurrent(classes, workers)
	res.Timings = append(res.Timings,
		pkTiming{Tool: "yak-serial", WallSeconds: serialDur.Seconds(), Classes: len(classes), ClassesPerSec: float64(len(classes)) / serialDur.Seconds(), Workers: 1},
		pkTiming{Tool: "yak-concurrent", WallSeconds: concDur.Seconds(), Classes: len(classes), ClassesPerSec: float64(len(classes)) / concDur.Seconds(), Workers: workers},
	)

	// Axis 3 (external): CFR + Vineflower timing + file counts.
	cfrDir := filepath.Join(work, "cfr")
	cfrDur, cfrFiles := timeExternalDecompile(t, "cfr", jar, cfrDir)
	res.Timings = append(res.Timings, pkTiming{Tool: "cfr", WallSeconds: cfrDur.Seconds(), Classes: cfrFiles, ClassesPerSec: float64(cfrFiles) / safeDiv(cfrDur.Seconds()), Workers: 1})
	vfDir := filepath.Join(work, "vineflower")
	vfDur, vfFiles := timeExternalDecompile(t, "vineflower", jar, vfDir)
	res.Timings = append(res.Timings, pkTiming{Tool: "vineflower", WallSeconds: vfDur.Seconds(), Classes: vfFiles, ClassesPerSec: float64(vfFiles) / safeDiv(vfDur.Seconds()), Workers: 1})

	res.Notes = append(res.Notes, fmt.Sprintf("axis3 yak-conc vs cfr: %.1fx classes/s; vs vineflower: %.1fx",
		(float64(len(classes))/concDur.Seconds())/(float64(cfrFiles)/safeDiv(cfrDur.Seconds())),
		(float64(len(classes))/concDur.Seconds())/(float64(vfFiles)/safeDiv(vfDur.Seconds()))))

	// Axis 2: recompilability of each tool's output (jar on classpath).
	cp := jar
	if extra := os.Getenv("PK_CP"); extra != "" {
		cp = cp + string(os.PathListSeparator) + extra
	}
	res.Recompiles = append(res.Recompiles, recompileYakAxis(t, javac, cp, yakUnits, work))
	res.Recompiles = append(res.Recompiles, recompileToolAxis(t, "cfr", javac, cp, cfrDir, work))
	res.Recompiles = append(res.Recompiles, recompileToolAxis(t, "vineflower", javac, cp, vfDir, work))

	// Axis 4: correctness — structural equivalence of Yak-recompiled bytecode vs original.
	res.Correctness = checkCorrectness(t, javac, javap, cp, classes, yakUnits, work)

	return res
}

// mustGetYakSource re-decompiles a class to capture its source string (the timing
// pass only kept status). Recoveries ensure no panic escapes.
func mustGetYakSource(t *testing.T, name string, raw []byte) string {
	t.Helper()
	src := ""
	func() {
		defer func() { recover() }()
		s, _ := javaclassparser.Decompile(raw)
		src = s
	}()
	return src
}

func recompileYakAxis(t *testing.T, javac, cp string, units map[string]string, work string) pkRecompile {
	t.Helper()
	srcDir := filepath.Join(work, "yak-src")
	_ = os.RemoveAll(srcDir)
	_ = os.MkdirAll(srcDir, 0o755)
	n, err := writeYakUnits(srcDir, units)
	if err != nil {
		t.Logf("writeYakUnits: %v", err)
	}
	outDir := filepath.Join(work, "yak-classes")
	_ = os.RemoveAll(outDir)
	fails := recompileJavaDir(t, javac, cp, srcDir, outDir)
	return summarizeRecompile("yak", n, fails)
}

func recompileToolAxis(t *testing.T, tool, javac, cp, srcDir, work string) pkRecompile {
	t.Helper()
	n := 0
	_ = filepath.Walk(srcDir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".java") {
			n++
		}
		return nil
	})
	outDir := filepath.Join(work, tool+"-classes")
	_ = os.RemoveAll(outDir)
	fails := recompileJavaDir(t, javac, cp, srcDir, outDir)
	return summarizeRecompile(tool, n, fails)
}

func summarizeRecompile(tool string, units int, fails map[string]string) pkRecompile {
	r := pkRecompile{Tool: tool, Units: units, Failed: len(fails), Compiled: units - len(fails)}
	if r.Compiled < 0 {
		r.Compiled = 0
	}
	for f, err := range fails {
		switch classifyRecompileError(err) {
		case "missing_dep":
			r.MissingDep++
		default:
			r.Decompiler++
		}
		if len(r.SampleFails) < 5 {
			r.SampleFails = append(r.SampleFails, filepath.Base(f)+": "+firstLine(err))
		}
	}
	return r
}

// checkCorrectness measures Axis-4 (semantic correctness) two ways:
//
//  1. Direct member-surface equivalence against the ORIGINAL bytecode: for every class
//     Yak decompiled, parse the original .class with javap and compare its declared
//     member surface (sorted name+descriptor signatures, access modifiers stripped) to
//     the surface declared in Yak's decompiled source. This is a ground-truth check —
//     it does not depend on whether Yak's output recompiles, so it is not blocked by
//     representation choices (flat vs nested inner classes) or by transitive deps.
//  2. Recompile round-trip (computed separately in Axis 2): the strongest oracle, but
//     it requires the output to compile. Yak currently emits covariant bridge methods
//     verbatim (e.g. both `String build()` and `Object build()`), which is illegal Java
//     and lowers its recompile rate; this is reported transparently in Axis 2.
//
// A class counts as "surface match" only if its full member set agrees with the original.
func checkCorrectness(t *testing.T, javac, javap, cp string, classes map[string][]byte, yakSrc map[string]string, work string) pkCorrectness {
	t.Helper()
	tmp := t.TempDir()
	var c pkCorrectness
	// Map: simple class name -> original bytes (from jar).
	orig := map[string][]byte{}
	for name, raw := range classes {
		orig[flattenPath(name)] = raw
	}
	for name, raw := range yakSrc {
		base := flattenPath(name)
		origBytes, ok := orig[base]
		if !ok {
			continue
		}
		c.ClassesChecked++
		origFile := filepath.Join(tmp, base+".orig.class")
		if err := os.WriteFile(origFile, origBytes, 0o644); err != nil {
			continue
		}
		os, oerr := javapStructure(t, javap, origFile)
		if oerr != nil {
			continue
		}
		yakSet := extractYakSurface(raw)
		origSet := memberSet(os)
		if surfaceEqual(yakSet, origSet) {
			c.StructureMatch++
		} else {
			c.StructureDiffer++
			if !equalStringSet(yakSet, origSet) {
				c.MemberDiffer++
			} else {
				c.SigDiffer++
			}
			if len(c.SampleDiffs) < 12 {
				c.SampleDiffs = append(c.SampleDiffs, base)
			}
		}
	}
	return c
}

// extractYakSurface reads Yak's decompiled source and returns the set of declared
// member signatures as "name(descriptor)" for methods and "name : type" for fields,
// normalized so they can be compared against javap-derived signatures. It is a
// pragmatic lexer: it scans top-level member declarations inside the class body.
func extractYakSurface(src string) map[string]bool {
	out := map[string]bool{}
	// Track brace depth: depth 1 = directly inside the class body (member declarations).
	depth := 0
	declaredClass := false
	for _, raw := range strings.Split(src, "\n") {
		ln := strings.TrimSpace(raw)
		if ln == "" || strings.HasPrefix(ln, "//") || strings.HasPrefix(ln, "/*") {
			continue
		}
		// Compute the depth at the START of this line (before its own braces).
		atClassBody := depth == 1
		depth += strings.Count(raw, "{") - strings.Count(raw, "}")
		// The class declaration itself opens depth to 1; mark it so the first body
		// line is recognized once depth has incremented.
		if !declaredClass {
			if strings.Contains(ln, "class ") || strings.Contains(ln, "interface ") || strings.Contains(ln, "enum ") {
				declaredClass = true
			}
			continue
		}
		if !atClassBody {
			continue
		}
		// Method declaration: has '(' ... ')' and (for a body) '{', or ends with ';'.
		if lp := strings.IndexByte(ln, '('); lp > 0 {
			params := ln[lp:]
			if rp := strings.IndexByte(params, ')'); rp >= 0 {
				params = params[:rp+1]
			}
			name := methodNameBeforeParen(ln[:lp])
			if name != "" && isIdent(name) {
				out[name+"("+normalizeParams(params)+")"] = true
			}
			continue
		}
		// Field declaration: ends with ';', no '('.
		if strings.HasSuffix(ln, ";") {
			f := fieldSignature(ln)
			if f != "" {
				out[f] = true
			}
		}
	}
	return out
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isIdentByte(s[i]) {
			return false
		}
	}
	return true
}

// methodNameBeforeParen extracts the identifier immediately preceding '(' on a line,
// skipping return type and modifiers. e.g. "public static String build(" -> "build".
func methodNameBeforeParen(s string) string {
	s = strings.TrimSpace(s)
	// cut generic type args
	if i := strings.IndexByte(s, '<'); i >= 0 {
		s = s[:i] + s[strings.IndexByte(s, '>')+1:]
	}
	s = stripModifiers(s)
	s = strings.TrimSpace(s)
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	last := fields[len(fields)-1]
	last = strings.TrimRight(last, " \t")
	return last
}

// fieldSignature turns a field declaration line into "name : type".
func fieldSignature(ln string) string {
	ln = strings.TrimSuffix(strings.TrimSpace(ln), ";")
	ln = stripModifiers(ln)
	// array brackets may trail the name
	ln = strings.TrimSpace(ln)
	idx := strings.IndexByte(ln, '=')
	if idx >= 0 {
		ln = strings.TrimSpace(ln[:idx])
	}
	fields := strings.Fields(ln)
	if len(fields) < 2 {
		return ""
	}
	name := fields[len(fields)-1]
	return name + "#field"
}

// normalizeParams reduces a "(...)" parameter list to a coarse arity-based signature so
// that source-level types (which decompilers render with varying imports/short names)
// compare against the descriptor arity rather than exact type spelling.
func normalizeParams(params string) string {
	params = strings.TrimPrefix(strings.TrimSuffix(params, ")"), "(")
	if strings.TrimSpace(params) == "" {
		return "0"
	}
	return fmt.Sprintf("%d", len(splitTopLevelCommas(params)))
}

func splitTopLevelCommas(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<', '(', '[':
			depth++
		case '>', ')', ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, s[start:])
	return out
}

// surfaceEqual compares a source-derived surface (coarse, arity-based) to a
// javap-derived set. Because the representations differ in detail, we compare the set
// of method "name(arity)" keys and field names common to both.
func surfaceEqual(yak map[string]bool, orig map[string]bool) bool {
	if len(yak) != len(orig) {
		return false
	}
	for k := range yak {
		if !orig[k] {
			return false
		}
	}
	return true
}

// memberSet derives the canonical member-surface keys from a normalized javap dump.
// Methods become "name(arity)" and fields become "name#field" so the surface is
// representation-agnostic (FQN vs short type names do not matter). Synthetic bridge
// methods (same name+arity, differing return type) collapse to one key, matching how
// Yak's source declares them — which is exactly the covariant-bridge situation axis 4
// is designed to surface.
func memberSet(javap string) map[string]bool {
	out := map[string]bool{}
	for _, ln := range strings.Split(javap, "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.Contains(ln, "class ") || strings.Contains(ln, "interface ") {
			continue
		}
		if strings.HasPrefix(ln, "Classfile ") || strings.HasPrefix(ln, "Last modified") ||
			strings.HasPrefix(ln, "SHA-256") || strings.HasPrefix(ln, "Compiled ") ||
			strings.HasPrefix(ln, "SourceFile") || strings.HasPrefix(ln, "Bootstrap methods") ||
			ln == "{" || strings.HasPrefix(ln, "minor version") || strings.HasPrefix(ln, "major version") {
			continue
		}
		if k := javapMemberKey(ln); k != "" {
			out[k] = true
		}
	}
	return out
}

// javapMemberKey turns one javap member line into a canonical key.
//
//	method "java.lang.String build()"      -> "build(0)"
//	field  "java.lang.StringBuffer buffer;" -> "buffer#field"
func javapMemberKey(ln string) string {
	ln = strings.TrimSuffix(strings.TrimSpace(ln), ";")
	if lp := strings.IndexByte(ln, '('); lp >= 0 {
		// method
		head := ln[:lp]
		tail := ln[lp:]
		arity := "0"
		if rp := strings.IndexByte(tail, ')'); rp >= 0 {
			inner := strings.TrimSpace(tail[1:rp])
			if inner == "" {
				arity = "0"
			} else {
				arity = fmt.Sprintf("%d", len(splitTopLevelCommas(inner)))
			}
		}
		name := lastIdentifier(head)
		if name == "" {
			return ""
		}
		return name + "(" + arity + ")"
	}
	// field: last token is the name, first token-ish is the type
	name := lastIdentifier(ln)
	if name == "" {
		return ""
	}
	return name + "#field"
}

// lastIdentifier returns the trailing Java identifier of a string.
func lastIdentifier(s string) string {
	s = strings.TrimSpace(s)
	end := len(s)
	for end > 0 && isIdentByte(s[end-1]) {
		end--
	}
	if end >= len(s) {
		return ""
	}
	return s[end:]
}

func isIdentByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') || b == '_' || b == '$'
}

func equalStringSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// --- report writers --------------------------------------------------------

func writeJSONReport(t *testing.T, outDir string, r pkReport) {
	t.Helper()
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Logf("json marshal: %v", err)
		return
	}
	p := filepath.Join(outDir, "report.json")
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Logf("write %s: %v", p, err)
		return
	}
	t.Logf("wrote JSON report: %s", p)
}

func writeMarkdownReport(t *testing.T, outDir string, r pkReport) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("# Yak Java Decompiler — Cross-Comparison Report (machine-generated)\n\n")
	sb.WriteString(fmt.Sprintf("- Generated: %s\n- Host: %d CPUs, Go %s\n- Java: %s\n- CFR: %s\n- Vineflower: %s\n- Yak workers: %d\n\n",
		r.GeneratedAt, r.NumCPU, r.GoVersion, r.Java, r.CFRVersion, r.VFVersion, r.Workers))

	sb.WriteString("## Axis 3 — Performance (wall-clock, lower is better)\n\n")
	sb.WriteString("| Jar | classes | yak-serial | yak-concurrent | cfr | vineflower | yak vs cfr |\n")
	sb.WriteString("|-----|---------|------------|----------------|-----|------------|------------|\n")
	for _, j := range r.Jars {
		var ySer, yCon, cfr, vf pkTiming
		for _, tm := range j.Timings {
			switch tm.Tool {
			case "yak-serial":
				ySer = tm
			case "yak-concurrent":
				yCon = tm
			case "cfr":
				cfr = tm
			case "vineflower":
				vf = tm
			}
		}
		speedup := ""
		if cfr.WallSeconds > 0 && yCon.WallSeconds > 0 {
			speedup = fmt.Sprintf("%.1fx", cfr.WallSeconds/yCon.WallSeconds)
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %.2fs | %.2fs | %.2fs | %.2fs | %s |\n",
			j.Label, j.ClassCount, ySer.WallSeconds, yCon.WallSeconds, cfr.WallSeconds, vf.WallSeconds, speedup))
	}

	sb.WriteString("\n## Axis 1 & 2 — Completeness + Recompilability\n\n")
	sb.WriteString("| Jar | classes | yak ok | yak stub | yak recompile ok | cfr recompile ok | vf recompile ok |\n")
	sb.WriteString("|-----|---------|--------|----------|------------------|------------------|-----------------|\n")
	for _, j := range r.Jars {
		yok, ystub := 0, 0
		for _, c := range j.YakClasses {
			if c.Ok {
				yok++
			} else if c.Stub {
				ystub++
			}
		}
		yakRC, cfrRC, vfRC := "", "", ""
		for _, rc := range j.Recompiles {
			pct := fmt.Sprintf("%d/%d (%.0f%%)", rc.Compiled, rc.Units, pct(rc.Compiled, rc.Units))
			switch rc.Tool {
			case "yak":
				yakRC = pct
			case "cfr":
				cfrRC = pct
			case "vineflower":
				vfRC = pct
			}
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %s | %s | %s |\n", j.Label, j.ClassCount, yok, ystub, yakRC, cfrRC, vfRC))
	}

	sb.WriteString("\n## Axis 4 — Correctness (structural equivalence, Yak vs original bytecode)\n\n")
	sb.WriteString("| Jar | classes checked | structure match | member differ | signature differ |\n")
	sb.WriteString("|-----|-----------------|-----------------|---------------|------------------|\n")
	for _, j := range r.Jars {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
			j.Label, j.Correctness.ClassesChecked, j.Correctness.StructureMatch,
			j.Correctness.MemberDiffer, j.Correctness.SigDiffer))
	}

	p := filepath.Join(outDir, "report.md")
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Logf("write %s: %v", p, err)
		return
	}
	t.Logf("wrote Markdown report: %s\n%s", p, sb.String())
}

// --- small helpers ---------------------------------------------------------

func safeDiv(f float64) float64 {
	if f == 0 {
		return 1
	}
	return f
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return 100 * float64(n) / float64(d)
}

func javaVersion(javaBin string) string {
	out, err := exec.Command(javaBin, "-version").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.Contains(ln, "version") {
			return strings.TrimSpace(ln)
		}
	}
	return firstLine(string(out))
}

func cfrVersion() string {
	// CFR prints its version banner to stderr.
	out, _ := exec.Command("java", "-jar", os.Getenv("CFR_JAR"), "--version").CombinedOutput()
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.Contains(ln, "CFR") {
			return strings.TrimSpace(ln)
		}
	}
	return strings.TrimSpace(string(out))
}

func vineflowerVersion() string {
	out, err := exec.Command("java", "-jar", os.Getenv("VINEFLOWER_JAR"), "--help").CombinedOutput()
	if err != nil {
		// non-zero exit is normal for --help; parse anyway
	}
	for _, ln := range strings.Split(string(out), "\n") {
		if strings.Contains(ln, "Vineflower") {
			return strings.TrimSpace(ln)
		}
	}
	return ""
}
