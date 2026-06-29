package tests

// Gated GA jar-recompile measurement harness (set SCRATCH=1 to run; skips by default so it never runs
// in CI). It decompiles a real-world jar through the PRODUCTION JarFS path (enum constant-body folding,
// $N suppression), writes each emitted unit to its proper package directory, recompiles the whole tree
// with javac (the library's transitive deps on the classpath via jarDeps so annotation/dep symbols are
// not counted as decompiler errors), and reports either an error-category histogram (TestScratchProfile
// / TestScratchSymbolDrill), a single decompiled unit (TestScratchDumpClass), or a per-jar
// before/after error delta for a given KILL_SWITCH (TestScratchJarErrDelta). Used to find the highest-
// leverage decompiler defects and to verify a fix's whole-jar impact without regression. Jar paths are
// local ~/.m2 coordinates; missing jars are skipped.

import (
	"archive/zip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

var jarPaths = map[string]string{
	"codec":     "/Users/v1ll4n/.m2/repository/commons-codec/commons-codec/1.15/commons-codec-1.15.jar",
	"guava":     "/Users/v1ll4n/.m2/repository/com/google/guava/guava/28.2-android/guava-28.2-android.jar",
	"fastjson2": "/Users/v1ll4n/.m2/repository/com/alibaba/fastjson2/fastjson2/2.0.43/fastjson2-2.0.43.jar",
	"spring":    "/Users/v1ll4n/.m2/repository/org/springframework/spring-core/5.3.27/spring-core-5.3.27.jar",
}

const m2 = "/Users/v1ll4n/.m2/repository/"

// jarDeps lists the transitive compile-time dependency jars per library so javac sees the same symbols
// the original was compiled against. Without these, annotation types (@Beta, @DoNotMock, ...) and
// transitive packages show up as spurious "cannot find symbol" / "package does not exist" noise that
// is NOT a decompiler defect (CFR/Vineflower output would fail identically).
var jarDeps = map[string][]string{
	"guava": {
		m2 + "com/google/errorprone/error_prone_annotations/2.3.4/error_prone_annotations-2.3.4.jar",
		m2 + "org/checkerframework/checker-compat-qual/2.5.5/checker-compat-qual-2.5.5.jar",
		m2 + "com/google/code/findbugs/jsr305/3.0.2/jsr305-3.0.2.jar",
		m2 + "com/google/guava/failureaccess/1.0.1/failureaccess-1.0.1.jar",
		m2 + "com/google/j2objc/j2objc-annotations/1.3/j2objc-annotations-1.3.jar",
	},
}

func scratchGate(t *testing.T) {
	if os.Getenv("SCRATCH") == "" {
		t.Skip("scratch measurement test; set SCRATCH=1 to run")
	}
}

func classpathFor(name, jarPath string) string {
	cp := jarPath
	for _, d := range jarDeps[name] {
		if _, err := os.Stat(d); err == nil {
			cp += string(os.PathListSeparator) + d
		}
	}
	return cp
}

// decompileJarUnits uses the PRODUCTION JarFS path (enum folding, $N suppression) to decompile every
// .class in the jar, writing one .java per emitted unit into a flat dir (matching the cross-comparison
// report's "each flat inner class = 1 unit" convention). Returns dir, written file list, #units, #decErr.
func decompileJarUnits(t *testing.T, jarPath string) (string, []string, int, int) {
	fsys, err := javaclassparser.NewJarFSFromLocal(jarPath)
	if err != nil {
		t.Fatalf("open jarfs: %v", err)
	}
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	defer zr.Close()
	dir := t.TempDir()
	var files []string
	units, decErr := 0, 0
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") {
			continue
		}
		base := filepath.Base(f.Name)
		if base == "module-info.class" || base == "package-info.class" {
			continue
		}
		raw, e := fsys.ReadFile(f.Name)
		if e != nil || len(raw) == 0 {
			continue
		}
		src := string(raw)
		trimmed := strings.TrimSpace(src)
		// Suppression markers / decompile failures begin with "//" and have no real type decl.
		if strings.HasPrefix(trimmed, "//") && !strings.Contains(src, "class ") &&
			!strings.Contains(src, "interface ") && !strings.Contains(src, "enum ") {
			continue
		}
		if strings.Contains(src, javaclassparser.DecompileStubMarker) ||
			strings.HasPrefix(trimmed, "// decompile") {
			decErr++
			continue
		}
		units++
		// Write to the proper package directory with the simple (last-segment) class name so
		// javac's "public class X should be in X.java" check is satisfied (removes harness noise).
		rel := strings.TrimSuffix(f.Name, ".class") // e.g. com/google/common/base/Ascii
		pkgDir := filepath.Join(dir, filepath.Dir(rel))
		os.MkdirAll(pkgDir, 0o755)
		jf := filepath.Join(pkgDir, filepath.Base(rel)+".java")
		os.WriteFile(jf, []byte(src), 0o644)
		files = append(files, jf)
	}
	return dir, files, units, decErr
}

func compileUnits(dir string, files []string, cp string) string {
	outDir := filepath.Join(dir, "out")
	os.MkdirAll(outDir, 0o755)
	args := append([]string{"-J-Duser.language=en", "-nowarn", "-Xlint:none", "-proc:none",
		"-Xmaxerrs", "100000", "--release", "8", "-cp", cp, "-d", outDir}, files...)
	cmd := exec.Command("javac", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	_ = cmd.Run()
	return stderr.String()
}

// normalizeJavacErr strips identifiers/numbers so similar errors group together.
func normalizeJavacErr(msg string) string {
	reSym := regexp.MustCompile(`'[^']*'`)
	msg = reSym.ReplaceAllString(msg, "'X'")
	reType := regexp.MustCompile(`\b[A-Z][A-Za-z0-9_$.<>]+\b`)
	msg = reType.ReplaceAllString(msg, "T")
	reNum := regexp.MustCompile(`\b\d+\b`)
	msg = reNum.ReplaceAllString(msg, "N")
	reVar := regexp.MustCompile(`\bvar[A-Za-z0-9_]+\b`)
	msg = reVar.ReplaceAllString(msg, "v")
	return strings.TrimSpace(msg)
}

func TestScratchProfile(t *testing.T) {
	scratchGate(t)
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		target = "guava"
	}
	jar := jarPaths[target]
	dir, files, units, decErr := decompileJarUnits(t, jar)
	stderr := compileUnits(dir, files, classpathFor(target, jar))

	cat := map[string]int{}
	samples := map[string][]string{}
	reLine := regexp.MustCompile(`error:\s*(.*)$`)
	total := 0
	for _, ln := range strings.Split(stderr, "\n") {
		m := reLine.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		key := normalizeJavacErr(strings.TrimSpace(m[1]))
		cat[key]++
		total++
		if len(samples[key]) < 3 {
			samples[key] = append(samples[key], strings.TrimSpace(filepath.Base(ln)))
		}
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range cat {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	fmt.Printf("==== %s units=%d decErr=%d totalJavacErr=%d distinctCats=%d ====\n",
		target, units, decErr, total, len(list))
	for i, e := range list {
		if i >= 25 {
			break
		}
		fmt.Printf("[%4d] %s\n", e.v, e.k)
		for _, s := range samples[e.k] {
			fmt.Printf("        e.g. %s\n", s)
		}
	}
}

// TestScratchSymbolDrill drills into "cannot find symbol" by tallying the following `symbol:` line.
func TestScratchSymbolDrill(t *testing.T) {
	scratchGate(t)
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		target = "guava"
	}
	jar := jarPaths[target]
	dir, files, units, decErr := decompileJarUnits(t, jar)
	stderr := compileUnits(dir, files, classpathFor(target, jar))
	os.WriteFile("/tmp/javac_"+target+".txt", []byte(stderr), 0o644)

	lines := strings.Split(stderr, "\n")
	symCat := map[string]int{}
	symSample := map[string][]string{}
	reSymbol := regexp.MustCompile(`^\s*symbol:\s*(.*)$`)
	reCFS := regexp.MustCompile(`error: cannot find symbol`)
	cfsTotal := 0
	for i, ln := range lines {
		if !reCFS.MatchString(ln) {
			continue
		}
		cfsTotal++
		// find the next symbol: line within a few lines
		for j := i + 1; j < len(lines) && j < i+4; j++ {
			if m := reSymbol.FindStringSubmatch(lines[j]); m != nil {
				detail := strings.TrimSpace(m[1])
				// normalize: drop the specific identifier, keep kind + shape
				key := regexp.MustCompile(`\b[A-Za-z_$][A-Za-z0-9_$]*\b`).ReplaceAllStringFunc(detail, func(s string) string {
					switch s {
					case "class", "variable", "method", "interface", "enum":
						return s
					}
					return "X"
				})
				symCat[key]++
				if len(symSample[key]) < 4 {
					symSample[key] = append(symSample[key], detail)
				}
				break
			}
		}
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range symCat {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })
	fmt.Printf("==== %s units=%d decErr=%d cannot-find-symbol=%d distinct=%d ====\n",
		target, units, decErr, cfsTotal, len(list))
	for i, e := range list {
		if i >= 30 {
			break
		}
		fmt.Printf("[%4d] %s\n", e.v, e.k)
		for _, s := range symSample[e.k] {
			fmt.Printf("        e.g. %s\n", s)
		}
	}
}

// TestScratchDumpClass dumps the production-path decompiled source for one .class (DUMP_CLASS substring).
func TestScratchDumpClass(t *testing.T) {
	scratchGate(t)
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		target = "fastjson2"
	}
	want := os.Getenv("DUMP_CLASS")
	if want == "" {
		t.Skip("set DUMP_CLASS")
	}
	jar := jarPaths[target]
	fsys, err := javaclassparser.NewJarFSFromLocal(jar)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(jar)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") || !strings.Contains(f.Name, want) {
			continue
		}
		raw, e := fsys.ReadFile(f.Name)
		if e != nil {
			continue
		}
		out := "/tmp/dump_" + strings.ReplaceAll(strings.TrimSuffix(filepath.Base(f.Name), ".class"), "$", "_") + ".java"
		os.WriteFile(out, raw, 0o644)
		fmt.Printf("WROTE %s (%d bytes) from %s\n", out, len(raw), f.Name)
	}
}

// TestScratchPerFileIso compiles every decompiled unit INDIVIDUALLY against the original jar (+deps),
// so each file's defects are counted independently of the others. It exists to defeat the whole-tree
// compile's MASKING confound (TestScratchProfile / TestScratchJarErrDelta): javac aborts a file's
// attribution after certain errors, so a file with hundreds of genuine defects can report as a single
// error, and a fix that removes one early error then "increases" the visible jar total by un-masking
// the rest (observed: the lambda-local rename took fastjson2's whole-tree total 492->852 while the
// per-file picture was flat). CAVEAT: the ABSOLUTE rate here is PESSIMISTIC because the production
// JarFS path emits inner classes as flat `Outer$Inner` units that cannot compile in isolation (they
// need the enclosing-instance context), so every inner unit is counted as a failure regardless of
// decompiler quality (e.g. spring shows ~14 genuine whole-tree errors but ~365 ISO "failures"). The
// flat-inner confound is CONSTANT across a fix's on/off, so the ON-vs-OFF DELTA is reliable even though
// the absolute is not — use this test to measure a fix's true per-file impact via KILL_SWITCH, not to
// quote an absolute recompile rate. Set PROFILE_JAR (default fastjson2); the optional KILL_SWITCH env
// var is applied during decompile. Concurrent (one javac per unit, NumCPU workers).
func TestScratchPerFileIso(t *testing.T) {
	scratchGate(t)
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		target = "fastjson2"
	}
	jar := jarPaths[target]
	if _, err := os.Stat(jar); err != nil {
		t.Skipf("jar missing: %s", jar)
	}
	if ks := os.Getenv("KILL_SWITCH"); ks != "" {
		os.Setenv(ks, "1")
		defer os.Unsetenv(ks)
	}
	dir, files, units, decErr := decompileJarUnits(t, jar)
	cp := classpathFor(target, jar)

	workers := runtime.NumCPU()
	jobs := make(chan string)
	var clean, failed int64
	var wg sync.WaitGroup
	outBase := filepath.Join(dir, "iso_out")
	os.MkdirAll(outBase, 0o755)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			outDir := filepath.Join(outBase, fmt.Sprintf("w%d", id))
			os.MkdirAll(outDir, 0o755)
			for f := range jobs {
				args := []string{"-J-Duser.language=en", "-nowarn", "-Xlint:none", "-proc:none",
					"-Xmaxerrs", "100000", "--release", "8", "-cp", cp, "-d", outDir, f}
				cmd := exec.Command("javac", args...)
				var stderr strings.Builder
				cmd.Stderr = &stderr
				_ = cmd.Run()
				if strings.Contains(stderr.String(), "error:") {
					atomic.AddInt64(&failed, 1)
				} else {
					atomic.AddInt64(&clean, 1)
				}
			}
		}(w)
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	total := clean + failed
	pct := 0.0
	if total > 0 {
		pct = 100.0 * float64(clean) / float64(total)
	}
	fmt.Printf("==== PER-FILE ISO %s units=%d decErr=%d  cleanCompile=%d failCompile=%d  recompRate=%.1f%% (KILL_SWITCH=%q) ====\n",
		target, units, decErr, clean, failed, pct, os.Getenv("KILL_SWITCH"))
}

func TestScratchJarErrDelta(t *testing.T) {
	scratchGate(t)
	ks := os.Getenv("KILL_SWITCH")
	if ks == "" {
		ks = "JDEC_REF_SLOT_PHI_MERGE_OFF"
	}
	for _, name := range []string{"codec", "guava", "fastjson2", "spring"} {
		jar := jarPaths[name]
		if _, err := os.Stat(jar); err != nil {
			fmt.Printf("SKIP %s (missing)\n", name)
			continue
		}
		cp := classpathFor(name, jar)
		os.Unsetenv(ks)
		dir, files, u1, d1 := decompileJarUnits(t, jar)
		e1 := strings.Count(compileUnits(dir, files, cp), "error:")
		os.Setenv(ks, "1")
		dir2, files2, _, _ := decompileJarUnits(t, jar)
		e2 := strings.Count(compileUnits(dir2, files2, cp), "error:")
		os.Unsetenv(ks)
		fmt.Printf("JARDELTA[%s] %-10s units=%d decErr=%d  javacErr ON=%d OFF=%d  improvement=%d\n",
			ks, name, u1, d1, e1, e2, e2-e1)
	}
}

// TestScratchWholeTreePerFile does a whole-tree javac compile, then buckets errors by file. It reports
// a histogram of files-by-error-count and lists the files closest to flipping clean (fewest errors)
// with their actual error lines - the right "next lever" view for the real decompile-whole-jar-then-
// recompile workflow (a file with 0 whole-tree errors IS clean; whole-tree masking can undercount a
// file's errors but never reports a clean file as failing). Set PROFILE_JAR and optional MAXERR (show
// files with <= MAXERR errors, default 3) and TOPN (default 40).
func TestScratchWholeTreePerFile(t *testing.T) {
	scratchGate(t)
	target := os.Getenv("PROFILE_JAR")
	if target == "" {
		target = "fastjson2"
	}
	jar := jarPaths[target]
	if _, err := os.Stat(jar); err != nil {
		t.Skipf("jar missing: %s", jar)
	}
	if ks := os.Getenv("KILL_SWITCH"); ks != "" {
		os.Setenv(ks, "1")
		defer os.Unsetenv(ks)
	}
	dir, files, units, decErr := decompileJarUnits(t, jar)
	stderr := compileUnits(dir, files, classpathFor(target, jar))

	// Each error line looks like: /abs/path/Pkg/Name.java:NN: error: <msg>
	reErr := regexp.MustCompile(`^(.*\.java):(\d+): error: (.*)$`)
	perFile := map[string]int{}
	sample := map[string][]string{}
	totalErr := 0
	for _, ln := range strings.Split(stderr, "\n") {
		m := reErr.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		totalErr++
		rel := strings.TrimPrefix(m[1], dir+"/")
		perFile[rel]++
		if len(sample[rel]) < 8 {
			sample[rel] = append(sample[rel], strings.TrimSpace(m[2]+": "+m[3]))
		}
	}
	// Histogram of files-by-error-count.
	hist := map[int]int{}
	for _, c := range perFile {
		hist[c]++
	}
	failFiles := len(perFile)
	cleanFiles := units - failFiles
	fmt.Printf("==== WHOLE-TREE PER-FILE %s units=%d decErr=%d totalErr=%d cleanFiles=%d failFiles=%d (KILL_SWITCH=%q) ====\n",
		target, units, decErr, totalErr, cleanFiles, failFiles, os.Getenv("KILL_SWITCH"))
	var buckets []int
	for c := range hist {
		buckets = append(buckets, c)
	}
	sort.Ints(buckets)
	fmt.Printf("-- files-by-errorcount histogram --\n")
	for _, c := range buckets {
		fmt.Printf("   %3d err: %4d files\n", c, hist[c])
	}
	maxErr := 3
	if v := os.Getenv("MAXERR"); v != "" {
		fmt.Sscanf(v, "%d", &maxErr)
	}
	topN := 40
	if v := os.Getenv("TOPN"); v != "" {
		fmt.Sscanf(v, "%d", &topN)
	}
	type fe struct {
		f string
		c int
	}
	var list []fe
	for f, c := range perFile {
		if c <= maxErr {
			list = append(list, fe{f, c})
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].c != list[j].c {
			return list[i].c < list[j].c
		}
		return list[i].f < list[j].f
	})
	fmt.Printf("-- files closest to flipping (<=%d err), top %d --\n", maxErr, topN)
	for i, e := range list {
		if i >= topN {
			break
		}
		fmt.Printf("[%d err] %s\n", e.c, e.f)
		for _, s := range sample[e.f] {
			fmt.Printf("        %s\n", s)
		}
	}
}
