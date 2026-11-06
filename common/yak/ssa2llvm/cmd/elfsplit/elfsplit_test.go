package main

import (
	"encoding/binary"
	"runtime"
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/textsectlayout"
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

// x86asm has no AVX2 tables, so the four bytes of `rorx $0x29,%rax,%r13`
// (c4 63 fb f0) plus the following bytes decode as an unrelated 9-byte CALL
// whose immediate happens to sit five bytes in. The byte-by-byte recovery scan
// then treats the middle of a real instruction as a PC-relative field and
// rewrites it in place, corrupting everything that follows. This is how
// crypto/internal/fips140/sha512.blockAVX2 lost three instructions and the
// compiled binary died of SIGILL. Any PC-relative operand found in a function
// whose body does not decode cleanly, and which does not name an exact
// function entry, must be left alone.
func TestSkipFalseBranchInAVX2Body(t *testing.T) {
	const (
		textSize   = 0x400
		avxOld     = 0x100
		avxSize    = 0x40
		avxNew     = 0x200
		targetOld  = 0x300
		targetSize = 0x40
		// The bogus CALL must land here: inside the target function, but not
		// on its entry, which is what marks it as a decoding artifact.
		targetMid = targetOld + 8
	)
	avx := make([]byte, avxSize)
	for i := range avx {
		avx[i] = 0x90
	}
	// rorx's own bytes, followed by a displacement that makes the phantom CALL
	// reach targetMid.
	copy(avx, []byte{0xc4, 0x63, 0xfb, 0xf0, 0xe8})
	binary.LittleEndian.PutUint32(avx[5:], uint32(int64(targetMid)-(int64(avxOld)+9)))
	// A byte x86asm cannot decode marks the body as drifted.
	avx[avxSize-1] = 0x06

	data := make([]byte, textSize)
	copy(data[avxOld:], avx)
	for i := 0; i < targetSize; i++ {
		data[targetOld+i] = 0x90
	}
	dest := make([]byte, textSize)
	copy(dest[avxNew:], avx)

	placements := []codePlacement{
		{symIdx: 3, name: "avx", oldOff: avxOld, size: avxSize, newOff: avxNew, module: "shared"},
		{symIdx: 4, name: "target", oldOff: targetOld, size: targetSize, newOff: avxNew + 0x80, module: "shared"},
	}
	if _, err := rewritePCRelativeBranches(data, dest, 0, 0, placements, map[uint64]struct{}{}, textSize, newCallGraph(len(placements))); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < avxSize; i++ {
		if dest[avxNew+i] != avx[i] {
			t.Fatalf("byte %d of an AVX2 body was rewritten: got %#02x want %#02x", i, dest[avxNew+i], avx[i])
		}
	}
}

// A real rel32 call emitted from inside an AVX2 routine (the Go assembler puts
// one runtime.morestack call at the end of each) still has to be relocated: it
// names the callee's exact entry, which a drifted decode cannot invent.
func TestRelocateRealCallInsideAVX2Body(t *testing.T) {
	const (
		textSize   = 0x400
		avxOld     = 0x100
		avxSize    = 0x40
		avxNew     = 0x200
		targetOld  = 0x300
		targetSize = 0x40
	)
	data := make([]byte, textSize)
	avx := make([]byte, avxSize)
	avx[0] = 0xc5 // vpor prefix: x86asm cannot decode it, so the body drifts
	avx[1] = 0xed
	avx[2] = 0x74
	avx[3] = 0xd9
	// call rel32 at offset 0x20, landing exactly on targetOld.
	callOff := 0x20
	avx[callOff] = 0xe8
	binary.LittleEndian.PutUint32(avx[callOff+1:], uint32(int64(targetOld)-(int64(avxOld)+int64(callOff)+5)))
	for i := range avx {
		if avx[i] == 0 && i != callOff+1 && i != callOff+2 && i != callOff+3 && i != callOff+4 {
			avx[i] = 0x90
		}
	}
	avx[0], avx[1], avx[2], avx[3] = 0xc5, 0xed, 0x74, 0xd9
	copy(data[avxOld:], avx)
	dest := make([]byte, textSize)
	copy(dest[avxNew:], avx)

	placements := []codePlacement{
		{symIdx: 3, name: "avx", oldOff: avxOld, size: avxSize, newOff: avxNew, module: "shared"},
		{symIdx: 4, name: "target", oldOff: targetOld, size: targetSize, newOff: 0, module: "other"},
	}
	rel, err := rewritePCRelativeBranches(data, dest, 0, 0, placements, map[uint64]struct{}{}, textSize, newCallGraph(len(placements)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rel) != 24 {
		t.Fatalf("relocation entry len=%d, want 24", len(rel))
	}
	if off := binary.LittleEndian.Uint64(rel[0:]); off != avxNew+uint64(callOff)+1 {
		t.Fatalf("reloc offset=%#x, want %#x", off, avxNew+callOff+1)
	}
	if addend := int64(binary.LittleEndian.Uint64(rel[16:])); addend != -4 {
		t.Fatalf("reloc addend=%d, want -4 (exact entry, immediate width 4)", addend)
	}
}

func TestModuledataTextsectMapUsesGo126Layout(t *testing.T) {
	res, err := textsectlayout.Check(runtime.GOROOT())
	if err != nil {
		t.Fatal(err)
	}
	want := 0
	switch {
	case res.Major == 1 && res.Minor == 26:
		want = 0x150
	case res.Major == 1 && res.Minor == 27:
		want = 0x168
	default:
		t.Fatalf("toolchain %s is not a verified moduledata layout", res.Version)
	}
	if res.Offset != want {
		t.Fatalf("textsectmap offset=%#x, want %#x for %s", res.Offset, want, res.Version)
	}
	if res.Offset+8 != want+8 || res.Offset+16 != want+16 {
		t.Fatalf("textsectmap header offsets are inconsistent around %#x", res.Offset)
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
