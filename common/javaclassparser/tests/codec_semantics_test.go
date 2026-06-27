package tests

import (
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// codecBatteryFS holds every self-contained algorithm battery used by the differential-execution
// oracle. Each *.java file declares `package codec;` and a single public top-level class whose
// main() prints a deterministic fingerprint. The class name MUST equal the file name.
//
//go:embed testdata/codec/*.java
var codecBatteryFS embed.FS

// batterySanity maps a battery class name to a constant that MUST appear in its golden fingerprint.
// This catches a broken oracle source independently of the decompiler (e.g. an algorithm that was
// mis-typed in the .java itself), so a green test really means "decompiler preserved correct math".
var batterySanity = map[string]string{
	// MD5("") — the canonical empty-string digest; if this is missing the oracle source is broken.
	"CodecAlgorithms": "d41d8cd98f00b204e9800998ecf8427e",
	// SHA-256("") — the canonical empty-string digest; guards the from-scratch SHA-256 oracle source.
	"Sha256Algorithms": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
	// SHA-512("") — the canonical empty-string digest; guards the from-scratch SHA-512 oracle source.
	"Sha512Algorithms": "cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e",
}

// compileAndRunJavaBattery writes src as <dir>/<className>.java, compiles it with javac into dir,
// then runs codec.<className> with java and returns trimmed stdout + ok. A single .java is expected
// to compile to exactly one top-level .class (batteries avoid extra top-level / inner classes so the
// single-class decompile round-trip stays well-defined).
func compileAndRunJavaBattery(t *testing.T, javac, java, dir, className, src string) (string, bool) {
	t.Helper()
	srcPath := filepath.Join(dir, className+".java")
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out, err := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", dir, srcPath).CombinedOutput()
	if err != nil {
		t.Logf("javac failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	out2, err := exec.Command(java, "-cp", dir, "codec."+className).CombinedOutput()
	if err != nil {
		t.Logf("java run failed in %s: %v\n%s", dir, err, string(out2))
		return string(out2), false
	}
	return strings.TrimSpace(string(out2)), true
}

// roundTripBattery is the per-battery oracle: javac compiles the source to ground-truth bytecode and
// runs it to get the golden fingerprint; Yak decompiles that bytecode; the decompiled source is
// recompiled and run with the SAME driver. The two fingerprints must be byte-identical. A divergence
// means the decompiler corrupted a computation (shift/arith promotion, narrowing cast, control-flow
// inversion, dropped statement, wrong index, long/switch/instanceof miscompile) that passes ANTLR
// syntax validation but changes program behavior — the kind of silent bug only behavioral
// differential testing catches.
func roundTripBattery(t *testing.T, javac, java, className, src, sanitySub string) {
	t.Helper()
	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, src)
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	t.Logf("golden fingerprint [%s]: %s", className, golden)
	if sanitySub != "" && !strings.Contains(golden, sanitySub) {
		t.Fatalf("golden fingerprint for %s is missing the expected sanity constant %q; the oracle source is broken: %s", className, sanitySub, golden)
	}

	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile %s failed: %v", className, err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("battery %s degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", className, decompiled)
	}

	rtDir := t.TempDir()
	got, ok := compileAndRunJavaBattery(t, javac, java, rtDir, className, decompiled)
	if !ok {
		t.Fatalf("decompiled battery %s failed to compile/run\n----- decompiled -----\n%s\n----- javac/java output -----\n%s", className, decompiled, got)
	}
	if got != golden {
		t.Fatalf("battery %s semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", className, golden, got, decompiled)
	}
	t.Logf("semantics preserved [%s]: %s", className, got)
}

// TestCodecSemanticsRoundTrip runs the differential-execution oracle over every algorithm battery in
// testdata/codec/. Gated on javac/java so a JDK-less CI skips cleanly; otherwise it is a HARD
// correctness gate (no opt-in env var) and any divergence fails the build.
//
// HISTORY: an earlier local-slot-reuse defect made md5()/xxHash32() emit `int var_1 = var_1 + ...`
// ("variable might not have been initialized") and kept this test behind CODEC_STRICT=1. The root
// cause (post-branch reassignment `x = f(x)` of a local assigned only inside if/else arms was bound
// to a fresh self-referencing id) is fixed in redirectPostBlockReassignments (rewriter/rewrite_var.go),
// so the gate is now always on.
func TestCodecSemanticsRoundTrip(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping codec semantics round-trip")
	}

	entries, err := codecBatteryFS.ReadDir("testdata/codec")
	if err != nil {
		t.Fatalf("read battery dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".java") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".java"))
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("no algorithm batteries found under testdata/codec")
	}

	for _, className := range names {
		src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
		if err != nil {
			t.Fatalf("read battery %s: %v", className, err)
		}
		className := className
		srcStr := string(src)
		t.Run(className, func(t *testing.T) {
			roundTripBattery(t, javac, java, className, srcStr, batterySanity[className])
		})
	}
}
