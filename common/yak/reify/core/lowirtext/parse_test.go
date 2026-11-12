package lowirtext

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

const roundTripCase = `target: linux/amd64 (64-bit, little-endian)
function sub_0x1000 @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = read_register %rdi [read_register]
  0x1000  v2:i64 = constant 0x10
  0x1002  v3:i64 = add v1, v2
  0x1003  write_register %rax, v3 [write_register]
  0x1004  branch v3 -> b2, b3
block b2 @ 0x1008
  0x1008  return v3 [may_trap, unknown]
`

func TestParseRoundTrips(t *testing.T) {
	ir, err := Parse(roundTripCase)
	if err != nil {
		t.Fatal(err)
	}
	if len(ir.Functions) != 1 || len(ir.Functions[0].Blocks) != 2 {
		t.Fatalf("unexpected shape: %+v", ir)
	}
	if got := Format(ir); got != roundTripCase {
		t.Fatalf("round trip changed the text:\n--- want ---\n%s\n--- got ---\n%s", roundTripCase, got)
	}
}

func TestParseKeepsEffectsAndTargets(t *testing.T) {
	ir, err := Parse(roundTripCase)
	if err != nil {
		t.Fatal(err)
	}
	branch := ir.Functions[0].Blocks[0].Instructions[4]
	if branch.Kind != core.OpBranch || len(branch.Targets) != 2 || branch.Targets[0] != 2 || branch.Targets[1] != 3 {
		t.Fatalf("branch = %+v", branch)
	}
	load := ir.Functions[0].Blocks[0].Instructions[0]
	if load.Effects&core.EffectReadRegister == 0 {
		t.Fatalf("register effect lost: %+v", load)
	}
	ret := ir.Functions[0].Blocks[1].Instructions[0]
	if ret.Effects&core.EffectMayTrap == 0 || ret.Effects&core.EffectUnknown == 0 {
		t.Fatalf("return effects = %v", ret.Effects)
	}
}

func TestParseRejectsMalformedText(t *testing.T) {
	for _, text := range []string{
		"block b1 @ 0x0\n",
		"function f @ 0x0\n  0x0  return\n",
	} {
		if _, err := Parse(text); err == nil {
			t.Fatalf("malformed text accepted: %q", text)
		}
	}
}
