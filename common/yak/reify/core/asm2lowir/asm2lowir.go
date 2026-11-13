// Package asm2lowir orchestrates the first representation layer: it asks an
// architecture parser to decode machine code into LowIR, resolves function
// boundaries from loader hints and symbols, and gives call targets their names.
package asm2lowir

import (
	"context"
	"fmt"
	"sort"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	sym "github.com/yaklang/yaklang/common/yak/reify/core/symbol"
)

// Result is one asm-to-LowIR conversion.
type Result struct {
	IR            *core.LowIR
	Assembly      string
	Diagnostics   []core.Diagnostic
	Boundaries    []core.FunctionBoundary
	BoundaryStats core.BoundaryStats
}

// Config controls Parse.
type Config struct {
	Region       *core.Region
	Section      string
	Function     string
	All          bool
	Exact        bool
	WithAssembly bool
	Syntax       core.AsmSyntax
}

// Option customizes Parse.
type Option func(*Config)

// WithRegion decodes one address range.
func WithRegion(region core.Region) Option { return func(config *Config) { config.Region = &region } }

// WithSection decodes one named executable section.
func WithSection(name string) Option { return func(config *Config) { config.Section = name } }

// WithFunction decodes one recovered function by symbol name.
func WithFunction(name string) Option { return func(config *Config) { config.Function = name } }

// WithAll decodes every executable section.
func WithAll() Option { return func(config *Config) { config.All = true } }

// WithExact decodes one region as exactly one function.
func WithExact() Option { return func(config *Config) { config.Exact = true } }

// WithAssembly collects decoded assembly text in the same pass.
func WithAssembly() Option { return func(config *Config) { config.WithAssembly = true } }

// WithAsmSyntax selects the assembly dialect.
func WithAsmSyntax(syntax core.AsmSyntax) Option {
	return func(config *Config) { config.Syntax = syntax }
}

func settings(options []Option) Config {
	var result Config
	for _, option := range options {
		option(&result)
	}
	return result
}

// Parse decodes machine code into LowIR. Core only defines data; the parser
// call, boundary discovery and symbol naming live here.
func Parse(ctx context.Context, artifact *core.Artifact, options ...Option) (*Result, error) {
	if artifact == nil {
		return nil, fmt.Errorf("asm2lowir: nil artifact")
	}
	if artifact.Parser == nil {
		return nil, fmt.Errorf("%w: %s", core.ErrNoParser, artifact.Target.Architecture)
	}
	if err := artifact.Parser.Check(artifact.Target); err != nil {
		return nil, fmt.Errorf("parser %s: %w", artifact.Parser.Name(), err)
	}
	config := settings(options)
	boundaries, stats, err := discover(artifact, config)
	if err != nil {
		return nil, err
	}
	if len(boundaries) == 0 {
		return nil, fmt.Errorf("asm2lowir: no function boundaries")
	}

	result := &Result{Boundaries: boundaries, BoundaryStats: stats}
	result.IR = &core.LowIR{Target: artifact.Target}
	nextID := core.FunctionID(1)
	for index, boundary := range boundaries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		size := boundaryRange(artifact, boundaries, index)
		if size == 0 {
			result.Diagnostics = append(result.Diagnostics, core.Diagnostic{
				Code:    "empty-function",
				Message: fmt.Sprintf("function at 0x%x has no file-backed bytes", boundary.Address),
			})
			continue
		}
		region, err := boundaryRegion(artifact, config.Region, boundary.Address, size)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, core.Diagnostic{
				Code:    "boundary-region",
				Message: fmt.Sprintf("%s at 0x%x: %v", boundary.Name, boundary.Address, err),
			})
			continue
		}
		reader, err := artifact.RegionReader(region)
		if err != nil {
			return nil, err
		}
		parsed, err := artifact.Parser.Parse(ctx, core.ParseRequest{
			Target:       artifact.Target,
			Address:      region.Address,
			Reader:       reader,
			Syntax:       config.Syntax,
			WithAssembly: config.WithAssembly,
		})
		if err != nil {
			return nil, err
		}
		if parsed == nil || parsed.IR == nil {
			return nil, fmt.Errorf("parser %s returned no LowIR", artifact.Parser.Name())
		}
		for functionIndex := range parsed.IR.Functions {
			function := &parsed.IR.Functions[functionIndex]
			function.ID = nextID
			nextID++
			function.Address = boundary.Address
			function.Size = size
			function.Boundary = boundary.Source
			if boundary.Name != "" {
				function.Name = boundary.Name
			}
		}
		result.IR.Functions = append(result.IR.Functions, parsed.IR.Functions...)
		result.Assembly += parsed.Assembly
		result.Diagnostics = append(result.Diagnostics, parsed.Diagnostics...)
	}
	annotateSymbols(artifact, result.IR)
	fillFunctionNames(result.IR)
	if len(boundaries) > 1 {
		result.Diagnostics = filterCrossFunctionDiagnostics(result.Diagnostics)
	}
	return result, nil
}

func discover(artifact *core.Artifact, config Config) ([]core.FunctionBoundary, core.BoundaryStats, error) {
	hints := core.BoundaryHints{}
	if artifact.Hints != nil {
		hints = *artifact.Hints
	}
	if config.Function != "" {
		boundary, err := functionBoundary(artifact, config.Function)
		if err != nil {
			return nil, core.BoundaryStats{}, err
		}
		return []core.FunctionBoundary{boundary}, core.BoundaryStats{Symbol: 1}, nil
	}
	if config.Region != nil {
		region := *config.Region
		if config.Exact {
			return []core.FunctionBoundary{{Address: region.Address, Size: region.Size, Source: core.BoundaryScan}}, core.BoundaryStats{}, nil
		}
		boundaries, stats := discoverWithParser(artifact, hints)
		filtered := filterBoundaries(boundaries, region)
		if len(filtered) > 0 {
			return filtered, stats, nil
		}
		return []core.FunctionBoundary{{Address: region.Address, Size: region.Size, Source: core.BoundaryScan}}, stats, nil
	}
	if config.Section != "" {
		region, err := sectionRegion(artifact, config.Section)
		if err != nil {
			return nil, core.BoundaryStats{}, err
		}
		boundaries, stats := discoverWithParser(artifact, hints)
		if filtered := filterBoundaries(boundaries, region); len(filtered) > 0 {
			return filtered, stats, nil
		}
		return []core.FunctionBoundary{{Address: region.Address, Size: region.Size, Source: core.BoundaryScan}}, stats, nil
	}
	if config.All {
		boundaries, stats := discoverWithParser(artifact, hints)
		if len(boundaries) > 0 {
			return boundaries, stats, nil
		}
		return execSectionBoundaries(artifact), core.BoundaryStats{}, nil
	}
	return nil, core.BoundaryStats{}, fmt.Errorf("asm2lowir: no region, section or function selected")
}

func discoverWithParser(artifact *core.Artifact, hints core.BoundaryHints) ([]core.FunctionBoundary, core.BoundaryStats) {
	discoverer, ok := artifact.Parser.(core.BoundaryDiscoverer)
	if !ok {
		return symbolBoundaries(artifact), core.BoundaryStats{}
	}
	result, err := discoverer.DiscoverBoundaries(artifact.Reader, hints, artifact.Image)
	if err != nil {
		return symbolBoundaries(artifact), core.BoundaryStats{}
	}
	return result.Functions, result.Stats
}

func symbolBoundaries(artifact *core.Artifact) []core.FunctionBoundary {
	result := make([]core.FunctionBoundary, 0)
	if artifact.Image == nil {
		return result
	}
	for _, symbol := range artifact.Image.Symbols {
		if symbol.Kind != sym.SymbolFunction || symbol.Imported || symbol.Address == 0 {
			continue
		}
		result = append(result, core.FunctionBoundary{Address: symbol.Address, Size: symbol.Size, Name: symbol.Name, Source: core.BoundarySymbol})
	}
	return result
}

func execSectionBoundaries(artifact *core.Artifact) []core.FunctionBoundary {
	result := make([]core.FunctionBoundary, 0)
	if artifact.Hints == nil {
		return result
	}
	for _, region := range artifact.Hints.ExecSections {
		result = append(result, core.FunctionBoundary{Address: region.Address, Size: region.Size, Source: core.BoundaryScan})
	}
	return result
}

func filterBoundaries(boundaries []core.FunctionBoundary, region core.Region) []core.FunctionBoundary {
	end := region.Address + region.Size
	result := make([]core.FunctionBoundary, 0, len(boundaries))
	for _, boundary := range boundaries {
		if boundary.Address < region.Address || boundary.Address >= end {
			continue
		}
		if boundary.Size == 0 || boundary.Size > end-boundary.Address {
			boundary.Size = end - boundary.Address
		}
		result = append(result, boundary)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Address < result[j].Address })
	return result
}

func boundaryRegion(artifact *core.Artifact, scope *core.Region, address, size uint64) (core.Region, error) {
	if scope != nil {
		if address < scope.Address || size > scope.Size-(address-scope.Address) {
			return core.Region{}, fmt.Errorf("asm2lowir: boundary 0x%x is outside the selected region", address)
		}
		return core.Region{Address: address, Offset: scope.Offset + (address - scope.Address), Size: size}, nil
	}
	return artifact.RegionAt(address, size)
}

func boundaryRange(artifact *core.Artifact, boundaries []core.FunctionBoundary, index int) uint64 {
	boundary := boundaries[index]
	if boundary.Size > 0 {
		return boundary.Size
	}
	if index+1 < len(boundaries) {
		return boundaries[index+1].Address - boundary.Address
	}
	if region := regionFor(artifact, boundary.Address); region != nil {
		return region.Address + region.Size - boundary.Address
	}
	return 0
}

func regionFor(artifact *core.Artifact, address uint64) *core.Region {
	if artifact.Hints == nil {
		return nil
	}
	for index := range artifact.Hints.ExecSections {
		region := &artifact.Hints.ExecSections[index]
		if address >= region.Address && address-region.Address < region.Size {
			return region
		}
	}
	return nil
}

func functionBoundary(artifact *core.Artifact, name string) (core.FunctionBoundary, error) {
	if artifact.Image == nil {
		return core.FunctionBoundary{}, fmt.Errorf("function %q: artifact has no symbols", name)
	}
	for _, symbol := range artifact.Image.Symbols {
		if symbol.Name != name {
			continue
		}
		if symbol.Kind != sym.SymbolFunction {
			return core.FunctionBoundary{}, fmt.Errorf("symbol %q is not a function", name)
		}
		if symbol.Imported || symbol.Address == 0 {
			return core.FunctionBoundary{}, fmt.Errorf("function %q has no local body", name)
		}
		return core.FunctionBoundary{Address: symbol.Address, Size: symbol.Size, Name: name, Source: core.BoundarySymbol}, nil
	}
	return core.FunctionBoundary{}, fmt.Errorf("symbol %q does not exist", name)
}

func sectionRegion(artifact *core.Artifact, name string) (core.Region, error) {
	if artifact.Image == nil {
		return core.Region{}, fmt.Errorf("asm2lowir: artifact has no image")
	}
	if name == "" {
		name = ".text"
	}
	for _, section := range artifact.Image.Sections {
		if section.Name == name {
			if section.FileSize == 0 {
				return core.Region{}, fmt.Errorf("asm2lowir: section %q has no file data", name)
			}
			return artifact.RegionAt(section.VirtualAddress, section.FileSize)
		}
	}
	return core.Region{}, fmt.Errorf("asm2lowir: section %q not found", name)
}

func filterCrossFunctionDiagnostics(diagnostics []core.Diagnostic) []core.Diagnostic {
	filtered := make([]core.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case "branch-target-outside-region", "lowir2ssa-incomplete-branch":
			continue
		default:
			filtered = append(filtered, diagnostic)
		}
	}
	return filtered
}

func fillFunctionNames(ir *core.LowIR) {
	for index := range ir.Functions {
		if ir.Functions[index].Name == "" {
			ir.Functions[index].Name = fmt.Sprintf("sub_0x%x", ir.Functions[index].Address)
		}
	}
}

// Disassemble renders machine code as text. It uses the same boundary selection
// as Parse and calls the parser's optional Disassembler per function.
func Disassemble(ctx context.Context, artifact *core.Artifact, options ...Option) (*core.Disassembly, error) {
	if artifact == nil {
		return nil, fmt.Errorf("asm2lowir: nil artifact")
	}
	if artifact.Parser == nil {
		return nil, fmt.Errorf("%w: %s", core.ErrNoParser, artifact.Target.Architecture)
	}
	disassembler, ok := artifact.Parser.(core.Disassembler)
	if !ok {
		return nil, fmt.Errorf("%w: %s", core.ErrNoDisassembler, artifact.Parser.Name())
	}
	if err := artifact.Parser.Check(artifact.Target); err != nil {
		return nil, fmt.Errorf("parser %s: %w", artifact.Parser.Name(), err)
	}
	config := settings(options)
	boundaries, _, err := discover(artifact, config)
	if err != nil {
		return nil, err
	}
	var text string
	total := 0
	diagnostics := make([]core.Diagnostic, 0)
	for index, boundary := range boundaries {
		size := boundaryRange(artifact, boundaries, index)
		region, err := boundaryRegion(artifact, config.Region, boundary.Address, size)
		if err != nil {
			diagnostics = append(diagnostics, core.Diagnostic{Code: "boundary-region", Message: err.Error()})
			continue
		}
		reader, err := artifact.RegionReader(region)
		if err != nil {
			return nil, err
		}
		result, err := disassembler.Disassemble(ctx, core.ParseRequest{
			Target:  artifact.Target,
			Address: region.Address,
			Reader:  reader,
			Syntax:  config.Syntax,
		})
		if err != nil {
			return nil, err
		}
		if result == nil {
			continue
		}
		text += result.Text
		total += result.Instructions
		diagnostics = append(diagnostics, result.Diagnostics...)
	}
	if total == 0 {
		return nil, fmt.Errorf("no instructions recovered from function boundaries")
	}
	if len(boundaries) > 1 {
		diagnostics = filterCrossFunctionDiagnostics(diagnostics)
	}
	return &core.Disassembly{Text: text, Instructions: total, Diagnostics: diagnostics}, nil
}
