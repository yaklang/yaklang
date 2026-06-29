package tests

import (
	"archive/zip"
	"bytes"
	"embed"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/utils/filesys"
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

	// Batteries that compile to MORE than one class file (e.g. an anonymous/inner subclass) cannot be
	// validated by this single-file round-trip oracle; they carry their own dedicated multi-class
	// load-bearing test instead.
	multiClassBatteries := map[string]bool{
		"AnonSubclassOwnPrivateField": true, // -> TestAnonSubclassOwnPrivateFieldCastIsLoadBearing
		"CtorArgFreshLocalRename":     true, // -> TestCtorArgFreshLocalRenameIsLoadBearing
	}

	for _, className := range names {
		if multiClassBatteries[className] {
			continue
		}
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

// TestNullInitSlotReuseIsLoadBearing proves the null-init disjoint-slot-reuse split is load-bearing
// AND isolates the two complementary mechanisms that implement it. The NullInitSlotReuse battery packs
// a null-initialized Throwable holder (committed in a dominated catch on one branch) and an
// unrelated-typed local on a disjoint branch onto ONE jvm slot:
//   - the dominance gate (nullInitDefDominates, JDEC_NULLADOPT_REACH_OFF) blocks the null-init type
//     adoption when the null initializer does not reach the store along the CFG (disjoint branch);
//   - the adopt-once guard (AssignVarGuarded + JavaRef.nullTypeAdopted, JDEC_NO_NULL_ADOPT_ONCE)
//     blocks a SECOND, incompatible adoption after the ref already committed to a concrete type.
//
// The test runs all four kill-switch combinations and asserts each mechanism is INDIVIDUALLY
// sufficient (so neither is dead code), and the merge only surfaces when BOTH are disabled:
//   - gate ON,  guard OFF -> round-trips  (the gate alone splits the slot)
//   - gate OFF, guard ON  -> round-trips  (the guard alone splits the slot, since the catch commits the
//     holder to Throwable before the disjoint store is visited in DFS order)
//   - gate OFF, guard OFF -> FAILS        (unconditional adoption unifies them onto one mis-typed
//     variable: "Throwable cannot be converted to String" / "cannot find symbol getMessage")
//   - gate ON,  guard ON  -> round-trips  (production configuration)
//
// Deleting EITHER mechanism flips its "alone" sub-case from pass to fail, so both stay verified. The
// adopt-once guard additionally has TestTwrSlotReuseNullAdoptOnceIsLoadBearing, where the null-init
// dominates both stores so the gate never fires and only the guard can keep them apart.
// setEnv sets the kill-switch env var to "1" when on, or unsets it when off, so a single boolean drives
// each toggle in the multi-combination load-bearing checks.
func setEnv(name string, on bool) {
	if on {
		os.Setenv(name, "1")
	} else {
		os.Unsetenv(name)
	}
}

func TestNullInitSlotReuseIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping null-init slot-reuse load-bearing test")
	}

	const className = "NullInitSlotReuse"
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

	// roundTrips decompiles under the given kill-switch combination and reports whether the result
	// recompiles+runs to the golden fingerprint.
	roundTrips := func(gateOff, guardOff bool) (string, bool) {
		setEnv("JDEC_NULLADOPT_REACH_OFF", gateOff)
		setEnv("JDEC_NO_NULL_ADOPT_ONCE", guardOff)
		defer os.Unsetenv("JDEC_NULLADOPT_REACH_OFF")
		defer os.Unsetenv("JDEC_NO_NULL_ADOPT_ONCE")
		decompiled, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			return "decompile error: " + derr.Error(), false
		}
		got, okRT := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, okRT && got == golden
	}

	// Gate alone (guard disabled): must still split -> round-trips.
	if got, ok := roundTrips(false, true); !ok {
		t.Fatalf("expected %s to round-trip with the dominance gate alone (adopt-once guard disabled); the gate is not load-bearing: %s", className, got)
	}
	// Guard alone (gate disabled): must still split -> round-trips.
	if got, ok := roundTrips(true, false); !ok {
		t.Fatalf("expected %s to round-trip with the adopt-once guard alone (dominance gate disabled); the guard is not load-bearing: %s", className, got)
	}
	// Both disabled: the merge surfaces -> must FAIL to recompile.
	if got, ok := roundTrips(true, true); ok {
		t.Fatalf("expected %s to FAIL the round-trip with both null-init split defenses disabled, but it succeeded; the defenses are not load-bearing: %s", className, got)
	}
	// Production config: both enabled -> clean behavior-preserving round-trip.
	got, ok := roundTrips(false, false)
	if !ok {
		t.Fatalf("expected %s to round-trip cleanly with the null-init split defenses enabled; got: %s", className, got)
	}
	t.Logf("null-init split defenses verified (gate-only, guard-only, both-off, both-on) for %s: %s", className, got)
}

// TestTwrDuplicateCatchMergeIsLoadBearing proves mergeNestedSameTypeCatches (dumper.go) is
// load-bearing. JDK8 desugars try-with-resources inline: a Throwable primaryExc-capture handler
// (`catch (Throwable t) { primaryExc = t; throw t; }`) whose region is itself covered by a Throwable
// cleanup ("any") handler. The decompiler renders both, producing two sibling catch(Throwable)
// clauses on one try — illegal Java ("exception Throwable already caught"). The embedded class is
// commons-codec-style twr compiled by JDK8 (system javac is JDK17 and would emit the compact
// single-handler shape, so the broken shape must come from a pinned JDK8 .class). With the merge
// DISABLED (JDEC_NO_CATCH_MERGE=1) the decompiled source no longer recompiles; with it enabled the
// two handlers collapse into one and the round-trip is behavior-identical to the original bytecode.
func TestTwrDuplicateCatchMergeIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping twr duplicate-catch merge test")
	}

	const className = "TwrSingleResource"
	raw, err := regressionFS.ReadFile("testdata/regression/twr_jdk8_single_resource.class")
	if err != nil {
		t.Fatalf("read embedded twr class: %v", err)
	}

	// Golden: run the original JDK8 bytecode directly (java can run a JDK8 class).
	goldDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(goldDir, "codec"), 0755); err != nil {
		t.Fatalf("mkdir golden: %v", err)
	}
	if err := os.WriteFile(filepath.Join(goldDir, "codec", className+".class"), raw, 0644); err != nil {
		t.Fatalf("write golden class: %v", err)
	}
	goldOut, err := exec.Command(java, "-cp", goldDir, "codec."+className).CombinedOutput()
	if err != nil {
		t.Fatalf("run original twr class: %v\n%s", err, goldOut)
	}
	golden := strings.TrimSpace(string(goldOut))
	t.Logf("golden fingerprint [%s]: %s", className, golden)

	roundTrips := func() (string, bool) {
		decompiled, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			return "decompile error: " + derr.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	// Merge OFF: two sibling catch(Throwable) -> duplicate catch -> recompile fails.
	os.Setenv("JDEC_NO_CATCH_MERGE", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_CATCH_MERGE")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with catch-merge disabled (duplicate catch), but it succeeded; the merge is not load-bearing", className)
	}
	t.Logf("merge OFF (expected failure) for %s: %s", className, got)

	// Merge ON: single merged catch -> clean, behavior-preserving round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with catch-merge enabled; got: %s (golden %s)", className, got, golden)
	}
	t.Logf("merge ON (round-trip preserved) for %s: %s", className, got)
}

// TestDollarIdentifierValidationToleranceIsLoadBearing proves the post-decompile syntax safety net's
// standalone-'$' tolerance (neutralizeStandaloneDollarForValidation in syntax_validate.go) is
// load-bearing. Obfuscators emit members literally named "$" (a legal JVM AND javac identifier, e.g.
// asm-6.0_BETA's MethodWriter). yak's Java grammar lexes a lone '$' as the dedicated Dollar token
// (used for "${...}") rather than IDENTIFIER, so faithfully-decompiled, javac-valid output like
// `this.$` fails the grammar safety net. Without the tolerance the '$' field is DROPPED and the '$'
// method is degraded to a throwing stub, so the round-trip crashes; with it the body survives and the
// round-trip is behavior-identical. The sibling "$$" field (already a valid IDENTIFIER) must keep
// working in both modes, guarding the tolerance against over-reaching onto multi-'$' names.
func TestDollarIdentifierValidationToleranceIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping dollar-identifier validation tolerance test")
	}

	const className = "DollarIdentField"
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
		decompiled, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			return "decompile error: " + derr.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	// Tolerance OFF: the '$' field is dropped and the '$' method is stubbed -> round-trip diverges.
	os.Setenv("JDEC_NO_DOLLAR_IDENT_VALIDATE", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_DOLLAR_IDENT_VALIDATE")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with '$'-tolerance disabled (dropped field / stubbed method), but it succeeded; the tolerance is not load-bearing", className)
	}
	t.Logf("tolerance OFF (expected failure) for %s: %s", className, got)

	// Tolerance ON: standalone '$' neutralized only for validation -> clean, behavior-preserving round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with '$'-tolerance enabled; got: %s (golden %s)", className, got, golden)
	}
	t.Logf("tolerance ON (round-trip preserved) for %s: %s", className, got)
}

// TestCrossScopeDeclDominanceIsLoadBearing proves the cross-scope declaration DOMINANCE check
// (topLevelDeclDominatesAllUses in rewrite_var.go) is load-bearing. One JVM slot is reused for three
// independent int locals on disjoint live ranges (an if-arm, the sibling else-arm, and the trailing
// code), so rewriteVar merges them onto one VariableId. The minted declaration lands in the if-arm and
// a LATER disjoint re-declaration sits at the block top level; the existence-only skip
// (isDeclaredAtTopLevel) treated that later declaration as already covering the block and never hoisted
// the variable, leaving the else-arm use out of scope ("cannot find symbol: var4"). With the dominance
// check DISABLED (JDEC_NO_CROSS_SCOPE_DOMINATE=1) the decompiled source must fail to recompile; with it
// enabled a single bare declaration is hoisted to the dominating block and the round-trip is
// byte-for-byte behavior identical.
func TestCrossScopeDeclDominanceIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping cross-scope declaration dominance test")
	}

	const className = "SlotReuseDisjointRanges"
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
		decompiled, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			return "decompile error: " + derr.Error(), false
		}
		got, okRT := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, okRT && got == golden
	}

	// Dominance check OFF: the later top-level re-declaration masks the un-dominated sibling-arm use,
	// so the hoist is skipped and the else-arm `var4 = ...` stays out of scope -> recompile fails.
	os.Setenv("JDEC_NO_CROSS_SCOPE_DOMINATE", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_CROSS_SCOPE_DOMINATE")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the cross-scope dominance check disabled, but it succeeded; the check is not load-bearing", className)
	}
	t.Logf("dominance OFF (expected failure) for %s: %s", className, got)

	// Dominance check ON: one bare declaration hoisted to the dominating block; clean round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the cross-scope dominance check enabled; got: %s (golden %s)", className, got, golden)
	}
	t.Logf("dominance ON (round-trip preserved) for %s: %s", className, got)
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

// TestIfElseEscapingSlotIsLoadBearing proves the if/else escaping-slot prebind (Bug AJ) is doing
// real work. A primitive local assigned in BOTH arms of an if/else and READ after the if (in a
// following loop + the return) shares one JVM slot across two live ranges; the loop counter that
// follows occupies the NEXT slot. Without the prebind each arm minted its own id for the if/else
// local, the post-if reads kept the slot's original (un-minted) id, and -- because the arm mints
// never advanced the parent name counter -- the loop counter was handed the SAME varN as the if/else
// local, so the recompiled source silently read the counter where the if/else local was meant. It
// still compiles (all the same primitive type), so only differential execution catches it. With the
// prebind DISABLED (JDEC_IFELSE_PREBIND_OFF=1) the decompiled source must diverge from the golden
// fingerprint; with it enabled the round-trip is byte-for-byte clean.
func TestIfElseEscapingSlotIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping if/else escaping-slot prebind test")
	}

	const className = "IfElseEscapingSlot"
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

	// Prebind OFF: the if/else local merges with the following loop counter, so the recompiled source
	// reads the wrong variable and the fingerprint diverges (the source still compiles).
	os.Setenv("JDEC_IFELSE_PREBIND_OFF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_IFELSE_PREBIND_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the if/else escaping-slot prebind disabled, but it succeeded; the prebind is not load-bearing", className)
	}
	t.Logf("prebind OFF (expected failure) for %s: %s", className, got)

	// Prebind ON: clean, behavior-preserving round-trip.
	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the if/else escaping-slot prebind enabled; got: %s", className, got)
	}
	t.Logf("prebind ON (round-trip preserved) for %s: %s", className, got)
}

// TestEmbeddedAssignRefIsLoadBearing proves the embedded-assignment reference-type recovery is load
// bearing. A reference local that only receives its value through an embedded assignment in a
// condition ((s = next(...)) != null / (s = arr[i]) != null) has no standalone declaration after
// the dup-collapse, so the string-level safety net must synthesize one. With the fix DISABLED
// (JDEC_NO_EMBED_ASSIGN_REF=1) it defaults to `Object s = null`, and every reference member access
// (s.length()/s.charAt()/s.isEmpty()) fails to recompile ("cannot find symbol"), so the round-trip
// must fail; with the fix enabled the type is recovered (array element type / in-class method return
// type) and the round-trip is byte-for-byte clean.
func TestEmbeddedAssignRefIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping embedded-assign ref type test")
	}

	const className = "EmbeddedAssignRef"
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

	os.Setenv("JDEC_NO_EMBED_ASSIGN_REF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_EMBED_ASSIGN_REF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with embedded-assign ref recovery disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("ref recovery OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with embedded-assign ref recovery enabled; got: %s", className, got)
	}
	t.Logf("ref recovery ON (round-trip preserved) for %s: %s", className, got)
}

// TestEmbeddedAssignGetClassIsLoadBearing proves the embedded-assignment getClass() reference-type
// recovery is load bearing. A `Class c` local whose only definition is the embedded assignment
// `(c = obj.getClass()) != type` loses its standalone declaration to the dup-collapse, so the dumper
// synthesizes one. With the reference recovery DISABLED (JDEC_NO_EMBED_ASSIGN_REF=1) it falls back to
// `Object c = null`, and the later Class member access (c.getName()/c.getSimpleName()) fails to
// recompile ("cannot find symbol"); with it enabled the type is recovered as Class and the round-trip
// is behavior-preserving. Mirrors fastjson2 JSONWriter.checkAndWriteTypeName.
func TestEmbeddedAssignGetClassIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping embedded-assign getClass type test")
	}

	const className = "EmbeddedAssignGetClass"
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

	os.Setenv("JDEC_NO_EMBED_ASSIGN_REF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_EMBED_ASSIGN_REF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with getClass ref recovery disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("getClass recovery OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with getClass ref recovery enabled; got: %s", className, got)
	}
	t.Logf("getClass recovery ON (round-trip preserved) for %s: %s", className, got)
}

// TestAnonSubclassOwnPrivateFieldCastIsLoadBearing proves the own-private-field up-cast (Bug AN (1))
// is load bearing. A method of class X reads X's own private field through a reference whose static
// type is the synthetic anonymous subclass `X$1`. The decompiler types the local as `X$1`, through
// which the private field is NOT an accessible member, so the read must be rendered `((X)r).field`.
// With the cast DISABLED (JDEC_NO_PRIV_FIELD_CAST=1) the output is `r.field` and fails to recompile
// ("field has private access in X"); with it enabled the round-trip is behavior-preserving. Mirrors
// commons-codec Rule.parseRules.
func TestAnonSubclassOwnPrivateFieldCastIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping anon-subclass own-private-field cast test")
	}

	const className = "AnonSubclassOwnPrivateField"
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

	// The battery has an anonymous inner class (codec.<className>$1). The single-file battery helper
	// would leave that sibling dangling, so we recompile the decompiled OUTER unit against the original
	// classes dir as a classpath backstop (the $1.class binary) — faithfully mirroring the whole-jar
	// round-trip model where intra-jar siblings resolve. The recompiled outer .class is then run with
	// the original $1 binary, exercising the cast at runtime.
	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		d := t.TempDir()
		srcPath := filepath.Join(d, className+".java")
		if werr := os.WriteFile(srcPath, []byte(decompiled), 0644); werr != nil {
			t.Fatalf("write decompiled source: %v", werr)
		}
		out, cerr := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-cp", origDir, "-d", d, srcPath).CombinedOutput()
		if cerr != nil {
			return string(out), false
		}
		// Run with the freshly recompiled outer on top of the original classes (which carry $1).
		out2, rerr := exec.Command(java, "-cp", d+string(os.PathListSeparator)+origDir, "codec."+className).CombinedOutput()
		if rerr != nil {
			return string(out2), false
		}
		got := strings.TrimSpace(string(out2))
		return got, got == golden
	}

	os.Setenv("JDEC_NO_PRIV_FIELD_CAST", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_PRIV_FIELD_CAST")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with own-private-field cast disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("priv-field cast OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with own-private-field cast enabled; got: %s", className, got)
	}
	t.Logf("priv-field cast ON (round-trip preserved) for %s: %s", className, got)
}

// TestCtorArgFreshLocalRenameIsLoadBearing proves the constructor-argument rename propagation (Bug
// AN(2)) is load bearing. RewriteVar renames a freshly bound local's DECLARATION by minting a new id
// and retargeting every USE via idReplaceMap+ReplaceVar. A `new T(...)` constructor argument is
// captured ONLY inside NewExpression.ArgumentsGetter (a render-time closure), so without the
// NewExpression.ConstructorCall back-reference ReplaceVar cannot reach it: the use keeps the stale
// slot-derived `varN` that now collides with another live variable of that name (an array), so the
// call binds the wrong operand (`new Rule(p, q, parts, ...)` — a String[] where a String is
// required) and fails to recompile. With the back-reference the rename reaches the argument and the
// round-trip is behavior-preserving. Mirrors commons-codec DaitchMokotoffSoundex.parseRules.
func TestCtorArgFreshLocalRenameIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping ctor-arg fresh-local rename test")
	}

	const className = "CtorArgFreshLocalRename"
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

	// The battery has a nested Rule class (codec.<className>$Rule). The single-file battery helper
	// would leave that sibling dangling, so we recompile the decompiled OUTER unit against the
	// original classes dir as a classpath backstop (the $Rule.class binary) — mirroring the whole-jar
	// round-trip model where intra-jar siblings resolve. The recompiled outer is then run with the
	// original $Rule binary, exercising the constructor at runtime.
	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		d := t.TempDir()
		srcPath := filepath.Join(d, className+".java")
		if werr := os.WriteFile(srcPath, []byte(decompiled), 0644); werr != nil {
			t.Fatalf("write decompiled source: %v", werr)
		}
		out, cerr := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-cp", origDir, "-d", d, srcPath).CombinedOutput()
		if cerr != nil {
			return string(out), false
		}
		out2, rerr := exec.Command(java, "-cp", d+string(os.PathListSeparator)+origDir, "codec."+className).CombinedOutput()
		if rerr != nil {
			return string(out2), false
		}
		got := strings.TrimSpace(string(out2))
		return got, got == golden
	}

	os.Setenv("JDEC_NO_CTOR_ARG_REPLACE", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_CTOR_ARG_REPLACE")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with ctor-arg rename propagation disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("ctor-arg rename OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with ctor-arg rename propagation enabled; got: %s", className, got)
	}
	t.Logf("ctor-arg rename ON (round-trip preserved) for %s: %s", className, got)
}

// TestTwrSlotReuseNullAdoptOnceIsLoadBearing proves the null-adopt-once guard (Bug AL,
// AssignVarGuarded + JavaRef.nullTypeAdopted) is load bearing. A single JVM slot holds the verbose
// try-with-resources synthetic `Throwable primaryExc = null` (committed to Throwable in the synthetic
// catch) and is later reused for a `Map.Entry e` loop variable with a disjoint live range. The slot's
// ref is null-initialized (Val stays the null literal forever because ResetVarType only repoints the
// type), so without committing the first adoption the same ref adopts a SECOND, incompatible type:
// the declaration becomes `Map.Entry var1 = null`, the synthetic catch assigns `var1 = <throwable>`
// and calls `var1.addSuppressed(..)` on it — both reject against Map.Entry, so the unit fails to
// recompile. With the guard, the first concrete adoption (Throwable) is committed and the loop store
// mints a fresh local, so primaryExc stays Throwable and the round-trip is behavior-preserving.
// Mirrors commons-codec DaitchMokotoffSoundex.<clinit>. The battery is a single class (no nested
// types) so the plain single-file oracle applies. Kill-switch: JDEC_NO_NULL_ADOPT_ONCE=1.
func TestTwrSlotReuseNullAdoptOnceIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping twr slot-reuse null-adopt-once test")
	}

	const className = "TwrSlotReuseNullAdopt"
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
		d := t.TempDir()
		got, ok := compileAndRunJavaBattery(t, javac, java, d, className, decompiled)
		if !ok {
			return got, false
		}
		return got, got == golden
	}

	os.Setenv("JDEC_NO_NULL_ADOPT_ONCE", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_NULL_ADOPT_ONCE")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with null-adopt-once disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("null-adopt-once OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with null-adopt-once enabled; got: %s", className, got)
	}
	t.Logf("null-adopt-once ON (round-trip preserved) for %s: %s", className, got)
}

// TestSwitchCaseLocalReadAfterIsLoadBearing pins the prebindEscapingSwitchSlots fix (Bug AL's dominant
// fastjson2 shape, fastjson2 integral-jar recompile -72). A local written inside switch cases and read
// AFTER the switch (fastjson2 DateUtils' hand-unrolled date parser: each pattern case copies the
// canonical digit chars into locals that are validated after the switch) used to keep its in-case `T x
// = ...` declaration scoped to the case while the post-switch read kept the slot's original (pre-mint)
// id, so it rendered out of scope ("cannot find symbol: variable varN"). The prebind unifies the
// case-written and post-switch-read references onto one parent-scope id so the declaration hoists ahead
// of the switch. The kill-switch JDEC_SWITCH_PREBIND_OFF=1 disables it; the battery must then FAIL to
// recompile and must round-trip cleanly with it enabled.
func TestSwitchCaseLocalReadAfterIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping switch-case read-after test")
	}

	const className = "SwitchCaseLocalReadAfter"
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
		d := t.TempDir()
		got, ok := compileAndRunJavaBattery(t, javac, java, d, className, decompiled)
		if !ok {
			return got, false
		}
		return got, got == golden
	}

	os.Setenv("JDEC_SWITCH_PREBIND_OFF", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_SWITCH_PREBIND_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with switch prebind disabled, but it succeeded; the fix is not load-bearing", className)
	}
	t.Logf("switch-prebind OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with switch prebind enabled; got: %s", className, got)
	}
	t.Logf("switch-prebind ON (round-trip preserved) for %s: %s", className, got)
}

// TestLambdaLocalRenameIsLoadBearing pins renameLambdaBodyLocals (Bug AL / lambda-inlining shadow
// family; the dominant "variable varN is already defined" shape across fastjson2). javac compiles each
// lambda to a private `lambda$...` method whose body locals begin a FRESH jvm slot namespace
// (slot0,slot1,...). The decompiler splices that body inline as an arrow expression inside the
// enclosing method, where those slots render as var0,var1,... - colliding by NAME with the enclosing
// method's own parameters/locals (also var0,var1,...). Java forbids a lambda-body local from shadowing
// an in-scope enclosing local, so the naive inline emits "variable var0 is already defined in method
// compute(int)" and does not recompile. The fix lifts each lambda body's locals into a private
// `lv<seq>_N` namespace. The kill-switch JDEC_NO_LAMBDA_LOCAL_RENAME=1 disables the rename; the battery
// must then FAIL to recompile (shadowed locals) and must round-trip cleanly with it enabled.
func TestLambdaLocalRenameIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping lambda-local rename test")
	}

	const className = "LambdaLocalShadowsCapture"
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
		decompiled, derr := javaclassparser.Decompile(raw)
		if derr != nil {
			return "decompile error: " + derr.Error(), false
		}
		got, okc := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, okc && got == golden
	}

	// Rename OFF: the inlined lambda body re-declares var0/var1, shadowing the enclosing
	// parameter/local, so javac rejects the recompile.
	os.Setenv("JDEC_NO_LAMBDA_LOCAL_RENAME", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_LAMBDA_LOCAL_RENAME")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the lambda-local rename disabled, but it succeeded; the rename is not load-bearing", className)
	}
	t.Logf("lambda-local rename OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the lambda-local rename enabled; got: %s", className, got)
	}
	t.Logf("lambda-local rename ON (round-trip preserved) for %s: %s", className, got)
}

// enumConstantBodyFoldSource is an enum with constant-specific bodies (Bug V). javac compiles it into
// codec.EnumConstantBody plus a synthetic subclass per body (EnumConstantBody$1 / $2, each ACC_ENUM
// and `extends EnumConstantBody`). Decompiled per class the output is uncompilable: the outer enum's
// constants do not override the abstract apply(), and each $N renders `class $N extends <enum>` (an
// enum is not extensible). The fold (DumpWithResolver) inlines each $N's body back into its constant.
const enumConstantBodyFoldSource = `package codec;

public enum EnumConstantBody {
    ADD("plus") {
        public long apply(long a, long b) {
            return a + b;
        }
    },
    MUL("times") {
        public long apply(long a, long b) {
            return a * b;
        }
    },
    MAXX("max") {
        public long apply(long a, long b) {
            return a > b ? a : b;
        }
    };

    private final String label;

    EnumConstantBody(String label) {
        this.label = label;
    }

    public abstract long apply(long a, long b);

    public String label() {
        return this.label;
    }

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder("EnumConstantBody:");
        for (EnumConstantBody op : EnumConstantBody.values()) {
            sb.append(op.name()).append("=").append(op.label()).append(":");
            sb.append(op.apply(6, 7)).append(";");
        }
        System.out.println(sb.toString());
    }
}
`

// TestEnumConstantBodyFoldIsLoadBearing proves the enum constant-body cross-class folding (Bug V) is
// load bearing. With the fold DISABLED (JDEC_NO_ENUM_FOLD=1) the decompiled outer enum has constants
// with no body and an unimplemented abstract method, so it fails to recompile; with the fold enabled
// (the synthetic EnumConstantBody$N bodies inlined via the sibling resolver) the round-trip recompiles
// to the same multi-class layout and runs to a byte-identical fingerprint.
func TestEnumConstantBodyFoldIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping enum constant-body fold test")
	}

	const className = "EnumConstantBody"
	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, enumConstantBodyFoldSource)
	if !ok {
		t.Fatalf("failed to compile/run original enum %s:\n%s", className, golden)
	}
	t.Logf("golden fingerprint [%s]: %s", className, golden)

	// Build a sibling resolver over every compiled codec.* class, keyed by binary internal name
	// (e.g. "codec/EnumConstantBody$1"), so the fold can pull each constant body's synthetic subclass.
	pkgDir := filepath.Join(origDir, "codec")
	classFiles, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("read compiled package dir: %v", err)
	}
	siblings := map[string][]byte{}
	for _, f := range classFiles {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".class") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(pkgDir, f.Name()))
		if err != nil {
			t.Fatalf("read sibling class %s: %v", f.Name(), err)
		}
		siblings["codec/"+strings.TrimSuffix(f.Name(), ".class")] = data
	}
	resolver := func(internalName string) ([]byte, bool) {
		b, ok := siblings[internalName]
		return b, ok
	}

	raw := siblings["codec/"+className]
	if raw == nil {
		t.Fatalf("compiled outer enum class %s not found", className)
	}

	roundTrips := func() (string, bool) {
		decompiled, err := javaclassparser.DecompileWithResolver(raw, resolver)
		if err != nil {
			return "decompile error: " + err.Error(), false
		}
		got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, decompiled)
		return got, ok && got == golden
	}

	os.Setenv("JDEC_NO_ENUM_FOLD", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_ENUM_FOLD")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with enum constant-body folding disabled, but it succeeded; the fold is not load-bearing", className)
	}
	t.Logf("enum fold OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with enum constant-body folding enabled; got: %s", className, got)
	}
	t.Logf("enum fold ON (round-trip preserved) for %s: %s", className, got)
}

// buildJarFromDir packs every file under root (recursively) into an in-memory jar (zip) keyed by the
// path relative to root (slash form), and returns a JarFS over it. This mirrors the real jar
// decompilation entry (JarFS.ReadFile) so the enum fold can be exercised through the production path
// rather than the bare DecompileWithResolver API.
func buildJarFromDir(t *testing.T, root string) *javaclassparser.JarFS {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		t.Fatalf("pack jar from %s: %v", root, err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close jar writer: %v", err)
	}
	zipFS, err := filesys.NewZipFSRaw(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatalf("open in-memory jar: %v", err)
	}
	return javaclassparser.NewJarFS(zipFS)
}

// TestEnumConstantBodyFoldJarPathIsLoadBearing proves the enum constant-body fold is wired into the
// PRODUCTION jar path (JarFS.ReadFile), not just the bare DecompileWithResolver API. It packs the
// compiled multi-class enum into an in-memory jar, then asserts: (1) reading the outer enum's .class
// through JarFS yields a folded source that recompiles to the golden fingerprint; (2) reading each
// synthetic EnumConstantBody$N.class yields a suppression marker (no illegal `extends <enum>`); and
// (3) with JDEC_NO_ENUM_FOLD=1 the very same $N read instead yields the uncompilable
// `class EnumConstantBody$N extends EnumConstantBody`, so the suppression is genuinely load-bearing.
func TestEnumConstantBodyFoldJarPathIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping enum fold jar-path test")
	}

	const className = "EnumConstantBody"
	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, enumConstantBodyFoldSource)
	if !ok {
		t.Fatalf("failed to compile/run original enum %s:\n%s", className, golden)
	}
	t.Logf("golden fingerprint [%s]: %s", className, golden)

	jarFS := buildJarFromDir(t, origDir)
	outerEntry := "codec/" + className + ".class"
	subEntry := "codec/" + className + "$1.class"

	// (1) outer enum read through the jar path must carry the folded constant bodies and recompile.
	outerSrc, err := jarFS.ReadFile(outerEntry)
	if err != nil {
		t.Fatalf("jar ReadFile(%s): %v", outerEntry, err)
	}
	if !strings.Contains(string(outerSrc), "apply") {
		t.Fatalf("expected folded enum source to inline the constant `apply` bodies; got:\n%s", outerSrc)
	}
	got, ok := compileAndRunJavaBattery(t, javac, java, t.TempDir(), className, string(outerSrc))
	if !ok || got != golden {
		t.Fatalf("jar-path folded enum failed round-trip (ok=%v): got %q want %q\nsource:\n%s", ok, got, golden, outerSrc)
	}
	t.Logf("jar-path fold ON: outer enum round-trips to golden")

	// (2) synthetic subclass read through the jar path must be suppressed (no illegal `extends`).
	subSrc, err := jarFS.ReadFile(subEntry)
	if err != nil {
		t.Fatalf("jar ReadFile(%s): %v", subEntry, err)
	}
	if strings.Contains(string(subSrc), "extends "+className) {
		t.Fatalf("expected synthetic %s$1 to be suppressed, but it rendered an illegal subclass:\n%s", className, subSrc)
	}
	if !strings.Contains(string(subSrc), "folded into enum") {
		t.Fatalf("expected synthetic %s$1 to carry a suppression marker; got:\n%s", className, subSrc)
	}
	t.Logf("jar-path fold ON: synthetic %s$1 suppressed", className)

	// (3) kill-switch: the SAME synthetic read must now emit the uncompilable illegal subclass, proving
	// the suppression in (2) is load-bearing (a fresh JarFS avoids the decompile cache).
	os.Setenv("JDEC_NO_ENUM_FOLD", "1")
	defer os.Unsetenv("JDEC_NO_ENUM_FOLD")
	jarFSOff := buildJarFromDir(t, origDir)
	subSrcOff, err := jarFSOff.ReadFile(subEntry)
	if err != nil {
		t.Fatalf("jar ReadFile(%s) [fold off]: %v", subEntry, err)
	}
	if !strings.Contains(string(subSrcOff), "extends "+className) {
		t.Fatalf("expected %s$1 to render the illegal `extends %s` with folding disabled; got:\n%s", className, className, subSrcOff)
	}
	t.Logf("jar-path fold OFF: synthetic %s$1 falls back to illegal subclass (suppression is load-bearing)", className)
}

// accEnumMarkerCtorSource is an enum with constant-specific bodies that ALSO call a private static
// member. Compiled with --release 8 (pre-nestmates), javac emits both a synthetic accessor
// (access$N -> secretBase) AND a synthetic "marker" constructor `AccEnum(String,int,AccEnum$1)` that
// gives the constant-body subclasses an accessible super-ctor. The marker ctor must be suppressed on
// decompile (it references the synthetic $N type and renders an illegal `this(...)` after locals);
// the accessor, by contrast, is legitimately retained.
const accEnumMarkerCtorSource = `package codec;

public enum AccEnum {
    ADD {
        long apply(long a, long b) { return a + secretBase(a) + SCALE; }
    },
    MUL {
        long apply(long a, long b) { return a * b * SCALE - secretBase(b); }
    };

    private static final long SCALE = 3;
    private static long secretBase(long x) { return x + 10; }
    abstract long apply(long a, long b);

    public static void main(String[] args) {
        StringBuilder sb = new StringBuilder("AccEnum:");
        for (AccEnum e : values()) sb.append(e.name()).append("=").append(e.apply(6, 7)).append(";");
        System.out.println(sb.toString());
    }
}
`

// compileRelease8AndRun writes src as <dir>/<className>.java, compiles it with `javac --release 8`
// (so the pre-nestmates synthetic accessor + marker-ctor shapes are emitted) into dir, then runs
// codec.<className>. Returns (output, ok). ok=false on either javac or java failure.
func compileRelease8AndRun(t *testing.T, javac, java, dir, className, src string) (string, bool) {
	t.Helper()
	srcPath := filepath.Join(dir, className+".java")
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out, err := exec.Command(javac, "--release", "8", "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", dir, srcPath).CombinedOutput()
	if err != nil {
		t.Logf("javac --release 8 failed in %s: %v\n%s", dir, err, string(out))
		return string(out), false
	}
	out2, err := exec.Command(java, "-cp", dir, "codec."+className).CombinedOutput()
	if err != nil {
		t.Logf("java run failed in %s: %v\n%s", dir, err, string(out2))
		return string(out2), false
	}
	return strings.TrimSpace(string(out2)), true
}

// TestEnumMarkerCtorSuppressionIsLoadBearing proves the synthetic enum marker-constructor suppression
// is load-bearing. An enum with constant bodies compiled under --release 8 carries a synthetic
// `AccEnum(String,int,AccEnum$1)` ctor; decompiled through the jar (fold) path it must be suppressed so
// the enum recompiles. With suppression DISABLED (JDEC_NO_ENUM_MARKER_CTOR=1) the same fold instead
// emits the garbage ctor (`this(var1,var2)` after local decls, referencing the folded-away $1) and the
// round-trip must fail; with suppression enabled the round-trip recompiles to the golden fingerprint.
func TestEnumMarkerCtorSuppressionIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping enum marker-ctor suppression test")
	}

	const className = "AccEnum"
	origDir := t.TempDir()
	golden, ok := compileRelease8AndRun(t, javac, java, origDir, className, accEnumMarkerCtorSource)
	if !ok {
		t.Skipf("could not compile/run --release 8 original (toolchain may lack release 8); got:\n%s", golden)
	}
	t.Logf("golden fingerprint [%s]: %s", className, golden)

	roundTrips := func() (string, bool) {
		jarFS := buildJarFromDir(t, origDir)
		src, err := jarFS.ReadFile("codec/" + className + ".class")
		if err != nil {
			return "jar ReadFile error: " + err.Error(), false
		}
		got, ok := compileRelease8AndRun(t, javac, java, t.TempDir(), className, string(src))
		return got, ok && got == golden
	}

	os.Setenv("JDEC_NO_ENUM_MARKER_CTOR", "1")
	got, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_ENUM_MARKER_CTOR")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with marker-ctor suppression disabled, but it succeeded; the suppression is not load-bearing", className)
	}
	t.Logf("marker-ctor suppression OFF (expected failure) for %s: %s", className, got)

	got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with marker-ctor suppression enabled; got: %s", className, got)
	}
	t.Logf("marker-ctor suppression ON (round-trip preserved) for %s: %s", className, got)
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

// TestNarrowParamFieldIsLoadBearing proves Bug AH's fix (re-declaring a narrow int-category parameter
// with its authoritative descriptor type) is load-bearing: with the kill-switch the constructor's
// char/byte/short parameters render as `int`, and storing them into their same-typed fields fails to
// recompile; with the fix the parameters keep their descriptor types and the battery round-trips with a
// matching fingerprint.
func TestNarrowParamFieldIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping narrow-param-field test")
	}

	const className = "NarrowParamField"
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

	// Disable the narrow-param descriptor fix to prove it is load-bearing. The compensating
	// narrowing-reassignment cast (java_statements.go: `this.f = (char) intParam`) would otherwise
	// mask the COMPILE error by casting the int-typed parameter back to the field type -- but that
	// silently changes the constructor descriptor (C)V -> (I)V, so the narrow-param fix is still
	// required for descriptor faithfulness. To isolate the narrow-param proof we also disable that
	// cast here so the original "possible lossy conversion" failure resurfaces.
	os.Setenv("JDEC_PARAM_DESC_NARROW_OFF", "1")
	os.Setenv("JDEC_NO_NARROW_REASSIGN_CAST", "1")
	srcOff, gotOff, okOff := roundTrips()
	os.Unsetenv("JDEC_PARAM_DESC_NARROW_OFF")
	os.Unsetenv("JDEC_NO_NARROW_REASSIGN_CAST")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the narrow-param descriptor fix disabled, but it succeeded; the fix is not load-bearing.\n----- src -----\n%s", className, srcOff)
	}
	t.Logf("narrow-param descriptor fix OFF (expected failure) for %s: %s", className, gotOff)

	srcOn, gotOn, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the narrow-param descriptor fix enabled; got: %s\n----- src -----\n%s", className, gotOn, srcOn)
	}
	// The fix must actually change the constructor parameter rendering (int -> char/byte/short);
	// identical output would mean the switch is inert and the proof above is hollow.
	if srcOn == srcOff {
		t.Fatalf("expected enabled vs disabled rendering of %s to differ in parameter types; they are identical:\n%s", className, srcOn)
	}
	t.Logf("narrow-param descriptor fix ON (round-trip preserved) for %s: %s", className, gotOn)
}

// TestNarrowFieldReassignCastIsLoadBearing proves the narrowing-reassignment cast in
// AssignStatement.String (java_statements.go) is load-bearing. The NarrowFieldReassign battery
// reassigns a char/byte/short FIELD from a non-constant int-category conditional (`this.quote =
// single ? '\'' : '"'`), which javac lowers to `putfield ... C` with NO i2c (the constants fit), so
// the decompiler recovers an int-typed conditional. With the cast DISABLED
// (JDEC_NO_NARROW_REASSIGN_CAST=1) the field store renders `this.quote = single ? 39 : 34`, which
// fails to recompile ("possible lossy conversion from int to char"). With the cast enabled the store
// renders `this.quote = (char)(single ? 39 : 34)` and the battery round-trips with a matching
// fingerprint. If someone deletes the cast, the "guard off" branch starts round-tripping and this
// test fails.
func TestNarrowFieldReassignCastIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping narrow-field-reassign-cast test")
	}

	const className = "NarrowFieldReassign"
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

	// Cast OFF: the int-typed conditional is stored into the char/byte/short field verbatim, which
	// fails to recompile ("possible lossy conversion").
	os.Setenv("JDEC_NO_NARROW_REASSIGN_CAST", "1")
	srcOff, gotOff, okOff := roundTrips()
	os.Unsetenv("JDEC_NO_NARROW_REASSIGN_CAST")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the narrowing-reassignment cast disabled, but it succeeded; the cast is not load-bearing.\n----- src -----\n%s", className, srcOff)
	}
	t.Logf("narrowing-reassign cast OFF (expected failure) for %s: %s", className, gotOff)

	// Cast ON: the explicit (char)/(byte)/(short) cast makes the field store recompile and the
	// fingerprint matches the original.
	srcOn, gotOn, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the narrowing-reassignment cast enabled; got: %s\n----- src -----\n%s", className, gotOn, srcOn)
	}
	if !strings.Contains(srcOn, "(char) (") {
		t.Fatalf("expected the enabled rendering to emit an explicit `(char)` narrowing cast; got:\n%s", srcOn)
	}
	t.Logf("narrowing-reassign cast ON (round-trip preserved) for %s: %s", className, gotOn)
}

// TestShortCircuitArrayArgIsLoadBearing proves the inline-array-init step-over in the shared-leaf
// ternary builder (Bug AK) is load-bearing. A method returning `(a && b) || call(..., new T[]{...})`
// (commons-codec DoubleMetaphone.conditionC0's shape) builds its varargs argument with the javac
// `anewarray; dup; idx; ldc; aastore` idiom. The element store made buildSharedLeafTernary decline
// (it conflated the inline array build with statement dispatch), dropping into the legacy combiner
// which mis-wired the leading condition and emitted a missing-return method. With the step-over
// DISABLED (JDEC_ARRAYINIT_TERNARY_OFF=1) the decompiled source must fail to round-trip (missing
// return / inverted boolean); with it enabled the boolean materializes correctly and the round-trip
// is byte-for-byte clean.
func TestShortCircuitArrayArgIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping short-circuit array-arg test")
	}

	const className = "ShortCircuitArrayArg"
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

	os.Setenv("JDEC_ARRAYINIT_TERNARY_OFF", "1")
	srcOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_ARRAYINIT_TERNARY_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the inline-array-init ternary step-over disabled, but it succeeded; the fix is not load-bearing.\n----- src -----\n%s", className, srcOff)
	}
	t.Logf("array-init ternary step-over OFF (expected failure) for %s: %s", className, got)

	srcOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the inline-array-init ternary step-over enabled; got: %s\n----- src -----\n%s", className, got, srcOn)
	}
	// The fix must produce a real short-circuit return, not the broken if-guard the legacy path emits.
	if !strings.Contains(srcOn, "||") {
		t.Fatalf("expected the enabled rendering to materialize a `||` short-circuit return; got:\n%s", srcOn)
	}
	t.Logf("array-init ternary step-over ON (round-trip preserved) for %s: %s", className, got)
}

// TestShortCircuitArrayLeafIsLoadBearing proves the merge-time true/false re-pin (Bug AL) is
// load-bearing. An if/else chain whose arms RETURN inline `new char[]{...}` arrays, guarded by
// `A && (B || C)` (and its De Morgan `A && !(B && C)`) shape, is the commons-codec Nysiis
// transcodeRemaining body. The multi-opcode array-construction leaf made an upstream fold reorder the
// if-node's Next to [true,false], so JmpNode pinning captured trueIndex=0; mergeCondition then rebuilt
// Next as [false,true] WITHOUT refreshing the stale TrueNode/FalseNode closures, silently inverting
// the merged condition (dropping the `!`) and swapping the then/else arms. The source still compiled,
// so only behavioral differential testing catches it. With the re-pin DISABLED
// (JDEC_MERGEIF_PIN_OFF=1) the decompiled source must diverge from the golden fingerprint; with it
// enabled the round-trip is byte-for-byte clean.
func TestShortCircuitArrayLeafIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping short-circuit array-leaf test")
	}

	const className = "ShortCircuitArrayLeaf"
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

	os.Setenv("JDEC_MERGEIF_PIN_OFF", "1")
	srcOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_MERGEIF_PIN_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the merge-time true/false re-pin disabled, but it succeeded; the fix is not load-bearing.\n----- src -----\n%s", className, srcOff)
	}
	t.Logf("mergeIf re-pin OFF (expected divergence) for %s: %s", className, got)

	srcOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the merge-time true/false re-pin enabled; got: %s\n----- src -----\n%s", className, got, srcOn)
	}
	t.Logf("mergeIf re-pin ON (round-trip preserved) for %s: %s", className, got)
}

// TestIfElseParallelArrayPhiIsLoadBearing proves prebindParallelTypedIfElseDefs (rewriter/rewrite_var.go)
// is load-bearing. The IfElseParallelArrayPhi battery assigns the SAME jvm slot a long[] in BOTH arms
// of an if (read after the if) and then reuses that slot for a different-typed scalar (long), so the
// simulator's DFS clobbers the slot table and mints DIFFERENT VariableIds for the two arm defs. The
// existing prebindEscapingIfElseSlots only unifies arms that SHARE a VarUid, so this cross-VarUid
// same-type phi slips past it: each arm keeps its own `long[] data = ...` declaration and the post-if
// `data[...]` reads bind to only one arm's id, leaving the variable out of scope ("cannot find symbol:
// variable varN"). With the parallel-typed prebind DISABLED (JDEC_IFELSE_PARALLEL_PREBIND_OFF=1) the
// decompiled source must fail to recompile; with it enabled the two defs converge onto one hoisted
// `long[] data;` declaration and the round-trip is byte-for-byte clean. This is the dominant fastjson2
// whole-tree blocker (ObjectReaderProvider.<init> long[] acceptHashCodes and the broader "variable
// varN not found" symbol family).
func TestIfElseParallelArrayPhiIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping if/else parallel-array phi test")
	}

	const className = "IfElseParallelArrayPhi"
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

	os.Setenv("JDEC_IFELSE_PARALLEL_PREBIND_OFF", "1")
	srcOff, got, okOff := roundTrips()
	os.Unsetenv("JDEC_IFELSE_PARALLEL_PREBIND_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the parallel-typed if/else prebind disabled, but it succeeded; the fix is not load-bearing.\n----- src -----\n%s", className, srcOff)
	}
	t.Logf("parallel if/else prebind OFF (expected failure) for %s: %s", className, got)

	srcOn, got, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the parallel-typed if/else prebind enabled; got: %s\n----- src -----\n%s", className, got, srcOn)
	}
	t.Logf("parallel if/else prebind ON (round-trip preserved) for %s: %s", className, got)
}

// varargsAbstractOverrideSource declares an ABSTRACT varargs method in a base class and OVERRIDES it
// in a concrete subclass. javac records ACC_VARARGS on the descriptor `(...[J)`; the last param type
// from the descriptor is the array `long[]`. The abstract-method render path must drop one array
// dimension and emit `long...` (the element type + ellipsis), NOT `long[]...` (the full array type +
// ellipsis). `long[]...` is a varargs of `long[]` (descriptor [[J), which is NOT override-equivalent
// to the subclass's faithfully-rendered `long...` (descriptor [J) -> javac rejects the subclass with
// "VAOImpl is not abstract and does not override abstract method combine(long,long,long[]) in VAOBase".
// Top-level sibling classes (no `$` nesting) keep the round-trip free of the flat-inner-class confound.
// Mirrors fastjson2 JSONPath.set(Object,Object,JSONReader.Feature...) and its 6 concrete subclasses.
const varargsAbstractOverrideSource = `package codec;

public class VarargsAbstractOverride {
    public static void main(String[] args) {
        VAOBase b = new VAOImpl();
        long r = b.combine(2L, 3L, 5L, 7L, 11L);
        System.out.println("fingerprint=" + r);
    }
}

abstract class VAOBase {
    abstract long combine(long base, long seed, long... extra);
}

class VAOImpl extends VAOBase {
    long combine(long base, long seed, long... extra) {
        long acc = (base * 1000003L) + seed;
        for (long e : extra) {
            acc = (acc * 31L) + e;
        }
        return acc;
    }
}
`

// TestVarargsAbstractMethodRenderIsLoadBearing proves the abstract-method varargs render fix
// (dumper.go: strip one array dimension for the last varargs param of an ABSTRACT method, kill-switch
// JDEC_VARARGS_ABSTRACT_FIX_OFF) is load-bearing. With the fix DISABLED the abstract base renders
// `long[]... extra` while the concrete override renders `long... extra`; the two are not
// override-equivalent so the subclass fails to recompile ("is not abstract and does not override").
// With the fix ENABLED both render `long...` and the whole-tree round-trip recompiles + runs to the
// golden fingerprint. This flipped 6 fastjson2 files (the JSONPath.set abstract-varargs family) clean.
func TestVarargsAbstractMethodRenderIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	java, err2 := exec.LookPath("java")
	if err1 != nil || err2 != nil {
		t.Skip("javac/java not available; skipping varargs abstract render test")
	}

	const className = "VarargsAbstractOverride"
	origDir := t.TempDir()
	golden, ok := compileAndRunJavaBattery(t, javac, java, origDir, className, varargsAbstractOverrideSource)
	if !ok {
		t.Fatalf("failed to compile/run original %s:\n%s", className, golden)
	}
	t.Logf("golden fingerprint [%s]: %s", className, golden)

	classDir := filepath.Join(origDir, "codec")
	entries, err := os.ReadDir(classDir)
	if err != nil {
		t.Fatalf("read class dir: %v", err)
	}
	var classFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".class") {
			classFiles = append(classFiles, e.Name())
		}
	}
	if len(classFiles) < 3 {
		t.Fatalf("expected >=3 compiled classes (outer+base+impl), got %v", classFiles)
	}

	// Decompile every top-level sibling .class, recompile them TOGETHER (whole-tree), and run main.
	roundTrips := func() (string, string, bool) {
		recDir := t.TempDir()
		pkgDir := filepath.Join(recDir, "codec")
		if e := os.MkdirAll(pkgDir, 0o755); e != nil {
			return "", "mkdir: " + e.Error(), false
		}
		var javaFiles []string
		var combined strings.Builder
		for _, cf := range classFiles {
			raw, e := os.ReadFile(filepath.Join(classDir, cf))
			if e != nil {
				return "", "read class: " + e.Error(), false
			}
			decompiled, e := javaclassparser.Decompile(raw)
			if e != nil {
				return "", "decompile error: " + e.Error(), false
			}
			base := strings.TrimSuffix(cf, ".class")
			jf := filepath.Join(pkgDir, base+".java")
			if e := os.WriteFile(jf, []byte(decompiled), 0o644); e != nil {
				return "", "write java: " + e.Error(), false
			}
			javaFiles = append(javaFiles, jf)
			combined.WriteString("// ===== " + base + ".java =====\n")
			combined.WriteString(decompiled)
			combined.WriteString("\n")
		}
		args := append([]string{"-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", recDir}, javaFiles...)
		out, e := exec.Command(javac, args...).CombinedOutput()
		if e != nil {
			return combined.String(), "javac: " + string(out), false
		}
		out2, e := exec.Command(java, "-cp", recDir, "codec."+className).CombinedOutput()
		if e != nil {
			return combined.String(), "java: " + string(out2), false
		}
		got := strings.TrimSpace(string(out2))
		return combined.String(), got, got == golden
	}

	os.Setenv("JDEC_VARARGS_ABSTRACT_FIX_OFF", "1")
	srcOff, gotOff, okOff := roundTrips()
	os.Unsetenv("JDEC_VARARGS_ABSTRACT_FIX_OFF")
	if okOff {
		t.Fatalf("expected %s to FAIL the round-trip with the abstract-varargs render fix disabled, but it succeeded; the fix is not load-bearing.\n----- src -----\n%s", className, srcOff)
	}
	t.Logf("varargs abstract render OFF (expected failure) for %s: %s", className, gotOff)

	srcOn, gotOn, okOn := roundTrips()
	if !okOn {
		t.Fatalf("expected %s to round-trip cleanly with the abstract-varargs render fix enabled; got: %s\n----- src -----\n%s", className, gotOn, srcOn)
	}
	t.Logf("varargs abstract render ON (round-trip preserved) for %s: %s", className, gotOn)
}

// fnvStubSource is a minimal hand-written stand-in for com.alibaba.fastjson2.util.Fnv, providing only
// the `hashCode64(String)` method that the pinned SymbolTable references. It isolates the SymbolTable
// final-field defect from Fnv's own (unrelated) decompile defects so the load-bearing signal is clean.
const fnvStubSource = `package com.alibaba.fastjson2.util;

public class Fnv {
    public static long hashCode64(String s) {
        long h = -3750763034362895579L;
        for (int i = 0; i < s.length(); i++) {
            h ^= s.charAt(i);
            h *= 1099511628211L;
        }
        return h;
    }
}
`

// TestFinalFieldRenamedLocalHoistIsLoadBearing proves the final-field-initializer hoist guard's
// renamed-local fix (dumper.go localSlotRefRe now matches the collision-renamed `varN_M`, kill-switch
// JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF) is load-bearing. The pinned fastjson2 SymbolTable.class assigns
// a `final long hashCode64` ONCE at the end of its constructor from a local the renamer disambiguates
// to `var7_1` (slot 7 is reused across disjoint live ranges - a javac slot allocation a synthetic
// javac-17 battery will not reliably reproduce, hence the pinned real class). With the legacy narrow
// matcher restored (kill-switch ON) the assignment is wrongly lifted into a field initializer
// `final long hashCode64 = var7_1;` that references an out-of-scope constructor local ("cannot find
// symbol: variable var7_1"); with the fix the assignment stays in the constructor (blank final) and
// the decompiled SymbolTable recompiles cleanly (against a minimal Fnv stub). Mirrors the whole-jar
// shape of fastjson2 SymbolTable.hashCode64 and FactoryFunction.function.
func TestFinalFieldRenamedLocalHoistIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping final-field renamed-local hoist test")
	}

	raw, err := regressionFS.ReadFile("testdata/regression/final_field_renamed_local.class")
	if err != nil {
		t.Fatalf("read pinned SymbolTable class: %v", err)
	}

	// Compile the decompiled SymbolTable together with a minimal Fnv stub; success/failure of javac is
	// the load-bearing signal (the defect is a pure "cannot find symbol", a recompilability defect).
	compiles := func() (string, string, bool) {
		decompiled, err := javaclassparser.Decompile(raw)
		if err != nil {
			return "", "decompile error: " + err.Error(), false
		}
		dir := t.TempDir()
		stPkg := filepath.Join(dir, "com", "alibaba", "fastjson2")
		fnvPkg := filepath.Join(stPkg, "util")
		if e := os.MkdirAll(fnvPkg, 0o755); e != nil {
			return decompiled, "mkdir: " + e.Error(), false
		}
		stFile := filepath.Join(stPkg, "SymbolTable.java")
		fnvFile := filepath.Join(fnvPkg, "Fnv.java")
		if e := os.WriteFile(stFile, []byte(decompiled), 0o644); e != nil {
			return decompiled, "write SymbolTable: " + e.Error(), false
		}
		if e := os.WriteFile(fnvFile, []byte(fnvStubSource), 0o644); e != nil {
			return decompiled, "write Fnv stub: " + e.Error(), false
		}
		out, e := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn", "-d", filepath.Join(dir, "out"), stFile, fnvFile).CombinedOutput()
		if e != nil {
			return decompiled, "javac: " + string(out), false
		}
		return decompiled, "", true
	}

	os.Setenv("JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF", "1")
	srcOff, gotOff, okOff := compiles()
	os.Unsetenv("JDEC_FIELD_HOIST_RENAMED_LOCAL_OFF")
	if okOff {
		t.Fatalf("expected pinned SymbolTable to FAIL recompile with the renamed-local hoist guard disabled, but it compiled; the fix is not load-bearing.\n----- src -----\n%s", srcOff)
	}
	if !strings.Contains(gotOff, "cannot find symbol") {
		t.Fatalf("expected the OFF failure to be a 'cannot find symbol' for the hoisted renamed local; got: %s\n----- src -----\n%s", gotOff, srcOff)
	}
	t.Logf("renamed-local hoist guard OFF (expected cannot-find-symbol) for SymbolTable: %s", gotOff)

	srcOn, gotOn, okOn := compiles()
	if !okOn {
		t.Fatalf("expected pinned SymbolTable to recompile cleanly with the renamed-local hoist guard enabled; got: %s\n----- src -----\n%s", gotOn, srcOn)
	}
	t.Logf("renamed-local hoist guard ON (SymbolTable recompiles clean)")
}

// TestParallelArmPhiOrphanHoistIsLoadBearing pins fastjson2's FieldWriterListFunc, whose writeValue
// jsonb loop has the canonical "if/else parallel-phi orphan read": a JVM slot first-declared in BOTH
// arms of one if/else (cross-VarUid, `ObjectWriter var10 = var5` vs `var10 = getItemWriter(..)`) then
// read after the join (`var10.write(..)`). Without parallelArmDeclHoist each arm keeps its own
// `ObjectWriter var10 = ...` so the post-join read is out of scope and javac rejects it as
// "cannot find symbol: variable var10". The pass emits one dominating `ObjectWriter var10;` and demotes
// both arms, putting the read back in scope.
//
// The class is compiled against the real fastjson2 jar (its writer hierarchy is too large to stub).
// Other isolated-compile noise is unrelated (e.g. the JSONWriter$Feature nested-type `$` reference,
// Bug AD), so the load-bearing signal is precisely the var10 symbol error: present with the pass OFF,
// absent with it ON. Kill-switch: JDEC_PARALLEL_ARM_HOIST_OFF.
func TestParallelArmPhiOrphanHoistIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping parallel-arm phi orphan hoist test")
	}
	jar := jarPaths["fastjson2"]
	if _, e := os.Stat(jar); e != nil {
		t.Skipf("fastjson2 jar missing (%s); skipping", jar)
	}
	raw, err := regressionFS.ReadFile("testdata/regression/parallel_arm_phi_orphan.class")
	if err != nil {
		t.Fatalf("read pinned FieldWriterListFunc class: %v", err)
	}

	// Returns the javac output of compiling the decompiled unit against the fastjson2 jar.
	compileOut := func() (string, string) {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		dir := t.TempDir()
		pkg := filepath.Join(dir, "com", "alibaba", "fastjson2", "writer")
		if e := os.MkdirAll(pkg, 0o755); e != nil {
			t.Fatalf("mkdir: %v", e)
		}
		src := filepath.Join(pkg, "FieldWriterListFunc.java")
		if e := os.WriteFile(src, []byte(decompiled), 0o644); e != nil {
			t.Fatalf("write src: %v", e)
		}
		out, _ := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-proc:none", "--release", "8", "-cp", jar, "-d", filepath.Join(dir, "out"), src).CombinedOutput()
		return decompiled, string(out)
	}

	const orphanSym = "variable var10"

	os.Setenv("JDEC_PARALLEL_ARM_HOIST_OFF", "1")
	srcOff, outOff := compileOut()
	os.Unsetenv("JDEC_PARALLEL_ARM_HOIST_OFF")
	if !strings.Contains(outOff, "cannot find symbol") || !strings.Contains(outOff, orphanSym) {
		t.Fatalf("expected pinned FieldWriterListFunc to FAIL with a 'cannot find symbol: %s' when the parallel-arm hoist is disabled, so the fix is load-bearing; got javac:\n%s\n----- src -----\n%s", orphanSym, outOff, srcOff)
	}
	t.Logf("parallel-arm hoist OFF (expected orphan-read %q cannot-find-symbol present)", orphanSym)

	srcOn, outOn := compileOut()
	if strings.Contains(outOn, orphanSym) {
		t.Fatalf("expected the orphan-read %q symbol error to be GONE with the parallel-arm hoist enabled; got javac:\n%s\n----- src -----\n%s", orphanSym, outOn, srcOn)
	}
	t.Logf("parallel-arm hoist ON (orphan-read %q resolved)", orphanSym)
}

// TestEmptyVoidOverrideEmittedIsLoadBearing pins fastjson2's ObjectWriterBaseModule$VoidObjectWriter,
// a no-op writer whose sole real method is an empty-bodied override `void write(JSONWriter,Object,
// Object,Type,long) {}` (bytecode: a bare `return`). The dumper previously DROPPED every empty-body
// void method, so VoidObjectWriter decompiled with no methods at all and javac rejected the class with
// "is not abstract and does not override abstract method write(...) in ObjectWriter". methodBodyIsTrivially
// Empty now keeps the faithful empty override. The pinned class compiles against the real fastjson2 jar
// (ObjectWriter lives there): with the fix OFF the override is missing, with it ON the class is clean.
// Kill-switch: JDEC_NO_EMIT_EMPTY_VOID.
func TestEmptyVoidOverrideEmittedIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping empty-void override test")
	}
	jar := jarPaths["fastjson2"]
	if _, e := os.Stat(jar); e != nil {
		t.Skipf("fastjson2 jar missing (%s); skipping", jar)
	}
	raw, err := regressionFS.ReadFile("testdata/regression/empty_void_override.class")
	if err != nil {
		t.Fatalf("read pinned VoidObjectWriter class: %v", err)
	}

	compileOut := func() (string, string) {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		dir := t.TempDir()
		pkg := filepath.Join(dir, "com", "alibaba", "fastjson2", "writer")
		if e := os.MkdirAll(pkg, 0o755); e != nil {
			t.Fatalf("mkdir: %v", e)
		}
		src := filepath.Join(pkg, "ObjectWriterBaseModule$VoidObjectWriter.java")
		if e := os.WriteFile(src, []byte(decompiled), 0o644); e != nil {
			t.Fatalf("write src: %v", e)
		}
		out, _ := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-proc:none", "--release", "8", "-cp", jar, "-d", filepath.Join(dir, "out"), src).CombinedOutput()
		return decompiled, string(out)
	}

	const overrideErr = "does not override abstract method write"

	os.Setenv("JDEC_NO_EMIT_EMPTY_VOID", "1")
	srcOff, outOff := compileOut()
	os.Unsetenv("JDEC_NO_EMIT_EMPTY_VOID")
	if strings.Contains(srcOff, "void write(") {
		t.Fatalf("expected the empty void override to be ABSENT in the decompiled source with the emit-empty-void fix disabled; got src:\n%s", srcOff)
	}
	if !strings.Contains(outOff, overrideErr) {
		t.Fatalf("expected pinned VoidObjectWriter to FAIL with %q when the empty-void emit is disabled, so the fix is load-bearing; got javac:\n%s\n----- src -----\n%s", overrideErr, outOff, srcOff)
	}
	t.Logf("emit-empty-void OFF (expected %q present)", overrideErr)

	srcOn, outOn := compileOut()
	if !strings.Contains(srcOn, "void write(") {
		t.Fatalf("expected the empty void override `void write(...) {}` to be PRESENT with the fix enabled; got src:\n%s", srcOn)
	}
	if strings.Contains(outOn, overrideErr) {
		t.Fatalf("expected the %q error to be GONE with the empty-void emit enabled; got javac:\n%s\n----- src -----\n%s", overrideErr, outOn, srcOn)
	}
	t.Logf("emit-empty-void ON (empty override emitted, class compiles clean)")
}

// TestTrySlotPhiMergeIsLoadBearing pins fastjson2's FieldReaderDoubleFunc, whose readFieldValue is the
// canonical try/catch value-fallback idiom: a JVM slot is assigned the read value inside the try body
// (`v = jsonReader.readDouble()`) and a `= null` fallback inside the catch handler, then read after the
// try (`function.accept(object, v)`). Because the catch store runs only on exception it does not
// dominate the try store, so the null-adopt dominator gate splits the slot into two differently-typed
// variables: the post-try read binds to the `Object var = null` branch (the read value is silently
// LOST) and javac additionally rejects `Object cannot be converted to Double`. reachingTrySlotPhiMerge
// (gated by slotDefPhiReachesLoad) proves the two stores converge at a common downstream load and
// continues one `Double var4;`, so the value flows through and the class compiles. Compiled against the
// real fastjson2 jar. Kill-switch: JDEC_TRY_SLOT_PHI_MERGE_OFF.
func TestTrySlotPhiMergeIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping try-slot phi merge test")
	}
	jar := jarPaths["fastjson2"]
	if _, e := os.Stat(jar); e != nil {
		t.Skipf("fastjson2 jar missing (%s); skipping", jar)
	}
	raw, err := regressionFS.ReadFile("testdata/regression/try_slot_phi_merge.class")
	if err != nil {
		t.Fatalf("read pinned FieldReaderDoubleFunc class: %v", err)
	}

	compileOut := func() (string, string) {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		dir := t.TempDir()
		pkg := filepath.Join(dir, "com", "alibaba", "fastjson2", "reader")
		if e := os.MkdirAll(pkg, 0o755); e != nil {
			t.Fatalf("mkdir: %v", e)
		}
		src := filepath.Join(pkg, "FieldReaderDoubleFunc.java")
		if e := os.WriteFile(src, []byte(decompiled), 0o644); e != nil {
			t.Fatalf("write src: %v", e)
		}
		out, _ := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-proc:none", "--release", "8", "-cp", jar, "-d", filepath.Join(dir, "out"), src).CombinedOutput()
		return decompiled, string(out)
	}

	const convErr = "cannot be converted to Double"

	os.Setenv("JDEC_TRY_SLOT_PHI_MERGE_OFF", "1")
	srcOff, outOff := compileOut()
	os.Unsetenv("JDEC_TRY_SLOT_PHI_MERGE_OFF")
	if !strings.Contains(outOff, convErr) {
		t.Fatalf("expected pinned FieldReaderDoubleFunc to FAIL with %q when the try-slot phi merge is disabled, so the fix is load-bearing; got javac:\n%s\n----- src -----\n%s", convErr, outOff, srcOff)
	}
	t.Logf("try-slot phi merge OFF (expected %q present, read value lost into Object var)", convErr)

	srcOn, outOn := compileOut()
	if strings.Contains(outOn, convErr) {
		t.Fatalf("expected the %q error to be GONE with the try-slot phi merge enabled; got javac:\n%s\n----- src -----\n%s", convErr, outOn, srcOn)
	}
	if !strings.Contains(srcOn, "Double var4") {
		t.Fatalf("expected the merged single `Double var4` declaration with the fix enabled; got src:\n%s", srcOn)
	}
	t.Logf("try-slot phi merge ON (one Double var4, value flows through, compiles clean)")
}

// TestCastEscapeHoistIsLoadBearing pins fastjson2's ObjectWriters, whose fieldWriterList is the
// canonical "if/else parallel-phi orphan read, DIFFERENT-rendered-type subfamily": one JVM slot is
// first-declared in BOTH arms of an if/else with DIFFERENT types (`ParameterizedType var3 = ...` vs
// `ParameterizedTypeImpl var3 = ...`) then read after the join only through an explicit cast
// (`createFieldWriter(.., (Type)(var3), ..)`). parallelArmDeclHoist refuses to merge the arms because
// their rendered type tokens disagree and the decompiler has no common-supertype facility, so each arm
// keeps its own decl and the post-join read is out of scope: javac rejects it as
// "cannot find symbol: variable var3". hoistCastGuardedEscapedLocals proves the merge is sound from the
// shape alone - every non-declaration use is a cast - and emits one `Object var3 = null;`, demoting both
// arms. Compiled against the real fastjson2 jar; the load-bearing signal is the var3 symbol error,
// present with the pass OFF and absent with it ON. Kill-switch: JDEC_CAST_ESCAPE_HOIST_OFF.
func TestCastEscapeHoistIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping cast-escape hoist test")
	}
	jar := jarPaths["fastjson2"]
	if _, e := os.Stat(jar); e != nil {
		t.Skipf("fastjson2 jar missing (%s); skipping", jar)
	}
	raw, err := regressionFS.ReadFile("testdata/regression/cast_escape_phi_orphan.class")
	if err != nil {
		t.Fatalf("read pinned ObjectWriters class: %v", err)
	}

	compileOut := func() (string, string) {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		dir := t.TempDir()
		pkg := filepath.Join(dir, "com", "alibaba", "fastjson2", "writer")
		if e := os.MkdirAll(pkg, 0o755); e != nil {
			t.Fatalf("mkdir: %v", e)
		}
		src := filepath.Join(pkg, "ObjectWriters.java")
		if e := os.WriteFile(src, []byte(decompiled), 0o644); e != nil {
			t.Fatalf("write src: %v", e)
		}
		out, _ := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-proc:none", "--release", "8", "-cp", jar, "-d", filepath.Join(dir, "out"), src).CombinedOutput()
		return decompiled, string(out)
	}

	const orphanSym = "variable var3"

	os.Setenv("JDEC_CAST_ESCAPE_HOIST_OFF", "1")
	srcOff, outOff := compileOut()
	os.Unsetenv("JDEC_CAST_ESCAPE_HOIST_OFF")
	if !strings.Contains(outOff, "cannot find symbol") || !strings.Contains(outOff, orphanSym) {
		t.Fatalf("expected pinned ObjectWriters to FAIL with a 'cannot find symbol: %s' when the cast-escape hoist is disabled, so the fix is load-bearing; got javac:\n%s\n----- src -----\n%s", orphanSym, outOff, srcOff)
	}
	t.Logf("cast-escape hoist OFF (expected orphan-read %q cannot-find-symbol present)", orphanSym)

	srcOn, outOn := compileOut()
	if strings.Contains(outOn, orphanSym) {
		t.Fatalf("expected the orphan-read %q symbol error to be GONE with the cast-escape hoist enabled; got javac:\n%s\n----- src -----\n%s", orphanSym, outOn, srcOn)
	}
	if !strings.Contains(srcOn, "Object var3 = null;") {
		t.Fatalf("expected the hoisted `Object var3 = null;` declaration with the fix enabled; got src:\n%s", srcOn)
	}
	t.Logf("cast-escape hoist ON (one Object var3, both arms demoted, orphan-read resolved)")
}

// TestPolymorphicSignatureCastIsLoadBearing pins fastjson2's JSONReader$BigIntegerCreator, whose static
// initializer is the canonical signature-polymorphic call: `LambdaMetafactory...getTarget().invokeExact()`
// is a @PolymorphicSignature MethodHandle method whose per-call-site bytecode descriptor returns the REAL
// type (`()Ljava/util/function/BiFunction;`) while the SOURCE-apparent return type is always Object - the
// original source therefore reads `(BiFunction) handle.invokeExact()`. The decompiler types the call from
// the descriptor and emitted `BiFunction var3 = handle.invokeExact()` with no cast, which javac rejects
// ("incompatible types: Object cannot be converted to BiFunction"). polymorphicSignatureCastType re-emits
// the descriptor-return cast `(BiFunction)(...)`. Compiled against the real fastjson2 jar; the load-bearing
// signal is the conversion error, present with the pass OFF and absent with it ON. This pattern (always
// `...getTarget().invokeExact()`) is fastjson2's single biggest 1-error family. Kill-switch:
// JDEC_NO_POLYSIG_CAST.
func TestPolymorphicSignatureCastIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping polymorphic-signature cast test")
	}
	jar := jarPaths["fastjson2"]
	if _, e := os.Stat(jar); e != nil {
		t.Skipf("fastjson2 jar missing (%s); skipping", jar)
	}
	raw, err := regressionFS.ReadFile("testdata/regression/polysig_invokeexact.class")
	if err != nil {
		t.Fatalf("read pinned JSONReader$BigIntegerCreator class: %v", err)
	}

	compileOut := func() (string, string) {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		dir := t.TempDir()
		pkg := filepath.Join(dir, "com", "alibaba", "fastjson2")
		if e := os.MkdirAll(pkg, 0o755); e != nil {
			t.Fatalf("mkdir: %v", e)
		}
		src := filepath.Join(pkg, "JSONReader$BigIntegerCreator.java")
		if e := os.WriteFile(src, []byte(decompiled), 0o644); e != nil {
			t.Fatalf("write src: %v", e)
		}
		out, _ := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-proc:none", "--release", "8", "-cp", jar, "-d", filepath.Join(dir, "out"), src).CombinedOutput()
		return decompiled, string(out)
	}

	const convErr = "cannot be converted to BiFunction"

	os.Setenv("JDEC_NO_POLYSIG_CAST", "1")
	srcOff, outOff := compileOut()
	os.Unsetenv("JDEC_NO_POLYSIG_CAST")
	if !strings.Contains(outOff, convErr) {
		t.Fatalf("expected pinned BigIntegerCreator to FAIL with %q when the polysig cast is disabled, so the fix is load-bearing; got javac:\n%s\n----- src -----\n%s", convErr, outOff, srcOff)
	}
	t.Logf("polysig cast OFF (expected %q present, invokeExact result typed as Object)", convErr)

	srcOn, outOn := compileOut()
	if strings.Contains(outOn, convErr) {
		t.Fatalf("expected the %q error to be GONE with the polysig cast enabled; got javac:\n%s\n----- src -----\n%s", convErr, outOn, srcOn)
	}
	if !strings.Contains(srcOn, "(BiFunction)(") {
		t.Fatalf("expected the re-emitted `(BiFunction)(...)` cast with the fix enabled; got src:\n%s", srcOn)
	}
	t.Logf("polysig cast ON (invokeExact result down-cast to descriptor return type, compiles clean)")
}

// TestTypeVarFieldStoreCastIsLoadBearing pins guava's CompactHashMap$MapEntry, whose constructor
// stores a raw `keys[]` element (typed Object) into the `private final K key` field. Bytecode erases
// the field to its bound, so without an explicit `(K)` cast the re-emitted source fails to recompile
// ("incompatible types: Object cannot be converted to K") - the whole-tree A/B confirms guava -22
// with this fix. The assertion is SOURCE-LEVEL (not javac) on purpose: this is a flat `Outer$Inner`
// unit, which javac 17 cannot compile standalone (it crashes in Flow$AliveAnalyzer, masking the body
// error); the dep-aware whole-tree compile is where the javac acceptance is measured. Here we pin the
// exact rendering both ways so the fix can never silently regress: with JDEC_NO_TYPEVAR_FIELD_CAST the
// store must be the bare `this.key = var...keys[...]` (no cast), and with the fix it must carry `(K)`.
func TestTypeVarFieldStoreCastIsLoadBearing(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/typevar_field_store.class")
	if err != nil {
		t.Fatalf("read pinned CompactHashMap$MapEntry class: %v", err)
	}
	decompileSrc := func() string {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		return decompiled
	}

	os.Setenv("JDEC_NO_TYPEVAR_FIELD_CAST", "1")
	srcOff := decompileSrc()
	os.Unsetenv("JDEC_NO_TYPEVAR_FIELD_CAST")
	if strings.Contains(srcOff, "this.key = (K) (") {
		t.Fatalf("expected NO `(K)` cast on the field store when JDEC_NO_TYPEVAR_FIELD_CAST=1, so the fix is load-bearing; got src:\n%s", srcOff)
	}
	if !strings.Contains(srcOff, "this.key = var") {
		t.Fatalf("expected the bare `this.key = var...` field store with the cast disabled; got src:\n%s", srcOff)
	}
	t.Logf("typevar field-store cast OFF (bare `this.key = var1.keys[var2]`, would fail Object->K)")

	srcOn := decompileSrc()
	if !strings.Contains(srcOn, "this.key = (K) (") {
		t.Fatalf("expected the re-emitted `this.key = (K) (...)` cast with the fix enabled; got src:\n%s", srcOn)
	}
	t.Logf("typevar field-store cast ON (Object keys[] element down-cast to K, recompiles clean)")
}

// TestTypeVarArrayCastIsLoadBearing pins the array-of-type-variable arm of the type-var cast family
// (whole-tree A/B: guava 757->731). Two real guava units, source-level assertions (both are flat
// `Outer$Inner` units that javac 17 cannot compile standalone; the dep-aware whole-tree compile is
// where javac acceptance is measured):
//   - return side: LocalCache$AbstractCacheSet.toArray, `public <E> E[] toArray(E[] v) { return
//     coll.toArray(v); }`. Collection.toArray(E[]) erases to Object[], so the `E[]` return needs an
//     unchecked `(E[])` cast (kill-switch JDEC_TYPEVAR_RET_CAST_OFF).
//   - field-store side: Lists$OnePlusArrayList stores `(Object[]) checkNotNull(var2)` into the
//     `final E[] rest` field; the erased Object[] needs an `(E[])` cast (kill-switch
//     JDEC_NO_TYPEVAR_FIELD_CAST).
func TestTypeVarArrayCastIsLoadBearing(t *testing.T) {
	decompilePinned := func(name string) func() string {
		raw, err := regressionFS.ReadFile("testdata/regression/" + name)
		if err != nil {
			t.Fatalf("read pinned %s: %v", name, err)
		}
		return func() string {
			decompiled, e := javaclassparser.Decompile(raw)
			if e != nil {
				t.Fatalf("decompile %s: %v", name, e)
			}
			return decompiled
		}
	}

	// return side: `<E> E[] toArray(E[])` -> `return (E[]) (...toArray(var1));`
	retSrc := decompilePinned("typevar_array_return.class")
	os.Setenv("JDEC_TYPEVAR_RET_CAST_OFF", "1")
	retOff := retSrc()
	os.Unsetenv("JDEC_TYPEVAR_RET_CAST_OFF")
	if strings.Contains(retOff, "return (E[]) (") {
		t.Fatalf("expected NO `(E[])` return cast with JDEC_TYPEVAR_RET_CAST_OFF=1, so the fix is load-bearing; got src:\n%s", retOff)
	}
	retOn := retSrc()
	if !strings.Contains(retOn, "return (E[]) (") {
		t.Fatalf("expected the re-emitted `return (E[]) (...)` cast with the fix enabled; got src:\n%s", retOn)
	}
	t.Logf("typevar array return cast ON (Collection.toArray(E[]) -> Object[] down-cast to E[])")

	// field-store side: `final E[] rest` <- `(Object[]) checkNotNull(var2)` -> `(E[]) (...)`
	fieldSrc := decompilePinned("typevar_array_field_store.class")
	os.Setenv("JDEC_NO_TYPEVAR_FIELD_CAST", "1")
	fieldOff := fieldSrc()
	os.Unsetenv("JDEC_NO_TYPEVAR_FIELD_CAST")
	if strings.Contains(fieldOff, "this.rest = (E[]) (") {
		t.Fatalf("expected NO `(E[])` field-store cast with JDEC_NO_TYPEVAR_FIELD_CAST=1; got src:\n%s", fieldOff)
	}
	fieldOn := fieldSrc()
	if !strings.Contains(fieldOn, "this.rest = (E[]) (") {
		t.Fatalf("expected the re-emitted `this.rest = (E[]) (...)` cast with the fix enabled; got src:\n%s", fieldOn)
	}
	t.Logf("typevar array field-store cast ON (Object[] checkNotNull down-cast to E[])")
}

// TestTypeVarArgCastSuppressionIsLoadBearing pins guava's AbstractRangeSet<C extends Comparable>,
// whose `contains(C var1)` calls the same-class `rangeContaining(C var0)`. The descriptor erases the
// parameter to its bound (Comparable), so without suppression the decompiler synthesizes a spurious
// upcast `this.rangeContaining((Comparable)(var1))`; javac binds the call to the generic signature and
// rejects it ("incompatible types: Comparable cannot be converted to C"). A type-variable-typed
// argument is pushed without a checkcast, so the source needs no cast - suppressTypeVarArgCast drops
// it (whole-tree A/B: guava 731->699). Source-level assertion (the unit depends on RangeSet/Range and
// the dep-aware whole-tree compile is where javac acceptance is measured): with the fix the call must
// be bare `rangeContaining(var1)`, and with JDEC_NO_TYPEVAR_ARG_NOCAST=1 it must carry the
// `(Comparable)(` upcast. Kill-switch: JDEC_NO_TYPEVAR_ARG_NOCAST.
func TestTypeVarArgCastSuppressionIsLoadBearing(t *testing.T) {
	raw, err := regressionFS.ReadFile("testdata/regression/typevar_arg_cast.class")
	if err != nil {
		t.Fatalf("read pinned AbstractRangeSet class: %v", err)
	}
	decompileSrc := func() string {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		return decompiled
	}

	os.Setenv("JDEC_NO_TYPEVAR_ARG_NOCAST", "1")
	srcOff := decompileSrc()
	os.Unsetenv("JDEC_NO_TYPEVAR_ARG_NOCAST")
	if !strings.Contains(srcOff, "rangeContaining((Comparable)(") {
		t.Fatalf("expected the spurious `rangeContaining((Comparable)(...))` upcast with JDEC_NO_TYPEVAR_ARG_NOCAST=1, so the fix is load-bearing; got src:\n%s", srcOff)
	}
	t.Logf("typevar arg-cast suppression OFF (spurious `(Comparable)(var1)` upcast, would fail Comparable->C)")

	srcOn := decompileSrc()
	if strings.Contains(srcOn, "rangeContaining((Comparable)(") {
		t.Fatalf("expected the `(Comparable)` upcast to be GONE with the fix enabled; got src:\n%s", srcOn)
	}
	if !strings.Contains(srcOn, "rangeContaining(var1)") {
		t.Fatalf("expected the bare `rangeContaining(var1)` call with the fix enabled; got src:\n%s", srcOn)
	}
	t.Logf("typevar arg-cast suppression ON (bare `rangeContaining(var1)`, type variable already assignable)")
}

// TestIincIntCategorySlotRepairIsLoadBearing pins fastjson2's Fnv.hashCode64LCase (Bug AL: same-slot
// disjoint live-range corruption of the single global slot table by DFS traversal order). Slot 5 is
// reused for THREE disjoint locals: an int loop counter in the first loop, an int char in the second,
// and a `long` hash accumulator in the third loop. The forward simulation's global slot table, mutated
// in DFS order, leaks the LATER `long` reincarnation back onto the first loop's counter iinc, so the
// `i++` renders as `var5_1++` against the long accumulator - which is declared far below, out of scope
// at the counter site ("cannot find symbol: variable var5_1"). The iinc reaching-definition repair
// proves the slot must be int-category at the iinc (the verifier guarantees it) and walks back to the
// reaching int-category definition, rebinding the increment to the correct loop counter `var4`. The
// repair previously fired ONLY when the leaked version was a reference; this extends it to long/float/
// double leaks. Compiled against the real fastjson2 jar (Fnv is a flat top-level class that recompiles
// standalone): the load-bearing signal is the cannot-find-symbol error, present with the repair OFF and
// absent with it ON. Kill-switch: JDEC_IINC_REACHING_OFF.
func TestIincIntCategorySlotRepairIsLoadBearing(t *testing.T) {
	javac, err1 := exec.LookPath("javac")
	if err1 != nil {
		t.Skip("javac not available; skipping iinc int-category slot repair test")
	}
	jar := jarPaths["fastjson2"]
	if _, e := os.Stat(jar); e != nil {
		t.Skipf("fastjson2 jar missing (%s); skipping", jar)
	}
	raw, err := regressionFS.ReadFile("testdata/regression/iinc_intcat_slot.class")
	if err != nil {
		t.Fatalf("read pinned Fnv class: %v", err)
	}

	compileOut := func() (string, string) {
		decompiled, e := javaclassparser.Decompile(raw)
		if e != nil {
			t.Fatalf("decompile: %v", e)
		}
		dir := t.TempDir()
		pkg := filepath.Join(dir, "com", "alibaba", "fastjson2", "util")
		if e := os.MkdirAll(pkg, 0o755); e != nil {
			t.Fatalf("mkdir: %v", e)
		}
		src := filepath.Join(pkg, "Fnv.java")
		if e := os.WriteFile(src, []byte(decompiled), 0o644); e != nil {
			t.Fatalf("write src: %v", e)
		}
		out, _ := exec.Command(javac, "-J-Duser.language=en", "-encoding", "UTF-8", "-nowarn",
			"-proc:none", "--release", "8", "-cp", jar, "-d", filepath.Join(dir, "out"), src).CombinedOutput()
		return decompiled, string(out)
	}

	const symErr = "cannot find symbol"

	os.Setenv("JDEC_IINC_REACHING_OFF", "1")
	srcOff, outOff := compileOut()
	os.Unsetenv("JDEC_IINC_REACHING_OFF")
	if !strings.Contains(outOff, symErr) {
		t.Fatalf("expected pinned Fnv to FAIL with %q when the iinc reaching repair is disabled, so the fix is load-bearing; got javac:\n%s\n----- src -----\n%s", symErr, outOff, srcOff)
	}
	if !strings.Contains(srcOff, "var5_1++") {
		t.Fatalf("expected the corrupted counter increment `var5_1++` (bound to the out-of-scope long accumulator) with the repair disabled; got src:\n%s", srcOff)
	}
	t.Logf("iinc reaching repair OFF (counter `i++` leaked onto long accumulator `var5_1++`, out of scope)")

	srcOn, outOn := compileOut()
	if strings.Contains(outOn, symErr) {
		t.Fatalf("expected the %q error to be GONE with the iinc reaching repair enabled; got javac:\n%s\n----- src -----\n%s", symErr, outOn, srcOn)
	}
	if strings.Contains(srcOn, "var5_1++") {
		t.Fatalf("expected the increment to be rebound off the long accumulator with the fix enabled; got src:\n%s", srcOn)
	}
	if !strings.Contains(srcOn, "var4++") {
		t.Fatalf("expected the increment rebound to the int loop counter `var4++` with the fix enabled; got src:\n%s", srcOn)
	}
	t.Logf("iinc reaching repair ON (increment rebound to int loop counter `var4++`, recompiles clean)")
}
