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

// TestM2StubReasons scans up to M2_MAX_JARS jars under ~/.m2, decompiles each class, and for every
// partial (stub-bearing) output extracts the embedded stub reasons (/* yak-decompiler: <reason> */),
// normalizes them (digits -> N) and tallies the buckets so we can see which CFG family dominates the
// remaining partials. Skipped unless STUB_REASONS is set so it never runs in normal CI.
func TestM2StubReasons(t *testing.T) {
	if os.Getenv("STUB_REASONS") == "" {
		t.Skip("set STUB_REASONS=1 to categorize remaining partial stub reasons")
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

	reasonRe := regexp.MustCompile(`/\* yak-decompiler:([^*]*)\*/`)
	digitsRe := regexp.MustCompile(`\d+`)
	normalize := func(s string) string {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "undecompilable method body")
		s = strings.TrimSpace(s)
		// keep the tail after the last "failed: " when present to focus on the leaf reason
		s = digitsRe.ReplaceAllString(s, "N")
		s = strings.TrimSpace(s)
		return s
	}

	counts := map[string]int{}
	examples := map[string][]string{}
	var nClasses, nPartial, nStubs int

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
			nPartial++
			for _, m := range reasonRe.FindAllStringSubmatch(out, -1) {
				reason := normalize(m[1])
				if reason == "" {
					continue
				}
				nStubs++
				counts[reason]++
				if len(examples[reason]) < 4 {
					examples[reason] = append(examples[reason], filepath.Base(jp)+"!"+f.Name)
				}
			}
		}
		zr.Close()
		if nClasses >= maxClasses {
			break
		}
	}

	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].v > list[j].v })

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("classes=%d partial=%d stubs=%d distinct_reasons=%d\n", nClasses, nPartial, nStubs, len(list)))
	for _, e := range list {
		sb.WriteString(fmt.Sprintf("%6d  %s\n", e.v, e.k))
		for _, ex := range examples[e.k] {
			sb.WriteString("          e.g. " + ex + "\n")
		}
	}
	out := sb.String()
	if p := os.Getenv("STUB_OUT"); p != "" {
		_ = os.WriteFile(p, []byte(out), 0644)
	}
	t.Log("\n" + out)
}
