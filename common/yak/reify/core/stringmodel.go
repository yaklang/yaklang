package core

import (
	"fmt"
	"unicode/utf8"
)

// StringLayout is how a language stores a string value. The two layouts need
// different reads: a C string is bytes behind a pointer, while Go and Rust pass
// a pointer and a length.
type StringLayout string

const (
	// StringLayoutNulTerminated is a pointer to bytes ending at the first NUL.
	StringLayoutNulTerminated StringLayout = "nul-terminated"
	// StringLayoutPointerLength is a pointer word followed by a length word.
	StringLayoutPointerLength StringLayout = "pointer-length"
)

// StringEncoding is the byte interpretation the language promises.
type StringEncoding string

const (
	StringEncodingBytes StringEncoding = "bytes"
	StringEncodingUTF8  StringEncoding = "utf-8"
)

// maxStringBytes bounds one read. A symbol table and a data section are not
// trusted input, so a missing terminator or an oversized length must fail.
const maxStringBytes = 1 << 12

// Strings describes one language's string representation and decodes a value
// from an image. The zero value has no layout and is rejected.
//
// A language registers one value; nothing else switches on the language name.
type Strings struct {
	Layout   StringLayout   `json:"layout"`
	Encoding StringEncoding `json:"encoding"`
	// ValidateUTF8 rejects bytes that are not valid UTF-8. Go returns the raw
	// bytes, Rust guarantees UTF-8.
	ValidateUTF8 bool `json:"validate_utf8,omitempty"`
}

// Valid reports whether the model names a layout.
func (s Strings) Valid() bool {
	return s.Layout == StringLayoutNulTerminated || s.Layout == StringLayoutPointerLength
}

// Decode reads one value from the image:
//
//   - nul-terminated: address points at the first byte; the read stops at NUL.
//   - pointer-length: address points at the descriptor; its words use the
//     artifact's byte order and word size, and the pointed bytes are read.
//
// A missing terminator, an oversized length, an unreadable address, or invalid
// UTF-8 in a validating model is an error.
func (s Strings) Decode(a *Artifact, address uint64) (string, error) {
	if a == nil || a.Image == nil {
		return "", fmt.Errorf("string: artifact has no image")
	}
	switch s.Layout {
	case StringLayoutNulTerminated:
		return s.decodeNulTerminated(a, address)
	case StringLayoutPointerLength:
		return s.decodePointerLength(a, address)
	}
	return "", fmt.Errorf("string: unsupported layout %q", s.Layout)
}

func (s Strings) decodeNulTerminated(a *Artifact, address uint64) (string, error) {
	section, ok := a.Image.SectionAt(address)
	if !ok {
		return "", fmt.Errorf("string: address 0x%x is not mapped", address)
	}
	end := section.VirtualAddress + section.FileSize
	if end < section.VirtualAddress {
		return "", fmt.Errorf("string: section %s address range overflows", section.Name)
	}
	value := make([]byte, 0, 64)
	for len(value) < maxStringBytes {
		position := address + uint64(len(value))
		if position >= end {
			return "", fmt.Errorf("string: no NUL before the end of %s at 0x%x", section.Name, address)
		}
		window := end - position
		if remaining := uint64(maxStringBytes - len(value)); window > remaining {
			window = remaining
		}
		if window > 64 {
			window = 64
		}
		chunk, err := readImageBytes(a, position, window)
		if err != nil {
			return "", err
		}
		for _, b := range chunk {
			if b == 0 {
				return string(value), nil
			}
			value = append(value, b)
		}
	}
	return "", fmt.Errorf("string: no NUL within %d bytes at 0x%x", maxStringBytes, address)
}

func (s Strings) decodePointerLength(a *Artifact, address uint64) (string, error) {
	word := a.Target.Bits / 8
	if word != 4 && word != 8 {
		return "", fmt.Errorf("string: unsupported word size %d", a.Target.Bits)
	}
	region, err := a.RegionAt(address, uint64(2*word))
	if err != nil {
		return "", err
	}
	reader, err := a.RegionReader(region)
	if err != nil {
		return "", err
	}
	var pointer, length uint64
	if word == 4 {
		pointer32, err := reader.Uint32(0)
		if err != nil {
			return "", err
		}
		length32, err := reader.Uint32(4)
		if err != nil {
			return "", err
		}
		pointer, length = uint64(pointer32), uint64(length32)
	} else {
		pointer, err = reader.Uint64(0)
		if err != nil {
			return "", err
		}
		length, err = reader.Uint64(8)
		if err != nil {
			return "", err
		}
	}
	if length > maxStringBytes {
		return "", fmt.Errorf("string: length %d exceeds %d", length, maxStringBytes)
	}
	if length == 0 {
		return "", nil
	}
	if pointer == 0 {
		return "", fmt.Errorf("string: descriptor at 0x%x has a zero pointer; relocations are not applied", address)
	}
	value, err := readImageBytes(a, pointer, length)
	if err != nil {
		return "", err
	}
	if s.ValidateUTF8 && !utf8.Valid(value) {
		return "", fmt.Errorf("string: invalid UTF-8 at 0x%x", pointer)
	}
	return string(value), nil
}

func readImageBytes(a *Artifact, address, size uint64) ([]byte, error) {
	region, err := a.RegionAt(address, size)
	if err != nil {
		return nil, err
	}
	reader, err := a.RegionReader(region)
	if err != nil {
		return nil, err
	}
	return reader.Bytes(0, size)
}
