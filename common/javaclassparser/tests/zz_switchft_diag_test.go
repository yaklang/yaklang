package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// Diagnostic for Bug G: a switch whose cases are written in DESCENDING value order with
// fall-through (case 3 -> case 2 -> case 1). javac lays the bodies out in source order; the
// decompiler must preserve that physical order or the fall-through direction inverts.
func TestSwitchFTDiag(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("no javac")
	}
	src := `public class SwitchFT {
    static int tail(int rem) {
        int k = 0;
        switch (rem) {
            case 3: k = k + 300;
            case 2: k = k + 20;
            case 1: k = k + 1;
        }
        return k;
    }
    static int asc(int rem) {
        int k = 0;
        switch (rem) {
            case 1: k = k + 1; break;
            case 2: k = k + 20; break;
            case 3: k = k + 300; break;
            default: k = -1;
        }
        return k;
    }
    public static void main(String[] a){
        System.out.println("" + tail(0) + tail(1) + tail(2) + tail(3)
            + "/" + asc(0) + asc(1) + asc(2) + asc(3));
    }
}`
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "SwitchFT.java")
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-d", dir, srcPath).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	classPath := filepath.Join(dir, "SwitchFT.class")
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("no java")
	}
	wantOut, err := exec.Command(java, "-cp", dir, "SwitchFT").CombinedOutput()
	if err != nil {
		t.Fatalf("run original: %v\n%s", err, wantOut)
	}
	t.Logf("ORIGINAL OUTPUT: %s", wantOut)

	raw, err := os.ReadFile(classPath)
	if err != nil {
		t.Fatal(err)
	}
	dec, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile: %v", err)
	}
	t.Logf("DECOMPILED:\n%s", dec)

	// Round-trip: recompile the decompiled source and re-run; the program output must match.
	dec2 := strings.Replace(dec, "package defaultpackagename;", "", 1)
	rtDir := t.TempDir()
	rtSrc := filepath.Join(rtDir, "SwitchFT.java")
	if err := os.WriteFile(rtSrc, []byte(dec2), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-d", rtDir, rtSrc).CombinedOutput(); err != nil {
		t.Fatalf("recompile decompiled source failed: %v\n%s\nSOURCE:\n%s", err, out, dec2)
	}
	gotOut, err := exec.Command(java, "-cp", rtDir, "SwitchFT").CombinedOutput()
	if err != nil {
		t.Fatalf("run decompiled: %v\n%s", err, gotOut)
	}
	t.Logf("DECOMPILED OUTPUT: %s", gotOut)
	if string(gotOut) != string(wantOut) {
		t.Fatalf("round-trip output mismatch:\n original = %q\n decompiled = %q", wantOut, gotOut)
	}
}
