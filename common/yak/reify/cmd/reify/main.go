package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"text/tabwriter"

	utilscli "github.com/yaklang/yaklang/common/utils/cli"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowir2ssa"
	"github.com/yaklang/yaklang/common/yak/reify/core/ssa2source"
	"github.com/yaklang/yaklang/common/yak/reify/loader"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

const usage = `Reify - executable loading and program representation

Usage:
  reify loaders [--json]
  reify parsers [--json]
  reify inspect [--json] <file>
  reify check [--json] <file>
  reify lift [--json] [--keep-stack-canary] [--mode asm|lowir|ssair|source] [--section NAME | --address ADDRESS --size SIZE] <file>
  reify help

Commands:
  loaders  List loaders registered by file format.
  parsers  List parsers registered by architecture.
  inspect  Load headers and display the artifact, target and sections.
  check    Validate headers and report the selected loader and parser.
  lift     Decode a code region; --mode selects asm, lowir, ssair or source.

Scope: file headers, target platforms, section metadata, AMD64 assembly,
LowIR, YakSSA and source-like text.
Loaders: ELF, PE and thin Mach-O. AMD64 has an instruction decoder; the other
architectures are registered without decoders. Flags may appear before or after
the file argument. Files are opened read-only. debug/* parsers are not a sandbox.
`

type inspectionReport struct {
	Scope    string           `json:"scope"`
	Artifact *core.Artifact   `json:"artifact"`
	Parser   *core.ParserInfo `json:"parser,omitempty"`
}

type checkReport struct {
	Scope string              `json:"scope"`
	Check *loader.CheckResult `json:"check"`
}

type liftReport struct {
	Scope        string            `json:"scope"`
	Mode         string            `json:"mode"`
	Region       core.Region       `json:"region"`
	Assembly     string            `json:"assembly,omitempty"`
	Instructions int               `json:"instructions,omitempty"`
	IR           *core.LowIR       `json:"ir,omitempty"`
	SSAIR        string            `json:"ssair,omitempty"`
	Source       string            `json:"source,omitempty"`
	Diagnostics  []core.Diagnostic `json:"diagnostics,omitempty"`
}

type liftMode string

const (
	liftModeASM    liftMode = "asm"
	liftModeLowIR  liftMode = "lowir"
	liftModeSSAIR  liftMode = "ssair"
	liftModeSource liftMode = "source"
)

func parseLiftMode(raw string) (liftMode, error) {
	switch liftMode(raw) {
	case liftModeASM, liftModeLowIR, liftModeSSAIR, liftModeSource:
		return liftMode(raw), nil
	default:
		return "", fmt.Errorf("unknown lift mode %q (want asm, lowir, ssair or source)", raw)
	}
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || (len(args) == 1 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h")) {
		_, err := io.WriteString(stdout, usage)
		return reportError(err, stderr)
	}
	command := args[0]
	switch command {
	case "loaders", "parsers", "inspect", "check", "lift":
	default:
		fmt.Fprintf(stderr, "unknown command %q\n%s", command, usage)
		return 2
	}
	if hasHelpFlag(args[1:]) {
		printCommandUsage(stdout, command)
		return 0
	}

	valueFlags := map[string]bool{}
	if command == "lift" {
		valueFlags["mode"] = true
		valueFlags["section"] = true
		valueFlags["address"] = true
		valueFlags["size"] = true
	}
	positional, err := splitCLIArgs(args[1:], valueFlags)
	if err != nil {
		fmt.Fprintf(stderr, "reify: %v\n", err)
		printCommandUsage(stderr, command)
		return 2
	}
	expectedArgs := 1
	if command == "loaders" || command == "parsers" {
		expectedArgs = 0
	}
	if len(positional) != expectedArgs {
		printCommandUsage(stderr, command)
		return 2
	}

	app := utilscli.NewCliApp()
	app.SetArgs(args[1:])
	asJSON := app.Bool("json")
	section := ".text"
	mode := liftModeASM
	keepStackCanary := false
	var addressText, sizeText string
	if command == "lift" {
		rawMode := app.String("mode", app.SetDefault("asm"))
		parsedMode, modeErr := parseLiftMode(rawMode)
		if modeErr != nil {
			fmt.Fprintf(stderr, "reify: %v\n", modeErr)
			printCommandUsage(stderr, command)
			return 2
		}
		mode = parsedMode
		section = app.String("section", app.SetDefault(".text"))
		addressText = app.String("address")
		sizeText = app.String("size")
		keepStackCanary = app.Bool("keep-stack-canary")
	}
	address, size := uint64(0), uint64(0)
	if addressText != "" {
		parsed, parseErr := strconv.ParseUint(addressText, 0, 64)
		if parseErr != nil {
			fmt.Fprintf(stderr, "reify: invalid --address %q\n", addressText)
			return 2
		}
		address = parsed
	}
	if sizeText != "" {
		parsed, parseErr := strconv.ParseUint(sizeText, 0, 64)
		if parseErr != nil {
			fmt.Fprintf(stderr, "reify: invalid --size %q\n", sizeText)
			return 2
		}
		size = parsed
	}

	catalog := loader.Default
	switch command {
	case "loaders":
		if asJSON {
			return reportError(writeJSON(stdout, catalog.Loaders()), stderr)
		}
		var text bytes.Buffer
		w := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "KIND\tNAME\tDESCRIPTION")
		for _, info := range catalog.Loaders() {
			fmt.Fprintf(w, "%s\t%s\t%s\n", info.Kind, info.Name, info.Description)
		}
		_ = w.Flush()
		_, err := io.Copy(stdout, &text)
		return reportError(err, stderr)
	case "parsers":
		if asJSON {
			return reportError(writeJSON(stdout, catalog.Parsers()), stderr)
		}
		var text bytes.Buffer
		w := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "KIND\tNAME\tDECODER")
		for _, info := range catalog.Parsers() {
			fmt.Fprintf(w, "%s\t%s\t%s\n", info.Kind, info.Name, decoderState(info.Available))
		}
		_ = w.Flush()
		_, err := io.Copy(stdout, &text)
		return reportError(err, stderr)
	}

	if command == "lift" {
		artifact, err := catalog.OpenFile(ctx, positional[0])
		if err != nil {
			return reportError(err, stderr)
		}
		defer func() { _ = artifact.Close() }()
		region, err := liftRegion(artifact, section, address, size)
		if err != nil {
			return reportError(err, stderr)
		}
		switch mode {
		case liftModeASM:
			disassembly, err := artifact.Disassemble(ctx, region)
			if err != nil {
				return reportError(err, stderr)
			}
			if asJSON {
				return reportError(writeJSON(stdout, liftReport{
					Scope:        "amd64-assembly",
					Mode:         string(mode),
					Region:       region,
					Assembly:     disassembly.Text,
					Instructions: disassembly.Instructions,
					Diagnostics:  disassembly.Diagnostics,
				}), stderr)
			}
			var text bytes.Buffer
			fmt.Fprintf(&text, "Scope: amd64 assembly\n")
			fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", region.Address, region.Size)
			fmt.Fprintf(&text, "Instructions: %d\n", disassembly.Instructions)
			fmt.Fprintf(&text, "Diagnostics: %d\n", len(disassembly.Diagnostics))
			text.WriteString(disassembly.Text)
			for _, diagnostic := range disassembly.Diagnostics {
				fmt.Fprintf(&text, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
			}
			_, err = io.Copy(stdout, &text)
			return reportError(err, stderr)
		case liftModeLowIR:
			result, err := artifact.Parse(ctx, region)
			if err != nil {
				return reportError(err, stderr)
			}
			if asJSON {
				return reportError(writeJSON(stdout, liftReport{
					Scope:       "amd64-lowir",
					Mode:        string(mode),
					Region:      region,
					IR:          result.IR,
					Diagnostics: result.Diagnostics,
				}), stderr)
			}
			var text bytes.Buffer
			fmt.Fprintf(&text, "Scope: amd64 LowIR\n")
			fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", region.Address, region.Size)
			fmt.Fprintf(&text, "Functions: %d\n", len(result.IR.Functions))
			fmt.Fprintf(&text, "Blocks: %d\n", countBlocks(result.IR))
			fmt.Fprintf(&text, "Instructions: %d\n", countInstructions(result.IR))
			fmt.Fprintf(&text, "Values: %d\n", countValues(result.IR))
			fmt.Fprintf(&text, "Diagnostics: %d\n", len(result.Diagnostics))
			text.WriteString(core.FormatLowIR(result.IR))
			for _, diagnostic := range result.Diagnostics {
				fmt.Fprintf(&text, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
			}
			_, err = io.Copy(stdout, &text)
			return reportError(err, stderr)
		case liftModeSSAIR, liftModeSource:
			result, err := artifact.Parse(ctx, region)
			if err != nil {
				return reportError(err, stderr)
			}
			var convertOptions []lowir2ssa.Option
			if keepStackCanary {
				convertOptions = append(convertOptions, lowir2ssa.WithKeepStackCanary())
			}
			converted, err := lowir2ssa.Convert(ctx, result.IR, convertOptions...)
			if err != nil {
				return reportError(err, stderr)
			}
			diagnostics := append([]core.Diagnostic{}, result.Diagnostics...)
			diagnostics = append(diagnostics, converted.Diagnostics...)
			var body string
			if mode == liftModeSSAIR {
				body = lowir2ssa.Text(converted.Program)
			} else {
				body, err = ssa2source.Render(ctx, converted.Program)
				if err != nil {
					return reportError(err, stderr)
				}
			}
			if asJSON {
				report := liftReport{
					Scope:       "amd64-" + string(mode),
					Mode:        string(mode),
					Region:      region,
					IR:          result.IR,
					Diagnostics: diagnostics,
				}
				if mode == liftModeSSAIR {
					report.SSAIR = body
				} else {
					report.Source = body
				}
				return reportError(writeJSON(stdout, report), stderr)
			}
			var text bytes.Buffer
			if mode == liftModeSSAIR {
				fmt.Fprintf(&text, "Scope: amd64 SSair\n")
			} else {
				fmt.Fprintf(&text, "Scope: amd64 source\n")
			}
			fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", region.Address, region.Size)
			fmt.Fprintf(&text, "Functions: %d\n", len(result.IR.Functions))
			fmt.Fprintf(&text, "Diagnostics: %d\n", len(diagnostics))
			text.WriteString(body)
			for _, diagnostic := range diagnostics {
				fmt.Fprintf(&text, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
			}
			_, err = io.Copy(stdout, &text)
			return reportError(err, stderr)
		}
	}

	if command == "check" {
		result, err := catalog.CheckFile(ctx, positional[0])
		if err != nil {
			return reportError(err, stderr)
		}
		if asJSON {
			return reportError(writeJSON(stdout, checkReport{Scope: "metadata-only", Check: result}), stderr)
		}
		var text bytes.Buffer
		fmt.Fprintf(&text, "Header check passed: %q (%s)\n", result.Path, result.Format)
		fmt.Fprintf(&text, "Loader: %s\n", result.Loader.Name)
		fmt.Fprintf(&text, "Target: %s\n", targetSummary(result.Target))
		fmt.Fprintf(&text, "Parser: %s\n", parserSummary(result.Parser))
		fmt.Fprintf(&text, "Sections: %d\n", result.Sections)
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(&text, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
		}
		fmt.Fprintln(&text, "No instructions or SSA were checked.")
		_, err = io.Copy(stdout, &text)
		return reportError(err, stderr)
	}

	artifact, err := catalog.OpenFile(ctx, positional[0])
	if err != nil {
		return reportError(err, stderr)
	}
	defer func() { _ = artifact.Close() }()
	parser := artifactParser(artifact)
	if asJSON {
		return reportError(writeJSON(stdout, inspectionReport{Scope: "metadata-only", Artifact: artifact, Parser: parser}), stderr)
	}
	var text bytes.Buffer
	fmt.Fprintln(&text, "Scope: metadata only; no disassembly or SSA")
	fmt.Fprintf(&text, "Input: %q\nSize: %d bytes\nLoader: %s\nFormat: %s\n", artifact.Origin.Name, artifact.Origin.Size, artifact.Loader.Name(), artifact.Format)
	fmt.Fprintf(&text, "Target: %s\n", targetSummary(artifact.Target))
	fmt.Fprintf(&text, "Parser: %s\n", parserSummary(parser))
	if image := artifact.Image; image != nil {
		fmt.Fprintf(&text, "Object: %s\n", image.ObjectType)
		if entry, present := image.EntryPoint(); present {
			fmt.Fprintf(&text, "Header entry: 0x%x\n", entry)
		} else {
			fmt.Fprintln(&text, "Header entry: not reported")
		}
		w := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "SECTION\tFILE OFFSET\tFILE SIZE\tVIRTUAL ADDRESS\tMEMORY SIZE")
		for _, section := range image.Sections {
			fmt.Fprintf(w, "%q\t0x%x\t0x%x\t0x%x\t0x%x\n", section.Name, section.FileOffset, section.FileSize, section.VirtualAddress, section.MemorySize)
		}
		_ = w.Flush()
	}
	for _, diagnostic := range artifact.Diagnostics {
		fmt.Fprintf(&text, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
	}
	_, err = io.Copy(stdout, &text)
	return reportError(err, stderr)
}

func hasHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func splitCLIArgs(args []string, valueFlags map[string]bool) ([]string, error) {
	var positional []string
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case arg == "--json", arg == "--keep-stack-canary":
			continue
		case strings.HasPrefix(arg, "--"):
			name := strings.TrimPrefix(arg, "--")
			if name == "" || strings.Contains(name, "=") {
				return nil, fmt.Errorf("invalid flag %q", arg)
			}
			if valueFlags[name] {
				if index+1 >= len(args) {
					return nil, fmt.Errorf("flag --%s requires a value", name)
				}
				index++
				continue
			}
			return nil, fmt.Errorf("unknown flag --%s", name)
		case strings.HasPrefix(arg, "-"):
			return nil, fmt.Errorf("unknown flag %s", arg)
		default:
			positional = append(positional, arg)
		}
	}
	return positional, nil
}

func printCommandUsage(w io.Writer, command string) {
	switch command {
	case "loaders", "parsers":
		fmt.Fprintf(w, "Usage: reify %s [--json]\n", command)
	case "inspect", "check":
		fmt.Fprintf(w, "Usage: reify %s [--json] <file>\n", command)
	case "lift":
		fmt.Fprint(w, "Usage: reify lift [--json] [--keep-stack-canary] [--mode asm|lowir|ssair|source] [--section NAME | --address ADDRESS --size SIZE] <file>\n")
	}
}

func artifactParser(artifact *core.Artifact) *core.ParserInfo {
	if artifact.Parser == nil {
		return nil
	}
	return &core.ParserInfo{Kind: artifact.Parser.Kind(), Name: artifact.Parser.Name(), Available: artifact.Parser.Available()}
}

func targetSummary(target core.Target) string {
	summary := fmt.Sprintf("%s/%s, %d-bit, %s-endian", target.OS, target.Architecture, target.Bits, target.ByteOrder)
	if target.ABI != "" {
		summary += ", abi " + target.ABI
	}
	if target.Machine != "" {
		summary += ", machine " + target.Machine
	}
	if target.Mode != "" {
		summary += ", mode " + target.Mode
	}
	return summary
}

func parserSummary(parser *core.ParserInfo) string {
	if parser == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", parser.Name, decoderState(parser.Available))
}

func decoderState(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func reportError(err error, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	fmt.Fprintf(stderr, "reify: %v\n", err)
	return 1
}

func liftRegion(artifact *core.Artifact, section string, address, size uint64) (core.Region, error) {
	if (address == 0) != (size == 0) {
		return core.Region{}, fmt.Errorf("lift: address and size must be provided together")
	}
	if address != 0 {
		return artifact.RegionAt(address, size)
	}
	if artifact.Image == nil {
		return core.Region{}, fmt.Errorf("lift: artifact has no image")
	}
	for _, candidate := range artifact.Image.Sections {
		if candidate.Name == section {
			if candidate.FileSize == 0 {
				return core.Region{}, fmt.Errorf("lift: section %q has no file data", section)
			}
			return artifact.RegionAt(candidate.VirtualAddress, candidate.FileSize)
		}
	}
	return core.Region{}, fmt.Errorf("lift: section %q not found", section)
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
