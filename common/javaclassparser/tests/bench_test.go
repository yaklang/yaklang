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
	"strings"
	"testing"

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
