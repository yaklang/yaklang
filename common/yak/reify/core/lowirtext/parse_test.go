package lowirtext

import (
	"strings"
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

// factsRoundTripCase carries every architecture fact: the frame, the register
// model and the stack guard. A round trip that dropped them would turn every
// LowIR pass into a no-op, because a pass reads these instead of guessing.
const factsRoundTripCase = `target: linux/arm64 (64-bit, little-endian)
frame: sp=sp fp=x29 ret=x30 flags=nzcv saved=x19,x20,x29 growth=-1
registers: naming=aarch64 width=64 flags=nzcv flags_width=64 args=x0,x1,x2,x3,x4,x5,x6,x7 return=x0
canary: kind=tls register=tpidr_el0 offset=0x28
function f @ 0x1000
block b1 @ 0x1000
  0x1000  return
`

// TestFactsRoundTrip keeps the architecture facts in the text form.
func TestFactsRoundTrip(t *testing.T) {
	ir, err := Parse(factsRoundTripCase)
	if err != nil {
		t.Fatal(err)
	}
	if ir.Frame.StackPointer != "sp" || ir.Frame.FramePointer != "x29" || ir.Frame.ReturnAddress != "x30" {
		t.Fatalf("frame = %+v", ir.Frame)
	}
	if len(ir.Frame.CalleeSaved) != 3 || ir.Frame.StackGrowth != -1 {
		t.Fatalf("frame = %+v", ir.Frame)
	}
	if ir.Registers.Naming != core.RegisterNamingAArch64 || ir.Registers.Width != 64 {
		t.Fatalf("registers = %+v", ir.Registers)
	}
	if len(ir.Registers.Arguments) != 8 || ir.Registers.Return != "x0" {
		t.Fatalf("registers = %+v", ir.Registers)
	}
	if ir.Canary.Kind != core.CanaryTLS || ir.Canary.Offset != 0x28 {
		t.Fatalf("canary = %+v", ir.Canary)
	}
	if got := Format(ir); got != factsRoundTripCase {
		t.Fatalf("round trip changed the text:\n--- want ---\n%s\n--- got ---\n%s", factsRoundTripCase, got)
	}
}

// TestFactsOmittedWhenEmpty keeps the text form compact for a program that
// carries no facts.
func TestFactsOmittedWhenEmpty(t *testing.T) {
	ir := &core.LowIR{Target: core.Target{OS: core.OSLinux, Architecture: core.ArchAMD64, Bits: 64, ByteOrder: core.LittleEndian}}
	text := Format(ir)
	for _, prefix := range []string{"frame:", "registers:", "canary:"} {
		if strings.Contains(text, prefix) {
			t.Fatalf("empty facts emitted %q:\n%s", prefix, text)
		}
	}
}
