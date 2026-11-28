package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/yaklang/gorm"
	"github.com/yaklang/yaklang/common/consts"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/reify"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
)

type commandEnv struct {
	ctx    context.Context
	stdout io.Writer
	stderr io.Writer
}

func newCLIApp(env *commandEnv) *cli.App {
	app := cli.NewApp()
	app.Name = "reify"
	app.HelpName = "reify"
	app.Usage = "executable loading and program representation"
	app.Description = "Scope: file headers, target platforms, section metadata, AMD64 and ARM64 assembly, LowIR, YakSSA and source-like text.\nLoaders: ELF, PE and thin Mach-O. AMD64 and ARM64 have instruction decoders; other architectures are registered without decoders."
	app.Writer = env.stdout
	app.ErrWriter = env.stderr
	app.HideVersion = true
	app.ExitErrHandler = func(*cli.Context, error) {}
	app.Action = env.rootAction
	app.OnUsageError = usageErrorHandler
	app.Commands = []cli.Command{
		{
			Name:   "loaders",
			Usage:  "List loaders registered by file format",
			Flags:  []cli.Flag{jsonFlag()},
			Action: env.loaders,
		},
		{
			Name:   "parsers",
			Usage:  "List parsers registered by architecture",
			Flags:  []cli.Flag{jsonFlag()},
			Action: env.parsers,
		},
		{
			Name:      "inspect",
			Usage:     "Load headers and display the artifact, target and sections",
			ArgsUsage: "<file>",
			Flags:     outputFlags(),
			Action:    env.inspect,
		},
		{
			Name:      "check",
			Usage:     "Validate headers and report the selected loader and parser",
			ArgsUsage: "<file>",
			Flags:     outputFlags(),
			Action:    env.check,
		},
		{
			Name:      "strings",
			Usage:     "Decode one string with the detected language model",
			ArgsUsage: "<file>",
			Flags: []cli.Flag{
				jsonFlag(),
				cli.StringFlag{Name: "at", Usage: "Address or symbol of the bytes (C) or descriptor (Go/Rust)"},
				formatFlag(),
			},
			Action: env.strings,
		},
		{
			Name:      "lift",
			Usage:     "Decode a code region; --mode selects asm, lowir, ssair, source or all",
			ArgsUsage: "<file>",
			Flags: []cli.Flag{
				jsonFlag(),
				cli.StringFlag{Name: "asm-syntax", Value: "intel", Usage: "Assembly syntax: intel or att"},
				cli.StringFlag{Name: "func", Usage: "Select a function by symbol name"},
				cli.StringFlag{Name: "mode", Value: "asm", Usage: "Output mode: asm, lowir, ssair, source or all"},
				cli.StringFlag{Name: "section", Value: ".text", Usage: "Section to lift when no address is given"},
				cli.StringFlag{Name: "address", Usage: "Start virtual address or symbol name"},
				cli.StringFlag{Name: "size", Usage: "Region size in bytes"},
				cli.StringFlag{Name: "lowir-pass", Usage: "Comma-separated LowIR pass names"},
				cli.StringFlag{Name: "ssa-pass", Usage: "Comma-separated SSA pass names"},
				formatFlag(),
			},
			Action: env.lift,
		},
		{
			Name:      "save",
			Usage:     "Lift a file to SSA and save the program to ssadb",
			ArgsUsage: "<file>",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "name", Usage: "ssadb program name (required)"},
				cli.BoolFlag{Name: "replace", Usage: "Replace an existing program after the new program is fully built"},
				cli.StringFlag{Name: "asm-syntax", Value: "intel", Usage: "Assembly syntax: intel or att"},
				cli.StringFlag{Name: "func", Usage: "Select a function by symbol name"},
				cli.StringFlag{Name: "section", Usage: "Section to lift when no address is given; empty lifts every region"},
				cli.StringFlag{Name: "address", Usage: "Start virtual address or symbol name"},
				cli.StringFlag{Name: "size", Usage: "Region size in bytes"},
				cli.StringFlag{Name: "lowir-pass", Usage: "Comma-separated LowIR pass names"},
				cli.StringFlag{Name: "ssa-pass", Usage: "Comma-separated SSA pass names"},
				formatFlag(),
			},
			Action: env.save,
		},
		{
			Name:      "audit",
			Usage:     "Run a syntaxflow rule against a lifted file or a saved program",
			ArgsUsage: "<file> <syntaxflow> | --program <name> <syntaxflow>",
			Flags: []cli.Flag{
				cli.StringFlag{Name: "program", Usage: "ssadb program name; omit it to audit a file"},
				cli.StringFlag{Name: "db", Usage: "SSA database path for --program; empty uses the default YAKIT_HOME database"},
				cli.StringFlag{Name: "rule", Usage: "SyntaxFlow rule text; alternatively pass it as the last argument"},
				cli.StringFlag{Name: "asm-syntax", Value: "intel", Usage: "Assembly syntax: intel or att"},
				cli.StringFlag{Name: "func", Usage: "Select a function by symbol name"},
				cli.StringFlag{Name: "section", Usage: "Section to lift when no address is given; empty lifts every region"},
				cli.StringFlag{Name: "address", Usage: "Start virtual address or symbol name"},
				cli.StringFlag{Name: "size", Usage: "Region size in bytes"},
				cli.StringFlag{Name: "lowir-pass", Usage: "Comma-separated LowIR pass names"},
				cli.StringFlag{Name: "ssa-pass", Usage: "Comma-separated SSA pass names"},
				formatFlag(),
				jsonFlag(),
			},
			Action: env.audit,
		},
	}
	for index := range app.Commands {
		app.Commands[index].OnUsageError = usageErrorHandler
	}
	return app
}

// formatFlag forces one loader. A COM image has no header magic, so the format
// cannot be detected and the caller must name it.
func formatFlag() cli.Flag {
	return cli.StringFlag{Name: "format", Usage: "Force one loader by format name (elf, pe, macho, coff, archive, mz)"}
}

func jsonFlag() cli.Flag {
	return cli.BoolFlag{Name: "json", Usage: "Emit JSON"}
}

// outputFlags are the flags every command that reports an artifact accepts.
func outputFlags() []cli.Flag {
	return []cli.Flag{jsonFlag(), formatFlag()}
}

func usageErrorHandler(_ *cli.Context, err error, _ bool) error {
	return cli.NewExitError(err, 2)
}

func (e *commandEnv) rootAction(c *cli.Context) error {
	if c.Args().Present() {
		return cli.NewExitError(fmt.Sprintf("unknown command %q", c.Args().First()), 2)
	}
	return cli.ShowAppHelp(c)
}

func (e *commandEnv) loaders(c *cli.Context) error {
	if c.NArg() != 0 {
		return cli.NewExitError("loaders takes no file arguments", 2)
	}
	infos := reify.Loaders()
	if c.Bool("json") {
		return writeJSON(e.stdout, infos)
	}
	var text bytes.Buffer
	w := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tNAME\tDESCRIPTION")
	for _, info := range infos {
		fmt.Fprintf(w, "%s\t%s\t%s\n", info.Kind, info.Name, info.Description)
	}
	_ = w.Flush()
	_, err := io.Copy(e.stdout, &text)
	return err
}

func (e *commandEnv) parsers(c *cli.Context) error {
	if c.NArg() != 0 {
		return cli.NewExitError("parsers takes no file arguments", 2)
	}
	infos := reify.Parsers()
	if c.Bool("json") {
		return writeJSON(e.stdout, infos)
	}
	var text bytes.Buffer
	w := tabwriter.NewWriter(&text, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "KIND\tNAME\tDECODER")
	for _, info := range infos {
		fmt.Fprintf(w, "%s\t%s\t%s\n", info.Kind, info.Name, decoderState(info.Available))
	}
	_ = w.Flush()
	_, err := io.Copy(e.stdout, &text)
	return err
}

func (e *commandEnv) inspect(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("inspect requires exactly one file", 2)
	}
	var options []reify.Option
	if c.IsSet("format") {
		options = append(options, reify.WithFormat(c.String("format")))
	}
	artifact, parser, err := reify.Inspect(e.ctx, c.Args().First(), options...)
	if err != nil {
		return err
	}
	defer func() { _ = artifact.Close() }()
	if c.Bool("json") {
		return writeInspectionJSON(e.stdout, inspectionReport{Scope: "metadata-only", Artifact: artifact, Parser: parser})
	}
	var text bytes.Buffer
	fmt.Fprintln(&text, "Scope: metadata only; no disassembly or SSA")
	fmt.Fprintf(&text, "Input: %q\nSize: %d bytes\nLoader: %s\nFormat: %s\n", artifact.Origin.Name, artifact.Origin.Size, artifact.Loader.Name(), artifact.Format)
	fmt.Fprintf(&text, "Target: %s\n", targetSummary(artifact.Target))
	fmt.Fprintf(&text, "Parser: %s\n", parserSummary(parser))
	reify.WriteLanguage(&text, artifact.Language)
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
	_, err = io.Copy(e.stdout, &text)
	return err
}

func (e *commandEnv) strings(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("strings requires exactly one file", 2)
	}
	if !c.IsSet("at") {
		return cli.NewExitError("strings requires --at", 2)
	}
	var options []reify.Option
	if c.IsSet("format") {
		options = append(options, reify.WithFormat(c.String("format")))
	}
	artifact, _, err := reify.Inspect(e.ctx, c.Args().First(), options...)
	if err != nil {
		return err
	}
	defer func() { _ = artifact.Close() }()
	if artifact.Language == nil || artifact.Language.Strings == nil {
		return cli.NewExitError("strings: this artifact has no string model", 1)
	}
	address, err := stringAddress(artifact, c.String("at"))
	if err != nil {
		return err
	}
	value, err := artifact.Language.Strings.Decode(artifact, address)
	if err != nil {
		return err
	}
	if c.Bool("json") {
		return writeJSON(e.stdout, stringReport{Scope: "one string", Address: address,
			Layout: artifact.Language.Strings.Layout, Encoding: artifact.Language.Strings.Encoding, Value: value})
	}
	var text bytes.Buffer
	fmt.Fprintf(&text, "Scope: one string\n")
	fmt.Fprintf(&text, "Input: %q\n", artifact.Origin.Name)
	reify.WriteLanguage(&text, artifact.Language)
	fmt.Fprintf(&text, "Address: 0x%x\n", address)
	fmt.Fprintf(&text, "Value: %q\n", value)
	_, err = io.Copy(e.stdout, &text)
	return err
}

type stringReport struct {
	Scope    string              `json:"scope"`
	Address  uint64              `json:"address"`
	Layout   core.StringLayout   `json:"layout"`
	Encoding core.StringEncoding `json:"encoding"`
	Value    string              `json:"value"`
}

func stringAddress(artifact *core.Artifact, text string) (uint64, error) {
	if value, err := parseAddress(text); err == nil {
		return value, nil
	}
	if artifact.Image != nil {
		if entry, ok := artifact.Image.SymbolByName(text); ok && !entry.Imported {
			return entry.Address, nil
		}
	}
	return 0, cli.NewExitError(fmt.Sprintf("strings: unknown address %q", text), 2)
}

func parseAddress(text string) (uint64, error) {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
		return strconv.ParseUint(text[2:], 16, 64)
	}
	if value, err := strconv.ParseUint(text, 10, 64); err == nil {
		return value, nil
	}
	return strconv.ParseUint(text, 16, 64)
}

func (e *commandEnv) check(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("check requires exactly one file", 2)
	}
	var options []reify.Option
	if c.IsSet("format") {
		options = append(options, reify.WithFormat(c.String("format")))
	}
	result, err := reify.Check(e.ctx, c.Args().First(), options...)
	if err != nil {
		return err
	}
	if c.Bool("json") {
		return writeJSON(e.stdout, checkReport{Scope: "metadata-only", Check: result})
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
	_, err = io.Copy(e.stdout, &text)
	return err
}

type auditReport struct {
	Program string   `json:"program"`
	Rule    string   `json:"rule"`
	Result  string   `json:"result"`
	Errors  []string `json:"errors,omitempty"`
}

// save lifts a binary into YakSSA on the ssadb write cache and keeps the
// program under --name. It is the producer half of the audit loop; audit is the
// consumer half.
//
// The lift writes to a staging program name. Only after the lift and the cache
// flush succeed does the staging program replace --name in one transaction, so
// a bad flag, unreadable file or lift failure leaves the old program intact.
func (e *commandEnv) save(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("save requires exactly one file", 2)
	}
	name := strings.TrimSpace(c.String("name"))
	if name == "" {
		return cli.NewExitError("save requires --name", 2)
	}
	replace := c.Bool("replace")
	_, err := ssadb.GetProgram(name, "")
	switch {
	case err == nil:
		if !replace {
			return cli.NewExitError(fmt.Sprintf("program %q already exists (use --replace)", name), 2)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// A new name is fine. Finalize rechecks under the transaction.
	case err != nil:
		return fmt.Errorf("inspect program %q: %w", name, err)
	}
	options, err := liftOptions(c, reify.ModeSSAIR)
	if err != nil {
		return err
	}
	staged := "reify-staging-" + uuid.NewString()
	options = append(options, reify.WithProgramName(staged))
	result, err := reify.Save(e.ctx, c.Args().First(), staged, options...)
	if err != nil {
		return discardStagedProgram(nil, staged, err)
	}
	if err := ssadb.FinalizeStagedProgramChecked(ssadb.GetDB(), staged, name, replace); err != nil {
		return discardStagedProgram(nil, staged, err)
	}
	fmt.Fprintf(e.stdout, "Program: %s\n", name)
	fmt.Fprintf(e.stdout, "Functions: %d\n", result.Functions)
	fmt.Fprintf(e.stdout, "Blocks: %d\n", result.Blocks)
	fmt.Fprintf(e.stdout, "Values: %d\n", result.Values)
	fmt.Fprintf(e.stdout, "Query: reify audit --program %s '<syntaxflow>'\n", name)
	return nil
}

// discardStagedProgram closes a cache that never reached the final flush and
// removes the staging rows. A cleanup failure is reported together with the
// original error, because leaving either an open writer or unreachable rows is
// part of the failure the caller must see.
func discardStagedProgram(program *ssa.Program, staged string, cause error) error {
	if program != nil && program.Cache != nil && !program.Cache.IsClosed() {
		program.Cache.CloseWithoutSave()
	}
	err := ssadb.DeleteProgramChecked(ssadb.GetDB(), staged)
	if err != nil {
		return errors.Join(cause, fmt.Errorf("discard staging program %q: %w", staged, err))
	}
	return cause
}

// liftOptions collects the loader, region and pass flags shared by lift, save
// and the file form of audit. The caller selects the mode and, for save, the
// persistence options.
func liftOptions(c *cli.Context, mode reify.Mode) ([]reify.Option, error) {
	syntax, err := reify.ParseAsmSyntax(c.String("asm-syntax"))
	if err != nil {
		return nil, cli.NewExitError(err, 2)
	}
	address, size, function := c.String("address"), c.String("size"), c.String("func")
	if function != "" && (address != "" || size != "") {
		return nil, cli.NewExitError("--func cannot be combined with --address or --size", 2)
	}
	options := []reify.Option{
		reify.WithMode(mode),
		reify.WithAsmSyntax(syntax),
	}
	if c.IsSet("section") {
		options = append(options, reify.WithSection(c.String("section")))
	}
	if address != "" || size != "" {
		options = append(options, reify.WithAddress(address, size))
	}
	if function != "" {
		options = append(options, reify.WithFunction(function))
	}
	if c.IsSet("format") {
		options = append(options, reify.WithFormat(c.String("format")))
	}
	if passes := splitPassNames(c.String("lowir-pass")); len(passes) > 0 {
		options = append(options, reify.WithLowIRPasses(passes...))
	}
	if passes := splitPassNames(c.String("ssa-pass")); len(passes) > 0 {
		options = append(options, reify.WithSSAPasses(passes...))
	}
	return options, nil
}

// audit runs one syntaxflow rule over either a freshly lifted file or a
// program loaded from ssadb. The file form keeps the SSA in memory; the
// --program form reads the stored instructions and optionally opens a
// specific SSA database.
func (e *commandEnv) audit(c *cli.Context) error {
	name := strings.TrimSpace(c.String("program"))
	var (
		file string
		rule string
	)
	if name != "" {
		if c.NArg() > 1 {
			return cli.NewExitError("audit --program accepts one syntaxflow argument", 2)
		}
		if c.NArg() == 1 {
			rule = strings.TrimSpace(c.Args().Get(0))
		}
	} else {
		if c.NArg() < 1 || c.NArg() > 2 {
			return cli.NewExitError("audit requires a file and a syntaxflow rule", 2)
		}
		file = c.Args().First()
		if c.NArg() == 2 {
			rule = strings.TrimSpace(c.Args().Get(1))
		}
	}
	if rule != "" && strings.TrimSpace(c.String("rule")) != "" {
		return cli.NewExitError("use either an argument or --rule, not both", 2)
	}
	if rule == "" {
		rule = strings.TrimSpace(c.String("rule"))
	}
	if rule == "" {
		return cli.NewExitError("audit requires a syntaxflow rule", 2)
	}
	if name == "" && c.IsSet("db") {
		return cli.NewExitError("--db requires --program", 2)
	}

	var (
		program     *ssaapi.Program
		programName string
		err         error
	)
	if name != "" {
		if dbPath := strings.TrimSpace(c.String("db")); dbPath != "" {
			database, openErr := consts.CreateSSAProjectDatabase(consts.SQLiteExtend, dbPath)
			if openErr != nil {
				return openErr
			}
			ssadb.SetDB(database)
		}
		program, err = ssaapi.FromDatabase(name)
		if err != nil {
			return err
		}
		programName = name
	} else {
		options, optionErr := liftOptions(c, reify.ModeSSAIR)
		if optionErr != nil {
			return optionErr
		}
		options = append(options, reify.WithDiscardIR(), reify.WithSkipText())
		lifted, liftErr := reify.Lift(e.ctx, file, options...)
		if liftErr != nil {
			return liftErr
		}
		defer func() { _ = lifted.Close() }()
		if lifted.Program == nil {
			return cli.NewExitError("lift produced no SSA program", 1)
		}
		lifted.Program.FlushInstruction()
		program = ssaapi.NewProgram(lifted.Program, nil)
		programName = lifted.ProgramName
	}

	result, err := program.SyntaxFlowWithError(rule)
	if err != nil {
		return err
	}
	report := auditReport{Program: programName, Rule: rule, Result: result.String(), Errors: result.GetErrors()}
	if c.Bool("json") {
		return writeJSON(e.stdout, report)
	}
	fmt.Fprintf(e.stdout, "Program: %s\n", report.Program)
	fmt.Fprint(e.stdout, report.Result)
	return nil
}

func (e *commandEnv) lift(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("lift requires exactly one file", 2)
	}
	mode, err := reify.ParseMode(c.String("mode"))
	if err != nil {
		return cli.NewExitError(err, 2)
	}
	options, err := liftOptions(c, mode)
	if err != nil {
		return err
	}
	if !c.Bool("json") && mode != reify.ModeAll {
		bw := bufio.NewWriterSize(e.stdout, 256*1024)
		streamOptions := append([]reify.Option(nil), options...)
		streamOptions = append(streamOptions, reify.WithBeforeStream(func(w io.Writer, header reify.StreamHeader) error {
			arch := string(header.Target.Architecture)
			switch header.Mode {
			case reify.ModeASM:
				fmt.Fprintf(w, "Scope: %s assembly\n", arch)
			case reify.ModeLowIR:
				fmt.Fprintf(w, "Scope: %s LowIR\n", arch)
			case reify.ModeSSAIR:
				fmt.Fprintf(w, "Scope: %s SSair\n", arch)
			case reify.ModeSource:
				fmt.Fprintf(w, "Scope: %s source\n", arch)
			}
			fmt.Fprintf(w, "Region: address=0x%x size=%d\n", header.Region.Address, header.Region.Size)
			if header.Mode != reify.ModeASM {
				reify.WriteBoundaryStats(w, header.BoundaryStats)
			}
			return nil
		}))
		streamed, err := reify.LiftStream(e.ctx, c.Args().First(), bw, streamOptions...)
		if err != nil {
			return err
		}
		switch streamed.Mode {
		case reify.ModeASM:
			fmt.Fprintf(bw, "Instructions: %d\n", streamed.Instructions)
			fmt.Fprintf(bw, "Diagnostics: %d\n", len(streamed.Diagnostics))
		case reify.ModeLowIR:
			fmt.Fprintf(bw, "Functions: %d\n", streamed.Functions)
			fmt.Fprintf(bw, "Blocks: %d\n", streamed.Blocks)
			fmt.Fprintf(bw, "Instructions: %d\n", streamed.Instructions)
			fmt.Fprintf(bw, "Values: %d\n", streamed.Values)
			fmt.Fprintf(bw, "Diagnostics: %d\n", len(streamed.Diagnostics))
		case reify.ModeSSAIR, reify.ModeSource:
			fmt.Fprintf(bw, "Functions: %d\n", streamed.Functions)
			fmt.Fprintf(bw, "Diagnostics: %d\n", len(streamed.Diagnostics))
		}
		reify.WriteLanguage(bw, streamed.Language)
		for _, diagnostic := range streamed.Diagnostics {
			fmt.Fprintf(bw, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
		}
		return bw.Flush()
	}

	result, err := reify.Lift(e.ctx, c.Args().First(), options...)
	if err != nil {
		return err
	}
	defer func() { _ = result.Close() }()

	report := liftReport{
		Scope:         scopeFor(result, mode),
		Mode:          string(mode),
		Region:        result.Region,
		Assembly:      result.Assembly,
		Instructions:  result.Instructions,
		IR:            result.IR,
		SSAIR:         result.SSAIR,
		Source:        result.Source,
		Diagnostics:   result.Diagnostics,
		BoundaryStats: result.BoundaryStats,
	}
	if c.Bool("json") {
		return writeJSON(e.stdout, report)
	}
	bw := bufio.NewWriterSize(e.stdout, 256*1024)
	if err := writeLiftText(bw, result); err != nil {
		return err
	}
	return bw.Flush()
}

func scopeFor(result *reify.Result, mode reify.Mode) string {
	arch := "unknown"
	if result != nil && result.Artifact != nil {
		arch = string(result.Artifact.Target.Architecture)
	}
	if suffix := modeScopeSuffix[mode]; suffix != "" {
		return arch + "-" + suffix
	}
	return arch + "-" + modeScopeSuffix[reify.ModeAll]
}

// modeScopeSuffix names each mode once, so the scope string has one source.
var modeScopeSuffix = map[reify.Mode]string{
	reify.ModeASM:    "assembly",
	reify.ModeLowIR:  "lowir",
	reify.ModeSSAIR:  "ssair",
	reify.ModeSource: "source",
	reify.ModeAll:    "all",
}

func writeLiftText(w io.Writer, result *reify.Result) error {
	arch := string(result.Artifact.Target.Architecture)
	switch result.Mode {
	case reify.ModeASM:
		fmt.Fprintf(w, "Scope: %s assembly\n", arch)
		fmt.Fprintf(w, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(w, "Instructions: %d\n", result.Instructions)
		fmt.Fprintf(w, "Diagnostics: %d\n", len(result.Diagnostics))
		if _, err := io.WriteString(w, result.Assembly); err != nil {
			return err
		}
	case reify.ModeLowIR:
		fmt.Fprintf(w, "Scope: %s LowIR\n", arch)
		fmt.Fprintf(w, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(w, "Functions: %d\n", result.Functions)
		fmt.Fprintf(w, "Blocks: %d\n", result.Blocks)
		fmt.Fprintf(w, "Instructions: %d\n", result.Instructions)
		fmt.Fprintf(w, "Values: %d\n", result.Values)
		fmt.Fprintf(w, "Diagnostics: %d\n", len(result.Diagnostics))
		reify.WriteBoundaryStats(w, result.BoundaryStats)
		if err := lowirtext.FormatTo(w, result.IR); err != nil {
			return err
		}
	case reify.ModeSSAIR, reify.ModeSource:
		if result.Mode == reify.ModeSSAIR {
			fmt.Fprintf(w, "Scope: %s SSair\n", arch)
		} else {
			fmt.Fprintf(w, "Scope: %s source\n", arch)
		}
		fmt.Fprintf(w, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(w, "Functions: %d\n", result.Functions)
		fmt.Fprintf(w, "Diagnostics: %d\n", len(result.Diagnostics))
		reify.WriteBoundaryStats(w, result.BoundaryStats)
		body := result.Source
		if result.Mode == reify.ModeSSAIR {
			body = result.SSAIR
		}
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
	default:
		fmt.Fprintf(w, "Scope: %s all\n", arch)
		fmt.Fprintf(w, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(w, "Functions: %d\n", result.Functions)
		fmt.Fprintf(w, "Diagnostics: %d\n", len(result.Diagnostics))
		reify.WriteBoundaryStats(w, result.BoundaryStats)
		fmt.Fprintf(w, "===== 1asm =====\n%s", result.Assembly)
		if result.Assembly != "" && !strings.HasSuffix(result.Assembly, "\n") {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(w, "===== 2lowir =====\n"); err != nil {
			return err
		}
		if err := lowirtext.FormatTo(w, result.IR); err != nil {
			return err
		}
		fmt.Fprintf(w, "===== 3ssair =====\n%s", result.SSAIR)
		fmt.Fprintf(w, "===== 4source =====\n%s", result.Source)
	}
	reify.WriteLanguage(w, result.Artifact.Language)
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(w, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
	}
	return nil
}
