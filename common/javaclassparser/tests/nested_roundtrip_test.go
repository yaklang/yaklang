package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// nestedRoundTripSrc is a self-contained multi-class battery for nested-type reference rendering.
// It compiles (via javac) to three classes: NestHolder, NestHolder$Context, NestHolder$Mode, where
// NestHolder.combine() references both nested types in its SIGNATURE and body. This is the exact
// shape of the commons-codec BaseNCodec$Context idiom that motivated the nested-type investigation.
const nestedRoundTripSrc = `package codec;

public class NestHolder {
    public static final class Context {
        final int seed;
        int acc;
        Context(int seed) { this.seed = seed; this.acc = seed; }
        int mix(int v) { this.acc = this.acc * 31 + v; return this.acc; }
    }

    enum Mode { ADD, MUL }

    static int combine(Context ctx, Mode mode, int[] data) {
        int r = 0;
        for (int i = 0; i < data.length; i++) {
            int m = ctx.mix(data[i]);
            if (mode == Mode.ADD) {
                r += m;
            } else {
                r ^= m;
            }
        }
        return r;
    }

    public static void main(String[] args) {
        int[] d = {1, 2, 3, 4, 5};
        StringBuilder sb = new StringBuilder();
        sb.append(combine(new Context(7), Mode.ADD, d)).append(';');
        sb.append(combine(new Context(3), Mode.MUL, d)).append(';');
        System.out.println(sb.toString());
    }
}
`

// assembleFlatNestedUnits replicates the harness's flat-$ nested-class assembly: Yak decompiles each
// inner class as a standalone top-level `class Outer$Inner` unit, so to recompile we append every
// inner unit body (package/import preamble stripped, leading visibility demoted) into the outer's
// .java file. References to `Outer$Inner` then resolve to these flat top-level types.
func assembleFlatNestedUnits(outer string, units map[string]string) string {
	var sb strings.Builder
	if src, ok := units[outer]; ok {
		sb.WriteString(src)
	}
	for stem, src := range units {
		if stem == outer {
			continue
		}
		body := stripUnitPreamble(src)
		if body != "" {
			sb.WriteString("\n\n")
			sb.WriteString(body)
		}
	}
	return sb.String()
}

// TestNestedTypeReferenceRoundTrip proves Yak's flat-$ nested-class representation round-trips at the
// whole-program level: a class whose method SIGNATURE references sibling nested types (Outer$Inner,
// Outer$Mode) decompiles, recompiles (all units in the outer's file), and runs to the SAME
// fingerprint as the javac original. This is the realistic "decompile jar -> recompile -> run"
// workflow; the single-file standalone recompile of such a class is intentionally NOT the contract,
// because a lone `Base32.java` referencing binary `BaseNCodec$Context` cannot resolve a nested type
// it does not also declare.
func TestNestedTypeReferenceRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping nested-type round-trip")
	}

	origDir := t.TempDir()
	srcPath := filepath.Join(origDir, "NestHolder.java")
	if err := os.WriteFile(srcPath, []byte(nestedRoundTripSrc), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", origDir, srcPath).CombinedOutput(); err != nil {
		t.Fatalf("javac original failed: %v\n%s", err, out)
	}
	golden, err := exec.Command(java, "-cp", origDir, "codec.NestHolder").CombinedOutput()
	if err != nil {
		t.Fatalf("run original failed: %v\n%s", err, golden)
	}
	goldenStr := strings.TrimSpace(string(golden))
	t.Logf("golden fingerprint: %s", goldenStr)

	// Decompile every produced .class, keyed by flat stem (NestHolder, NestHolder$Context, ...).
	classDir := filepath.Join(origDir, "codec")
	entries, err := os.ReadDir(classDir)
	if err != nil {
		t.Fatalf("read class dir: %v", err)
	}
	units := map[string]string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(classDir, e.Name()))
		if err != nil {
			t.Fatalf("read class %s: %v", e.Name(), err)
		}
		dec, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Fatalf("decompile %s failed: %v", e.Name(), err)
		}
		if strings.Contains(dec, javaclassparser.DecompileStubMarker) {
			t.Fatalf("class %s degraded to a stub:\n%s", e.Name(), dec)
		}
		units[strings.TrimSuffix(e.Name(), ".class")] = dec
	}
	if _, ok := units["NestHolder"]; !ok {
		t.Fatalf("missing decompiled outer class; got units %v", keysOf(units))
	}

	assembled := assembleFlatNestedUnits("NestHolder", units)
	t.Logf("assembled NestHolder.java:\n%s", assembled)

	rtDir := t.TempDir()
	rtSrc := filepath.Join(rtDir, "NestHolder.java")
	if err := os.WriteFile(rtSrc, []byte(assembled), 0o644); err != nil {
		t.Fatalf("write assembled: %v", err)
	}
	if out, err := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", rtDir, rtSrc).CombinedOutput(); err != nil {
		t.Fatalf("recompile decompiled NestHolder failed: %v\n%s\n----- source -----\n%s", err, out, assembled)
	}
	got, err := exec.Command(java, "-cp", rtDir, "codec.NestHolder").CombinedOutput()
	if err != nil {
		t.Fatalf("run recompiled failed: %v\n%s", err, got)
	}
	gotStr := strings.TrimSpace(string(got))
	if gotStr != goldenStr {
		t.Fatalf("nested round-trip diverged\n  golden: %s\n  got:    %s\n----- source -----\n%s", goldenStr, gotStr, assembled)
	}
	t.Logf("nested round-trip preserved: %s", gotStr)
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
