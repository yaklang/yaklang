package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// arrayBatterySource exercises the array-literal / initializer shapes that javac compiles with the
// `newarray; dup; i; v; Xastore ...` idiom. Before the fold fix these dropped every element store
// (rendering `new int[3]` or losing the local declaration entirely, producing uncompilable code);
// each method now must round-trip through decompile+recompile with identical runtime output:
//   - a literal assigned to a local then read by index
//   - a literal passed inline as a method argument
//   - a literal used as a varargs/collection source (object array)
//   - a literal returned directly
//   - a String[] literal with brace syntax
//   - a multi-dimensional literal
//   - a SPARSE sized array (new int[N] with scattered assignments) that must NOT be folded into a
//     literal, since only some indices are set
const arrayBatterySource = `import java.util.*;
public class ArrayBattery {
    static int localLiteral(){
        int[] a = new int[]{1,2,3};
        return a[0]+a[1]+a[2];
    }
    static int inlineLiteral(){
        return sum(new int[]{4,5,6});
    }
    static int sum(int[] a){ int s=0; for(int v:a) s+=v; return s; }
    static String varargs(){
        return String.join("-", Arrays.asList("x","yy","zzz"));
    }
    static int[] returnLiteral(){
        return new int[]{7,8,9};
    }
    static String stringArray(){
        String[] s = {"p","q"};
        return s[0]+s[1];
    }
    static int matrix(){
        int[][] m = {{1,2},{3,4}};
        return m[0][0]+m[0][1]+m[1][0]+m[1][1];
    }
    static int sparse(){
        int[] a = new int[10];
        a[0]=5; a[5]=9;
        return a[0]+a[5]+a.length;
    }
    static int byteLiteral(){
        byte[] b = {10,20,30};
        return b[0]+b[1]+b[2];
    }

    public static void main(String[] z){
        StringBuilder sb = new StringBuilder();
        sb.append(localLiteral()).append(",");
        sb.append(inlineLiteral()).append(",");
        sb.append(varargs()).append(",");
        sb.append(Arrays.toString(returnLiteral())).append(",");
        sb.append(stringArray()).append(",");
        sb.append(matrix()).append(",");
        sb.append(sparse()).append(",");
        sb.append(byteLiteral());
        System.out.println(sb.toString());
    }
}`

// TestArrayLiteralSemanticsRoundTrip compiles+runs the array-literal battery for a ground-truth
// fingerprint, then decompiles, recompiles, and runs the result, asserting identical output.
// Element stores folded into the wrong place (or dropped) silently change behaviour, so an
// execution-level check is the only reliable guard. Gated on javac/java.
func TestArrayLiteralSemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping array-literal semantics round-trip")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "ArrayBattery.java"), []byte(arrayBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	golden, ok := runJava(t, javac, java, origDir, "ArrayBattery")
	if !ok {
		t.Fatalf("failed to compile/run the original battery")
	}
	t.Logf("golden fingerprint: %s", golden)

	raw, err := os.ReadFile(filepath.Join(origDir, "ArrayBattery.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("an array method degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", decompiled)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "ArrayBattery.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	got, ok := runJava(t, javac, java, rtDir, "ArrayBattery")
	if !ok {
		t.Fatalf("decompiled battery failed to compile/run\n----- decompiled -----\n%s", src)
	}
	if got != golden {
		t.Fatalf("array semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, src)
	}
	t.Logf("array-literal semantics preserved: %s", got)
}
