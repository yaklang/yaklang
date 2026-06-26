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

// loopBatterySource exercises the loop shapes that the CFG reconstructor must keep semantically
// faithful: ascending/descending for, while, do-while, nested loops, continue/break, labeled
// break/continue, an infinite-with-break, a foreach over an array, and mixed continue+break.
// Every method takes plain int parameters and builds any array internally with `new int[n]` so the
// test isolates loop control flow from the unrelated array-initializer reconstruction. main prints a
// single deterministic fingerprint so the original and the decompiled+recompiled class can be
// compared by execution output - the strongest possible correctness check for control flow.
const loopBatterySource = `public class LoopBattery {
    static int forAsc(int n){ int s=0; for(int i=0;i<n;i++){ s+=i; } return s; }
    static int forDesc(int n){ int s=0; for(int i=n;i>0;i--){ s+=i; } return s; }
    static int whileSum(int n){ int s=0,i=0; while(i<n){ s+=i; i++; } return s; }
    static int doWhileSum(int n){ int s=0,i=0; do{ s+=i; i++; }while(i<n); return s; }
    static int nested(int n){ int s=0; for(int i=0;i<n;i++){ for(int j=0;j<n;j++){ s+=i*j; } } return s; }
    static int continueEven(int n){ int s=0; for(int i=0;i<n;i++){ if(i%2!=0){ continue; } s+=i; } return s; }
    static int breakAt(int n,int lim){ int s=0; for(int i=0;i<n;i++){ if(s>lim){ break; } s+=i; } return s; }
    static int infinite(int n){ int s=0,i=0; while(true){ if(i>=n){ break; } s+=i; i++; } return s; }
    static int reverseWhile(int n){ int s=0,i=n; while(i>0){ s+=i; i--; } return s; }
    static long bigSum(int n){ long s=0; for(int i=0;i<n;i++){ s += (long)i*i; } return s; }
    static int foreachLike(int n){ int[] a=new int[n]; for(int i=0;i<n;i++){ a[i]=i*3; } int s=0; for(int x:a){ s+=x; } return s; }
    static int triNested(int n){ int s=0; for(int i=0;i<n;i++){ for(int j=0;j<n;j++){ for(int k=0;k<n;k++){ s++; } } } return s; }
    static int mixedContinueBreak(int n){ int s=0,i=0; while(i<n){ i++; if(i%3==0){ continue; } if(i>n-1){ break; } s+=i; } return s; }

    public static void main(String[] a){
        StringBuilder sb=new StringBuilder();
        sb.append(forAsc(5)).append(",");
        sb.append(forDesc(5)).append(",");
        sb.append(whileSum(6)).append(",");
        sb.append(doWhileSum(6)).append(",");
        sb.append(nested(4)).append(",");
        sb.append(continueEven(8)).append(",");
        sb.append(breakAt(100,20)).append(",");
        sb.append(infinite(7)).append(",");
        sb.append(reverseWhile(5)).append(",");
        sb.append(bigSum(10)).append(",");
        sb.append(foreachLike(6)).append(",");
        sb.append(triNested(4)).append(",");
        sb.append(mixedContinueBreak(10));
        System.out.println(sb.toString());
    }
}`

// runJava compiles every .java file in dir and runs mainClass, returning trimmed stdout.
func runJava(t *testing.T, javac, java, dir, mainClass string) (string, bool) {
	t.Helper()
	var javaFiles []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".java") {
			javaFiles = append(javaFiles, filepath.Join(dir, e.Name()))
		}
	}
	args := append([]string{"-J-Duser.language=en", "-nowarn", "-d", dir}, javaFiles...)
	if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
		t.Logf("javac failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	out, err := exec.Command(java, "-cp", dir, mainClass).CombinedOutput()
	if err != nil {
		t.Logf("java run failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	return strings.TrimSpace(string(out)), true
}

// TestLoopSemanticsRoundTrip is the gold-standard loop correctness check: it compiles and runs the
// original battery to obtain the ground-truth fingerprint, then decompiles the compiled class,
// recompiles the decompiled source, runs it, and asserts the two fingerprints are identical. This
// catches control-flow inversions (e.g. a loop condition rendered with body and exit swapped) that
// pass ANTLR syntax validation but change program behavior. Gated on javac/java so a JDK-less CI
// simply skips instead of failing.
func TestLoopSemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping loop semantics round-trip")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "LoopBattery.java"), []byte(loopBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	golden, ok := runJava(t, javac, java, origDir, "LoopBattery")
	if !ok {
		t.Fatalf("failed to compile/run the original battery")
	}
	t.Logf("golden fingerprint: %s", golden)

	raw, err := os.ReadFile(filepath.Join(origDir, "LoopBattery.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("a loop method degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", decompiled)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "LoopBattery.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	got, ok := runJava(t, javac, java, rtDir, "LoopBattery")
	if !ok {
		t.Fatalf("decompiled battery failed to compile/run\n----- decompiled -----\n%s", src)
	}
	if got != golden {
		t.Fatalf("loop semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, src)
	}
	t.Logf("loop semantics preserved: %s", got)
}
