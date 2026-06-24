package tests

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestJSRInlineProbe decompiles every class in a single jar (default ant-1.6.5) and reports the
// per-status breakdown plus, for any still-partial class, whether a jsr stub remains. Gated by
// JSR_PROBE so it never runs in CI. Use JSR_JAR to point at a jar path.
func TestJSRInlineProbe(t *testing.T) {
	if os.Getenv("JSR_PROBE") == "" {
		t.Skip("set JSR_PROBE=1 to run the jsr inlining probe")
	}
	jar := os.Getenv("JSR_JAR")
	if jar == "" {
		home, _ := os.UserHomeDir()
		jar = filepath.Join(home, ".m2/repository/ant/ant/1.6.5/ant-1.6.5.jar")
	}
	zr, err := zip.OpenReader(jar)
	if err != nil {
		t.Fatalf("open jar: %v", err)
	}
	defer zr.Close()

	var nClasses, nOK, nPartial, nSyntax, nErr, nJSRStub int
	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".class") {
			continue
		}
		base := filepath.Base(f.Name)
		if base == "module-info.class" || base == "package-info.class" {
			continue
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
		if dump := os.Getenv("JSR_DUMP"); dump != "" && strings.Contains(f.Name, dump) {
			if op := os.Getenv("JSR_DUMP_OUT"); op != "" {
				_ = os.WriteFile(op, []byte(out), 0644)
			}
			t.Logf("=== dump %s (err=%v) ===\n%s", f.Name, derr, out)
		}
		switch {
		case derr != nil:
			nErr++
		case strings.Contains(out, javaclassparser.DecompileStubMarker):
			nPartial++
			if strings.Contains(out, "not support opcode: jsr") || strings.Contains(out, "not support opcode: ret") {
				nJSRStub++
				if nJSRStub <= 5 {
					t.Logf("remaining jsr stub: %s", f.Name)
				}
			}
		default:
			if dir := os.Getenv("JSR_OKDIR"); dir != "" && strings.Contains(out, "finally") {
				_ = os.MkdirAll(dir, 0o755)
				safe := strings.ReplaceAll(strings.TrimSuffix(f.Name, ".class"), "/", ".")
				_ = os.WriteFile(filepath.Join(dir, safe+".java"), []byte(out), 0644)
			}
			if _, ferr := java2ssa.Frontend(out); ferr != nil {
				nSyntax++
				if nSyntax <= 5 {
					t.Logf("syntax-invalid after inline: %s: %v", f.Name, ferr)
				}
			} else {
				nOK++
			}
		}
	}
	t.Logf("jar=%s classes=%d ok=%d partial=%d syntax=%d err=%d jsr_stub_remaining=%d",
		filepath.Base(jar), nClasses, nOK, nPartial, nSyntax, nErr, nJSRStub)
}
