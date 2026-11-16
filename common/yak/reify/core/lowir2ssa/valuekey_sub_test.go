package lowir2ssa

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
)

// TestValueKeyIdentifiesFrameSlotBySubtraction checks that two accesses to the
// same frame slot share a key. A target reaches a local through `base - offset`,
// and without the subtraction the key falls back to the value identifier, so a
// reload never sees what the store wrote.
func TestValueKeyIdentifiesFrameSlotBySubtraction(t *testing.T) {
	ir, err := lowirtext.Parse(`target: linux/loong64 (64-bit, little-endian)
function f @ 0x1000
block b1 @ 0x1000
  0x1000  v1:i64 = read_register %fp [read_register]
  0x1000  v2:i64 = constant 0x14
  0x1000  v3:i64 = sub v1, v2
  0x1004  v4:i64 = read_register %fp [read_register]
  0x1004  v5:i64 = constant 0x14
  0x1004  v6:i64 = sub v4, v5
  0x1008  return`)
	if err != nil {
		t.Fatal(err)
	}
	defs := make(map[core.ValueID]core.Instruction)
	for _, block := range ir.Functions[0].Blocks {
		for _, instruction := range block.Instructions {
			for _, result := range instruction.Results {
				defs[result.ID] = instruction
			}
		}
	}
	if first, second := valueKey(defs, 3), valueKey(defs, 6); first != second || first == "" {
		t.Fatalf("keys differ: %q and %q", first, second)
	}
}
