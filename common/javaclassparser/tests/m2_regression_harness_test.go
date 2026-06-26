package tests

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestM2RegressionHarness scans up to M2_MAX_JARS jars under ~/.m2, decompiles every class, and
// writes a deterministic fingerprint file (per-class status + output hash) plus a category summary
// to M2_OUT. Run it twice (e.g. with and without a change, swapping M2_OUT) and diff the files to
// detect regressions / measure partial-count movement. It is skipped unless M2_OUT is set so it
// never runs in normal CI.
//
// Categories per class:
//   - err     : Decompile returned an error
//   - partial : output contains DecompileStubMarker (a method/field could not be reconstructed)
//   - syntax  : output is non-empty, no stub, but java2ssa.Frontend rejects it (should be ~0)
//   - ok      : full, syntactically valid decompile
func TestM2RegressionHarness(t *testing.T) {
	outPath := os.Getenv("M2_OUT")
	if outPath == "" {
		t.Skip("set M2_OUT to run the .m2 regression harness")
	}
	maxJars := envInt("M2_MAX_JARS", 60)
	maxClasses := envInt("M2_MAX_CLASSES", 8000)
	// Industry mode (M2_INDUSTRY=1): scan EVERY jar in ~/.m2 instead of the first maxJars in
	// alpha order (which only covers a-c prefixes and misses spring/tomcat/netty/...), but cap the
	// number of classes taken per jar (M2_MAX_PER_JAR, default 200) so a few giant jars cannot eat
	// the whole class budget. This gives a broad, bounded GA-representative sample across the corpus.
	industry := os.Getenv("M2_INDUSTRY") != ""
	maxPerJar := envInt("M2_MAX_PER_JAR", 200)

	home, _ := os.UserHomeDir()
	m2 := filepath.Join(home, ".m2")

	var jars []string
	_ = filepath.Walk(m2, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.HasSuffix(p, ".jar") {
			jars = append(jars, p)
		}
		return nil
	})
	sort.Strings(jars)
	if !industry && len(jars) > maxJars {
		jars = jars[:maxJars]
	}

	type rec struct {
		status string
		hash   string
	}
	results := map[string]rec{}
	var nOK, nPartial, nSyntax, nErr, nClasses int

	for _, jp := range jars {
		zr, err := zip.OpenReader(jp)
		if err != nil {
			continue
		}
		perJar := 0
		for _, f := range zr.File {
			if !strings.HasSuffix(f.Name, ".class") {
				continue
			}
			base := filepath.Base(f.Name)
			if base == "module-info.class" || base == "package-info.class" {
				continue
			}
			if nClasses >= maxClasses {
				break
			}
			if industry && maxPerJar > 0 && perJar >= maxPerJar {
				break
			}
			perJar++
			rc, err := f.Open()
			if err != nil {
				continue
			}
			raw := readAll(rc)
			rc.Close()
			if len(raw) == 0 {
				continue
			}
			nClasses++
			key := f.Name
			out, derr := safeDecompileHarness(raw)
			r := rec{}
			switch {
			case derr != nil:
				r.status = "err"
				r.hash = shortHash(derr.Error())
				nErr++
			case strings.Contains(out, javaclassparser.DecompileStubMarker):
				r.status = "partial"
				r.hash = shortHash(out)
				nPartial++
			default:
				if _, ferr := java2ssa.Frontend(out); ferr != nil {
					r.status = "syntax"
					r.hash = shortHash(out)
					nSyntax++
				} else {
					r.status = "ok"
					r.hash = shortHash(out)
					nOK++
				}
			}
			// Disambiguate identical class names across jars by prefixing the jar base name.
			results[filepath.Base(jp)+"!"+key] = r
		}
		zr.Close()
		if nClasses >= maxClasses {
			break
		}
	}

	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# jars=%d classes=%d ok=%d partial=%d syntax=%d err=%d\n", len(jars), nClasses, nOK, nPartial, nSyntax, nErr))
	for _, k := range keys {
		r := results[k]
		sb.WriteString(fmt.Sprintf("%s\t%s\t%s\n", r.status, r.hash, k))
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write out: %v", err)
	}
	t.Logf("wrote %s: jars=%d classes=%d ok=%d partial=%d syntax=%d err=%d", outPath, len(jars), nClasses, nOK, nPartial, nSyntax, nErr)
}

func safeDecompileHarness(raw []byte) (out string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return javaclassparser.Decompile(raw)
}

func readAll(rc interface{ Read([]byte) (int, error) }) []byte {
	var buf []byte
	tmp := make([]byte, 32*1024)
	for {
		n, err := rc.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return buf
}

func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n := 0
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
