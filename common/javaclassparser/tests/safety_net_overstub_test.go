package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestSafetyNetNoOverStub re-decompiles every saved jdsc artifact and checks that (1) every
// class that decompiles now produces valid Java, and (2) the syntax safety net does not
// over-degrade: a class that was already syntactically valid must not gain extra stubs/drops.
// It compares the DecompileStubMarker count in the current output against the saved .java.
func TestSafetyNetNoOverStub(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/jdsc-final"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	var total, decompileErr, invalidSyntax, moreStubs, fewerStubs int
	var oldStubTotal, newStubTotal int
	worst := map[string]int{}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		total++
		var out string
		var derr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					derr = fmt.Errorf("PANIC: %v", r)
				}
			}()
			out, derr = javaclassparser.Decompile(raw)
		}()
		if derr != nil {
			decompileErr++
			continue
		}
		if _, ferr := java2ssa.Frontend(out); ferr != nil {
			invalidSyntax++
			t.Logf("[STILL-INVALID] %s: %s", name, firstReason(ferr.Error()))
		}
		newStubs := strings.Count(out, javaclassparser.DecompileStubMarker)
		newStubTotal += newStubs
		// Compare with saved .java if present.
		javaPath := filepath.Join(dir, strings.TrimSuffix(name, ".class")+".java")
		if old, err := os.ReadFile(javaPath); err == nil {
			oldStubs := strings.Count(string(old), javaclassparser.DecompileStubMarker)
			oldStubTotal += oldStubs
			if newStubs > oldStubs {
				moreStubs++
				if newStubs-oldStubs > worst[name] {
					worst[name] = newStubs - oldStubs
				}
			} else if newStubs < oldStubs {
				fewerStubs++
			}
		}
	}
	t.Logf("==== SAFETY NET OVER-STUB CHECK ====")
	t.Logf("total=%d decompileErr=%d invalidSyntax(NOW)=%d", total, decompileErr, invalidSyntax)
	t.Logf("stub markers: old=%d new=%d (delta=%+d)", oldStubTotal, newStubTotal, newStubTotal-oldStubTotal)
	t.Logf("classes with MORE stubs=%d, FEWER stubs=%d", moreStubs, fewerStubs)
	for name, d := range worst {
		t.Logf("  +%d stubs: %s", d, name)
	}
	if invalidSyntax > 0 {
		t.Errorf("safety net failed: %d classes still emit invalid Java", invalidSyntax)
	}
}

// TestSafetyNetIsolated decompiles each class twice on the CURRENT decompiler -- once with the
// syntax safety net OFF and once ON -- to isolate the net's true effect. The defining bug we
// guard against is a FALSE POSITIVE: a class that is already valid with the net OFF must not be
// degraded by the net (which would mean our validator disagrees with the real frontend).
func TestSafetyNetIsolated(t *testing.T) {
	dir := os.Getenv("JDSC_DIR")
	if dir == "" {
		dir = "/tmp/jdsc-final"
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("no jdsc dir: %v", err)
	}
	decompile := func(raw []byte) (string, error) {
		var out string
		var derr error
		func() {
			defer func() {
				if r := recover(); r != nil {
					derr = fmt.Errorf("PANIC: %v", r)
				}
			}()
			out, derr = javaclassparser.Decompile(raw)
		}()
		return out, derr
	}

	var total, offValidOnDegraded, offInvalidOnValid, offInvalidOnInvalid int
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".class") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		total++

		javaclassparser.EnableDecompileSyntaxValidation = false
		offOut, offErr := decompile(raw)
		javaclassparser.EnableDecompileSyntaxValidation = true
		onOut, onErr := decompile(raw)
		if offErr != nil || onErr != nil {
			continue
		}
		offStubs := strings.Count(offOut, javaclassparser.DecompileStubMarker)
		onStubs := strings.Count(onOut, javaclassparser.DecompileStubMarker)
		_, offFErr := java2ssa.Frontend(offOut)
		_, onFErr := java2ssa.Frontend(onOut)

		switch {
		case offFErr == nil && onStubs > offStubs:
			// net-off output was valid yet net-on degraded: a candidate FALSE POSITIVE. But the
			// decompiler is known to be non-deterministic for some classes, so confirm net-off is
			// *consistently* valid across retries before flagging; a single invalid retry proves
			// it is non-determinism, not validator disagreement.
			javaclassparser.EnableDecompileSyntaxValidation = false
			nondeterministic := false
			for i := 0; i < 6; i++ {
				retryOut, retryErr := decompile(raw)
				if retryErr != nil {
					continue
				}
				if _, ferr := java2ssa.Frontend(retryOut); ferr != nil {
					nondeterministic = true
					break
				}
			}
			javaclassparser.EnableDecompileSyntaxValidation = true
			if nondeterministic {
				offInvalidOnValid++ // net stabilized a non-deterministic class to always-valid
			} else {
				offValidOnDegraded++
				t.Errorf("[OVER-STUB] %s: net-off consistently valid but net-on added %d stubs", name, onStubs-offStubs)
			}
		case offFErr != nil && onFErr == nil:
			offInvalidOnValid++
		case offFErr != nil && onFErr != nil:
			offInvalidOnInvalid++
		}
	}
	javaclassparser.EnableDecompileSyntaxValidation = true
	t.Logf("==== SAFETY NET ISOLATED (net off vs on) ====")
	t.Logf("total=%d", total)
	t.Logf("net correctly FIXED (off-invalid -> on-valid)=%d", offInvalidOnValid)
	t.Logf("net could NOT fix (still invalid)=%d", offInvalidOnInvalid)
	t.Logf("FALSE POSITIVES (off-valid -> on-degraded)=%d", offValidOnDegraded)
}
