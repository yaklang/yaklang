package core

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func stringArtifact(t *testing.T, data []byte, base uint64, bits int, order ByteOrder) *Artifact {
	t.Helper()
	r, err := NewByteReader("strings", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	ordered := r
	if order == LittleEndian || order == BigEndian {
		if ordered, err = r.WithOrder(order); err != nil {
			t.Fatal(err)
		}
	}
	return &Artifact{Kind: KindNative, Format: FormatELF, Reader: ordered,
		Target: Target{Architecture: ArchAMD64, Bits: bits, ByteOrder: order},
		Image: &Image{Sections: []Section{{
			Name: ".rodata", FileSize: uint64(len(data)), MemorySize: uint64(len(data)), VirtualAddress: base,
		}}}}
}

func testByteOrder(order ByteOrder) binary.ByteOrder {
	if order == BigEndian {
		return binary.BigEndian
	}
	return binary.LittleEndian
}

func descriptor(data []byte, offset int, pointer, length uint64, word int, order ByteOrder) {
	buffer := make([]byte, 2*word)
	byteOrder := testByteOrder(order)
	if word == 4 {
		byteOrder.PutUint32(buffer[0:4], uint32(pointer))
		byteOrder.PutUint32(buffer[4:8], uint32(length))
	} else {
		byteOrder.PutUint64(buffer[0:8], pointer)
		byteOrder.PutUint64(buffer[8:16], length)
	}
	copy(data[offset:], buffer)
}

func TestDecodeNulTerminated(t *testing.T) {
	model := Strings{Layout: StringLayoutNulTerminated, Encoding: StringEncodingBytes}
	a := stringArtifact(t, []byte("hello\x00ignored"), 0x1000, 64, LittleEndian)
	value, err := model.Decode(a, 0x1000)
	if err != nil || value != "hello" {
		t.Fatalf("Decode = %q, %v", value, err)
	}
	if _, err := model.Decode(a, 0x2000); err == nil {
		t.Fatal("unmapped address decoded")
	}
	// A section that ends without a NUL is an error, not an empty string.
	if _, err := model.Decode(stringArtifact(t, []byte("no-terminator"), 0x1000, 64, LittleEndian), 0x1000); err == nil {
		t.Fatal("missing terminator accepted")
	}
}

func TestDecodePointerLength(t *testing.T) {
	for _, tc := range []struct {
		name  string
		bits  int
		order ByteOrder
		model Strings
	}{
		{"64-bit little, Go", 64, LittleEndian, Strings{Layout: StringLayoutPointerLength, Encoding: StringEncodingUTF8}},
		{"64-bit big, Rust", 64, BigEndian, Strings{Layout: StringLayoutPointerLength, Encoding: StringEncodingUTF8, ValidateUTF8: true}},
		{"32-bit little, Go", 32, LittleEndian, Strings{Layout: StringLayoutPointerLength, Encoding: StringEncodingUTF8}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := make([]byte, 0x80)
			copy(data, "hello")
			descriptor(data, 0x40, 0x1000, 5, tc.bits/8, tc.order)
			a := stringArtifact(t, data, 0x1000, tc.bits, tc.order)
			value, err := tc.model.Decode(a, 0x1040)
			if err != nil || value != "hello" {
				t.Fatalf("Decode = %q, %v", value, err)
			}
		})
	}
}

func TestDecodePointerLengthRejectsBadInput(t *testing.T) {
	model := Strings{Layout: StringLayoutPointerLength, Encoding: StringEncodingUTF8}
	data := make([]byte, 0x80)
	copy(data, "hello")
	descriptor(data, 0x40, 0x1000, maxStringBytes+1, 8, LittleEndian)
	a := stringArtifact(t, data, 0x1000, 64, LittleEndian)
	if _, err := model.Decode(a, 0x1040); err == nil {
		t.Fatal("oversized length accepted")
	}
	// A PIE keeps the pointer in a relocation and leaves zero in the file.
	descriptor(data, 0x40, 0, 5, 8, LittleEndian)
	if _, err := model.Decode(a, 0x1040); err == nil || !strings.Contains(err.Error(), "relocations are not applied") {
		t.Fatalf("zero pointer = %v", err)
	}
	// An empty string may have a zero pointer.
	descriptor(data, 0x40, 0, 0, 8, LittleEndian)
	if value, err := model.Decode(a, 0x1040); err != nil || value != "" {
		t.Fatalf("empty string = %q, %v", value, err)
	}
	// A pointer outside the mapped section fails instead of reading garbage.
	descriptor(data, 0x40, 0x9000, 5, 8, LittleEndian)
	a = stringArtifact(t, data, 0x1000, 64, LittleEndian)
	if _, err := model.Decode(a, 0x1040); err == nil {
		t.Fatal("unmapped string pointer accepted")
	}
}

func TestDecodeUTF8Validation(t *testing.T) {
	invalid := []byte{0xff, 0xfe, 0, 0}
	data := make([]byte, 0x80)
	copy(data, invalid)
	descriptor(data, 0x40, 0x1000, 2, 8, LittleEndian)
	a := stringArtifact(t, data, 0x1000, 64, LittleEndian)

	goModel := Strings{Layout: StringLayoutPointerLength, Encoding: StringEncodingUTF8}
	if value, err := goModel.Decode(a, 0x1040); err != nil || value != "\xff\xfe" {
		t.Fatalf("Go model = %q, %v", value, err)
	}
	rustModel := Strings{Layout: StringLayoutPointerLength, Encoding: StringEncodingUTF8, ValidateUTF8: true}
	if _, err := rustModel.Decode(a, 0x1040); err == nil {
		t.Fatal("Rust model accepted invalid UTF-8")
	}
}

func TestStringsRejectsUnsupportedModel(t *testing.T) {
	a := stringArtifact(t, []byte("x\x00"), 0x1000, 64, LittleEndian)
	if _, err := (Strings{}).Decode(a, 0x1000); err == nil {
		t.Fatal("zero model accepted")
	}
	if (Strings{}).Valid() {
		t.Fatal("zero model reported valid")
	}
	if _, err := (Strings{Layout: StringLayoutNulTerminated}).Decode(a, 0x1000); err != nil {
		t.Fatalf("layout without encoding should still decode: %v", err)
	}
	bad := stringArtifact(t, []byte{0, 0, 0, 0, 0, 0, 0, 0}, 0x1000, 64, "")
	if _, err := (Strings{Layout: StringLayoutPointerLength}).Decode(bad, 0x1000); err == nil {
		t.Fatal("unknown byte order accepted")
	}
}
