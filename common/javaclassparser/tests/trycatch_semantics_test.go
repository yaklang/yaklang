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

// tryCatchBatterySource exercises the exception-handling shapes whose reconstruction is supported:
// a basic catch, a multi-catch union (A | B), try-finally with no catch (return value computed before
// the duplicated finally runs), a finally that returns (overriding the try return), try-catch inside a
// loop, and reading the caught exception's message. Every method returns a value folded into the
// deterministic fingerprint, so the original and the decompiled+recompiled class are compared by
// execution. Shapes that still degrade to a safe stub or miscompile (try-catch-FINALLY, separate
// catch clauses, nested try with rethrow, try-with-resources) live in hardTryCatchSource and are
// guarded for "no corruption" instead.
const tryCatchBatterySource = `public class TryCatchBattery {
    static int basicCatch(int x){
        try { return 100/x; } catch(ArithmeticException e){ return -1; }
    }
    static String multiCatchUnion(Object o){
        try {
            if(o==null) throw new IllegalArgumentException("x");
            return o.toString().substring(0,2);
        } catch(IllegalArgumentException | IndexOutOfBoundsException e){
            return "C:"+e.getClass().getSimpleName();
        }
    }
    static int tryFinallyNoCatch(int n){
        int s=0;
        try { for(int i=1;i<=n;i++) s+=i; return s; } finally { s=-1; }
    }
    static int finallyReturn(int x){
        try { return x; } finally { return x+1; }
    }
    static int loopTryCatch(int n){
        int s=0;
        for(int i=0;i<n;i++){
            try { s += 10/(i-2); } catch(ArithmeticException e){ s += 1000; }
        }
        return s;
    }
    static String exMessage(int x){
        try { if(x<0) throw new RuntimeException("neg"); return "ok"; }
        catch(RuntimeException e){ return e.getMessage(); }
    }

    public static void main(String[] z){
        StringBuilder sb=new StringBuilder();
        sb.append(basicCatch(0)).append(",");
        sb.append(multiCatchUnion(null)).append(",");
        sb.append(tryFinallyNoCatch(3)).append(",");
        sb.append(finallyReturn(5)).append(",");
        sb.append(loopTryCatch(5)).append(",");
        sb.append(exMessage(-1));
        System.out.println(sb.toString());
    }
}`

// hardTryCatchSource collects exception-handling shapes whose CFG structuring is not yet complete:
// try-catch-FINALLY (javac inlines the finally block into the normal path, the catch path AND a
// synthetic catch(any) handler that rethrows - the duplicated-finally pattern), separate catch clauses
// (catch(A){} catch(B){}), nested try with rethrow, and try-with-resources (the synthetic
// close/addSuppressed handler whose loop-exit-after-close edge is currently dropped). These degrade to
// a safe stub or otherwise miscompile; this guard only requires that javac ACCEPTS the decompiled
// output, so a stub passes but a body that fails to type-check is caught. Once a shape is fixed it is
// promoted to tryCatchBatterySource for a full execution round-trip.
const hardTryCatchSource = `import java.io.*;

public class HardTryCatch {
    static String catchFinally(int x){
        StringBuilder sb = new StringBuilder();
        try { sb.append(10/x); } catch(ArithmeticException e){ sb.append("E"); } finally { sb.append("F"); }
        return sb.toString();
    }
    static int multiCatchClauses(int sel){
        try {
            if(sel==1) throw new IllegalArgumentException();
            if(sel==2) throw new IllegalStateException();
            return 0;
        } catch(IllegalArgumentException e){ return 1; }
          catch(IllegalStateException e){ return 2; }
    }
    static int nested(int x){
        try {
            try { return 10/x; }
            catch(ArithmeticException e){ throw new RuntimeException("inner"); }
        } catch(RuntimeException e){ return -2; }
    }
    static int rethrow(int x){
        try {
            try { if(x<0) throw new IllegalStateException(); return x; }
            catch(IllegalStateException e){ throw new RuntimeException("re", e); }
        } catch(RuntimeException e){ return e.getCause()!=null?-9:-8; }
    }
    static String withResource(String data){
        StringBuilder sb=new StringBuilder();
        try(StringReader r = new StringReader(data)){
            int c;
            while((c=r.read())!=-1) sb.append((char)c);
        } catch(IOException e){ sb.append("IO"); }
        return sb.toString();
    }
    public static void main(String[] z){
        System.out.println("" + catchFinally(0) + multiCatchClauses(2) + nested(0) + rethrow(-1) + withResource("abc"));
    }
}`

// TestTryCatchSemanticsRoundTrip compiles+runs the supported exception-handling battery for a
// ground-truth fingerprint, then decompiles, recompiles, and runs the result, asserting identical
// output. Try/catch structuring is a frequent source of control-flow corruption (dropped catch arms,
// finally duplicated into the body, swapped handlers), so an execution-level check is the most
// reliable guard. Gated on javac/java.
func TestTryCatchSemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping try/catch semantics round-trip")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "TryCatchBattery.java"), []byte(tryCatchBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	golden, ok := runJava(t, javac, java, origDir, "TryCatchBattery")
	if !ok {
		t.Fatalf("failed to compile/run the original battery")
	}
	t.Logf("golden fingerprint: %s", golden)

	raw, err := os.ReadFile(filepath.Join(origDir, "TryCatchBattery.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("a try/catch method degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", decompiled)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "TryCatchBattery.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	got, ok := runJava(t, javac, java, rtDir, "TryCatchBattery")
	if !ok {
		t.Fatalf("decompiled battery failed to compile/run\n----- decompiled -----\n%s", src)
	}
	if got != golden {
		t.Fatalf("try/catch semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, src)
	}
	t.Logf("try/catch semantics preserved: %s", got)
}

// TestTryCatchHardCasesNoCorruption guards the exception-handling shapes that still degrade to a
// stub (try-catch-finally, nested try with rethrow). It decompiles HardTryCatch and requires only
// that javac ACCEPTS the decompiled source: a safe stub compiles cleanly and passes, whereas silent
// corruption that produces an uncompilable body is rejected by javac and fails the test. Gated on javac.
func TestTryCatchHardCasesNoCorruption(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not available; skipping hard try/catch no-corruption guard")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "HardTryCatch.java"), []byte(hardTryCatchSource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", origDir, filepath.Join(origDir, "HardTryCatch.java")).CombinedOutput(); err != nil {
		t.Fatalf("failed to compile the original hard battery: %v\n%s", err, out)
	}

	raw, err := os.ReadFile(filepath.Join(origDir, "HardTryCatch.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	src := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`).ReplaceAllString(decompiled, "")

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "HardTryCatch.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", rtDir, filepath.Join(rtDir, "HardTryCatch.java")).CombinedOutput(); err != nil {
		t.Fatalf("decompiled hard try/catch battery does not compile (corruption, not a safe stub):\n%s\n----- decompiled -----\n%s", out, src)
	}
	t.Logf("hard try/catch cases decompiled without corruption (stub-or-correct)")
}
