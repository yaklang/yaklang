package tests

import (
	"archive/zip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestM2FindSmallest finds the smallest .class (by raw byte size) whose decompiled output contains a
// stub whose reason matches REASON_SUBSTR (default "multiple next"). It writes the raw class bytes to
// SMALLEST_OUT so we can disassemble it with javap and craft a minimal reproduction. Skipped unless
// FIND_SMALLEST is set.
func TestM2FindSmallest(t *testing.T) {
	if os.Getenv("FIND_SMALLEST") == "" {
		t.Skip("set FIND_SMALLEST=1 to locate the smallest failing class")
	}
	want := os.Getenv("REASON_SUBSTR")
	if want == "" {
		want = "multiple next"
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

	var bestRaw []byte
	bestSize := 1 << 30
	bestName := ""
	var nClasses int

	for _, jp := range jars {
		zr, err := zip.OpenReader(jp)
		if err != nil {
			continue
		}
		for _, f := range zr.File {
			if !strings.HasSuffix(f.Name, ".class") {
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
			if len(raw) == 0 || len(raw) >= bestSize {
				continue
			}
			nClasses++
			out, derr := safeDecompileHarness(raw)
			if derr != nil || !strings.Contains(out, javaclassparser.DecompileStubMarker) {
				continue
			}
			if strings.Contains(out, want) {
				bestSize = len(raw)
				bestRaw = append([]byte(nil), raw...)
				bestName = filepath.Base(jp) + "!" + f.Name
			}
		}
		zr.Close()
	}

	if bestRaw == nil {
		t.Fatalf("no class found with reason substring %q", want)
	}
	t.Logf("smallest failing class: %s (%d bytes)", bestName, bestSize)
	if p := os.Getenv("SMALLEST_OUT"); p != "" {
		if err := os.WriteFile(p, bestRaw, 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
		t.Logf("wrote raw class to %s", p)
	}
	fmt.Println("SMALLEST:", bestName, bestSize)
}
