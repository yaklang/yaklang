package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

var rtPublicClassRe = regexp.MustCompile(`(?m)^\s*(?:public\s+)?(?:final\s+|abstract\s+)?(?:class|interface|enum)\s+([A-Za-z_$][A-Za-z0-9_$]*)`)

// TestRoundTripDev is a development harness (gated by RT_DIR): for each top-level .class in RT_DIR
// it decompiles the class, reports whether the body was stubbed, and -- if javac is available --
// recompiles the decompiled source to catch SEMANTIC/type errors that the ANTLR syntax safety net
// cannot see (e.g. the "Exception var = Exception;" ternary-in-try corruption). Self-contained
// (JDK-only, default-package) classes recompile cleanly; cross-class references are expected to
// fail and are not the target of this harness.
func TestRoundTripDev(t *testing.T) {
	dir := os.Getenv("RT_DIR")
	if dir == "" {
		t.Skip("set RT_DIR to a directory of .class files")
	}
	javac, _ := exec.LookPath("javac")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".class") && !strings.Contains(e.Name(), "$") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	work := t.TempDir()
	var stubbed, semanticFail int
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		out, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Errorf("[%s] decompile error: %v", name, err)
			continue
		}
		stub := strings.Contains(out, javaclassparser.DecompileStubMarker)
		if stub {
			stubbed++
		}
		// strip package declaration so the file compiles standalone in the default package
		src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(out, "")
		m := rtPublicClassRe.FindStringSubmatch(src)
		javacResult := "skip(no-javac)"
		if javac != "" && m != nil {
			className := m[1]
			fpath := filepath.Join(work, className+".java")
			_ = os.WriteFile(fpath, []byte(src), 0644)
			cmd := exec.Command(javac, "-J-Duser.language=en", "-nowarn", "-d", work, fpath)
			combined, cerr := cmd.CombinedOutput()
			if cerr != nil {
				// keep only error lines (force English locale above so "error:" matches)
				lines := []string{}
				for _, ln := range strings.Split(string(combined), "\n") {
					ln = strings.TrimSpace(ln)
					if strings.Contains(ln, "error:") || strings.Contains(ln, "错误:") {
						lines = append(lines, ln)
					}
				}
				if len(lines) == 0 {
					lines = append(lines, strings.TrimSpace(string(combined)))
				}
				javacResult = "FAIL: " + strings.Join(lines, " | ")
				semanticFail++
			} else {
				javacResult = "ok"
			}
		}
		status := "full"
		if stub {
			status = "STUB"
		}
		t.Logf("[%s] decompile=%s javac=%s", name, status, javacResult)
		if strings.HasPrefix(javacResult, "FAIL") {
			t.Logf("    ----- offending decompiled source (%s) -----\n%s", name, out)
		}
	}
	t.Logf("==== ROUND-TRIP SUMMARY: classes=%d stubbed=%d semanticFail=%d ====", len(names), stubbed, semanticFail)
}
