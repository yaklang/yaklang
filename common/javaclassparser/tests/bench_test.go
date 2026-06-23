package tests

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// loadJarClasses reads every .class entry from a jar/zip into memory.
func loadJarClasses(tb testing.TB, jarPath string) map[string][]byte {
	zr, err := zip.OpenReader(jarPath)
	if err != nil {
		tb.Skipf("cannot open jar %s: %v", jarPath, err)
	}
	defer zr.Close()
	out := map[string][]byte{}
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") {
			continue
		}
		if strings.Contains(f.Name, "package-info") || strings.Contains(f.Name, "module-info") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		out[f.Name] = data
	}
	return out
}

// BenchmarkDecompileJar decompiles all classes of a jar repeatedly; use with
// -cpuprofile/-memprofile to locate hotspots. JAR path via BENCH_JAR env.
func BenchmarkDecompileJar(b *testing.B) {
	jarPath := os.Getenv("BENCH_JAR")
	if jarPath == "" {
		jarPath = "/Users/v1ll4n/.m2/repository/commons-codec/commons-codec/1.15/commons-codec-1.15.jar"
	}
	classes := loadJarClasses(b, jarPath)
	if len(classes) == 0 {
		b.Skip("no classes loaded")
	}
	b.Logf("loaded %d classes from %s", len(classes), jarPath)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, raw := range classes {
			func() {
				defer func() { recover() }()
				_, _ = javaclassparser.Decompile(raw)
			}()
		}
	}
}

// BenchmarkDecompileJarParallel decompiles all classes of a jar concurrently across
// CONC goroutines (default 100, mirroring the jdsc self-check). This is the workload
// where the per-variable crypto/rand UUID generation hurt most: parallel getentropy()
// calls serialize on a kernel lock. JAR path via BENCH_JAR, concurrency via BENCH_CONC.
func BenchmarkDecompileJarParallel(b *testing.B) {
	jarPath := os.Getenv("BENCH_JAR")
	if jarPath == "" {
		jarPath = "/Users/v1ll4n/.m2/repository/com/hazelcast/hazelcast/5.1.7/hazelcast-5.1.7.jar"
	}
	conc := 100
	if v := os.Getenv("BENCH_CONC"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			conc = n
		}
	}
	classes := loadJarClasses(b, jarPath)
	if len(classes) == 0 {
		b.Skip("no classes loaded")
	}
	raws := make([][]byte, 0, len(classes))
	for _, raw := range classes {
		raws = append(raws, raw)
	}
	b.Logf("loaded %d classes from %s, conc=%d", len(raws), jarPath, conc)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sem := make(chan struct{}, conc)
		var wg sync.WaitGroup
		for _, raw := range raws {
			wg.Add(1)
			sem <- struct{}{}
			go func(r []byte) {
				defer wg.Done()
				defer func() { <-sem }()
				defer func() { recover() }()
				_, _ = javaclassparser.Decompile(r)
			}(raw)
		}
		wg.Wait()
	}
}

// BenchmarkDecompileSingleClass repeatedly decompiles ONE class (the first whose name
// contains BENCH_CLASS) from BENCH_JAR. Use with -cpuprofile to get a clean, serial
// algorithmic profile of a single hot class, undistorted by the parallel scheduler's
// idle usleep/GC that dominates the whole-jar parallel profile.
func BenchmarkDecompileSingleClass(b *testing.B) {
	jarPath := os.Getenv("BENCH_JAR")
	if jarPath == "" {
		jarPath = "/Users/v1ll4n/.m2/repository/com/hazelcast/hazelcast/5.1.7/hazelcast-5.1.7.jar"
	}
	want := os.Getenv("BENCH_CLASS")
	if want == "" {
		want = "DefaultMessageTaskFactoryProvider"
	}
	classes := loadJarClasses(b, jarPath)
	var raw []byte
	for name, r := range classes {
		if strings.Contains(name, want) {
			raw = r
			break
		}
	}
	if raw == nil {
		b.Skipf("class containing %q not found in %s", want, jarPath)
	}
	b.Logf("decompiling %q (%d bytes) x%d", want, len(raw), b.N)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		func() {
			defer func() { recover() }()
			_, _ = javaclassparser.Decompile(raw)
		}()
	}
}

// TestDumpJarFingerprint decompiles every class of the JAR(s) given by DIFF_JARS
// (colon-separated; falls back to a built-in list) and writes, per jar, a sorted
// "<classname> <sha256(status+output)>" fingerprint file under OUT_DIR.
//
// Run it once on the original dominator-tree.go and once on the bitset version with
// different OUT_DIRs, then `diff -r` the two dirs: any byte-level difference in
// decompiled output (or in failure mode) shows up as a hash mismatch. This is the
// equivalence proof for the dominator-tree rewrite.
func TestDumpJarFingerprint(t *testing.T) {
	outDir := os.Getenv("OUT_DIR")
	if outDir == "" {
		t.Skip("OUT_DIR not set; skipping equivalence fingerprint dump")
	}
	jarsEnv := os.Getenv("DIFF_JARS")
	var jars []string
	if jarsEnv != "" {
		jars = strings.Split(jarsEnv, ":")
	} else {
		jars = []string{
			"/Users/v1ll4n/.m2/repository/com/ibm/icu/icu4j/71.1/icu4j-71.1.jar",
			"/Users/v1ll4n/.m2/repository/org/elasticsearch/elasticsearch/7.17.15/elasticsearch-7.17.15.jar",
			"/Users/v1ll4n/.m2/repository/com/hazelcast/hazelcast/5.1.7/hazelcast-5.1.7.jar",
		}
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir out: %v", err)
	}
	for _, jarPath := range jars {
		classes := loadJarClasses(t, jarPath)
		if len(classes) == 0 {
			t.Logf("no classes in %s, skip", jarPath)
			continue
		}
		lines := make([]string, 0, len(classes))
		for name, raw := range classes {
			status, out := safeDecompile(raw)
			sum := sha256.Sum256([]byte(status + "\x00" + out))
			lines = append(lines, name+" "+hex.EncodeToString(sum[:]))
		}
		sort.Strings(lines)
		fp := filepath.Join(outDir, filepath.Base(jarPath)+".fp")
		if err := os.WriteFile(fp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write fp: %v", err)
		}
		t.Logf("wrote %d fingerprints for %s -> %s", len(lines), filepath.Base(jarPath), fp)
	}
}

// safeDecompile returns a status tag (ok/err/panic) plus the decompiled body (or
// the error/panic text) so fingerprints capture both output and failure mode.
func safeDecompile(raw []byte) (status string, out string) {
	defer func() {
		if r := recover(); r != nil {
			status = "panic"
			out = fmt.Sprint(r)
		}
	}()
	s, err := javaclassparser.Decompile(raw)
	if err != nil {
		return "err", err.Error()
	}
	return "ok", s
}

// TestTopSlowClasses decompiles every class once, then reports the slowest classes
// and the cumulative-time distribution (top-1 / top-10 / top-1% / top-10% share of
// total). This pinpoints whether the workload is tail-bound (a few huge classes
// dominate) and gives concrete targets for the serial CPU profile. BENCH_JAR selects
// the jar; TOPN controls how many slow classes are printed.
func TestTopSlowClasses(t *testing.T) {
	jarPath := os.Getenv("BENCH_JAR")
	if jarPath == "" {
		jarPath = "/Users/v1ll4n/.m2/repository/com/hazelcast/hazelcast/5.1.7/hazelcast-5.1.7.jar"
	}
	topN := 30
	if v := os.Getenv("TOPN"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			topN = n
		}
	}
	classes := loadJarClasses(t, jarPath)
	if len(classes) == 0 {
		t.Skip("no classes loaded")
	}
	type rec struct {
		name string
		ns   int64
		size int
	}
	recs := make([]rec, 0, len(classes))
	var total int64
	for name, raw := range classes {
		start := time.Now()
		func() {
			defer func() { recover() }()
			_, _ = javaclassparser.Decompile(raw)
		}()
		d := time.Since(start).Nanoseconds()
		recs = append(recs, rec{name: name, ns: d, size: len(raw)})
		total += d
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].ns > recs[j].ns })

	share := func(n int) float64 {
		var s int64
		for i := 0; i < n && i < len(recs); i++ {
			s += recs[i].ns
		}
		if total == 0 {
			return 0
		}
		return float64(s) / float64(total) * 100
	}
	onePct := len(recs) / 100
	if onePct < 1 {
		onePct = 1
	}
	tenPct := len(recs) / 10
	if tenPct < 1 {
		tenPct = 1
	}
	t.Logf("jar=%s classes=%d total=%.2fs", filepath.Base(jarPath), len(recs), float64(total)/1e9)
	t.Logf("cumulative share: top-1=%.1f%% top-10=%.1f%% top-1%%(%d)=%.1f%% top-10%%(%d)=%.1f%%",
		share(1), share(10), onePct, share(onePct), tenPct, share(tenPct))
	for i := 0; i < topN && i < len(recs); i++ {
		t.Logf("#%-3d %8.1fms  %7dB  %s", i+1, float64(recs[i].ns)/1e6, recs[i].size, recs[i].name)
	}
}

// TestDecompileJarTiming reports wall-clock time and success rate for one full pass,
// useful as a quick perf+health probe without the benchmark harness.
func TestDecompileJarTiming(t *testing.T) {
	jarPath := os.Getenv("BENCH_JAR")
	if jarPath == "" {
		jarPath = "/Users/v1ll4n/.m2/repository/commons-codec/commons-codec/1.15/commons-codec-1.15.jar"
	}
	classes := loadJarClasses(t, jarPath)
	if len(classes) == 0 {
		t.Skip("no classes loaded")
	}
	var ok, fail, partial int
	for _, raw := range classes {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fail++
				}
			}()
			out, err := javaclassparser.Decompile(raw)
			if err != nil {
				fail++
			} else if strings.Contains(out, javaclassparser.DecompileStubMarker) {
				partial++
			} else {
				ok++
			}
		}()
	}
	t.Logf("classes=%d ok=%d partial=%d fail=%d", len(classes), ok, partial, fail)
}
