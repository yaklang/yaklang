package tests

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestEmptySlotInventory lists every class!method that the decompiler degrades to a stub with the
// "empty stack slot leaked" reason (the dominant partial bucket targeted by the merge-value
// reconstruction rewrite). It writes the inventory to EMPTYSLOT_OUT so we have a stable target list
// to verify against after each rewrite step. Skipped unless EMPTYSLOT_OUT is set.
func TestEmptySlotInventory(t *testing.T) {
	outPath := os.Getenv("EMPTYSLOT_OUT")
	if outPath == "" {
		t.Skip("set EMPTYSLOT_OUT to run the empty-slot inventory")
	}
	maxJars := envInt("M2_MAX_JARS", 120)
	maxClasses := envInt("M2_MAX_CLASSES", 12000)

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
	if len(jars) > maxJars {
		jars = jars[:maxJars]
	}

	// Each stubbed method renders as: <signature> { throw new RuntimeException("yak-decompiler: ...");
	// /* yak-decompiler: <reason> */ }. Capture the signature line that carries the empty-slot reason.
	emptySlotRe := regexp.MustCompile(`(?m)^.*yak-decompiler:[^\n]*empty stack slot leaked[^\n]*$`)
	sigRe := regexp.MustCompile(`([A-Za-z_$][\w$]*)\s*\([^)]*\)\s*\{`)

	var lines []string
	var nClasses, nEmptySlotClasses, nEmptySlotMethods int
	for _, jp := range jars {
		zr, err := zip.OpenReader(jp)
		if err != nil {
			continue
		}
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
			out, derr := safeDecompileHarness(raw)
			if derr != nil || !strings.Contains(out, javaclassparser.DecompileStubMarker) {
				continue
			}
			hits := emptySlotRe.FindAllString(out, -1)
			if len(hits) == 0 {
				continue
			}
			nEmptySlotClasses++
			for _, h := range hits {
				nEmptySlotMethods++
				method := "?"
				if m := sigRe.FindStringSubmatch(h); m != nil {
					method = m[1]
				}
				lines = append(lines, fmt.Sprintf("%s!%s::%s", filepath.Base(jp), f.Name, method))
			}
		}
		zr.Close()
		if nClasses >= maxClasses {
			break
		}
	}
	sort.Strings(lines)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# classes=%d empty_slot_classes=%d empty_slot_methods=%d\n", nClasses, nEmptySlotClasses, nEmptySlotMethods))
	for _, l := range lines {
		sb.WriteString(l + "\n")
	}
	if err := os.WriteFile(outPath, []byte(sb.String()), 0644); err != nil {
		t.Fatalf("write out: %v", err)
	}
	t.Logf("wrote %s: classes=%d empty_slot_classes=%d empty_slot_methods=%d", outPath, nClasses, nEmptySlotClasses, nEmptySlotMethods)
}

// TestDiagDecompileClass decompiles a single class from a jar (DIAG_JAR + DIAG_CLASS substring) or a
// raw .class file (DIAG_FILE) and logs the full output, for ad-hoc inspection of merge-value shapes.
func TestDiagDecompileClass(t *testing.T) {
	if p := os.Getenv("DIAG_FILE"); p != "" {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out, derr := safeDecompileHarness(raw)
		t.Logf("=== %s (err=%v) ===\n%s", p, derr, out)
		return
	}
	want := os.Getenv("DIAG_CLASS")
	if want == "" {
		t.Skip("set DIAG_CLASS (+optional DIAG_JAR) or DIAG_FILE to run")
	}
	jar := os.Getenv("DIAG_JAR")
	if jar != "" && !filepath.IsAbs(jar) {
		home, _ := os.UserHomeDir()
		jar = filepath.Join(home, ".m2", jar)
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
		rc, _ := f.Open()
		raw := readAll(rc)
		rc.Close()
		out, derr := safeDecompileHarness(raw)
		t.Logf("=== %s (err=%v) ===\n%s", f.Name, derr, out)
	}
}
