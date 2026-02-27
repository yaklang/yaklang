package main

import (
	"fmt"
	"os"

	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
)

func main() {
	app := cli.NewApp()
	app.Name = "ssa2llvm"
	app.Usage = "SSA to LLVM compiler - compile source code to native executables"
	app.Version = "1.0.0"

	app.Commands = []cli.Command{
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
	},
	Action: runAction,
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
	)
	if err != nil {
		return err
	}

	fmt.Printf("\n=== Execution Result ===\n")
	fmt.Printf("Function '%s' returned: %d\n", functionName, result)

	return nil
}
