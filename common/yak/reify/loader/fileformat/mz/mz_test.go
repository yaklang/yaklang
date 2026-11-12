package mz

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

// comImage builds one minimal COM image: mov ax, 0x4c00; int 0x21.
func comImage() []byte {
	return []byte{0xb8, 0x00, 0x4c, 0xcd, 0x21}
}

// mzImage builds one MZ executable with a 32-byte header and one code page.
func mzImage() []byte {
	header := make([]byte, 32)
	copy(header, "MZ")
	binary.LittleEndian.PutUint16(header[2:], 512)   // last page holds a full page
	binary.LittleEndian.PutUint16(header[4:], 1)     // one page
	binary.LittleEndian.PutUint16(header[6:], 0)     // no relocations
	binary.LittleEndian.PutUint16(header[8:], 2)     // header is 32 bytes
	binary.LittleEndian.PutUint16(header[10:], 0x10) // minimum extra paragraphs
	binary.LittleEndian.PutUint16(header[16:], 0xfffe)
	image := append(header, 0xb8, 0x00, 0x4c, 0xcd, 0x21)
	for len(image) < 512 {
		image = append(image, 0)
	}
	return image
}

func readerFor(t *testing.T, data []byte) *core.ByteReader {
	t.Helper()
	reader, err := core.NewByteReader("test", byteReaderAt(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	return reader
}

func TestMatchRejectsPE(t *testing.T) {
	// A PE image also starts with MZ, but its stub names a signature offset.
	pe := make([]byte, 0x100)
	copy(pe, "MZ")
	binary.LittleEndian.PutUint32(pe[0x3c:], 0x80)
	if (Loader{}).Match(pe) {
		t.Fatal("MZ loader claimed a PE image")
	}
	if !(Loader{}).Match(mzImage()) {
		t.Fatal("MZ loader rejected a plain MZ image")
	}
}

func TestLoadMZ(t *testing.T) {
	artifact, err := Loader{}.Load(context.Background(), readerFor(t, mzImage()))
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Target.Architecture != core.Arch8086 || artifact.Target.Bits != 16 {
		t.Fatalf("target = %+v", artifact.Target)
	}
	if artifact.Target.OS != core.OSDOS {
		t.Fatalf("os = %q", artifact.Target.OS)
	}
	if len(artifact.Image.Sections) != 1 {
		t.Fatalf("sections = %d", len(artifact.Image.Sections))
	}
	section := artifact.Image.Sections[0]
	if section.FileOffset != 32 {
		t.Fatalf("section offset = %d, want 32", section.FileOffset)
	}
	entry, ok := artifact.Image.EntryPoint()
	if !ok || entry != 0 {
		t.Fatalf("entry = %#x", entry)
	}
	if len(artifact.Hints.ExecSections) != 1 {
		t.Fatalf("exec sections = %d", len(artifact.Hints.ExecSections))
	}
}

func TestLoadCOM(t *testing.T) {
	artifact, err := Loader{}.Load(context.Background(), readerFor(t, comImage()))
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := artifact.Image.EntryPoint()
	if !ok || entry != comLoadAddress {
		t.Fatalf("entry = %#x, want %#x", entry, comLoadAddress)
	}
	if artifact.Image.Sections[0].VirtualAddress != comLoadAddress {
		t.Fatalf("section address = %#x", artifact.Image.Sections[0].VirtualAddress)
	}
}
