package main

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
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

var compileCommand = cli.Command{
	Name:      "compile",
	Aliases:   []string{"c"},
	Usage:     "Compile source code to native executable (default) or LLVM IR",
	ArgsUsage: "<source-file>",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "Output file path (default: a.out on Unix, a.exe on Windows)",
		},
		cli.StringFlag{
			Name:  "language,l",
			Usage: "Source language (yak, go, python, javascript, java, php, c, typescript)",
		},
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
	},
	Action: compileAction,
}

var runCommand = cli.Command{
	Name:      "run",
	Aliases:   []string{"r"},
	Usage:     "Compile and execute via JIT",
	ArgsUsage: "<source-file>",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "language,l",
			Usage: "Source language (yak, go, python, javascript, java, php, c, typescript)",
		},
		cli.StringFlag{
			Name:  "function,f",
			Usage: "Entry function name to execute",
			Value: "check",
		},
		cli.BoolFlag{
			Name:  "print-ir",
			Usage: "Print generated LLVM IR before execution",
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
	},
	Action: runAction,
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
	if c.NArg() < 1 {
		return fmt.Errorf("missing source file argument")
	}

	return compiler.CompileToExecutable(
		compiler.WithCompileSourceFile(c.Args().First()),
		compiler.WithCompileLanguage(c.String("language")),
		compiler.WithCompileOutputFile(c.String("output")),
		compiler.WithCompileEmitLLVM(c.Bool("emit-llvm")),
		compiler.WithCompileEmitAsm(c.Bool("emit-asm")),
		compiler.WithCompileOnly(c.Bool("c")),
		compiler.WithCompilePrintIR(c.Bool("print-ir")),
		compiler.WithCompileSSAObfuscators(c.StringSlice("ssa-obf")...),
		compiler.WithCompileLLVMObfuscators(c.StringSlice("llvm-obf")...),
	)
}

func runAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("missing source file argument")
	}

	functionName := c.String("function")
	result, err := compiler.RunViaJIT(
		compiler.WithRunSourceFile(c.Args().First()),
		compiler.WithRunLanguage(c.String("language")),
		compiler.WithRunFunction(functionName),
		compiler.WithRunPrintIR(c.Bool("print-ir")),
		compiler.WithRunSSAObfuscators(c.StringSlice("ssa-obf")...),
		compiler.WithRunLLVMObfuscators(c.StringSlice("llvm-obf")...),
	)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== Execution Result ===\n")
	fmt.Printf("Function '%s' returned: %d\n", functionName, result)

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
