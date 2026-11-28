package x86

import (
	"bytes"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestDiscoverBoundariesManySymbolsFastAndNonOverlappingGap(t *testing.T) {
	const count = 10000
	const stride = uint64(16)
	textAddr := uint64(0x1000)
	// Fill .text with simple `ret` instructions (0xc3) plus trailing gap of 64 bytes of NOPs/ret.
	textSize := uint64(count)*stride + 64
	code := bytes.Repeat([]byte{0xc3}, int(textSize))
	reader, err := core.NewByteReader("many.bin", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]sym.Symbol, count)
	for i := 0; i < count; i++ {
		symbols[i] = sym.Symbol{
			Name:    "fn",
			Address: textAddr + uint64(i)*stride,
			Kind:    sym.SymbolFunction,
		}
	}
	image := &core.Image{Symbols: symbols}
	hints := core.BoundaryHints{
		ExecSections: []core.Region{{Address: textAddr, Offset: 0, Size: textSize}},
	}
	profile := Profile{Mode: 64}

	start := time.Now()
	result, err := discoverBoundaries(profile, reader, hints, image)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Functions) < count {
		t.Fatalf("discovered %d functions, want >= %d", len(result.Functions), count)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("10k symbol boundary discovery took %v, expected < 2s", elapsed)
	}
	// Verify tail scan boundaries do not overlap each other.
	for i := 1; i < len(result.Functions); i++ {
		prev := result.Functions[i-1]
		curr := result.Functions[i]
		if prev.Address+prev.Size > curr.Address {
			t.Fatalf("overlapping boundary: [0x%x+0x%x) vs 0x%x", prev.Address, prev.Size, curr.Address)
		}
	}
}
