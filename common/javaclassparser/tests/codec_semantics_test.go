package tests

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

//go:embed testdata/codec/CodecAlgorithms.java
var codecAlgorithmsSrc string

// compileAndRunJava compiles the given .java file (plus any siblings in dir) with javac into dir,
// then runs the named main class with java, returning stdout (trimmed) and ok.
func compileAndRunJava(t *testing.T, javac, java, dir, mainClass string, srcContent string) (string, bool) {
	t.Helper()
	srcPath := filepath.Join(dir, "CodecAlgorithms.java")
	if err := os.WriteFile(srcPath, []byte(srcContent), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out, err := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", dir, srcPath).CombinedOutput()
	if err != nil {
		t.Logf("javac failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	out2, err := exec.Command(java, "-cp", dir, mainClass).CombinedOutput()
	if err != nil {
		t.Logf("java run failed in %s: %v\n%s", dir, err, string(out2))
		return string(out2), false
	}
	return strings.TrimSpace(string(out2)), true
}

// TestCodecSemanticsRoundTrip is the algorithm-correctness oracle: it compiles a battery of
// self-contained crypto/codec algorithms (MD5, CRC32, CRC32C, MurmurHash2/3, XXHash32, Base64,
// MD5-crypt) with javac to produce ground-truth bytecode, then decompiles that bytecode with Yak,
// recompiles the decompiled source, and runs it with the SAME driver. The two fingerprints must be
// byte-identical. A divergence means the decompiler corrupted a computation (shift/arith promotion,
// narrowing cast, control-flow inversion, dropped statement) that passes ANTLR syntax validation
// but changes program behavior. This is the kind of silent bug only behavioral differential testing
// catches. Gated on javac/java so a JDK-less CI skips cleanly; otherwise it is a HARD correctness
// gate (no opt-in env var) and a divergence fails the build.
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

	origDir := t.TempDir()
	golden, ok := compileAndRunJava(t, javac, java, origDir, "codec.CodecAlgorithms", codecAlgorithmsSrc)
	if !ok {
		t.Fatalf("failed to compile/run original codec battery:\n%s", golden)
	}
	t.Logf("golden codec fingerprint: %s", golden)

	// sanity: the golden fingerprint must contain the canonical MD5 of "" (d41d8cd9...),
	// otherwise the test is verifying the wrong thing (a regression in the oracle source itself).
	if !strings.Contains(golden, "d41d8cd98f00b204e9800998ecf8427e") {
		t.Fatalf("golden fingerprint is missing the canonical MD5 of the empty string; the oracle source is broken: %s", golden)
	}

	raw, err := os.ReadFile(filepath.Join(origDir, "codec", "CodecAlgorithms.class"))
	if err != nil {
		t.Fatalf("read compiled class: %v", err)
	}
	decompiled, err := javaclassparser.Decompile(raw)
	if err != nil {
		t.Fatalf("decompile failed: %v", err)
	}
	if strings.Contains(decompiled, javaclassparser.DecompileStubMarker) {
		t.Fatalf("a codec method degraded to a stub; cannot verify semantics\n----- decompiled -----\n%s", decompiled)
	}
	// Keep the decompiled source's `package codec;` so the recompiled class lands in the same
	// package as the golden and the driver resolves identically.
	src := decompiled

	rtDir := t.TempDir()
	got, ok := compileAndRunJava(t, javac, java, rtDir, "codec.CodecAlgorithms", src)
	if !ok {
		t.Fatalf("decompiled codec battery failed to compile/run\n----- decompiled -----\n%s\n----- javac/java output -----\n%s", decompiled, got)
	}
	if got != golden {
		t.Fatalf("codec semantics diverged after decompilation\n  golden: %s\n  got:    %s\n----- decompiled -----\n%s", golden, got, decompiled)
	}
	t.Logf("codec semantics preserved: %s", got)
}
