package tests

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/javaclassparser"
)

// TestSwitchSlotHoistDeterminism locks the decompiler's output to be byte-for-byte stable across
// repeated runs of one embedded regression class (runs by default, no external jars needed). The
// jasperreports getTextAlignHolder method is the canonical reproducer: two same-named locals (var2)
// share a slot depth across nested switch arms, and the variable declaration placement used to order
// them by Go map iteration / lexical VarUid compare, so which local kept the bare `var2` name (vs
// `var2_1`) - and the whole method's naming - flipped run to run. The fix makes that ordering
// deterministic (numeric VarUid tie-break + duplicate-declaration dedupe).
func TestSwitchSlotHoistDeterminism(t *testing.T) {
	path := os.Getenv("DET_FILE")
	if path == "" {
		path = "testdata/regression/jasperreports_excel_abstract_exporter_switch_slot_hoist.class"
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	const runs = 30
	first := ""
	for i := 0; i < runs; i++ {
		out, err := javaclassparser.Decompile(raw)
		if err != nil {
			t.Fatalf("decompile iter %d: %v", i, err)
		}
		if i == 0 {
			first = out
			continue
		}
		if out != first {
			// Report the first diverging line to make the flake actionable.
			fa := strings.Split(first, "\n")
			fb := strings.Split(out, "\n")
			n := len(fa)
			if len(fb) < n {
				n = len(fb)
			}
			for l := 0; l < n; l++ {
				if fa[l] != fb[l] {
					t.Fatalf("non-deterministic output at iter %d, line %d:\n  run0: %s\n  run%d: %s", i, l, fa[l], i, fb[l])
				}
			}
			t.Fatalf("non-deterministic output at iter %d (length %d vs %d)", i, len(first), len(out))
		}
	}
}
