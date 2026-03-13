package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/clibuild"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtimeembed"
)

func main() {
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
	outputFile string
	language   string
	function   string
	printIR    bool
	ssaObf     []string
	llvmObf    []string
	stdlibComp bool
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
			Name:  "ssa-obf",
			Usage: "Apply SSA obfuscators by name or glob pattern (repeatable or comma-separated; see `ssa2llvm obfuscators`)",
			Value: &cli.StringSlice{},
		},
		cli.StringSliceFlag{
			Name:  "llvm-obf",
			Usage: "Apply LLVM obfuscators by name or glob pattern (repeatable or comma-separated; see `ssa2llvm obfuscators`)",
			Value: &cli.StringSlice{},
		},
		cli.BoolFlag{
			Name:  "stdlib-compile",
			Usage: "Compile stdlib together with source and apply obfuscation (reserved; not implemented yet)",
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

Use the printed names with --ssa-obf or --llvm-obf on the compile/run commands.
Names can be repeated, passed as comma-separated lists, or selected with glob patterns.`,
	Action: listObfuscatorsAction,
}

func compileAction(c *cli.Context) error {
	cfg, err := newBuildCommandConfig(c)
	if err != nil {
		return err
	}

	buildDir, err := os.MkdirTemp("", "ssa2llvm-build-*")
	if err != nil {
		return fmt.Errorf("create temp build dir failed: %w", err)
	}
	defer os.RemoveAll(buildDir)

	finalOutput := strings.TrimSpace(c.String("output"))
	emitLLVM := c.Bool("emit-llvm")
	emitAsm := c.Bool("emit-asm")
	compileOnly := c.Bool("c")
	if finalOutput == "" {
		finalOutput = defaultCompileOutputPath(cfg.sourceFile, emitLLVM, emitAsm, compileOnly)
	}
	tempOutput := filepath.Join(buildDir, filepath.Base(finalOutput))

	runtimeArchive := ""
	extraLinkArgs := make([]string, 0, 1)
	if !emitLLVM && !emitAsm && !compileOnly {
		if cfg.stdlibComp {
			archivePath, gcLibDir, buildErr := clibuild.BuildRuntimeArchiveFromEmbeddedSource(buildDir)
			if buildErr != nil {
				return buildErr
			}
			runtimeArchive = archivePath
			if strings.TrimSpace(gcLibDir) != "" {
				extraLinkArgs = append(extraLinkArgs, "-L"+gcLibDir)
			}
		} else {
			archivePath, extractErr := runtimeembed.ExtractLibyakToDir(buildDir)
			if extractErr == nil {
				runtimeArchive = archivePath
			} else if extractErr != runtimeembed.ErrNoEmbeddedRuntime {
				return extractErr
			}

			if _, gcErr := runtimeembed.ExtractLibgcToDir(buildDir); gcErr == nil {
				extraLinkArgs = append(extraLinkArgs, "-L"+buildDir)
			} else if gcErr != runtimeembed.ErrNoEmbeddedRuntime {
				return gcErr
			}
		}
	}

	options := append(cfg.compileOptions(),
		compiler.WithCompileWorkDir(buildDir),
		compiler.WithCompileOutputFile(tempOutput),
		compiler.WithCompileStdlibCompile(cfg.stdlibComp),
		compiler.WithCompileExtraLinkArgs(extraLinkArgs...),
		compiler.WithCompileEmitLLVM(c.Bool("emit-llvm")),
		compiler.WithCompileEmitAsm(c.Bool("emit-asm")),
		compiler.WithCompileOnly(c.Bool("c")),
	)
	if runtimeArchive != "" {
		options = append(options, compiler.WithCompileRuntimeArchive(runtimeArchive))
	}
	if err := compiler.CompileToExecutable(options...); err != nil {
		return err
	}
	return moveFile(tempOutput, finalOutput)
}

func runAction(c *cli.Context) error {
	cfg, err := newBuildCommandConfig(c)
	if err != nil {
		return err
	}

	buildDir, err := os.MkdirTemp("", "ssa2llvm-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)

	finalOutput := strings.TrimSpace(c.String("output"))
	tempOutput := ""
	runPath := ""
	if finalOutput != "" {
		tempOutput = filepath.Join(buildDir, filepath.Base(finalOutput))
		runPath = finalOutput
	} else {
		tempOutput = filepath.Join(buildDir, defaultRunBinaryName())
		runPath = tempOutput
	}

	runtimeArchive := ""
	extraLinkArgs := make([]string, 0, 1)
	if cfg.stdlibComp {
		archivePath, gcLibDir, buildErr := clibuild.BuildRuntimeArchiveFromEmbeddedSource(buildDir)
		if buildErr != nil {
			return buildErr
		}
		runtimeArchive = archivePath
		if strings.TrimSpace(gcLibDir) != "" {
			extraLinkArgs = append(extraLinkArgs, "-L"+gcLibDir)
		}
	} else {
		archivePath, extractErr := runtimeembed.ExtractLibyakToDir(buildDir)
		if extractErr == nil {
			runtimeArchive = archivePath
		} else if extractErr != runtimeembed.ErrNoEmbeddedRuntime {
			return extractErr
		}

		if _, gcErr := runtimeembed.ExtractLibgcToDir(buildDir); gcErr == nil {
			extraLinkArgs = append(extraLinkArgs, "-L"+buildDir)
		} else if gcErr != runtimeembed.ErrNoEmbeddedRuntime {
			return gcErr
		}
	}

	options := append(cfg.compileOptions(),
		compiler.WithCompileWorkDir(buildDir),
		compiler.WithCompileOutputFile(tempOutput),
		compiler.WithCompileStdlibCompile(cfg.stdlibComp),
		compiler.WithCompileExtraLinkArgs(extraLinkArgs...),
	)
	if runtimeArchive != "" {
		options = append(options, compiler.WithCompileRuntimeArchive(runtimeArchive))
	}

	if err := compiler.CompileToExecutable(options...); err != nil {
		return err
	}

	if finalOutput != "" {
		if err := moveFile(tempOutput, finalOutput); err != nil {
			return err
		}
	}

	execPath := runPath
	if !filepath.IsAbs(execPath) {
		abs, absErr := filepath.Abs(execPath)
		if absErr == nil {
			execPath = abs
		}
	}

	cmd := exec.Command(execPath)
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
	printObfuscatorGroup("SSA obfuscators", "--ssa-obf <name>", obfuscation.ListSSA())
	fmt.Println()
	printObfuscatorGroup("LLVM obfuscators", "--llvm-obf <name>", obfuscation.ListLLVM())
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  ssa2llvm compile demo.yak --ssa-obf addsub")
	fmt.Println("  ssa2llvm run demo.yak --ssa-obf addsub")
	fmt.Println("  ssa2llvm compile demo.yak --ssa-obf addsub --llvm-obf xor")
	fmt.Println("  ssa2llvm run demo.yak --ssa-obf 'add*' --llvm-obf 'x*'")
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
		outputFile: strings.TrimSpace(c.String("output")),
		language:   c.String("language"),
		function:   c.String("function"),
		printIR:    c.Bool("print-ir"),
		ssaObf:     c.StringSlice("ssa-obf"),
		llvmObf:    c.StringSlice("llvm-obf"),
		stdlibComp: c.Bool("stdlib-compile"),
	}, nil
}

func (cfg *buildCommandConfig) compileOptions() []compiler.CompileOption {
	return []compiler.CompileOption{
		compiler.WithCompileSourceFile(cfg.sourceFile),
		compiler.WithCompileLanguage(cfg.language),
		compiler.WithCompileOutputFile(cfg.outputFile),
		compiler.WithCompileEntryFunction(cfg.function),
		compiler.WithCompilePrintIR(cfg.printIR),
		compiler.WithCompileSSAObfuscators(cfg.ssaObf...),
		compiler.WithCompileLLVMObfuscators(cfg.llvmObf...),
	}
}

func defaultRunBinaryName() string {
	if isWindows() {
		return "run.exe"
	}
	return "run.out"
}

func isWindows() bool {
	return filepath.Separator == '\\'
}

func defaultCompileOutputPath(sourceFile string, emitLLVM, emitAsm, compileOnly bool) string {
	switch {
	case emitLLVM:
		return replaceExt(sourceFile, ".ll")
	case emitAsm:
		return replaceExt(sourceFile, ".s")
	case compileOnly:
		return replaceExt(sourceFile, ".o")
	default:
		if isWindows() {
			return "a.exe"
		}
		return "a.out"
	}
}

func replaceExt(filename, newExt string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	return base + newExt
}

func moveFile(src, dst string) error {
	if src == dst {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create output dir failed: %w", err)
	}
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("rename output failed: %w", err)
	}
	return nil
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
