package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

func main() {
	// Keep CLI output quiet by default (more like `go build`). Users can override via LOG_LEVEL.
	if strings.TrimSpace(os.Getenv("LOG_LEVEL")) == "" {
		cfg := log.GetConfig().Clone()
		cfg.Level = "error"
		log.SetConfig(cfg)
	}

	app := cli.NewApp()
	app.Name = "ssa2llvm"
	app.Usage = "SSA to LLVM compiler - compile source code to native executables"
	app.Version = "1.0.0"

	app.Commands = []cli.Command{
		obfuscatorsCommand,
		compileCommand,
		runCommand,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type buildCommandConfig struct {
	sourceFile string
	language   string
	function   string
	printIR    bool
	obf        []string
	stdlibComp bool
	trace      bool
	force      bool
}

func sharedBuildFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "Output executable path (run keeps and executes it when set)",
		},
		cli.StringFlag{
			Name:  "language,l",
			Usage: "Source language (yak, go, python, javascript, java, php, c, typescript)",
		},
		cli.StringFlag{
			Name:  "function,f",
			Usage: "Entry function name",
			Value: "check",
		},
		cli.BoolFlag{
			Name:  "print-ir",
			Usage: "Print generated LLVM IR to stdout",
		},
		cli.StringSliceFlag{
			Name:  "obf",
			Usage: "Apply obfuscators by name or glob pattern (repeatable or comma-separated; see `ssa2llvm obfuscators`)",
			Value: &cli.StringSlice{},
		},
		cli.BoolFlag{
			Name:  "stdlib-compile",
			Usage: "Build libyak.a from embedded runtime source in the work dir and link against it (future: compile+obfuscate stdlib together with user code)",
		},
		cli.BoolFlag{
			Name:  "x",
			Usage: "Print the external commands as they are executed (like `go build -x`)",
		},
		cli.BoolFlag{
			Name:  "a",
			Usage: "Force rebuilding cached work directories (like `go build -a`)",
		},
	}
}

var compileCommand = cli.Command{
	Name:      "compile",
	Aliases:   []string{"c"},
	Usage:     "Compile source code to native executable (default) or LLVM IR",
	ArgsUsage: "<source-file>",
	Flags: append(sharedBuildFlags(),
		cli.BoolFlag{
			Name:  "emit-llvm,S",
			Usage: "Emit LLVM IR (.ll) instead of native binary",
		},
		cli.BoolFlag{
			Name:  "emit-asm,s",
			Usage: "Emit assembly (.s) instead of native binary",
		},
		cli.BoolFlag{
			Name:  "c",
			Usage: "Compile only (no linking), output object file",
		},
	),
	Action: compileAction,
}

var runCommand = cli.Command{
	Name:      "run",
	Aliases:   []string{"r"},
	Usage:     "Compile and run the executable (use -o to keep the binary)",
	ArgsUsage: "<source-file>",
	Flags:     sharedBuildFlags(),
	Action:    runAction,
}

var obfuscatorsCommand = cli.Command{
	Name:      "obfuscators",
	Aliases:   []string{"obf"},
	Usage:     "List registered SSA and LLVM obfuscators",
	UsageText: "ssa2llvm obfuscators",
	Description: `Show the obfuscators currently registered in ssa2llvm.

Use the printed names with --obf on the compile/run commands.
Names can be repeated, passed as comma-separated lists, or selected with glob patterns.`,
	Action: listObfuscatorsAction,
}

func compileAction(c *cli.Context) error {
	cfg, err := newBuildCommandConfig(c)
	if err != nil {
		return err
	}
	maybeEnableInfoLoggingForTrace(cfg.trace)

	emitLLVM := c.Bool("emit-llvm")
	emitAsm := c.Bool("emit-asm")
	compileOnly := c.Bool("c")
	finalOutput := strings.TrimSpace(c.String("output"))
	finalAuto := finalOutput == ""

	_, err = compiler.CompileToExecutable(
		compiler.WithCompileSourceFile(cfg.sourceFile),
		compiler.WithCompileLanguage(cfg.language),
		compiler.WithCompileEntryFunction(cfg.function),
		compiler.WithCompileEmitLLVM(emitLLVM),
		compiler.WithCompileEmitAsm(emitAsm),
		compiler.WithCompileOnly(compileOnly),
		compiler.WithCompilePrintIR(cfg.printIR),
		compiler.WithCompileObfuscators(cfg.obf...),
		compiler.WithCompileStdlibCompile(cfg.stdlibComp),
		compiler.WithCompileCacheEnabled(true),
		compiler.WithCompileTrace(cfg.trace),
		compiler.WithCompileForceRebuild(cfg.force),
		compiler.WithCompileFinalOutputFile(finalOutput),
		compiler.WithCompileFinalOutputAuto(finalAuto),
	)
	return err
}

func runAction(c *cli.Context) error {
	cfg, err := newBuildCommandConfig(c)
	if err != nil {
		return err
	}
	maybeEnableInfoLoggingForTrace(cfg.trace)

	finalOutput := strings.TrimSpace(c.String("output"))
	res, err := compiler.CompileToExecutable(
		compiler.WithCompileSourceFile(cfg.sourceFile),
		compiler.WithCompileLanguage(cfg.language),
		compiler.WithCompileEntryFunction(cfg.function),
		compiler.WithCompilePrintIR(cfg.printIR),
		compiler.WithCompileObfuscators(cfg.obf...),
		compiler.WithCompileStdlibCompile(cfg.stdlibComp),
		compiler.WithCompileCacheEnabled(true),
		compiler.WithCompileTrace(cfg.trace),
		compiler.WithCompileForceRebuild(cfg.force),
	)
	if err != nil {
		return err
	}

	runPath := res.Artifact
	if finalOutput != "" {
		if err := compiler.CopyFilePreserveMode(res.Artifact, finalOutput); err != nil {
			return err
		}
		runPath = finalOutput
	}

	execPath := runPath
	if !filepath.IsAbs(execPath) {
		abs, absErr := filepath.Abs(execPath)
		if absErr == nil {
			execPath = abs
		}
	}

	cmd := exec.Command(execPath)
	trace.PrintCmd(cmd)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return cli.NewExitError("", exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func listObfuscatorsAction(c *cli.Context) error {
	printObfuscatorGroup("SSA obfuscators", "--obf <name>", obfuscation.ListByKind(obfuscation.KindSSA))
	fmt.Println()
	printObfuscatorGroup("Hybrid obfuscators", "--obf <name>", obfuscation.ListByKind(obfuscation.KindHybrid))
	fmt.Println()
	printObfuscatorGroup("LLVM obfuscators", "--obf <name>", obfuscation.ListByKind(obfuscation.KindLLVM))
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ssa2llvm compile demo.yak --obf addsub")
	fmt.Println("  ssa2llvm run demo.yak --obf addsub")
	fmt.Println("  ssa2llvm compile demo.yak --obf xor")
	fmt.Println("  ssa2llvm run demo.yak --obf 'add*' --obf 'x*'")
	fmt.Println()
	fmt.Println("Names can be repeated, passed as comma-separated lists, or selected with glob patterns.")
	fmt.Println("Quote glob patterns like '*' to avoid shell expansion.")
	fmt.Println("Run `ssa2llvm compile --help` or `ssa2llvm run --help` for full flag details.")
	return nil
}

func newBuildCommandConfig(c *cli.Context) (*buildCommandConfig, error) {
	if c.NArg() < 1 {
		return nil, fmt.Errorf("missing source file argument")
	}

	return &buildCommandConfig{
		sourceFile: c.Args().First(),
		language:   c.String("language"),
		function:   c.String("function"),
		printIR:    c.Bool("print-ir"),
		obf:        c.StringSlice("obf"),
		stdlibComp: c.Bool("stdlib-compile"),
		trace:      c.Bool("x"),
		force:      c.Bool("a"),
	}, nil
}

func printObfuscatorGroup(title string, flagExample string, names []string) {
	fmt.Println(title + ":")
	if len(names) == 0 {
		fmt.Println("  (none registered)")
		return
	}
	for _, name := range names {
		fmt.Printf("  - %s\n", name)
	}
	fmt.Printf("  use with: %s\n", flagExample)
}

// maybeEnableInfoLoggingForTrace upgrades the global log level when -x is set
// and the user didn't explicitly choose a LOG_LEVEL. This makes hidden internal
// compile/link logs visible, similar to what users expect from a verbose build.
func maybeEnableInfoLoggingForTrace(traceEnabled bool) {
	if !traceEnabled {
		return
	}
	if strings.TrimSpace(os.Getenv("LOG_LEVEL")) != "" {
		return
	}
	cfg := log.GetConfig().Clone()
	cfg.Level = "info"
	log.SetConfig(cfg)
}
