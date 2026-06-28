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

// TestJumpEnteredTryCatchAnchorIsLoadBearing proves that anchorJumpEnteredTryCatch (the post-pass that
// repairs try-catch regions whose body is entered via a jump) is actually doing work, not dead code.
// It compiles the JumpEnteredTryCatch battery, decompiles it with the anchor pass DISABLED
// (JDEC_TRY_JUMP_ANCHOR_OFF=1) and asserts the result no longer round-trips (the dropped catch either
// makes the checked-exception method fail to recompile, or lets an unchecked exception escape main at
// runtime so the fingerprint diverges / the run crashes). It then re-enables the pass and asserts a
// clean, behavior-preserving round-trip. If someone deletes the anchor pass, the "guard off" branch
// starts passing and this test fails.
func TestJumpEnteredTryCatchAnchorIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping jump-entered try-catch anchor test")
	}

	const className = "JumpEnteredTryCatch"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	// Anchor pass OFF: jump-entered try-catch regions are dropped, so the round-trip must fail
	// (checked-exception method does not recompile, or an unchecked exception escapes main at runtime).
	os.Setenv("JDEC_TRY_JUMP_ANCHOR_OFF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_TRY_JUMP_ANCHOR_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the jump-entered try-catch anchor disabled, but it succeeded; the anchor pass is not load-bearing", className)
	}
	t.Logf("anchor OFF (expected failure) for %s: %s", className, got)

	// Anchor pass ON: clean, behavior-preserving round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the jump-entered try-catch anchor enabled; got: %s", className, got)
	}
	t.Logf("anchor ON (round-trip preserved) for %s: %s", className, got)
}

// TestClinitHoistBarrierIsLoadBearing proves the <clinit> contiguous-prefix hoist barrier actually
// does work. It decompiles the StaticInitForwardRef battery with the barrier DISABLED
// (JDEC_NO_CLINIT_HOIST_BARRIER=1) and asserts the result no longer round-trips (lifting
// `DERIVED = (BitSet) SAFE.clone()` to a field declaration forward-references SAFE and/or clones it
// before the set() loop, so javac rejects it or the fingerprint diverges), then re-enables the barrier
// and asserts a clean, behavior-preserving round-trip.
func TestClinitHoistBarrierIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping clinit hoist barrier test")
	}

	const className = "StaticInitForwardRef"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	os.Setenv("JDEC_NO_CLINIT_HOIST_BARRIER", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_CLINIT_HOIST_BARRIER")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the clinit hoist barrier disabled, but it succeeded; the barrier is not load-bearing", className)
	}
	t.Logf("barrier OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the clinit hoist barrier enabled; got: %s", className, got)
	}
	t.Logf("barrier ON (round-trip preserved) for %s: %s", className, got)
}

// TestClassLiteralSlotTypeIsLoadBearing proves the class-literal slot-typing fix is load-bearing. A
// class literal stored in a local (`Class<?> c = Long.class; c.getName();`) must declare the local
// as java.lang.Class, not the referenced class (Long). With the fix DISABLED
// (JDEC_NO_CLASSLIT_SLOT_TYPE=1) the local is declared `Long c = Long.class;` and every member read
// (c.getName()/c.isPrimitive()/c.getSimpleName()) fails to recompile ("cannot find symbol"), so the
// round-trip must fail; with the fix enabled the round-trip is byte-for-byte clean.
func TestClassLiteralSlotTypeIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping class-literal slot type test")
	}

	const className = "ClassLiteralRendering"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	os.Setenv("JDEC_NO_CLASSLIT_SLOT_TYPE", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_CLASSLIT_SLOT_TYPE")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with class-literal slot typing disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("classlit slot type OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with class-literal slot typing enabled; got: %s", className, got)
	}
	t.Logf("classlit slot type ON (round-trip preserved) for %s: %s", className, got)
}

// TestIincWidenDeclIsLoadBearing proves the iinc-target slot widen (Bug W) is load-bearing. An int
// local fed by baload is inferred as byte; a non-±1 iinc (`b += 256`) desugars to `b = b + 256`,
// which is a possible-lossy conversion for a byte declaration. With the fix DISABLED
// (JDEC_IINC_WIDEN_OFF=1) the decompiled method fails to recompile; with the fix the slot is
// declared int and the round-trip is byte-for-byte clean.
func TestIincWidenDeclIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping iinc widen decl test")
	}

	const className = "IincWidenDecl"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	os.Setenv("JDEC_IINC_WIDEN_OFF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_IINC_WIDEN_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with iinc-target widen disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("iinc-widen OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with iinc-target widen enabled; got: %s", className, got)
	}
	t.Logf("iinc-widen ON (round-trip preserved) for %s: %s", className, got)
}

// TestLoopCounterSlotReuseIsLoadBearing proves the iinc reaching-definition repair (Bug X) is
// load-bearing. A loop counter slot reused for a byte[] after the loop makes GetVar return the
// byte[] reincarnation at the iinc; with the fix DISABLED (JDEC_IINC_REACHING_OFF=1) the loop's
// `i++` renders as `someByteArray++` and fails to recompile; with the fix it binds to the int
// counter and the round-trip is byte-for-byte clean.
func TestLoopCounterSlotReuseIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping loop-counter slot-reuse test")
	}

	const className = "LoopCounterSlotReuse"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	os.Setenv("JDEC_IINC_REACHING_OFF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_IINC_REACHING_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with iinc reaching-definition repair disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("iinc-reaching OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with iinc reaching-definition repair enabled; got: %s", className, got)
	}
	t.Logf("iinc-reaching ON (round-trip preserved) for %s: %s", className, got)
}

// TestEmbeddedAssignDeclIntIsLoadBearing proves the embedded-assignment int-declaration fix is
// load-bearing. An embedded assignment in a condition (`(v = a[i]) == 0`) leaves the variable with
// no ordinary declaration, so the dumper must synthesize one. With the fix DISABLED
// (JDEC_NO_EMBED_ASSIGN_INT=1) it defaults to `Object v = null` and the int store / arithmetic read
// fail to recompile; with the fix it synthesizes `int v = 0` and the round-trip is byte-for-byte
// clean.
func TestEmbeddedAssignDeclIntIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping embedded-assign int decl test")
	}

	const className = "EmbeddedAssignDecl"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	os.Setenv("JDEC_NO_EMBED_ASSIGN_INT", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_EMBED_ASSIGN_INT")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with embedded-assign int detection disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("embed-assign int OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with embedded-assign int detection enabled; got: %s", className, got)
	}
	t.Logf("embed-assign int ON (round-trip preserved) for %s: %s", className, got)
}

// TestStdlibNestedDotIsLoadBearing proves the standard-library nested-type dotted rendering is
// load-bearing. With it DISABLED (JDEC_STDLIB_NESTED_DOT_OFF=1) a reference to java.util.Map.Entry is
// emitted as the binary flat form `Map$Entry`, which javac rejects ("cannot find symbol: class
// Map$Entry") so the decompiled StdlibNestedDot no longer recompiles. With the fix it renders
// `Map.Entry` and the round-trip is byte-for-byte identical. This was the single largest guava/spring
// recompile blocker (hundreds of `Map$Entry` "cannot find symbol").
func TestStdlibNestedDotIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping stdlib nested-dot test")
	}

	const className = "StdlibNestedDot"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "", "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return decompiled, got, ok && got == golden
	}

	os.Setenv("JDEC_STDLIB_NESTED_DOT_OFF", "1")
	srcOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_STDLIB_NESTED_DOT_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with stdlib nested-dot disabled, but it succeeded; the fix is not load-bearing", className)
	}
	if !strings.Contains(srcOff, "Map$Entry") {
		t.Fatalf("expected the disabled rendering to emit the flat `Map$Entry`; got:\n%s", srcOff)
	}
	t.Logf("stdlib nested-dot OFF (expected failure) for %s: %s", className, got)

	srcOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with stdlib nested-dot enabled; got: %s", className, got)
	}
	if !strings.Contains(srcOn, "Map.Entry") || strings.Contains(srcOn, "Map$Entry") {
		t.Fatalf("expected the enabled rendering to emit dotted `Map.Entry` and no `Map$Entry`; got:\n%s", srcOn)
	}
	t.Logf("stdlib nested-dot ON (round-trip preserved) for %s: %s", className, got)
}

// TestCrossConstructorHoistGuardIsLoadBearing proves the class-wide (cross-constructor) half of the
// field-initializer hoist guard is actually doing work, not dead defensive code. It compiles the
// BlankFinalMultiCtor battery (a blank final assigned once in each of several overloaded
// constructors), decompiles it with the cross-constructor guard DISABLED, and asserts the result no
// longer round-trips (javac rejects the double-assigned final, or the program diverges). It then
// re-enables the guard and asserts a clean, behavior-preserving round-trip. If someone deletes the
// cross-constructor check, the "guard off" branch starts passing and this test fails.
func TestCrossConstructorHoistGuardIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping cross-constructor hoist guard test")
	}

	const className = "BlankFinalMultiCtor"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	// Cross-constructor guard OFF: the per-body guard alone cannot see the field is assigned in
	// multiple constructors, so the hoist corrupts the round-trip (compile error or divergence).
	javaclassparser.EnableCrossConstructorHoistGuard = false
	got, okOff := roundTrips()
	javaclassparser.EnableCrossConstructorHoistGuard = true
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the cross-constructor guard disabled, but it succeeded; the guard is not load-bearing", className)
	}
	t.Logf("guard OFF (expected failure) for %s: %s", className, got)

	// Cross-constructor guard ON: clean, behavior-preserving round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the cross-constructor guard enabled; got: %s", className, got)
	}
	t.Logf("guard ON (round-trip preserved) for %s: %s", className, got)
}

// annotationDefaultSource is a self-contained two-top-level-type battery for the AnnotationDefault
// attribute (the `default <value>` clause of an @interface element). It exercises every
// element_value tag the default renderer must handle: Z (boolean), I (int), s (String), c (Class),
// e (enum) and [ (array of enums). The user type applies @AnnotationDefaultFlags specifying ONLY
// `n`, so every other element MUST keep its default or the use site no longer recompiles. main reads
// the annotation reflectively (RUNTIME retention) and folds all values into a deterministic
// fingerprint, so a dropped/garbled default also surfaces as a fingerprint divergence.
const annotationDefaultSource = `package codec;

import java.lang.annotation.ElementType;
import java.lang.annotation.Retention;
import java.lang.annotation.RetentionPolicy;
import java.lang.annotation.Target;

@AnnotationDefaultFlags(n = 42)
public class AnnotationDefaultDecl {
	public static void main(String[] args) {
		AnnotationDefaultFlags f = AnnotationDefaultDecl.class.getAnnotation(AnnotationDefaultFlags.class);
		long fp = 1469598103934665603L;
		fp = fp * 1099511628211L + (f.a() ? 1 : 0);
		fp = fp * 1099511628211L + f.n();
		fp = fp * 1099511628211L + (long) f.s().hashCode();
		fp = fp * 1099511628211L + f.arr().length;
		fp = fp * 1099511628211L + (long) f.c().getName().hashCode();
		fp = fp * 1099511628211L + (long) f.policy().ordinal();
		System.out.println("fp=" + fp);
	}
}

@Retention(RetentionPolicy.RUNTIME)
@Target(ElementType.TYPE)
@interface AnnotationDefaultFlags {
	boolean a() default true;

	int n() default 7;

	String s() default "default-string";

	ElementType[] arr() default {ElementType.TYPE, ElementType.METHOD};

	Class<?> c() default Object.class;

	RetentionPolicy policy() default RetentionPolicy.RUNTIME;
}
`

// compileAndRunMultiClass writes each (name -> source) into dir, compiles them together, runs
// codec.<mainClass> and returns trimmed stdout + ok. Unlike compileAndRunJavaBattery this supports a
// battery split across several top-level types (e.g. an @interface plus the class that uses it).
func compileAndRunMultiClass(t *testing.T, javac, java, dir, mainClass string, sources map[string]string) (string, bool) {
	t.Helper()
	var paths []string
	for name, src := range sources {
		p := filepath.Join(dir, name+".java")
		if err := os.WriteFile(p, []byte(src), 0644); err != nil {
			t.Fatalf("write source %s: %v", name, err)
		}
		paths = append(paths, p)
	}
	args := append([]string{"-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", dir}, paths...)
	if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
		t.Logf("javac failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	out, err := exec.Command(java, "-cp", dir, "codec."+mainClass).CombinedOutput()
	if err != nil {
		t.Logf("java run failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	return strings.TrimSpace(string(out)), true
}

// TestAnnotationDefaultIsLoadBearing proves the AnnotationDefault rendering (the `default <value>`
// clause of @interface elements) is load-bearing. With it DISABLED (JDEC_ANNO_DEFAULT_OFF=1) the
// decompiled @interface drops every default, so the use site `@AnnotationDefaultFlags(n = 42)` no
// longer recompiles ("annotation is missing a default value for the element 'a'"). With the fix the
// defaults round-trip and the reflective fingerprint is byte-for-byte identical. This is the codec
// regression that was driving down guava recompilability (@GwtCompatible / @GwtIncompatible defaults
// are dropped on most guava classes).
func TestAnnotationDefaultIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping annotation-default test")
	}

	const mainClass = "AnnotationDefaultDecl"
	const annoClass = "AnnotationDefaultFlags"

	origDir := t.TempDir()
	golden, ok := compileAndRunMultiClass(t, javac, java, origDir, mainClass, map[string]string{
		mainClass: annotationDefaultSource,
	})
	if !ok {
		t.Fatalf("failed to compile/run original annotation-default battery:\n%s", golden)
	}
	t.Logf("golden fingerprint: %s", golden)

	decompileBoth := func() (map[string]string, error) {
		out := map[string]string{}
		for _, cls := range []string{mainClass, annoClass} {
			raw, err := os.ReadFile(filepath.Join(origDir, "codec", cls+".class"))
			if err != nil {
				return nil, err
			}
			src, err := javaclassparser.Decompile(raw)
			if err != nil {
				return nil, err
			}
			out[cls] = src
		}
		return out, nil
	}

	roundTrips := func() (string, bool) {
		sources, err := decompileBoth()
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunMultiClass(t, javac, java, t.TempDir(), mainClass, sources)
		return got, ok && got == golden
	}

	// Default rendering OFF: the @interface loses its `default <value>` clauses, so the use site that
	// omits those elements no longer recompiles.
	os.Setenv("JDEC_ANNO_DEFAULT_OFF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_ANNO_DEFAULT_OFF")
	if okOff {
		t.Fatalf("expected the annotation-default battery to FAIL the round-trip with default rendering disabled, but it succeeded; the fix is not load-bearing")
	}
	t.Logf("anno-default OFF (expected failure): %s", got)

	// Default rendering ON: clean, behavior-preserving round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected the annotation-default battery to round-trip cleanly with default rendering enabled; got: %s", got)
	}
	t.Logf("anno-default ON (round-trip preserved): %s", got)
}

// innerTypeVarSource is a two-type battery (an outer generic class + a non-static inner class with NO
// type parameters of its own that inherits the enclosing K, V). When Yak flattens the inner class to a
// top-level `InnerTypeVar$Wrapped` unit, K and V lose their declaration unless the inner-type-variable
// injection re-declares them. Wrapped references BOTH variables in its supertypes (AbstractCollection<V>
// + Supplier<K>) so the injected arity (2) matches the arity the outer's wrap() reference carries (the
// `LInnerTypeVar<TK;TV;>.Wrapped;` signature form), and uses them only at declaration level: iterator()
// returns a raw->parameterized Iterator (an unchecked warning, not an error) and get() returns null.
// This isolates the injection (the declaration) from the separate raw-this$0 field-access and
// inner-constructor-synthetic-param residuals (which would surface as "Object cannot be converted to V"
// rather than the "cannot find symbol: class V" the injection actually fixes). main folds the iteration
// and size into a deterministic fingerprint so a behavioral drift also fails, not just a recompile error.
const innerTypeVarSource = `package codec;

import java.util.AbstractCollection;
import java.util.ArrayList;
import java.util.Collection;
import java.util.Iterator;
import java.util.function.Supplier;

public class InnerTypeVar<K, V> {
	final K key;
	final Collection<V> values;

	InnerTypeVar(K key, Collection<V> values) {
		this.key = key;
		this.values = values;
	}

	class Wrapped extends AbstractCollection<V> implements Supplier<K> {
		public K get() {
			return null;
		}

		public Iterator<V> iterator() {
			return values.iterator();
		}

		public int size() {
			return values.size();
		}
	}

	Wrapped wrap() {
		return new Wrapped();
	}

	public static void main(String[] args) {
		Collection<Integer> vs = new ArrayList<Integer>();
		vs.add(10);
		vs.add(20);
		vs.add(30);
		InnerTypeVar<String, Integer> m = new InnerTypeVar<String, Integer>("guava", vs);
		InnerTypeVar<String, Integer>.Wrapped w = m.wrap();
		long fp = 1469598103934665603L;
		for (Integer x : w) {
			fp = (fp ^ (long) x.intValue()) * 1099511628211L;
		}
		fp = (fp ^ (long) w.size()) * 1099511628211L;
		System.out.println("fp=" + Long.toHexString(fp));
	}
}
`

// TestInnerClassTypeVarIsLoadBearing proves the non-static-inner-class type-variable injection is
// load-bearing. A non-static inner class with no type parameters of its own (InnerTypeVar.Wrapped)
// inherits the enclosing K, V; flattened to a top-level `InnerTypeVar$Wrapped` unit those variables
// have no declaration. With the injection DISABLED (JDEC_INNER_TYPEVAR_OFF=1) the flat unit renders
// `class InnerTypeVar$Wrapped extends AbstractCollection<V> implements Supplier<K>` with K, V
// undeclared, so javac rejects it ("cannot find symbol: class V") and the round-trip fails. With the
// injection the flat unit declares the inherited variables and the round-trip is byte-for-byte clean.
// This was the single largest remaining guava recompile blocker (~2000 undeclared type-variable
// errors across the Multimap / Table / cache inner-class families).
func TestInnerClassTypeVarIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping inner-class type-var test")
	}

	const mainClass = "InnerTypeVar"
	const innerClass = "InnerTypeVar$Wrapped"

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, mainClass, innerTypeVarSource)
	if !ok {
		t.Fatalf("failed to compile/run original inner-type-var battery:\n%s", golden)
	}
	t.Logf("golden fingerprint: %s", golden)

	decompileBoth := func() (map[string]string, string, error) {
		out := map[string]string{}
		innerSrc := ""
		for _, cls := range []string{mainClass, innerClass} {
			raw, err := os.ReadFile(filepath.Join(origDir, "codec", cls+".class"))
			if err != nil {
				return nil, "", err
			}
			src, err := javaclassparser.Decompile(raw)
			if err != nil {
				return nil, "", err
			}
			out[cls] = src
			if cls == innerClass {
				innerSrc = src
			}
		}
		return out, innerSrc, nil
	}

	roundTrips := func() (string, string, bool) {
		sources, innerSrc, err := decompileBoth()
		if err != nil {
			return "", "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunMultiClass(t, javac, java, t.TempDir(), mainClass, sources)
		return innerSrc, got, ok && got == golden
	}

	os.Setenv("JDEC_INNER_TYPEVAR_OFF", "1")
	innerOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_INNER_TYPEVAR_OFF")
	if okOff {
		t.Fatalf("expected the inner-type-var battery to FAIL the round-trip with injection disabled, but it succeeded; the fix is not load-bearing")
	}
	if !strings.Contains(innerOff, "class InnerTypeVar$Wrapped extends") || strings.Contains(innerOff, "InnerTypeVar$Wrapped<") {
		t.Fatalf("expected the disabled inner class to render without injected type parameters; got:\n%s", innerOff)
	}
	t.Logf("inner-type-var OFF (expected failure): %s", got)

	innerOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected the inner-type-var battery to round-trip cleanly with injection enabled; got: %s\n----- inner -----\n%s", got, innerOn)
	}
	if !strings.Contains(innerOn, "InnerTypeVar$Wrapped<") {
		t.Fatalf("expected the enabled inner class to declare injected type parameters (InnerTypeVar$Wrapped<...>); got:\n%s", innerOn)
	}
	t.Logf("inner-type-var ON (round-trip preserved): %s", got)
}

// TestGenericMethodReturnIsLoadBearing proves the method-Signature return-type recovery for ZERO-ARG
// generic methods is load-bearing. A generic class implementing Map.Entry<K,V> has accessors whose
// return type is a type variable; the descriptor is erased (getKey()Ljava/lang/Object;) and the real
// return type lives only in the method Signature (()TK;). ParseMethodSignature returns nil params for a
// zero-arg method, so the old `sigParams != nil` gate skipped exactly these and rendered
// `Object getKey()`, which fails to override Map.Entry.getKey() ("return type Object is not compatible
// with K"). With the fix DISABLED (JDEC_METHOD_SIG_RET_OFF=1) the round-trip fails to recompile; with
// the fix the return type is recovered as K/V and the round-trip is byte-for-byte clean. This was the
// dominant guava blocker unmasked after the stdlib nested-dot fix (AbstractMapEntry / Maps$* /
// Multimaps$* and friends).
func TestGenericMethodReturnIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping generic-method-return test")
	}

	const className = "GenericMethodReturn"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "", "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return decompiled, got, ok && got == golden
	}

	os.Setenv("JDEC_METHOD_SIG_RET_OFF", "1")
	srcOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_METHOD_SIG_RET_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with method-signature return recovery disabled, but it succeeded; the fix is not load-bearing", className)
	}
	if !strings.Contains(srcOff, "Object getKey(") {
		t.Fatalf("expected the disabled rendering to erase the return type to `Object getKey(`; got:\n%s", srcOff)
	}
	t.Logf("method-sig return OFF (expected failure) for %s: %s", className, got)

	srcOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with method-signature return recovery enabled; got: %s", className, got)
	}
	if !strings.Contains(srcOn, "K getKey(") || strings.Contains(srcOn, "Object getKey(") {
		t.Fatalf("expected the enabled rendering to recover `K getKey(` and no `Object getKey(`; got:\n%s", srcOn)
	}
	t.Logf("method-sig return ON (round-trip preserved) for %s: %s", className, got)
}

// TestTypeVarReturnCastIsLoadBearing proves the type-variable return cast is load-bearing. Once the
// method-Signature return recovery (Bug AG) correctly types `max()` as returning the type variable `T`,
// a body that returns a LOCAL inferred as the erased bound (`Comparable`) no longer compiles
// ("incompatible types: Comparable cannot be converted to T"). The fix emits an unchecked `(T)` cast at
// the return site (matching CFR/Fernflower). With the fix DISABLED (JDEC_TYPEVAR_RET_CAST_OFF=1) the
// round-trip fails to recompile; with the fix the cast is emitted and the round-trip is byte-for-byte
// clean with a matching fingerprint. This was the regression surfaced in the gated `Generics` corpus
// category by the method-Signature return recovery.
func TestTypeVarReturnCastIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping type-var-return-cast test")
	}

	const className = "TypeVarReturnCast"
	src, err := codecBatteryFS.ReadFile("testdata/codec/" + className + ".java")
	if err != nil {
		t.Fatalf("read battery %s: %v", className, err)
	}

	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, string(src))
	if !ok {
		t.Fatalf("failed to compile/run original battery %s:\n%s", className, golden)
	}
	raw, err := os.ReadFile(filepath.Join(origDir, "codec", className+".class"))
	if err != nil {
		t.Fatalf("read compiled class %s: %v", className, err)
	}

	roundTrips := func() (string, string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "", "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return decompiled, got, ok && got == golden
	}

	os.Setenv("JDEC_TYPEVAR_RET_CAST_OFF", "1")
	srcOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_TYPEVAR_RET_CAST_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the type-var return cast disabled, but it succeeded; the fix is not load-bearing", className)
	}
	// The disabled rendering still recovers `T max()` (method-sig return recovery is independent), but
	// returns the bound-typed local without a cast.
	if !strings.Contains(srcOff, "T max(") {
		t.Fatalf("expected the disabled rendering to still recover `T max(`; got:\n%s", srcOff)
	}
	t.Logf("type-var return cast OFF (expected failure) for %s: %s", className, got)

	srcOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the type-var return cast enabled; got: %s\n----- src -----\n%s", className, got, srcOn)
	}
	if !strings.Contains(srcOn, "(T) (") {
		t.Fatalf("expected the enabled rendering to emit an explicit `(T)` return cast; got:\n%s", srcOn)
	}
	t.Logf("type-var return cast ON (round-trip preserved) for %s: %s", className, got)
}
