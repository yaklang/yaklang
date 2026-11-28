package x86

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

func TestDiscoverBoundariesManySymbolsFastAndNonOverlappingGap(t *testing.T) {
	const count = 8000
	codeSize := uint64(count*16 + 64)
	code := bytes.Repeat([]byte{0xc3}, int(codeSize))
	// Gap functions aligned at 16 bytes after symbol functions (push rbp; mov rbp, rsp; ret).
	gapStart := count * 16
	copy(code[gapStart:], []byte{0x55, 0x48, 0x89, 0xe5, 0xc3, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc, 0xcc})
	copy(code[gapStart+16:], []byte{0x55, 0x48, 0x89, 0xe5, 0xc3})

	symbols := make([]sym.Symbol, count)
	for i := 0; i < count; i++ {
		symbols[i] = sym.Symbol{
			Name:    fmt.Sprintf("fn_%d", i),
			Address: 0x1000 + uint64(i*16),
			Kind:    sym.SymbolFunction,
		}
	}
	reader, err := core.NewByteReader("many.bin", bytes.NewReader(code), int64(len(code)))
	if err != nil {
		t.Fatal(err)
	}
	hints := core.BoundaryHints{
		ExecSections: []core.Region{{Address: 0x1000, Offset: 0, Size: codeSize}},
	}
	image := &core.Image{Symbols: symbols}
	start := time.Now()
	res, err := discoverBoundaries(Profile64(), reader, hints, image)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("discoverBoundaries took too long: %v", elapsed)
	}
	if len(res.Functions) < count {
		t.Fatalf("got %d functions, want >= %d", len(res.Functions), count)
	}
	for i := 0; i+1 < len(res.Functions); i++ {
		cur := res.Functions[i]
		next := res.Functions[i+1]
		if cur.Size > 0 && cur.Address+cur.Size > next.Address {
			t.Fatalf("overlapping boundaries: [%#x, %#x) and %#x", cur.Address, cur.Address+cur.Size, next.Address)
		}
	}
}
