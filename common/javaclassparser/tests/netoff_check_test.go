package tests

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/yak/java/java2ssa"
)

// TestNetOffCheck decompiles a single class (path from NETOFF_CLASS) with the syntax safety net
// OFF and ON, reporting stub counts and whether the net-off output is actually valid Java
// (validated with the real frontend, no budget). This isolates whether the validation-budget
// is over-stubbing methods whose bodies are in fact valid (but slow for ANTLR to accept).
func TestNetOffCheck(t *testing.T) {
	path := os.Getenv("NETOFF_CLASS")
	if path == "" {
		t.Skip("set NETOFF_CLASS to a .class file path")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read class: %v", err)
	}

	javaclassparser.EnableDecompileSyntaxValidation = false
	t0 := time.Now()
	offOut, offErr := javaclassparser.Decompile(raw)
	offDecompile := time.Since(t0)
	javaclassparser.EnableDecompileSyntaxValidation = true
	if offErr != nil {
		t.Fatalf("net-off decompile error: %v", offErr)
	}
	offStubs := strings.Count(offOut, javaclassparser.DecompileStubMarker)

	t1 := time.Now()
	_, offFErr := java2ssa.Frontend(offOut)
	offFrontend := time.Since(t1)

	t2 := time.Now()
	onOut, onErr := javaclassparser.Decompile(raw)
	onDecompile := time.Since(t2)
	if onErr != nil {
		t.Fatalf("net-on decompile error: %v", onErr)
	}
	onStubs := strings.Count(onOut, javaclassparser.DecompileStubMarker)

	t.Logf("net-OFF: stubs=%d valid=%v (decompile=%s frontend=%s)", offStubs, offFErr == nil, offDecompile, offFrontend)
	t.Logf("net-ON : stubs=%d (decompile=%s)", onStubs, onDecompile)
	if offFErr == nil && onStubs > offStubs {
		t.Logf("VERDICT: OVER-STUB -- net-off output is VALID Java but net-on added %d stubs", onStubs-offStubs)
	} else if offFErr != nil {
		t.Logf("VERDICT: net-off output is INVALID (%s); net-on stubbing is justified", firstReason(offFErr.Error()))
	} else {
		t.Logf("VERDICT: no over-stub (onStubs<=offStubs)")
	}
}
