package core

import (
	"bytes"
	"context"
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

	Reader         *ByteReader `json:"-"`
	Loader         Loader      `json:"-"`
	Parser         Parser      `json:"-"`
	AssemblySyntax AsmSyntax   `json:"-"`
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

// Parse decodes a region with the artifact's architecture parser. ELF images
// with recovered boundaries are decoded one function at a time; a region
// without boundaries remains one function. The returned LowIR may therefore
// contain multiple functions.
func (a *Artifact) Parse(ctx context.Context, region Region) (*ParseResult, error) {
	return a.parseWithAssembly(ctx, region, false)
}

// ParseWithAssembly is Parse plus the parser's textual assembly, so an "all"
// view needs only one machine-code decode.
func (a *Artifact) ParseWithAssembly(ctx context.Context, region Region) (*ParseResult, error) {
	return a.parseWithAssembly(ctx, region, true)
}

// ParseExact decodes exactly one requested region. It is used by --address and
// --func, where the caller has already selected one function.
func (a *Artifact) ParseExact(ctx context.Context, region Region) (*ParseResult, error) {
	return a.parseExact(ctx, region, false)
}

// ParseExactWithAssembly is ParseExact plus assembly text.
func (a *Artifact) ParseExactWithAssembly(ctx context.Context, region Region) (*ParseResult, error) {
	return a.parseExact(ctx, region, true)
}

func (a *Artifact) parseExact(ctx context.Context, region Region, withAssembly bool) (*ParseResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.Reader == nil {
		return nil, fmt.Errorf("artifact has no reader")
	}
	if a.Parser == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoParser, a.Target.Architecture)
	}
	if err := a.Parser.Check(a.Target); err != nil {
		return nil, fmt.Errorf("parser %s: %w", a.Parser.Name(), err)
	}
	boundary := FunctionBoundary{Address: region.Address, Size: region.Size, Source: BoundaryScan}
	for _, candidate := range a.Image.Functions {
		if candidate.Address == region.Address {
			boundary = candidate
			break
		}
	}
	result, err := a.parseFunction(ctx, region, boundary, 1, withAssembly)
	if err != nil {
		return nil, err
	}
	a.annotateSymbols(result.IR)
	fillFunctionNames(result.IR)
	return result, nil
}

func (a *Artifact) parseWithAssembly(ctx context.Context, region Region, withAssembly bool) (*ParseResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.Reader == nil {
		return nil, fmt.Errorf("artifact has no reader")
	}
	if a.Parser == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoParser, a.Target.Architecture)
	}
	if err := a.Parser.Check(a.Target); err != nil {
		return nil, fmt.Errorf("parser %s: %w", a.Parser.Name(), err)
	}
	boundaries := a.FunctionBoundaries(region)
	if len(boundaries) > 0 && boundaries[0].Address != region.Address {
		boundaries = nil
	}
	if len(boundaries) == 1 && boundaries[0].Address == region.Address && boundaries[0].Size < region.Size {
		boundaries[0].Size = region.Size
	}
	if len(boundaries) == 0 {
		boundary := FunctionBoundary{Address: region.Address, Size: region.Size, Source: BoundaryScan}
		result, err := a.parseFunction(ctx, region, boundary, 1, withAssembly)
		if err != nil {
			return nil, err
		}
		a.annotateSymbols(result.IR)
		fillFunctionNames(result.IR)
		return result, nil
	}
	merged := &LowIR{Target: a.Target}
	diagnostics := make([]Diagnostic, 0)
	for index, boundary := range boundaries {
		subregion, err := a.functionRegion(region, boundary)
		if err != nil {
			return nil, err
		}
		result, err := a.parseFunction(ctx, subregion, boundary, index+1, withAssembly)
		if err != nil {
			return nil, err
		}
		merged.Functions = append(merged.Functions, result.IR.Functions...)
		merged.Assembly += result.Assembly
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	a.annotateSymbols(merged)
	fillFunctionNames(merged)
	return &ParseResult{IR: merged, Assembly: merged.Assembly, Diagnostics: diagnostics}, nil
}

func (a *Artifact) parseFunction(ctx context.Context, region Region, boundary FunctionBoundary, id int, withAssembly bool) (*ParseResult, error) {
	reader, err := a.regionReader(region)
	if err != nil {
		return nil, err
	}
	result, err := a.Parser.Parse(ctx, ParseRequest{
		Target:       a.Target,
		Address:      region.Address,
		Reader:       reader,
		Syntax:       a.AssemblySyntax,
		WithAssembly: withAssembly,
	})
	if err != nil {
		return nil, err
	}
	if result == nil || result.IR == nil {
		return nil, fmt.Errorf("parser %s returned no LowIR", a.Parser.Name())
	}
	if result.IR.Target != a.Target {
		return nil, fmt.Errorf("parser %s returned an inconsistent target", a.Parser.Name())
	}
	if len(result.IR.Functions) == 0 {
		return nil, fmt.Errorf("parser %s returned no functions", a.Parser.Name())
	}
	for functionIndex := range result.IR.Functions {
		function := &result.IR.Functions[functionIndex]
		function.ID = FunctionID(id + functionIndex)
		function.Address = region.Address
		function.Size = region.Size
		function.Boundary = boundary.Source
		if boundary.Name != "" {
			function.Name = boundary.Name
		}
	}
	return result, nil
}

func fillFunctionNames(ir *LowIR) {
	if ir == nil {
		return
	}
	for index := range ir.Functions {
		if ir.Functions[index].Name == "" {
			ir.Functions[index].Name = fmt.Sprintf("sub_0x%x", ir.Functions[index].Address)
		}
	}
}

// ParseAll decodes every recovered function in the image. Sections that are
// not executable have no boundaries and are not parsed.
func (a *Artifact) ParseAll(ctx context.Context) (*ParseResult, error) {
	return a.parseAll(ctx, false)
}

// ParseAllWithAssembly parses every recovered function and collects its
// assembly in the same decode.
func (a *Artifact) ParseAllWithAssembly(ctx context.Context) (*ParseResult, error) {
	return a.parseAll(ctx, true)
}

func (a *Artifact) parseAll(ctx context.Context, withAssembly bool) (*ParseResult, error) {
	if a == nil || a.Image == nil {
		return nil, fmt.Errorf("artifact has no image")
	}
	boundaries := make([]FunctionBoundary, 0, len(a.Image.Functions))
	for _, function := range a.Image.Functions {
		boundaries = append(boundaries, function)
	}
	if len(boundaries) == 0 {
		for _, section := range a.Image.Sections {
			if section.Name != ".text" || section.FileSize == 0 {
				continue
			}
			region, err := a.RegionAt(section.VirtualAddress, section.FileSize)
			if err != nil {
				return nil, err
			}
			return a.Parse(ctx, region)
		}
		return nil, fmt.Errorf("artifact has no executable functions")
	}
	merged := &LowIR{Target: a.Target}
	diagnostics := make([]Diagnostic, 0)
	for index, boundary := range boundaries {
		region, err := a.RegionAt(boundary.Address, boundary.Size)
		if err != nil {
			return nil, err
		}
		result, err := a.parseFunction(ctx, region, boundary, index+1, withAssembly)
		if err != nil {
			return nil, err
		}
		merged.Functions = append(merged.Functions, result.IR.Functions...)
		merged.Assembly += result.Assembly
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	a.annotateSymbols(merged)
	fillFunctionNames(merged)
	return &ParseResult{IR: merged, Assembly: merged.Assembly, Diagnostics: diagnostics}, nil
}

// AllFunctionBoundaries returns every recovered function in the image.
func (a *Artifact) AllFunctionBoundaries() []FunctionBoundary {
	if a == nil || a.Image == nil {
		return nil
	}
	return append([]FunctionBoundary(nil), a.Image.Functions...)
}

// FunctionBoundaries returns discovered function starts inside region. Sizes
// are resolved from the next boundary when discovery could not prove one.
func (a *Artifact) FunctionBoundaries(region Region) []FunctionBoundary {
	if a == nil || a.Image == nil || len(a.Image.Functions) == 0 || region.Size == 0 {
		return nil
	}
	end := region.Address + region.Size
	if end < region.Address {
		return nil
	}
	result := make([]FunctionBoundary, 0)
	for _, function := range a.Image.Functions {
		if function.Address < region.Address || function.Address >= end {
			continue
		}
		size := function.Size
		if size == 0 || size > end-function.Address {
			size = end - function.Address
		}
		function.Size = size
		result = append(result, function)
	}
	return result
}

// FunctionByName resolves a symbol name to a recovered function body.
func (a *Artifact) FunctionByName(name string) (FunctionBoundary, error) {
	if a == nil || a.Image == nil {
		return FunctionBoundary{}, fmt.Errorf("function %q: artifact has no symbols", name)
	}
	for _, symbol := range a.Image.Symbols {
		if symbol.Name != name {
			continue
		}
		if symbol.Kind != SymbolFunction {
			return FunctionBoundary{}, fmt.Errorf("symbol %q is not a function", name)
		}
		if symbol.Imported || symbol.Address == 0 {
			return FunctionBoundary{}, fmt.Errorf("function %q has no local body", name)
		}
		for _, function := range a.Image.Functions {
			if function.Address == symbol.Address {
				function.Name = symbol.Name
				if function.Size == 0 {
					for _, candidate := range a.Image.Functions {
						if candidate.Address > function.Address && (function.Size == 0 || candidate.Address-function.Address < function.Size) {
							function.Size = candidate.Address - function.Address
						}
					}
				}
				if function.Size == 0 {
					return FunctionBoundary{}, fmt.Errorf("function %q has an empty range", name)
				}
				return function, nil
			}
		}
		if symbol.Size == 0 {
			return FunctionBoundary{}, fmt.Errorf("function %q has an empty range", name)
		}
		return FunctionBoundary{Address: symbol.Address, Size: symbol.Size, Name: name, Source: BoundarySymbol}, nil
	}
	return FunctionBoundary{}, fmt.Errorf("symbol %q does not exist", name)
}

func (a *Artifact) functionRegion(region Region, boundary FunctionBoundary) (Region, error) {
	size := boundary.Size
	if size == 0 {
		size = region.Address + region.Size - boundary.Address
	}
	if size == 0 {
		return Region{}, fmt.Errorf("function at 0x%x has an empty range", boundary.Address)
	}
	if end := boundary.Address + size; end < boundary.Address || end > region.Address+region.Size {
		size = region.Address + region.Size - boundary.Address
	}
	return a.RegionAt(boundary.Address, size)
}

// Disassemble renders one region as text through an optional parser capability.
// It does not build LowIR.
func (a *Artifact) Disassemble(ctx context.Context, region Region) (*Disassembly, error) {
	return a.DisassembleWithOptions(ctx, region, DisassembleOptions{})
}

// DisassembleWithOptions renders one region with explicit rendering options.
func (a *Artifact) DisassembleWithOptions(ctx context.Context, region Region, options DisassembleOptions) (*Disassembly, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if a == nil || a.Reader == nil {
		return nil, fmt.Errorf("artifact has no reader")
	}
	if a.Parser == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoParser, a.Target.Architecture)
	}
	disassembler, ok := a.Parser.(Disassembler)
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrNoDisassembler, a.Parser.Name())
	}
	reader, err := a.regionReader(region)
	if err != nil {
		return nil, err
	}
	if err := a.Parser.Check(a.Target); err != nil {
		return nil, fmt.Errorf("parser %s: %w", a.Parser.Name(), err)
	}
	result, err := disassembler.Disassemble(ctx, ParseRequest{
		Target:  a.Target,
		Address: region.Address,
		Reader:  reader,
		Syntax:  options.Syntax,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("parser %s returned no disassembly", a.Parser.Name())
	}
	return result, nil
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

func (a *Artifact) regionReader(region Region) (*ByteReader, error) {
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
