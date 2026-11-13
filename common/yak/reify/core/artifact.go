package core

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"unicode"
	"unicode/utf8"
)

type Origin struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Artifact is the result of loading one input. It owns the per-input ByteReader
// and records the loader and parser selected for that input, so callers can go
// from a path to LowIR through this value.
type Artifact struct {
	Origin      Origin       `json:"origin"`
	Kind        InputKind    `json:"kind"`
	Format      Format       `json:"format"`
	Target      Target       `json:"target"`
	Image       *Image       `json:"image,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`

	Hints *BoundaryHints `json:"-"`

	Reader *ByteReader `json:"-"`
	Loader Loader      `json:"-"`
	Parser Parser      `json:"-"`
}

var (
	ErrNoParser       = errors.New("no parser for target architecture")
	ErrNoDisassembler = errors.New("parser cannot render assembly")
	ErrRegion         = errors.New("region is outside the artifact")
)

// Close releases the input source when it is owned by the artifact, as in
// Catalog.OpenFile. Borrowed sources are not closed.
func (a *Artifact) Close() error {
	if a == nil || a.Reader == nil {
		return nil
	}
	return a.Reader.Close()
}

// Region identifies a contiguous range in both file and image space.
type Region struct {
	Address uint64 `json:"address"`
	Offset  uint64 `json:"offset"`
	Size    uint64 `json:"size"`
}

// RegionAt maps a virtual address range to a file-backed region. Zero-filled
// ranges and ranges crossing a section boundary are rejected.
func (a *Artifact) RegionAt(address, size uint64) (Region, error) {
	if a == nil || a.Image == nil {
		return Region{}, fmt.Errorf("%w: artifact has no image", ErrRegion)
	}
	if size == 0 {
		return Region{}, fmt.Errorf("%w: empty range", ErrRegion)
	}
	if size > math.MaxUint64-address {
		return Region{}, fmt.Errorf("%w: address range overflows", ErrRegion)
	}
	section, ok := a.Image.SectionAt(address)
	if !ok {
		return Region{}, fmt.Errorf("%w: address 0x%x is not in a section", ErrRegion, address)
	}
	delta := address - section.VirtualAddress
	if delta >= section.FileSize {
		return Region{}, fmt.Errorf("%w: address 0x%x is zero-fill only", ErrRegion, address)
	}
	if size > section.FileSize-delta {
		return Region{}, fmt.Errorf("%w: range crosses section %q file data", ErrRegion, section.Name)
	}
	return Region{Address: address, Offset: section.FileOffset + delta, Size: size}, nil
}

// EntryRegion returns the file-backed region that starts at the image entry
// point. The caller supplies the byte count because the header does not encode
// the size of the entry function.
func (a *Artifact) EntryRegion(size uint64) (Region, error) {
	if a == nil || a.Image == nil {
		return Region{}, fmt.Errorf("%w: artifact has no image", ErrRegion)
	}
	entry, ok := a.Image.EntryPoint()
	if !ok {
		return Region{}, fmt.Errorf("%w: image has no entry point", ErrRegion)
	}
	return a.RegionAt(entry, size)
}

// StringAt returns a NUL-terminated printable string that starts at an image
// address. It reads at most max bytes from the file-backed part of the section.
func (a *Artifact) StringAt(address, max uint64) (string, bool) {
	if a == nil || a.Reader == nil || a.Image == nil {
		return "", false
	}
	section, ok := a.Image.SectionAt(address)
	if !ok || section.VirtualAddress == 0 {
		return "", false
	}
	delta := address - section.VirtualAddress
	if delta >= section.FileSize {
		return "", false
	}
	size := section.FileSize - delta
	if max > 0 && size > max {
		size = max
	}
	data, err := a.Reader.Bytes(section.FileOffset+delta, size)
	if err != nil {
		return "", false
	}
	end := bytes.IndexByte(data, 0)
	if end <= 0 {
		return "", false
	}
	data = data[:end]
	if !utf8.Valid(data) {
		return "", false
	}
	for _, r := range string(data) {
		if !unicode.IsPrint(r) && r != '\n' && r != '\r' && r != '\t' {
			return "", false
		}
	}
	return string(data), true
}

func (a *Artifact) RegionReader(region Region) (*ByteReader, error) {
	if region.Size == 0 {
		return nil, fmt.Errorf("%w: empty range", ErrRegion)
	}
	if err := a.Reader.CheckRange(region.Offset, region.Size); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRegion, err)
	}
	if region.Size > math.MaxUint64-region.Address {
		return nil, fmt.Errorf("%w: address range overflows", ErrRegion)
	}
	reader, err := a.Reader.Slice(region.Offset, region.Size)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRegion, err)
	}
	return reader, nil
}
