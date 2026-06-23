package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// innerClassBatterySource exercises the full inner/nested-class family that javac compiles into
// separate .class files (Outer$Inner, Outer$1, Outer$1Local, Outer$StaticNested$Deeper, ...):
//   - static nested class (and a doubly-nested static class)
//   - non-static inner class reading an outer field and calling an outer private method
//   - anonymous class implementing an interface (with captured local)
//   - anonymous class extending an abstract class (with captured local)
//   - local class capturing a final local
//   - lambdas (no capture, with capture, Supplier)
//   - method references in streams (instance, bound, unbound)
//   - nested enum with a method, generic inner class, generic static method
//
// Because the public Decompile API works one .class at a time, a whole-program recompile/run
// round-trip is not possible here (an anonymous Outer$1 cannot be recompiled standalone). What we
// CAN guarantee - and what this battery enforces against regressions - is that EVERY produced class
// decompiles without an error, never degrades to a stub, and the result is accepted by the Java
// frontend (no syntax error / no corruption). This protects the inner-class family during the
// higher-risk CFG/control-flow refactors.
const innerClassBatterySource = `import java.util.*;
import java.util.function.*;

public class InnerBattery {
    private int field = 42;
    static int sfield = 7;

    static class StaticNested {
        int v;
        StaticNested(int v){ this.v = v; }
        int twice(){ return v*2; }
        static class Deeper { int z(){ return 9; } }
    }

    class Inner {
        int read(){ return field + 1; }
        int readOuterMethod(){ return helper() + field; }
    }

    private int helper(){ return 100; }

    interface Op { int apply(int x); }
    interface Sup { int get(); }
    static abstract class Base { abstract int v(); int doubled(){ return v()*2; } }

    enum Color { RED, GREEN, BLUE; int idx(){ return ordinal(); } }

    static <T extends Comparable<T>> T maxOf(T a, T b){ return a.compareTo(b)>=0?a:b; }

    class GenBox<T> {
        T val;
        GenBox(T v){ val = v; }
        T get(){ return val; }
    }

    int useAnon(int base){
        Op o = new Op(){
            public int apply(int x){ return x + base; }
        };
        return o.apply(10);
    }

    int useAnonAbstract(int n){
        Base b = new Base(){ int v(){ return n; } };
        return b.doubled();
    }

    int useLambda(int base){
        Op o = x -> x * base + sfield;
        return o.apply(5);
    }

    int useLambdaNoCap(){
        Op o = x -> x + 1;
        return o.apply(41);
    }

    int useSupplier(int seed){
        Sup s = () -> seed * 2;
        return s.get();
    }

    int useLocal(final int seed){
        class Local {
            int compute(){ return seed * 3; }
        }
        return new Local().compute();
    }

    List<String> useStreams(List<String> in){
        List<String> r = new ArrayList<>();
        in.stream().filter(s -> s.length() > 2).map(String::toUpperCase).forEach(r::add);
        return r;
    }

    int useMethodRef(List<Integer> xs){
        return xs.stream().mapToInt(Integer::intValue).sum();
    }

    public static void main(String[] a){
        InnerBattery p = new InnerBattery();
        StringBuilder sb = new StringBuilder();
        sb.append(new StaticNested(5).twice()).append(",");
        sb.append(new StaticNested.Deeper().z()).append(",");
        sb.append(p.new Inner().read()).append(",");
        sb.append(p.new Inner().readOuterMethod()).append(",");
        sb.append(p.useAnon(3)).append(",");
        sb.append(p.useAnonAbstract(8)).append(",");
        sb.append(p.useLambda(4)).append(",");
        sb.append(p.useLambdaNoCap()).append(",");
        sb.append(p.useSupplier(11)).append(",");
        sb.append(p.useLocal(6)).append(",");
        sb.append(Color.GREEN.idx()).append(",");
        sb.append(maxOf(3,9)).append(",");
        sb.append(p.new GenBox<String>("hi").get()).append(",");
        sb.append(p.useMethodRef(Arrays.asList(1,2,3))).append(",");
        sb.append(p.useStreams(Arrays.asList("a","abc","de","fghi")));
        System.out.println(sb.toString());
    }
}`

// TestInnerClassFamilyNoStubNoSyntaxError compiles the inner-class battery, then decompiles EVERY
// .class javac produced (outer + every nested/anonymous/local/lambda-host class) and asserts each
// one: (1) decompiles without an error, (2) does not degrade to a stub, (3) is accepted by the Java
// frontend. Gated on javac so a JDK-less environment simply skips.
func TestInnerClassFamilyNoStubNoSyntaxError(t *testing.T) {
	javac, err := exec.LookPath("javac")
	if err != nil {
		t.Skip("javac not available; skipping inner-class family battery")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "InnerBattery.java"), []byte(innerClassBatterySource), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-d", dir, filepath.Join(dir, "InnerBattery.java")).CombinedOutput(); err != nil {
		t.Fatalf("failed to compile inner-class battery: %v\n%s", err, out)
	}

	var classes []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".class") {
			classes = append(classes, e.Name())
		}
	}
	sort.Strings(classes)
	if len(classes) < 10 {
		t.Fatalf("expected the inner-class battery to produce many classes, got %d: %v", len(classes), classes)
	}
	t.Logf("decompiling %d produced classes: %v", len(classes), classes)

	var failures []string
	for _, c := range classes {
		raw, rerr := os.ReadFile(filepath.Join(dir, c))
		if rerr != nil {
			failures = append(failures, c+": read error: "+rerr.Error())
			continue
		}
		var out string
		var derr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					derr = fmt.Errorf("panic: %v", r)
				}
			}()
			out, derr = javaclassparser.Decompile(raw)
		}()
		if derr != nil {
			failures = append(failures, c+": decompile error: "+derr.Error())
			continue
		}
		if strings.Contains(out, javaclassparser.DecompileStubMarker) {
			failures = append(failures, c+": degraded to a stub (incomplete decompilation)")
			continue
		}
		if _, ferr := java2ssa.Frontend(out); ferr != nil {
			failures = append(failures, c+": frontend rejected decompiled output: "+firstReason(ferr.Error()))
			continue
		}
	}
	if len(failures) > 0 {
		t.Fatalf("inner-class family decompilation regressed (%d/%d classes):\n  %s", len(failures), len(classes), strings.Join(failures, "\n  "))
	}
	t.Logf("all %d inner-class-family classes decompiled cleanly (no stub, valid syntax)", len(classes))
}
