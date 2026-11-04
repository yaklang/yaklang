package native

import (
	"bytes"
	"context"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func newFixtureReader(t *testing.T, raw []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("fixture.bin", bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func loadFixture(t *testing.T, loader core.Loader, raw []byte) (*core.Artifact, error) {
	t.Helper()
	return loader.Load(context.Background(), newFixtureReader(t, raw))
}

func writeValue(t *testing.T, buffer *bytes.Buffer, order binary.ByteOrder, value any) {
	t.Helper()
	if err := binary.Write(buffer, order, value); err != nil {
		t.Fatal(err)
	}
}

// The fixtures below exercise file metadata only; they are not runnable programs.
func elf64Fixture(t *testing.T) []byte {
	t.Helper()
	raw := make([]byte, 0x200)
	names := "\x00.text\x00.bss\x00.shstrtab\x00"
	copy(raw[0x90:], names)
	var ident [elf.EI_NIDENT]byte
	copy(ident[:], []byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS64), byte(elf.ELFDATA2LSB), byte(elf.EV_CURRENT)})
	header := elf.Header64{
		Ident: ident, Type: uint16(elf.ET_EXEC), Machine: uint16(elf.EM_X86_64), Version: uint32(elf.EV_CURRENT),
		Entry: 0x400000, Ehsize: 64, Shoff: 0x100, Shentsize: 64, Shnum: 4, Shstrndx: 3,
	}
	var buffer bytes.Buffer
	writeValue(t, &buffer, binary.LittleEndian, header)
	copy(raw, buffer.Bytes())
	buffer.Reset()
	sections := []elf.Section64{
		{},
		{Name: uint32(strings.Index(names, ".text")), Type: uint32(elf.SHT_PROGBITS), Flags: uint64(elf.SHF_ALLOC | elf.SHF_EXECINSTR), Addr: 0x400000, Off: 0x80, Size: 4},
		{Name: uint32(strings.Index(names, ".bss")), Type: uint32(elf.SHT_NOBITS), Flags: uint64(elf.SHF_ALLOC | elf.SHF_WRITE), Addr: 0x401000, Off: 0x200, Size: 0x1000},
		{Name: uint32(strings.Index(names, ".shstrtab")), Type: uint32(elf.SHT_STRTAB), Off: 0x90, Size: uint64(len(names))},
	}
	writeValue(t, &buffer, binary.LittleEndian, sections)
	copy(raw[0x100:], buffer.Bytes())
	return raw
}

func elf32BigEndianFixture(t *testing.T) []byte {
	t.Helper()
	var ident [elf.EI_NIDENT]byte
	copy(ident[:], []byte{0x7f, 'E', 'L', 'F', byte(elf.ELFCLASS32), byte(elf.ELFDATA2MSB), byte(elf.EV_CURRENT)})
	var buffer bytes.Buffer
	writeValue(t, &buffer, binary.BigEndian, elf.Header32{
		Ident: ident, Type: uint16(elf.ET_REL), Machine: uint16(elf.EM_PPC), Version: uint32(elf.EV_CURRENT), Ehsize: 52,
	})
	return buffer.Bytes()
}

func peFixture(t *testing.T, is64 bool) []byte {
	t.Helper()
	raw := make([]byte, 0x400)
	copy(raw, "MZ")
	binary.LittleEndian.PutUint32(raw[0x3c:], 0x80)
	var optional any
	machine := uint16(pe.IMAGE_FILE_MACHINE_I386)
	if is64 {
		machine = pe.IMAGE_FILE_MACHINE_AMD64
		optional = pe.OptionalHeader64{
			Magic: 0x20b, ImageBase: 0x140000000, AddressOfEntryPoint: 0x1000,
			SectionAlignment: 0x1000, FileAlignment: 0x200, SizeOfImage: 0x2000, SizeOfHeaders: 0x200,
			NumberOfRvaAndSizes: 16,
		}
	} else {
		optional = pe.OptionalHeader32{
			Magic: 0x10b, ImageBase: 0x400000, AddressOfEntryPoint: 0x1000,
			SectionAlignment: 0x1000, FileAlignment: 0x200, SizeOfImage: 0x2000, SizeOfHeaders: 0x200,
			NumberOfRvaAndSizes: 16,
		}
	}
	var buffer bytes.Buffer
	buffer.WriteString("PE\x00\x00")
	writeValue(t, &buffer, binary.LittleEndian, pe.FileHeader{
		Machine: machine, NumberOfSections: 1, SizeOfOptionalHeader: uint16(binary.Size(optional)), Characteristics: 0x0002,
	})
	writeValue(t, &buffer, binary.LittleEndian, optional)
	section := pe.SectionHeader32{VirtualSize: 0x20, VirtualAddress: 0x1000, SizeOfRawData: 0x200, PointerToRawData: 0x200, Characteristics: 0x60000020}
	copy(section.Name[:], ".text")
	writeValue(t, &buffer, binary.LittleEndian, section)
	copy(raw[0x80:], buffer.Bytes())
	return raw
}

func macho64Fixture(t *testing.T) []byte {
	t.Helper()
	const dataOffset = 0x200
	const base = 0x100000000
	raw := make([]byte, dataOffset+16)
	segment := macho.Segment64{
		Cmd: macho.LoadCmdSegment64, Len: 72 + 2*80, Addr: base, Memsz: 0x2000,
		Filesz: uint64(len(raw)), Maxprot: 7, Prot: 5, Nsect: 2,
	}
	copy(segment.Name[:], "__TEXT")
	text := macho.Section64{Addr: base + dataOffset, Size: 16, Offset: dataOffset, Flags: 0x80000400}
	copy(text.Name[:], "__text")
	copy(text.Seg[:], "__TEXT")
	bss := macho.Section64{Addr: base + 0x1000, Size: 0x1000, Flags: 0x1}
	copy(bss.Name[:], "__bss")
	copy(bss.Seg[:], "__DATA")
	var buffer bytes.Buffer
	writeValue(t, &buffer, binary.LittleEndian, macho.FileHeader{Magic: macho.Magic64, Cpu: macho.CpuAmd64, SubCpu: 3, Type: macho.TypeExec, Ncmd: 1, Cmdsz: segment.Len})
	writeValue(t, &buffer, binary.LittleEndian, uint32(0)) // reserved in the 64-bit header
	writeValue(t, &buffer, binary.LittleEndian, segment)
	writeValue(t, &buffer, binary.LittleEndian, text)
	writeValue(t, &buffer, binary.LittleEndian, bss)
	copy(raw, buffer.Bytes())
	return raw
}

func macho32BigEndianFixture(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writeValue(t, &buffer, binary.BigEndian, macho.FileHeader{Magic: macho.Magic32, Cpu: macho.CpuPpc, Type: macho.TypeObj})
	return buffer.Bytes()
}

func TestFormatLoadersMetadata(t *testing.T) {
	for _, tc := range []struct {
		name     string
		loader   core.Loader
		raw      []byte
		format   core.Format
		bits     int
		order    core.ByteOrder
		sections int
	}{
		{"elf64", ELF{}, elf64Fixture(t), core.FormatELF, 64, core.LittleEndian, 3},
		{"elf32 big endian", ELF{}, elf32BigEndianFixture(t), core.FormatELF, 32, core.BigEndian, 0},
		{"pe32", PE{}, peFixture(t, false), core.FormatPE, 32, core.LittleEndian, 1},
		{"pe64", PE{}, peFixture(t, true), core.FormatPE, 64, core.LittleEndian, 1},
		{"macho64", MachO{}, macho64Fixture(t), core.FormatMachO, 64, core.LittleEndian, 2},
		{"macho32 big endian", MachO{}, macho32BigEndianFixture(t), core.FormatMachO, 32, core.BigEndian, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := append([]byte(nil), tc.raw...)
			artifact, err := loadFixture(t, tc.loader, tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if artifact.Format != tc.format || artifact.Kind != core.KindNative || artifact.Target.Bits != tc.bits ||
				artifact.Target.ByteOrder != tc.order || len(artifact.Image.Sections) != tc.sections {
				t.Fatalf("unexpected metadata: %+v", artifact)
			}
			if artifact.Reader.Kind() != tc.order {
				t.Fatalf("artifact reader order = %s, want %s", artifact.Reader.Kind(), tc.order)
			}
			if !bytes.Equal(before, tc.raw) {
				t.Fatal("loader changed input bytes")
			}
			checked, err := tc.loader.Check(newFixtureReader(t, tc.raw))
			if err != nil {
				t.Fatal(err)
			}
			if checked.Format != tc.format || checked.Sections != tc.sections || checked.Target != artifact.Target {
				t.Fatalf("check disagrees with load: %+v", checked)
			}
		})
	}
}

func TestFormatLoaderEntryPointsAndZeroFill(t *testing.T) {
	elfArtifact, err := loadFixture(t, ELF{}, elf64Fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if entry, present := elfArtifact.Image.EntryPoint(); !present || entry != 0x400000 {
		t.Fatal("ELF entry point lost")
	}
	if section := elfArtifact.Image.Sections[1]; section.FileSize != 0 || section.MemorySize != 0x1000 {
		t.Fatalf("ELF NOBITS treated as file data: %+v", section)
	}
	peArtifact, err := loadFixture(t, PE{}, peFixture(t, true))
	if err != nil {
		t.Fatal(err)
	}
	if entry, present := peArtifact.Image.EntryPoint(); !present || entry != 0x140001000 || peArtifact.Image.Sections[0].VirtualAddress != 0x140001000 {
		t.Fatal("PE RVA was not translated using image base")
	}
	machoArtifact, err := loadFixture(t, MachO{}, macho64Fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, present := machoArtifact.Image.EntryPoint(); present || len(machoArtifact.Diagnostics) != 1 {
		t.Fatal("Mach-O entry point omission was not reported")
	}
	if section := machoArtifact.Image.Sections[1]; section.FileSize != 0 || section.MemorySize != 0x1000 {
		t.Fatalf("Mach-O zero fill treated as file data: %+v", section)
	}
}

func TestFormatLoaderRejectsMalformedInput(t *testing.T) {
	for _, loader := range []core.Loader{ELF{}, PE{}, MachO{}} {
		if _, err := loadFixture(t, loader, []byte("plain text")); err == nil {
			t.Fatalf("%s accepted plain text", loader.Name())
		}
	}
	for _, tc := range []struct {
		loader core.Loader
		raw    []byte
	}{
		{ELF{}, elf64Fixture(t)[:20]},
		{PE{}, peFixture(t, true)[:20]},
		{MachO{}, macho64Fixture(t)[:20]},
	} {
		if _, err := loadFixture(t, tc.loader, tc.raw); err == nil {
			t.Fatalf("%s accepted a truncated header", tc.loader.Name())
		}
	}
	raw := elf64Fixture(t)
	binary.LittleEndian.PutUint64(raw[0x100+64+24:], uint64(len(raw)+100))
	if _, err := loadFixture(t, ELF{}, raw); err == nil || !strings.Contains(err.Error(), "beyond the input") {
		t.Fatalf("out-of-bounds section accepted: %v", err)
	}
	if _, err := peAddress(math.MaxUint64, 1); err == nil {
		t.Fatal("overflowing PE address accepted")
	}
}

func TestFormatLoaderContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (ELF{}).Load(ctx, newFixtureReader(t, elf64Fixture(t))); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled context, got %v", err)
	}
}

func TestFormatLoaderMatch(t *testing.T) {
	for _, tc := range []struct {
		loader core.Loader
		prefix []byte
		want   bool
	}{
		{ELF{}, []byte{0x7f, 'E', 'L', 'F'}, true},
		{PE{}, []byte("MZ"), true},
		{MachO{}, []byte{0xcf, 0xfa, 0xed, 0xfe}, true},
		{ELF{}, []byte("MZ"), false},
		{PE{}, []byte("plain"), false},
		{MachO{}, []byte{0xca, 0xfe, 0xba, 0xbe}, false},
	} {
		if got := tc.loader.Match(tc.prefix); got != tc.want {
			t.Fatalf("%s.Match(%x) = %v, want %v", tc.loader.Name(), tc.prefix, got, tc.want)
		}
	}
}
