package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"tinygo.org/x/go-llvm"
)

var SSACommand = &cli.Command{
	Name:  "ssa",
	Usage: "SSA to LLVM compiler toolchain",
	Subcommands: []cli.Command{
		*compileCommand,
		*runCommand,
	},
}

var compileCommand = &cli.Command{
	Name:      "compile",
	Usage:     "Compile source code to LLVM IR",
	ArgsUsage: "<source-file>",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "output,o",
			Usage: "Output file path (LLVM IR)",
		},
		cli.StringFlag{
			Name:  "language,l",
			Usage: "Source language (yak, go, python, javascript, java, php, c, typescript)",
		},
		cli.BoolFlag{
			Name:  "verify",
			Usage: "Verify LLVM module after compilation",
		},
		cli.BoolFlag{
			Name:  "print-ir",
			Usage: "Print generated LLVM IR to stdout",
		},
	},
	Action: compileAction,
}

var runCommand = &cli.Command{
	Name:      "run",
	Usage:     "Compile and execute source code via JIT",
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
		return utils.Error("missing source file argument")
	}

	sourceFile := c.Args().First()
	code, err := os.ReadFile(sourceFile)
	if err != nil {
		return utils.Errorf("failed to read source file: %v", err)
	}

	language := c.String("language")
	if language == "" {
		language = detectLanguage(sourceFile)
		log.Infof("auto-detected language: %s", language)
	}

	log.Infof("compiling %s (%s)", sourceFile, language)

	opts := buildSSAOptions(language)
	prog, err := ssaapi.Parse(string(code), opts...)
	if err != nil {
		return utils.Errorf("SSA parse failed: %v", err)
	}

	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	comp := compiler.NewCompiler(context.Background(), prog.Program)
	if err := comp.Compile(); err != nil {
		return utils.Errorf("LLVM compilation failed: %v", err)
	}

	if c.Bool("verify") {
		if err := llvm.VerifyModule(comp.Mod, llvm.ReturnStatusAction); err != nil {
			return utils.Errorf("LLVM module verification failed: %v", err)
		}
		log.Info("LLVM module verified successfully")
	}

	if c.Bool("print-ir") {
		fmt.Println("\n=== Generated LLVM IR ===")
		fmt.Println(comp.Mod.String())
	}

	outputFile := c.String("output")
	if outputFile != "" {
		ir := comp.Mod.String()
		if err := os.WriteFile(outputFile, []byte(ir), 0644); err != nil {
			return utils.Errorf("failed to write output file: %v", err)
		}
		log.Infof("LLVM IR written to: %s", outputFile)
	}

	log.Info("compilation successful")
	return nil
}

func runAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return utils.Error("missing source file argument")
	}

	sourceFile := c.Args().First()
	code, err := os.ReadFile(sourceFile)
	if err != nil {
		return utils.Errorf("failed to read source file: %v", err)
	}

	language := c.String("language")
	if language == "" {
		language = detectLanguage(sourceFile)
		log.Infof("auto-detected language: %s", language)
	}

	log.Infof("running %s (%s)", sourceFile, language)

	opts := buildSSAOptions(language)
	prog, err := ssaapi.Parse(string(code), opts...)
	if err != nil {
		return utils.Errorf("SSA parse failed: %v", err)
	}

	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	comp := compiler.NewCompiler(context.Background(), prog.Program)
	if err := comp.Compile(); err != nil {
		return utils.Errorf("LLVM compilation failed: %v", err)
	}

	if err := llvm.VerifyModule(comp.Mod, llvm.ReturnStatusAction); err != nil {
		return utils.Errorf("LLVM module verification failed: %v", err)
	}

	if c.Bool("print-ir") {
		fmt.Println("\n=== Generated LLVM IR ===")
		fmt.Println(comp.Mod.String())
		fmt.Println()
	}

	engine, err := llvm.NewExecutionEngine(comp.Mod)
	if err != nil {
		comp.Dispose()
		return utils.Errorf("failed to create JIT engine: %v", err)
	}
	defer engine.Dispose()

	functionName := c.String("function")
	fn := comp.Mod.NamedFunction(functionName)
	if fn.IsNil() {
		return utils.Errorf("function '%s' not found in module", functionName)
	}

	log.Infof("executing function: %s()", functionName)
	result := engine.RunFunction(fn, []llvm.GenericValue{})

	fmt.Printf("\n=== Execution Result ===\n")
	fmt.Printf("Function '%s' returned: %d\n", functionName, result.Int(true))

	return nil
}

func detectLanguage(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".yak":
		return "yak"
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".java":
		return "java"
	case ".php":
		return "php"
	case ".c", ".h":
		return "c"
	default:
		return "yak"
	}
}

func buildSSAOptions(language string) []ssaconfig.Option {
	var opts []ssaconfig.Option

	if language != "" {
		lang, err := ssaconfig.ValidateLanguage(language)
		if err == nil {
			opts = append(opts, ssaconfig.WithProjectLanguage(lang))
		}
	}

	return opts
}
