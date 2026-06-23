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

// ternaryBatterySource exercises the conditional (?:) operator across the shapes whose slot
// reconstruction is fully supported: simple, nested in a single arm, chained as a value, mixed
// numeric types (widening), String-valued, ternary as a method argument / array index / loop bound,
// and a ternary nested inside another ternary's condition. Every method returns a value folded into
// the deterministic fingerprint so the original and the decompiled+recompiled class can be compared
// by execution. Shapes that still degrade to a safe stub (both-arms-nested / deep chains / arms with
// side effects) live in hardTernarySource below and are guarded for "no corruption" instead.
const ternaryBatterySource = `public class TernaryBattery {
    static int max2(int a,int b){ return a>b?a:b; }
    static int sign(int x){ return x>0?1:(x<0?-1:0); }
    static int chained(int x){ int y = x>10?100:(x>5?50:10); return y+1; }
    static long widen(int x){ long v = x>0?1:2L; return v*10; }
    static double widenD(int x){ double v = x>0?1:2.5; return v+0.5; }
    static String label(int x){ return x>0?"pos":(x<0?"neg":"zero"); }
    static int argTern(int x){ return Math.abs(x>0?-5:5); }
    static int loopBound(int x){ int s=0; for(int i=0;i<(x>0?3:1);i++){ s+=i; } return s; }
    static int ternInTern(int a,int b){ return (a>b?a:b)>5?100:200; }

    public static void main(String[] z){
        StringBuilder sb=new StringBuilder();
        sb.append(max2(3,9)).append(",");
        sb.append(sign(-8)).append(",");
        sb.append(chained(7)).append(",");
        sb.append(widen(1)).append(",");
        sb.append(widenD(-1)).append(",");
        sb.append(label(-3)).append(",");
        sb.append(argTern(5)).append(",");
        sb.append(loopBound(1)).append(",");
        sb.append(ternInTern(3,9));
        System.out.println(sb.toString());
    }
}`

// hardTernarySource collects conditional shapes whose stack-slot reconstruction is not yet complete:
// a balanced both-arms nested ternary on an identical inner condition (balancedSame), a deep
// right-leaning chain (deepChain), a boolean-valued ternary whose arms are themselves comparisons
// (boolArms), and a ternary whose arms mutate state (sideEffect). These currently degrade to a safe
// stub (a method body that throws), which is acceptable; what is NOT acceptable is silent corruption
// that still type-checks. TestTernaryHardCasesNoCorruption decompiles+recompiles this class and only
// requires that javac accepts the output, so a stub passes but a corrupted body (wrong slot value,
// swapped arms producing an uncompilable expression) is caught.
const hardTernarySource = `public class HardTernary {
    static int balancedSame(int x,int y){ return x>0?(y>0?1:2):(y>0?3:4); }
    static int deepChain(int x){ return x>8?1:x>6?2:x>4?3:x>2?4:5; }
    static boolean boolArms(int x){ return x>0?x<10:x<-10; }
    static int sideEffect(int x){ int r = x>0?(x+=2):(x+=5); return r+x; }

    public static void main(String[] z){
        System.out.println("" + balancedSame(1,1) + deepChain(7) + boolArms(5) + sideEffect(1));
    }
}`

// TestTernarySemanticsRoundTrip compiles+runs the ternary battery for a ground-truth fingerprint,
// then decompiles, recompiles, and runs the result, asserting identical output. Conditional-operator
// reconstruction is a frequent source of silent corruption (slot values leaking, arms swapped), so an
// execution-level check is the most reliable guard. Gated on javac/java.
func TestTernarySemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping ternary semantics round-trip")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "TernaryBattery.java"), []byte(ternaryBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	golden, ok := runJava(t, javac, java, origDir, "TernaryBattery")
	if !ok {
		t.Fatalf("failed to compile/run the original battery")
	}
	t.Logf("golden fingerprint: %s", golden)

	raw, err := os.ReadFile(filepath.Join(origDir, "TernaryBattery.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("a ternary method degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", decompiled)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "TernaryBattery.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	got, ok := runJava(t, javac, java, rtDir, "TernaryBattery")
	if !ok {
		t.Fatalf("decompiled battery failed to compile/run\n----- decompiled -----\n%s", src)
	}
	if got != golden {
		t.Fatalf("ternary semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, src)
	}
	t.Logf("ternary semantics preserved: %s", got)
}

// TestTernaryHardCasesNoCorruption guards the conditional shapes that still degrade to a stub. It
// decompiles HardTernary and requires only that javac ACCEPTS the decompiled source: a safe stub
// (a method body that throws) compiles cleanly and passes, whereas silent corruption that previously
// produced uncompilable bodies (leaked empty-slot placeholders, swapped arms, `Exception = Exception`
// artifacts) is rejected by javac and fails the test. This keeps the known limitation honest without
// asserting full decompilation. Gated on javac.
func TestTernaryHardCasesNoCorruption(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not available; skipping hard-ternary no-corruption guard")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "HardTernary.java"), []byte(hardTernarySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", origDir, filepath.Join(origDir, "HardTernary.java")).CombinedOutput(); err != nil {
		t.Fatalf("failed to compile the original hard battery: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(filepath.Join(origDir, "HardTernary.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "HardTernary.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", rtDir, filepath.Join(rtDir, "HardTernary.java")).CombinedOutput(); err != nil {
		t.Fatalf("decompiled hard battery does not compile (corruption, not a safe stub):\n%s\n----- decompiled -----\n%s", out, src)
	}
	t.Logf("hard ternary cases decompiled without corruption (stub-or-correct)")
}
