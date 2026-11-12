package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/reify"
	"github.com/yaklang/yaklang/common/yak/reify/core"
	"github.com/yaklang/yaklang/common/yak/reify/core/lowirtext"
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
	app.Description = "Scope: file headers, target platforms, section metadata, AMD64 assembly, LowIR, YakSSA and source-like text.\nLoaders: ELF, PE and thin Mach-O. AMD64 has an instruction decoder; other architectures are registered without decoders."
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
			Flags:     []cli.Flag{jsonFlag()},
			Action:    env.inspect,
		},
		{
			Name:      "check",
			Usage:     "Validate headers and report the selected loader and parser",
			ArgsUsage: "<file>",
			Flags:     []cli.Flag{jsonFlag()},
			Action:    env.check,
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
			},
			Action: env.lift,
		},
	}
	for index := range app.Commands {
		app.Commands[index].OnUsageError = usageErrorHandler
	}
	return app
}

func jsonFlag() cli.Flag {
	return cli.BoolFlag{Name: "json", Usage: "Emit JSON"}
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
	artifact, parser, err := reify.Inspect(e.ctx, c.Args().First())
	if err != nil {
		return err
	}
	defer func() { _ = artifact.Close() }()
	if c.Bool("json") {
		return writeJSON(e.stdout, inspectionReport{Scope: "metadata-only", Artifact: artifact, Parser: parser})
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
	_, err = io.Copy(e.stdout, &text)
	return err
}

func (e *commandEnv) check(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("check requires exactly one file", 2)
	}
	result, err := reify.Check(e.ctx, c.Args().First())
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

func (e *commandEnv) lift(c *cli.Context) error {
	if c.NArg() != 1 {
		return cli.NewExitError("lift requires exactly one file", 2)
	}
	mode, err := reify.ParseMode(c.String("mode"))
	if err != nil {
		return cli.NewExitError(err, 2)
	}
	syntax, err := reify.ParseAsmSyntax(c.String("asm-syntax"))
	if err != nil {
		return cli.NewExitError(err, 2)
	}
	address, size, function := c.String("address"), c.String("size"), c.String("func")
	if function != "" && (address != "" || size != "") {
		return cli.NewExitError("--func cannot be combined with --address or --size", 2)
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
	if passes := splitPassNames(c.String("lowir-pass")); len(passes) > 0 {
		options = append(options, reify.WithLowIRPasses(passes...))
	}
	if passes := splitPassNames(c.String("ssa-pass")); len(passes) > 0 {
		options = append(options, reify.WithSSAPasses(passes...))
	}
	result, err := reify.Lift(e.ctx, c.Args().First(), options...)
	if err != nil {
		return err
	}
	defer func() { _ = result.Close() }()

	report := liftReport{
		Scope:         scopeFor(mode),
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
	_, err = io.Copy(e.stdout, liftText(result))
	return err
}

func scopeFor(mode reify.Mode) string {
	switch mode {
	case reify.ModeASM:
		return "amd64-assembly"
	case reify.ModeLowIR:
		return "amd64-lowir"
	case reify.ModeSSAIR:
		return "amd64-ssair"
	case reify.ModeSource:
		return "amd64-source"
	default:
		return "amd64-all"
	}
}

func liftText(result *reify.Result) io.Reader {
	var text bytes.Buffer
	switch result.Mode {
	case reify.ModeASM:
		fmt.Fprintf(&text, "Scope: amd64 assembly\n")
		fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(&text, "Instructions: %d\n", result.Instructions)
		fmt.Fprintf(&text, "Diagnostics: %d\n", len(result.Diagnostics))
		text.WriteString(result.Assembly)
	case reify.ModeLowIR:
		fmt.Fprintf(&text, "Scope: amd64 LowIR\n")
		fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(&text, "Functions: %d\n", result.Functions)
		fmt.Fprintf(&text, "Blocks: %d\n", result.Blocks)
		fmt.Fprintf(&text, "Instructions: %d\n", result.Instructions)
		fmt.Fprintf(&text, "Values: %d\n", result.Values)
		fmt.Fprintf(&text, "Diagnostics: %d\n", len(result.Diagnostics))
		writeBoundaryStats(&text, result.BoundaryStats)
		text.WriteString(lowirtext.Format(result.IR))
	case reify.ModeSSAIR, reify.ModeSource:
		if result.Mode == reify.ModeSSAIR {
			fmt.Fprintf(&text, "Scope: amd64 SSair\n")
		} else {
			fmt.Fprintf(&text, "Scope: amd64 source\n")
		}
		fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(&text, "Functions: %d\n", result.Functions)
		fmt.Fprintf(&text, "Diagnostics: %d\n", len(result.Diagnostics))
		writeBoundaryStats(&text, result.BoundaryStats)
		if result.Mode == reify.ModeSSAIR {
			text.WriteString(result.SSAIR)
		} else {
			text.WriteString(result.Source)
		}
	default:
		fmt.Fprintf(&text, "Scope: amd64 all\n")
		fmt.Fprintf(&text, "Region: address=0x%x size=%d\n", result.Region.Address, result.Region.Size)
		fmt.Fprintf(&text, "Functions: %d\n", result.Functions)
		fmt.Fprintf(&text, "Diagnostics: %d\n", len(result.Diagnostics))
		writeBoundaryStats(&text, result.BoundaryStats)
		fmt.Fprintf(&text, "===== 1asm =====\n%s", result.Assembly)
		if result.Assembly != "" && !bytes.HasSuffix([]byte(result.Assembly), []byte("\n")) {
			text.WriteByte('\n')
		}
		fmt.Fprintf(&text, "===== 2lowir =====\n%s", lowirtext.Format(result.IR))
		fmt.Fprintf(&text, "===== 3ssair =====\n%s", result.SSAIR)
		fmt.Fprintf(&text, "===== 4source =====\n%s", result.Source)
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(&text, "Note [%s]: %s\n", diagnostic.Code, diagnostic.Message)
	}
	return &text
}

func writeBoundaryStats(text *bytes.Buffer, stats *core.BoundaryStats) {
	if stats == nil {
		return
	}
	fmt.Fprintf(text, "Boundaries: symbol=%d eh_frame=%d init_array=%d plt=%d entry=%d call_target=%d scan=%d duplicates=%d\n",
		stats.Symbol, stats.EHFrame, stats.InitArray, stats.PLT, stats.Entry, stats.Call, stats.Scan, stats.Duplicates)
}
