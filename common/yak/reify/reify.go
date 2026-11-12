// Package reify is the public entry point for loading an executable, lifting
// one region through the three representation layers, and reporting the
// result. Command-line code only parses flags, calls this package, and prints.
package reify

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/asm2lowir"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	ssapass "github.com/yaklang/yaklang/common/yak/reify/core/ssa2source/pass"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// Mode selects the deepest representation rendered by Lift.
type Mode string

const (
	ModeASM    Mode = "asm"
	ModeLowIR  Mode = "lowir"
	ModeSSAIR  Mode = "ssair"
	ModeSource Mode = "source"
	ModeAll    Mode = "all"
)

// ParseMode validates a mode name.
func ParseMode(raw string) (Mode, error) {
	switch Mode(raw) {
	case ModeASM, ModeLowIR, ModeSSAIR, ModeSource, ModeAll:
		return Mode(raw), nil
	default:
		return "", fmt.Errorf("unknown lift mode %q (want asm, lowir, ssair, source or all)", raw)
	}
}

// ParseAsmSyntax validates an assembly dialect name.
func ParseAsmSyntax(raw string) (core.AsmSyntax, error) {
	switch raw {
	case "", "intel":
		return core.AsmSyntaxIntel, nil
	case "att", "gnu":
		return core.AsmSyntaxATT, nil
	default:
		return "", fmt.Errorf("unknown assembly syntax %q (want att or intel)", raw)
	}
}

// Result is one completed lift.
type Result struct {
	Mode          Mode
	Artifact      *core.Artifact
	Parser        *core.ParserInfo
	Region        core.Region
	Assembly      string
	Instructions  int
	IR            *core.LowIR
	SSAIR         string
	Source        string
	Diagnostics   []core.Diagnostic
	BoundaryStats *core.BoundaryStats
	Functions     int
	Blocks        int
	Values        int
}

// Close releases the input opened by Lift.
func (r *Result) Close() error {
	if r == nil || r.Artifact == nil {
		return nil
	}
	return r.Artifact.Close()
}

// Config controls Lift.
type Config struct {
	Mode        Mode
	AsmSyntax   core.AsmSyntax
	LowIRPasses []string
	SSAPasses   []string
	Section     string
	Address     string
	Size        string
	Function    string
}

// Option customizes Lift.
type Option func(*Config)

// WithMode selects the rendered representation. The default is asm.
func WithMode(mode Mode) Option { return func(config *Config) { config.Mode = mode } }

// WithAsmSyntax selects the assembly dialect.
func WithAsmSyntax(syntax core.AsmSyntax) Option {
	return func(config *Config) { config.AsmSyntax = syntax }
}

// WithLowIRPasses selects registered LowIR passes.
func WithLowIRPasses(names ...string) Option {
	return func(config *Config) { config.LowIRPasses = append([]string(nil), names...) }
}

// WithSSAPasses selects registered SSA passes.
func WithSSAPasses(names ...string) Option {
	return func(config *Config) { config.SSAPasses = append([]string(nil), names...) }
}

// WithSection limits decoding to one section.
func WithSection(name string) Option { return func(config *Config) { config.Section = name } }

// WithAddress limits decoding to an address and size. Address may be a symbol.
func WithAddress(address, size string) Option {
	return func(config *Config) {
		config.Address = address
		config.Size = size
	}
}

// WithFunction limits decoding to one recovered function.
func WithFunction(name string) Option { return func(config *Config) { config.Function = name } }

func config(options []Option) Config {
	result := Config{Mode: ModeASM, AsmSyntax: core.AsmSyntaxIntel}
	for _, option := range options {
		option(&result)
	}
	return result
}

// Loaders lists every registered native loader.
func Loaders() []core.LoaderInfo { return loader.Default.Loaders() }

// Parsers lists every registered architecture parser.
func Parsers() []core.ParserInfo { return loader.Default.Parsers() }

// Check validates one file header.
func Check(ctx context.Context, path string) (*loader.CheckResult, error) {
	return loader.Default.CheckFile(ctx, path)
}

// Inspect loads one file and returns the artifact plus the selected parser.
// The caller must close the artifact.
func Inspect(ctx context.Context, path string) (*core.Artifact, *core.ParserInfo, error) {
	artifact, err := loader.Default.OpenFile(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	return artifact, ParserInfo(artifact), nil
}

// ParserInfo describes the parser selected for an artifact.
func ParserInfo(artifact *core.Artifact) *core.ParserInfo {
	if artifact == nil || artifact.Parser == nil {
		return nil
	}
	return &core.ParserInfo{
		Kind:      artifact.Parser.Kind(),
		Name:      artifact.Parser.Name(),
		Available: artifact.Parser.Available(),
	}
}

// Lift loads path and runs the requested representation layers.
func Lift(ctx context.Context, path string, options ...Option) (*Result, error) {
	settings := config(options)
	if settings.Size != "" && settings.Address == "" {
		return nil, fmt.Errorf("lift: --size requires --address")
	}
	artifact, err := loader.Default.OpenFile(ctx, path)
	if err != nil {
		return nil, err
	}
	result := &Result{Mode: settings.Mode, Artifact: artifact, Parser: ParserInfo(artifact)}
	ok := false
	defer func() {
		if !ok {
			_ = result.Close()
		}
	}()
	asmOptions := []asm2lowir.Option{asm2lowir.WithAsmSyntax(settings.AsmSyntax)}
	switch {
	case settings.Function != "":
		asmOptions = append(asmOptions, asm2lowir.WithFunction(settings.Function))
	case settings.Address != "":
		region, err := addressRegion(artifact, settings.Address, settings.Size)
		if err != nil {
			return nil, err
		}
		asmOptions = append(asmOptions, asm2lowir.WithRegion(region), asm2lowir.WithExact())
	case settings.Section != "":
		asmOptions = append(asmOptions, asm2lowir.WithSection(settings.Section))
	default:
		asmOptions = append(asmOptions, asm2lowir.WithAll())
	}
	if settings.Mode == ModeAll {
		asmOptions = append(asmOptions, asm2lowir.WithAssembly())
	}

	if settings.Mode == ModeASM {
		disassembly, err := asm2lowir.Disassemble(ctx, artifact, asmOptions...)
		if err != nil {
			return nil, err
		}
		result.Assembly = disassembly.Text
		result.Instructions = disassembly.Instructions
		result.Diagnostics = disassembly.Diagnostics
		result.Region = displayRegion(artifact, settings)
		ok = true
		return result, nil
	}

	parsed, err := asm2lowir.Parse(ctx, artifact, asmOptions...)
	if err != nil {
		return nil, err
	}
	result.IR = parsed.IR
	result.Assembly = parsed.Assembly
	result.BoundaryStats = &parsed.BoundaryStats
	result.Region = boundariesRegion(parsed.Boundaries)
	if result.Region.Size == 0 {
		result.Region = displayRegion(artifact, settings)
	}
	result.Functions = len(parsed.IR.Functions)
	result.Blocks = countBlocks(parsed.IR)
	result.Instructions = countInstructions(parsed.IR)
	result.Values = countValues(parsed.IR)
	result.Diagnostics = append(result.Diagnostics, parsed.Diagnostics...)

	if settings.Mode == ModeLowIR {
		ok = true
		return result, nil
	}

	convertOptions := []lowir2ssa.Option{}
	if len(settings.LowIRPasses) > 0 {
		convertOptions = append(convertOptions, lowir2ssa.WithLowIRPasses(settings.LowIRPasses...))
	}
	converted, err := lowir2ssa.Convert(ctx, parsed.IR, convertOptions...)
	if err != nil {
		return nil, err
	}
	result.Diagnostics = append(result.Diagnostics, converted.Diagnostics...)
	if err := applySSAPasses(converted.Program, artifact, settings.SSAPasses); err != nil {
		return nil, err
	}
	ssa2source.AnnotateArtifact(converted.Program, artifact)
	if settings.Mode == ModeSSAIR || settings.Mode == ModeAll {
		result.SSAIR = lowir2ssa.Text(converted.Program)
	}
	if settings.Mode == ModeSSAIR {
		ok = true
		return result, nil
	}
	source, err := ssa2source.Render(ctx, converted.Program)
	if err != nil {
		return nil, err
	}
	result.Source = source
	if settings.Mode == ModeSource {
		result.Functions = countSourceFunctions(parsed.IR)
	}
	ok = true
	return result, nil
}

func applySSAPasses(program *ssa.Program, artifact *core.Artifact, selected []string) error {
	stringAt := func(address uint64) (string, bool) { return artifact.StringAt(address, 128) }
	if len(selected) > 0 {
		return ssapass.ApplySelectedWith(program, stringAt, selected...)
	}
	ssapass.ApplyDefault(program, stringAt)
	return nil
}

func addressRegion(artifact *core.Artifact, addressText, sizeText string) (core.Region, error) {
	if address, err := strconv.ParseUint(addressText, 0, 64); err == nil {
		if sizeText == "" {
			return core.Region{}, fmt.Errorf("lift: --address requires --size")
		}
		size, err := strconv.ParseUint(sizeText, 0, 64)
		if err != nil {
			return core.Region{}, fmt.Errorf("invalid --size %q", sizeText)
		}
		return artifact.RegionAt(address, size)
	}
	name := strings.TrimPrefix(addressText, "0x")
	if artifact.Image == nil {
		return core.Region{}, fmt.Errorf("lift: artifact has no symbols")
	}
	for _, symbol := range artifact.Image.Symbols {
		if symbol.Name != name {
			continue
		}
		if symbol.Address == 0 || symbol.Imported {
			return core.Region{}, fmt.Errorf("function %q has no local body", name)
		}
		size := symbol.Size
		if sizeText != "" {
			parsed, err := strconv.ParseUint(sizeText, 0, 64)
			if err != nil {
				return core.Region{}, fmt.Errorf("invalid --size %q", sizeText)
			}
			size = parsed
		}
		if size == 0 {
			return core.Region{}, fmt.Errorf("function %q has an empty range", name)
		}
		return artifact.RegionAt(symbol.Address, size)
	}
	return core.Region{}, fmt.Errorf("symbol %q does not exist", name)
}

func displayRegion(artifact *core.Artifact, settings Config) core.Region {
	switch {
	case settings.Function != "":
		if region, err := addressRegion(artifact, settings.Function, settings.Size); err == nil {
			return region
		}
	case settings.Address != "":
		if region, err := addressRegion(artifact, settings.Address, settings.Size); err == nil {
			return region
		}
	case settings.Section != "":
		if region, err := sectionRegion(artifact, settings.Section); err == nil {
			return region
		}
	}
	if artifact.Hints == nil || len(artifact.Hints.ExecSections) == 0 {
		return core.Region{}
	}
	start := artifact.Hints.ExecSections[0].Address
	end := start + artifact.Hints.ExecSections[0].Size
	for _, region := range artifact.Hints.ExecSections[1:] {
		if region.Address < start {
			start = region.Address
		}
		if regionEnd := region.Address + region.Size; regionEnd > end {
			end = regionEnd
		}
	}
	return core.Region{Address: start, Size: end - start}
}

func boundariesRegion(boundaries []core.FunctionBoundary) core.Region {
	if len(boundaries) == 0 {
		return core.Region{}
	}
	start := boundaries[0].Address
	end := start + boundaries[0].Size
	for _, boundary := range boundaries[1:] {
		if boundary.Address < start {
			start = boundary.Address
		}
		if boundaryEnd := boundary.Address + boundary.Size; boundaryEnd > end {
			end = boundaryEnd
		}
	}
	return core.Region{Address: start, Size: end - start}
}

func sectionRegion(artifact *core.Artifact, section string) (core.Region, error) {
	if artifact.Image == nil {
		return core.Region{}, fmt.Errorf("lift: artifact has no image")
	}
	if section == "" {
		section = ".text"
	}
	for _, candidate := range artifact.Image.Sections {
		if candidate.Name != section {
			continue
		}
		if candidate.FileSize == 0 {
			return core.Region{}, fmt.Errorf("lift: section %q has no file data", section)
		}
		return artifact.RegionAt(candidate.VirtualAddress, candidate.FileSize)
	}
	return core.Region{}, fmt.Errorf("lift: section %q not found", section)
}

// FilterCrossFunctionDiagnostics drops diagnostics that only reflect decoding
// several functions in one pass.
func FilterCrossFunctionDiagnostics(diagnostics []core.Diagnostic) []core.Diagnostic {
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

func countSourceFunctions(ir *core.LowIR) int {
	if ir == nil {
		return 0
	}
	total := 0
	for _, function := range ir.Functions {
		if function.Boundary == core.BoundaryPLT {
			continue
		}
		total++
	}
	return total
}

func countBlocks(ir *core.LowIR) int {
	total := 0
	for _, function := range ir.Functions {
		total += len(function.Blocks)
	}
	return total
}

func countInstructions(ir *core.LowIR) int {
	total := 0
	for _, function := range ir.Functions {
		for _, block := range function.Blocks {
			total += len(block.Instructions)
		}
	}
	return total
}

func countValues(ir *core.LowIR) int {
	total := 0
	for _, function := range ir.Functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instructions {
				total += len(instruction.Results)
			}
		}
	}
	return total
}
