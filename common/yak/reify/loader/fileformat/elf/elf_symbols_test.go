package elf

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// TestARMPltStubOffsets checks the ARM PLT reader. The linker does not space the
// stubs evenly: a four-byte Thumb trampoline is inserted where an import needs a
// shorter reach, so a fixed stride misses entries or lands inside one.
func TestARMPltStubOffsets(t *testing.T) {
	add := func(immediate uint32) uint32 { return 0xe28cc000 | immediate }
	payload := make([]byte, 0, 4*14)
	appendWord := func(word uint32) {
		payload = binary.LittleEndian.AppendUint32(payload, word)
	}
	// Header: the resolver stub, which must not be taken for an entry.
	appendWord(0xe52de004)
	appendWord(0xe59fe004)
	appendWord(0xe08fe00e)
	appendWord(0xe5bef008)
	// Two evenly spaced stubs, then one with a Thumb trampoline between.
	appendWord(0xe28fc600)
	appendWord(add(0x1e))
	appendWord(0xe5bcfcd0)
	appendWord(0xe28fc600)
	appendWord(add(0x1e))
	appendWord(0xe5bcfcc8)
	appendWord(0xe28fc600)
	appendWord(add(0x1e))
	appendWord(0xe5bcfcc0)

	offsets := armPLTStubOffsets(payload)
	want := []uint64{16, 28, 40}
	if len(offsets) != len(want) {
		t.Fatalf("offsets = %v, want %v", offsets, want)
	}
	for index := range want {
		if offsets[index] != want[index] {
			t.Fatalf("offsets = %v, want %v", offsets, want)
		}
	}
}

func TestELFSymbolsFromSystemBinary(t *testing.T) {
	path := "/bin/true"
	file, err := os.Open(path)
	if err != nil {
		t.Skipf("%s unavailable: %v", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	reader, err := core.NewByteReader(path, file, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	image, _, _, _, err := inspectELF(reader)
	if err != nil {
		t.Fatal(err)
	}
	if len(image.Symbols) == 0 {
		t.Skipf("%s has no symbols", path)
	}
	found := false
	for _, symbol := range image.Symbols {
		if symbol.Imported && symbol.Address != 0 && symbol.Kind == sym.SymbolFunction {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no imported function symbol with an address in %s", path)
	}
}

// TestPLTStubOffsets checks where the first import stub starts on each
// architecture. A wrong header size names a resolver stub after an import, and
// every call to that import then reports a function that is not the one called.
func TestPLTStubOffsets(t *testing.T) {
	cases := []struct {
		machine elf.Machine
		want    []uint64
	}{
		{elf.EM_X86_64, []uint64{16, 32, 48}},
		{elf.EM_386, []uint64{16, 32, 48}},
		{elf.EM_ARM, []uint64{32, 48, 64}},
		{elf.EM_AARCH64, []uint64{32, 48, 64}},
		{elf.EM_RISCV, []uint64{32, 48, 64}},
		{elf.EM_LOONGARCH, []uint64{32, 48, 64}},
		{elf.EM_PPC64, []uint64{24, 48, 72}},
		{elf.EM_S390, []uint64{32, 64, 96}},
		{elf.EM_MIPS, nil},
	}
	for _, testCase := range cases {
		if testCase.want == nil {
			if _, ok := pltStubOffset(testCase.machine, 0); ok {
				t.Errorf("%v should not report a stub offset", testCase.machine)
			}
			continue
		}
		for index, want := range testCase.want {
			got, ok := pltStubOffset(testCase.machine, index)
			if !ok || got != want {
				t.Errorf("%v index %d = %#x (ok=%v), want %#x", testCase.machine, index, got, ok, want)
			}
		}
	}
}
