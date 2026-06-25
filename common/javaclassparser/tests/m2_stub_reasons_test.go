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
	// Stop-on-first (STOP_ON_FIRST=1): the harness is meant to drive a fix loop one class at a time
	// (HARNESS_WORKFLOW §0). When set, the scan ABORTS as soon as it has captured the very first
	// failing class (a partial/err/panic, in that severity order) under PROBLEM_DIR, instead of
	// grinding through the whole corpus. This keeps every iteration a few seconds rather than
	// minutes, and makes "the first problem class" a stable, reproducible target. The bucket files
	// it writes (raw .class + decompiled .java/.err.txt) are consumed unchanged by DIAG_FILE.
	// STOP_ON_FIRST_FIRST_OK (default 0): if set, treat an already-clear corpus (0 failures) as a
	// success and exit normally; otherwise the run keeps scanning to confirm the zero.
	stopOnFirst := os.Getenv("STOP_ON_FIRST") != ""
	// Industry mode (M2_INDUSTRY=1): scan EVERY jar in ~/.m2 (covers spring/tomcat/netty/... not just
	// the a-c alpha prefix), capping classes per jar (M2_MAX_PER_JAR, default 200) so a few giant jars
	// cannot eat the whole budget. Mirrors TestM2RegressionHarness so both report the same GA sample.
	industry := os.Getenv("M2_INDUSTRY") != ""
	maxPerJar := envInt("M2_MAX_PER_JAR", 200)
	// Progress cadence: every PROGRESS_EVERY classes, stream a live tally to stderr so the run is not
	// a black box (set PROGRESS_EVERY=0 to silence). Default 500 so a stuck/regressing run surfaces
	// quickly during fix iterations.
	progressEvery := envInt("PROGRESS_EVERY", 500)

	// Problem capture: every partial/err class is saved under PROBLEM_DIR (default /tmp/jdec-problems),
	// bucketed by reason, with BOTH the raw .class (re-run directly via DIAG_FILE) and the decompiled
	// .java / .err.txt. This turns "scan -> pick a failing class -> reproduce -> fix" into a one-liner.
	// Set PROBLEM_DIR= (empty) to disable. MAX_SAVE_PER_BUCKET caps files per bucket (default 30).
	problemDir := "/tmp/jdec-problems"
	if v, ok := os.LookupEnv("PROBLEM_DIR"); ok {
		problemDir = v
	}
	maxSavePerBucket := envInt("MAX_SAVE_PER_BUCKET", 30)
	if problemDir != "" {
		_ = os.RemoveAll(problemDir)
		_ = os.MkdirAll(problemDir, 0755)
	}
	savedPerBucket := map[string]int{}
	slugRe := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	bucketSlug := func(reason string) string {
		s := slugRe.ReplaceAllString(reason, "_")
		s = strings.Trim(s, "_")
		if len(s) > 48 {
			s = s[:48]
		}
		if s == "" {
			s = "unknown"
		}
		return s
	}
	sanitizeName := func(name string) string {
		return slugRe.ReplaceAllString(name, "_")
	}
	// saveProblem writes the raw class plus a sibling artifact (decompiled java or error text) into the
	// reason bucket folder, capped per bucket. kind is "partial" or "err". Returns the bucket
	// directory it wrote to (or "" if it did not write), so stop-on-first can point the user at it.
	saveProblem := func(kind, reason, name string, raw []byte, artifactExt, artifact string) string {
		if problemDir == "" {
			return ""
		}
		bucket := kind + "__" + bucketSlug(reason)
		if savedPerBucket[bucket] >= maxSavePerBucket {
			return ""
		}
		savedPerBucket[bucket]++
		dir := filepath.Join(problemDir, bucket)
		_ = os.MkdirAll(dir, 0755)
		stem := sanitizeName(name)
		if len(stem) > 150 {
			stem = stem[len(stem)-150:]
		}
		_ = os.WriteFile(filepath.Join(dir, stem+".class"), raw, 0644)
		if artifact != "" {
			_ = os.WriteFile(filepath.Join(dir, stem+artifactExt), []byte(name+"\n\n"+artifact), 0644)
		}
		return dir
	}

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
	fmt.Fprintf(os.Stderr, "[stub-reasons] start: jars=%d industry=%v maxClasses=%d maxPerJar=%d\n", len(jars), industry, maxClasses, maxPerJar)

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
	var nClasses, nPartial, nStubs, nOK, nErr int
	// errClasses records every class whose decompile returned an error or escaped a panic (the harness
	// recovers panics into derr with a "panic:" prefix). These are the most severe boundary cases — a
	// hard failure rather than a graceful stub — so we capture the class name and message for triage.
	type errRec struct{ name, msg string }
	var errClasses []errRec
	errReasonCounts := map[string]int{}

	// firstFailure holds the name + bucket path of the first captured failing class, for the
	// stop-on-first summary. Empty until a partial/err is seen.
	var firstFailureName, firstFailureBucket string

jarLoop:
	for ji, jp := range jars {
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
			out, derr := safeDecompileHarness(raw)
			if derr != nil {
				// A hard decompile error or escaped panic (the harness recovers panics into derr).
				nErr++
				name := filepath.Base(jp) + "!" + f.Name
				msg := derr.Error()
				if len(msg) > 200 {
					msg = msg[:200]
				}
				errClasses = append(errClasses, errRec{name, msg})
				errReasonCounts[normalize(msg)]++
				dir := saveProblem("err", normalize(msg), name, raw, ".err.txt", derr.Error())
				fmt.Fprintf(os.Stderr, "[stub-reasons] ERR %s :: %s\n", name, strings.ReplaceAll(msg, "\n", " "))
				if stopOnFirst {
					firstFailureName, firstFailureBucket = name, dir
					break jarLoop
				}
			} else if !strings.Contains(out, javaclassparser.DecompileStubMarker) {
				nOK++
			} else {
				nPartial++
				name := filepath.Base(jp) + "!" + f.Name
				var primaryReason string
				for _, m := range reasonRe.FindAllStringSubmatch(out, -1) {
					reason := normalize(m[1])
					if reason == "" {
						continue
					}
					if primaryReason == "" {
						primaryReason = reason
					}
					nStubs++
					counts[reason]++
					if len(examples[reason]) < 4 {
						examples[reason] = append(examples[reason], name)
					}
				}
				// Save the failing class under its dominant reason bucket so it can be reproduced
				// directly: `DIAG_FILE=<path> go test -run TestDiagDecompileClass ...`.
				dir := saveProblem("partial", primaryReason, name, raw, ".java", out)
				if stopOnFirst {
					firstFailureName, firstFailureBucket = name, dir
					break jarLoop
				}
			}
			if progressEvery > 0 && nClasses%progressEvery == 0 {
				fmt.Fprintf(os.Stderr, "[stub-reasons] progress: jar %d/%d  scanned=%d  ok=%d  partial=%d  err=%d\n",
					ji+1, len(jars), nClasses, nOK, nPartial, nErr)
			}
		}
		zr.Close()
		if nClasses >= maxClasses {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "[stub-reasons] DONE: scanned=%d  ok=%d  partial=%d  err=%d  stubs=%d\n",
		nClasses, nOK, nPartial, nErr, nStubs)
	if stopOnFirst && firstFailureName != "" {
		fmt.Fprintf(os.Stderr, "[stub-reasons] STOP_ON_FIRST: aborted after first failure at class %d\n", nClasses)
		fmt.Fprintf(os.Stderr, "[stub-reasons]   class: %s\n", firstFailureName)
		fmt.Fprintf(os.Stderr, "[stub-reasons]   bucket dir: %s\n", firstFailureBucket)
		fmt.Fprintf(os.Stderr, "[stub-reasons]   reproduce: DIAG_FILE=%s/*.class go test -run TestDiagDecompileClass -v ./common/javaclassparser/tests/\n", firstFailureBucket)
	} else if stopOnFirst {
		fmt.Fprintf(os.Stderr, "[stub-reasons] STOP_ON_FIRST: no failure found in scanned range (scanned=%d ok=%d)\n", nClasses, nOK)
	}
	if problemDir != "" {
		nSaved := 0
		for _, c := range savedPerBucket {
			nSaved += c
		}
		fmt.Fprintf(os.Stderr, "[stub-reasons] saved %d problem classes (capped %d/bucket) under %s\n", nSaved, maxSavePerBucket, problemDir)
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
	sb.WriteString(fmt.Sprintf("classes=%d ok=%d partial=%d err=%d stubs=%d distinct_reasons=%d\n", nClasses, nOK, nPartial, nErr, nStubs, len(list)))
	if nErr > 0 {
		sb.WriteString(fmt.Sprintf("\n==== ERR classes (hard failure / escaped panic): %d ====\n", nErr))
		type ekv struct {
			k string
			v int
		}
		var elist []ekv
		for k, v := range errReasonCounts {
			elist = append(elist, ekv{k, v})
		}
		sort.Slice(elist, func(i, j int) bool { return elist[i].v > elist[j].v })
		for _, e := range elist {
			sb.WriteString(fmt.Sprintf("%6d  %s\n", e.v, e.k))
		}
		sb.WriteString("---- per-class ----\n")
		for _, e := range errClasses {
			sb.WriteString("  " + e.name + " :: " + strings.ReplaceAll(e.msg, "\n", " ") + "\n")
		}
		sb.WriteString("\n==== PARTIAL stub reason buckets ====\n")
	}
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
