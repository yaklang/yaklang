package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestNestedSwitchBreakRoundTrip pins the nested-switch shared-exit break-absorption fix. javac emits
// no instruction between an inner switch and the outer `break` that immediately follows it, so both
// the inner switch's arms and the outer break target the SAME shared exit offset. Dominator-based
// merge detection cannot attribute that shared point to the inner switch, so the structured inner
// switch was left without an exit edge and the enclosing case ended up with neither a break leaf nor a
// fall-through edge - it silently fell through to the next case label. Without the repair, f(0,*) would
// fall through to default and return 13 instead of 2/5, so the recompiled program's output diverges.
func TestNestedSwitchBreakRoundTrip(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("no javac")
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("no java")
	}
	src := `public class NestedSwitchBreak {
    static int f(int a, int b) {
        int r;
        switch (a) {
            case 0:
                switch (b) {
                    case 0: r = 2; break;
                    default: r = 5; break;
                }
                break;
            default:
                r = 13;
        }
        return r;
    }
    public static void main(String[] x) {
        System.out.println("" + f(0,0) + "," + f(0,1) + "," + f(9,9) + "," + f(0,7));
    }
}`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "NestedSwitchBreak.java")
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-d", dir, srcPath).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	classPath := filepath.Join(dir, "NestedSwitchBreak.class")
	wantOut, err := exec.Command(java, "-cp", dir, "NestedSwitchBreak").CombinedOutput()
	if err != nil {
		t.Fatalf("run original: %v\n%s", err, wantOut)
	}

	raw, err := os.ReadFile(classPath)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	t.Logf("DECOMPILED:\n%s", dec)

	dec2 := strings.Replace(dec, "package defaultpackagename;", "", 1)
	rtDir := t.TempDir()
	rtSrc := filepath.Join(rtDir, "NestedSwitchBreak.java")
	if err := os.WriteFile(rtSrc, []byte(dec2), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-d", rtDir, rtSrc).CombinedOutput(); err != nil {
		t.Fatalf("recompile decompiled source failed: %v\n%s\nSOURCE:\n%s", err, out, dec2)
	}
	gotOut, err := exec.Command(java, "-cp", rtDir, "NestedSwitchBreak").CombinedOutput()
	if err != nil {
		t.Fatalf("run decompiled: %v\n%s", err, gotOut)
	}
	if string(gotOut) != string(wantOut) {
		t.Fatalf("round-trip output mismatch (nested-switch break absorbed):\n original = %q\n decompiled = %q", wantOut, gotOut)
	}
}
