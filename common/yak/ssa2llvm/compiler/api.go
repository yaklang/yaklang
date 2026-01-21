package compiler

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"tinygo.org/x/go-llvm"
)

type CompileOptions struct {
	SourceFile  string
	Language    string
	OutputFile  string
	EmitLLVM    bool
	EmitAsm     bool
	CompileOnly bool
	PrintIR     bool
}

type RunOptions struct {
	SourceFile   string   // Path to source file
	SourceCode   string   // Raw source code string (optional, overrides SourceFile)
	Language     string   // Source language (optional, detected from extension)
	FunctionName string   // Function to execute (default: "check" or "main")
	Args         []uint64 // Arguments to pass to the function
	PrintIR      bool     // Print LLVM IR before execution
}

// RunViaJIT compiles and executes the code using LLVM JIT.
func RunViaJIT(opts RunOptions) (int64, error) {
	var comp *Compiler
	var err error

	if opts.SourceCode != "" {
		_, comp, _, err = compileToIRFromCode(opts.SourceCode, opts.Language)
	} else {
		_, comp, _, err = compileToIR(opts.SourceFile, opts.Language)
	}

	if err != nil {
		return 0, err
	}

	if opts.PrintIR {
		fmt.Println("\n=== Generated LLVM IR ===")
		fmt.Println(comp.Mod.String())
		fmt.Println()
	}

	engine, err := llvm.NewExecutionEngine(comp.Mod)
	if err != nil {
		comp.Dispose()
		return 0, utils.Errorf("failed to create JIT engine: %v", err)
	}

	// Dispose order is important (LIFO):
	// 1. engine.Dispose() (releases Module)
	// 2. comp.Builder.Dispose()
	// 3. comp.LLVMCtx.Dispose()
	defer comp.LLVMCtx.Dispose()
	defer comp.Builder.Dispose()
	defer engine.Dispose()

	functionName := opts.FunctionName
	if functionName == "" {
		functionName = "check"
	}

	fn := comp.Mod.NamedFunction(functionName)
	if fn.IsNil() {
		// Try fallback to main if check not found
		if functionName == "check" {
			fn = comp.Mod.NamedFunction("main")
			if !fn.IsNil() {
				functionName = "main"
			}
		}
		if fn.IsNil() {
			return 0, utils.Errorf("function '%s' not found in module", functionName)
		}
	}

	// Prepare arguments
	llvmArgs := make([]llvm.GenericValue, len(opts.Args))
	for i, arg := range opts.Args {
		llvmArgs[i] = llvm.NewGenericValueFromInt(comp.LLVMCtx.Int64Type(), arg, false)
	}

	log.Infof("executing function: %s()", functionName)
	result := engine.RunFunction(fn, llvmArgs)

	return int64(result.Int(true)), nil
}

func compileToIRFromCode(code, language string) (*ssaapi.Program, *Compiler, string, error) {
	ctx := context.Background()

	opts := buildSSAOptions(language)
	progBundle, err := ssaapi.Parse(code, opts...)
	if err != nil {
		return nil, nil, "", utils.Errorf("SSA parse failed: %v", err)
	}
	ssaProg := progBundle.Program

	// Initialize LLVM
	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	comp := NewCompiler(ctx, ssaProg)
	if err := comp.Compile(); err != nil {
		comp.Dispose()
		return nil, nil, "", utils.Errorf("LLVM compilation failed: %v", err)
	}

	// Verify module
	if err := llvm.VerifyModule(comp.Mod, llvm.PrintMessageAction); err != nil {
		comp.Dispose()
		return nil, nil, "", utils.Errorf("LLVM verification failed: %v", err)
	}

	ir := comp.Mod.String()

	return progBundle, comp, ir, nil
}

func compileToIR(sourceFile, language string) (*ssaapi.Program, *Compiler, string, error) {
	code, err := os.ReadFile(sourceFile)
	if err != nil {
		return nil, nil, "", utils.Errorf("failed to read source file: %v", err)
	}

	if language == "" {
		language = detectLanguageFromExt(sourceFile)
	}

	log.Infof("compiling %s (%s)", sourceFile, language)

	prog, comp, ir, err := compileToIRFromCode(string(code), language)
	if err != nil {
		return nil, nil, "", err
	}

	// Add main wrapper for executable generation (this only affects the returned IR string)
	ir = addMainWrapper(ir)

	return prog, comp, ir, nil
}

func CompileToExecutable(opts CompileOptions) error {
	_, comp, ir, err := compileToIR(opts.SourceFile, opts.Language)
	if err != nil {
		return err
	}
	defer comp.Dispose()

	if opts.PrintIR {
		fmt.Println(ir)
	}

	outputFile := opts.OutputFile
	if outputFile == "" {
		if runtime.GOOS == "windows" {
			outputFile = "a.exe"
		} else {
			outputFile = "a.out"
		}
	}

	if opts.EmitLLVM {
		if outputFile == opts.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(opts.SourceFile, ".ll")
		}
		if err := os.WriteFile(outputFile, []byte(ir), 0644); err != nil {
			return utils.Errorf("failed to write LLVM IR: %v", err)
		}
		log.Infof("LLVM IR written to: %s", outputFile)
		return nil
	}

	tmpLL, err := ioutil.TempFile("", "ssa2llvm-*.ll")
	if err != nil {
		return utils.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpLL.Name())

	if _, err := tmpLL.Write([]byte(ir)); err != nil {
		return utils.Errorf("failed to write temp IR: %v", err)
	}
	tmpLL.Close()

	if opts.EmitAsm {
		if outputFile == opts.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(opts.SourceFile, ".s")
		}
		if err := compileLLVMToAsm(tmpLL.Name(), outputFile); err != nil {
			return err
		}
		log.Infof("Assembly written to: %s", outputFile)
		return nil
	}

	if opts.CompileOnly {
		if outputFile == opts.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(opts.SourceFile, ".o")
		}
		if err := compileLLVMToObject(tmpLL.Name(), outputFile); err != nil {
			return err
		}
		log.Infof("Object file written to: %s", outputFile)
		return nil
	}

	if err := compileLLVMToBinary(tmpLL.Name(), outputFile); err != nil {
		return err
	}

	log.Infof("Executable written to: %s", outputFile)
	return nil
}

func addMainWrapper(ir string) string {
	mainWrapper := `
define i32 @main() {
entry:
  %result = call i64 @check()
  %exit_code = trunc i64 %result to i32
  ret i32 %exit_code
}
`
	return ir + mainWrapper
}

func replaceExt(filename, newExt string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	return base + newExt
}

func compileLLVMToAsm(llFile, asmFile string) error {
	llcPath, err := findLLVMTool("llc")
	if err != nil {
		return err
	}

	cmd := exec.Command(llcPath, llFile, "-o", asmFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("llc failed: %v\n%s", err, output)
	}
	return nil
}

func compileLLVMToObject(llFile, objFile string) error {
	llcPath, err := findLLVMTool("llc")
	if err != nil {
		return err
	}

	cmd := exec.Command(llcPath, "-filetype=obj", llFile, "-o", objFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("llc failed: %v\n%s", err, output)
	}
	return nil
}

func compileLLVMToBinary(llFile, binFile string) error {
	llcPath, err := findLLVMTool("llc")
	if err != nil {
		return err
	}

	tmpAsm, err := ioutil.TempFile("", "ssa2llvm-*.s")
	if err != nil {
		return utils.Errorf("failed to create temp asm file: %v", err)
	}
	defer os.Remove(tmpAsm.Name())
	tmpAsm.Close()

	cmd := exec.Command(llcPath, llFile, "-o", tmpAsm.Name())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("llc failed: %v\n%s", err, output)
	}

	clangPath := "clang"
	if p, err := exec.LookPath("clang"); err == nil {
		clangPath = p
	}

	cmd = exec.Command(clangPath, tmpAsm.Name(), "-o", binFile)
	output, err = cmd.CombinedOutput()
	if err != nil {
		return utils.Errorf("clang linking failed: %v\n%s", err, output)
	}

	return nil
}

func findLLVMTool(tool string) (string, error) {
	paths := []string{
		tool,
		"/opt/homebrew/opt/llvm/bin/" + tool,
		"/usr/local/opt/llvm/bin/" + tool,
		"/usr/bin/" + tool,
	}

	for _, p := range paths {
		if _, err := exec.LookPath(p); err == nil {
			return p, nil
		}
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", utils.Errorf("%s not found, please install LLVM", tool)
}

func detectLanguageFromExt(filename string) string {
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
