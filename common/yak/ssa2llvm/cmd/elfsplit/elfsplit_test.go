package main

import (
	"encoding/binary"
	"testing"
)

func encodeLeaRCX(disp int32) []byte {
	b := []byte{0x48, 0x8d, 0x0d, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(b[3:], uint32(disp))
	return b
}

// A closure body usually sits at a lower address than the function that
// materializes it with `leaq fn(%rip)`. Packing must rewrite that negative
// displacement; leaving the Go linker's original bytes points at whatever
// now occupies the old slot.
func TestRewriteBackwardRIPRelativeLEA(t *testing.T) {
	const (
		textSize    = 0x400
		closureOld  = 0x100
		closureSize = 0x20
		initOld     = 0x200
		initSize    = 0x20
		closureNew  = 0x0
		initNew     = 0x300
	)
	nextIP := int64(initOld + 7)
	disp := int32(int64(closureOld) - nextIP)
	data := make([]byte, textSize)
	copy(data[initOld:], encodeLeaRCX(disp))
	dest := make([]byte, textSize)
	copy(dest[initNew:], data[initOld:initOld+initSize])

	placements := []codePlacement{
		{name: "closure", oldOff: closureOld, size: closureSize, newOff: closureNew, module: "ssafront"},
		{name: "init", oldOff: initOld, size: initSize, newOff: initNew, module: "ssafront"},
	}
	_, err := rewritePCRelativeBranches(data, dest, 0, 1, placements, map[uint64]struct{}{}, textSize, newCallGraph(len(placements)))
	if err != nil {
		t.Fatal(err)
	}
	got := int32(binary.LittleEndian.Uint32(dest[initNew+3:]))
	want := int32(int64(closureNew) - int64(initNew+7))
	if got != want {
		t.Fatalf("rewritten disp=%#x, want %#x (lands at %#x)", got, want, int64(initNew+7)+int64(got))
	}
}

func TestRewriteBackwardRIPRelativeLEACrossSection(t *testing.T) {
	const (
		textSize    = 0x400
		closureOld  = 0x100
		closureSize = 0x20
		initOld     = 0x200
		initSize    = 0x20
		initNew     = 0x40
	)
	nextIP := int64(initOld + 7)
	disp := int32(int64(closureOld) - nextIP)
	data := make([]byte, textSize)
	copy(data[initOld:], encodeLeaRCX(disp))
	dest := make([]byte, textSize)
	copy(dest[initNew:], data[initOld:initOld+initSize])

	placements := []codePlacement{
		{symIdx: 7, name: "closure", oldOff: closureOld, size: closureSize, newOff: 0, module: ""},
		{name: "init", oldOff: initOld, size: initSize, newOff: initNew, module: "ssafront"},
	}
	rel, err := rewritePCRelativeBranches(data, dest, 0, 1, placements, map[uint64]struct{}{}, textSize, newCallGraph(len(placements)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rel) != 24 {
		t.Fatalf("relocation entry len=%d, want 24", len(rel))
	}
	off := binary.LittleEndian.Uint64(rel[0:])
	if off != initNew+3 {
		t.Fatalf("reloc offset=%#x, want %#x", off, initNew+3)
	}
	addend := int64(binary.LittleEndian.Uint64(rel[16:]))
	if addend != -4 {
		t.Fatalf("reloc addend=%d, want -4", addend)
	}
	unchanged := int32(binary.LittleEndian.Uint32(dest[initNew+3:]))
	if unchanged != disp {
		t.Fatalf("cross-section lea was rewritten in place (%#x -> %#x)", disp, unchanged)
	}
}

func TestClassifyPackage(t *testing.T) {
	tests := []struct {
		sym  string
		want string
	}{
		{"runtime.main", "runtime"},
		{"github.com/yaklang/yaklang/common/utils/lowhttp/poc.Get", "github.com/yaklang/yaklang/common/utils/lowhttp/poc"},
		{"fmt.Sprintf", "fmt"},
		{"strings.(*Builder).Grow", "strings"},
		{"github.com/yaklang/yaklang/common/yak/ssaapi.(*Program).Parse", "github.com/yaklang/yaklang/common/yak/ssaapi"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.sym, func(t *testing.T) {
			got := classifyPackage(tt.sym)
			if got != tt.want {
				t.Errorf("classifyPackage(%q) = %q, want %q", tt.sym, got, tt.want)
			}
		})
	}
}

func TestMatchModule(t *testing.T) {
	modulePkgs := map[string][]string{
		"poc": {"github.com/yaklang/yaklang/common/utils/lowhttp/poc"},
		"ssa": {"github.com/yaklang/yaklang/common/yak/ssaapi"},
	}
	tests := []struct {
		pkg  string
		want string
	}{
		{"github.com/yaklang/yaklang/common/utils/lowhttp/poc", "poc"},
		{"github.com/yaklang/yaklang/common/utils/lowhttp/poc/sub", "poc"},
		{"github.com/yaklang/yaklang/common/yak/ssaapi", "ssa"},
		{"runtime", ""},
		{"fmt", ""},
	}
	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			got := matchModule(tt.pkg, modulePkgs)
			if got != tt.want {
				t.Errorf("matchModule(%q) = %q, want %q", tt.pkg, got, tt.want)
			}
		})
	}
}

func TestBuildModulePackageMap(t *testing.T) {
	modules := []string{"poc", "ssa", "unknown_module"}
	m := buildModulePackageMap(modules)
	if _, ok := m["poc"]; !ok {
		t.Error("expected poc in map")
	}
	if _, ok := m["ssa"]; !ok {
		t.Error("expected ssa in map")
	}
	if _, ok := m["unknown_module"]; ok {
		t.Error("expected unknown_module to be absent from map")
	}
}
