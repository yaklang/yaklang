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

// lambdaBatterySource exercises the invokedynamic / LambdaMetafactory captured-variable handling that
// the decompiler must reconstruct: no-capture, single / multiple captured primitives, a captured String,
// a captured field via `this`, a boolean-returning SAM, and a static method reference. The crux is the
// captured variables - javac prepends them to the synthetic impl method's parameter list (in reverse of
// the operand-stack order), so a naive dump leaks them as extra lambda parameters and/or swaps them,
// changing behaviour. Each lambda is used exactly once so it inlines at the call site (with the
// functional-interface cast a bare lambda receiver needs); multi-use lambdas kept as locals hit a
// separate, pre-existing variable-numbering limitation that is out of scope here.
//
// The functional interfaces are deliberately custom and NON-generic (primitive/String signatures): the
// decompiler erases generics (List, not List<Integer>), so JDK interfaces like Function<Integer,Integer>
// cannot be recompiled - their erased raw form conflicts with the concrete-typed lambda bodies. Custom
// non-generic interfaces keep the types intact, isolating the captured-variable logic for an exact
// compile+run round-trip. They are top-level classes so each decompiled .class can be concatenated into
// one source file (nested-type nesting is a separate, unreconstructed concern).
const lambdaBatterySource = `public class LambdaBattery {
    static int noCapture(){
        IntSup s = () -> 42;
        return s.get();
    }
    static int oneCapture(int base){
        IntOp f = x -> x + base;
        return f.apply(10);
    }
    static int twoCapture(int a, int b){
        IntBiOp op = (x,y) -> x*a + y*b;
        return op.apply(2,3);
    }
    static String stringCapture(String prefix){
        StrOp f = s -> prefix + s;
        return f.apply("World");
    }
    static boolean predicate(int probe){
        IntPred even = x -> x % 2 == 0;
        return even.test(probe);
    }
    static int methodRef(int a, int b){
        IntBiOp op = LambdaBattery::addStatic;
        return op.apply(a,b);
    }
    static int addStatic(int a, int b){ return a + b; }

    int field = 7;
    int instanceCapture(int x){
        IntOp f = y -> y + field + x;
        return f.apply(100);
    }

    public static void main(String[] z){
        StringBuilder sb = new StringBuilder();
        sb.append(noCapture()).append(",");
        sb.append(oneCapture(5)).append(",");
        sb.append(twoCapture(1,2)).append(",");
        sb.append(stringCapture("Hello")).append(",");
        sb.append(predicate(4)).append(",");
        sb.append(methodRef(20,30)).append(",");
        sb.append(new LambdaBattery().instanceCapture(100));
        System.out.println(sb.toString());
    }
}
interface IntSup { int get(); }
interface IntOp { int apply(int x); }
interface IntBiOp { int apply(int x, int y); }
interface StrOp { String apply(String s); }
interface IntPred { boolean test(int x); }`

// decompileAllToOneSource decompiles every .class in dir and concatenates the bodies (package line
// stripped) into a single source string. The battery's functional interfaces are top-level classes, so
// their decompiled forms compose into one compilable file alongside the main class.
func decompileAllToOneSource(t *testing.T, dir string) (string, bool) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	var classFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".class") {
			classFiles = append(classFiles, e.Name())
		}
	}
	sort.Strings(classFiles)
	pkgLine := regexp.MustCompile(`(?m)^package\s+[^;]+;\s*$`)
	var parts []string
	for _, cf := range classFiles {
		raw, err := os.ReadFile(filepath.Join(dir, cf))
		if err != nil {
			continue
		}
		dec, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Logf("decompile %s failed: %v", cf, err)
			return "", false
		}
		if strings.Contains(dec, javaclassparser.DecompileStubMarker) {
			t.Logf("decompiled %s contains a stub:\n%s", cf, dec)
			return "", false
		}
		parts = append(parts, pkgLine.ReplaceAllString(dec, ""))
	}
	return strings.Join(parts, "\n"), true
}

// TestLambdaSemanticsRoundTrip compiles+runs the lambda battery for a ground-truth fingerprint, then
// decompiles every class, concatenates them into one source, recompiles, and runs it, asserting
// identical output. Captured-variable handling (parameter arity + body substitution) is invisible to a
// syntax-only check but changes behaviour, so an execution round-trip is the reliable guard. Gated on
// javac/java.
func TestLambdaSemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping lambda semantics round-trip")
	}

	origDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(origDir, "LambdaBattery.java"), []byte(lambdaBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	golden, ok := runJava(t, javac, java, origDir, "LambdaBattery")
	if !ok {
		t.Fatalf("failed to compile/run the original battery")
	}
	t.Logf("golden fingerprint: %s", golden)

	src, ok := decompileAllToOneSource(t, origDir)
	if !ok {
		t.Fatalf("decompilation produced a stub or error; cannot verify semantics")
	}

	rtDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rtDir, "LambdaBattery.java"), []byte(src), 0644); err != nil {
		t.Fatalf("write decompiled source: %v", err)
	}
	got, ok := runJava(t, javac, java, rtDir, "LambdaBattery")
	if !ok {
		t.Fatalf("decompiled battery failed to compile/run\n----- decompiled -----\n%s", src)
	}
	if got != golden {
		t.Fatalf("lambda semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, src)
	}
	t.Logf("lambda semantics preserved: %s", got)
}
