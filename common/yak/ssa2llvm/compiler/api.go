package compiler

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type CompileOptions struct {
	SourceFile        string
	SourceCode        string
	Language          string
	OutputFile        string
	EntryFunctionName string
	EmitLLVM          bool
	EmitAsm           bool
	CompileOnly       bool
	PrintIR           bool
}

type RunOptions struct {
	SourceFile    string
	SourceCode    string
	Language      string
	FunctionName  string
	Args          []uint64
	PrintIR       bool
	ExternalHooks map[string]unsafe.Pointer
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

	// Register external hooks (mappings)
	for name, addr := range opts.ExternalHooks {
		fnVal := comp.Mod.NamedFunction(name)
		if !fnVal.IsNil() {
			engine.AddGlobalMapping(fnVal, addr)
		}
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
			} else {
				// Try @main
				fn = comp.Mod.NamedFunction("@main")
				if !fn.IsNil() {
					functionName = "@main"
				}
			}
		} else {
			// Try with @ prefix
			fn = comp.Mod.NamedFunction("@" + functionName)
			if !fn.IsNil() {
				functionName = "@" + functionName
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

	return prog, comp, ir, nil
}

func CompileToExecutable(opts CompileOptions) error {
	var comp *Compiler
	var ir string
	var err error

	if opts.SourceCode != "" {
		_, comp, ir, err = compileToIRFromCode(opts.SourceCode, opts.Language)
	} else {
		_, comp, ir, err = compileToIR(opts.SourceFile, opts.Language)
	}

	if err != nil {
		return err
	}
	defer comp.Dispose()

	// Add main wrapper for executable generation
	// Use configured entry function or default to "@main" (standard Yak SSA entry)
	entryFunc := opts.EntryFunctionName
	if entryFunc == "" {
		entryFunc = "check"
	}

	// Logic to resolve entry function and avoid collision with wrapper @main
	fn := comp.Mod.NamedFunction(entryFunc)
	if fn.IsNil() {
		// Fallback logic
		if entryFunc == "check" {
			fn = comp.Mod.NamedFunction("main")
			if !fn.IsNil() {
				entryFunc = "main"
			} else {
				fn = comp.Mod.NamedFunction("@main")
				if !fn.IsNil() {
					entryFunc = "@main"
				}
			}
		}
	}
	// Try with @ if not found
	if fn.IsNil() && !strings.HasPrefix(entryFunc, "@") {
		fn = comp.Mod.NamedFunction("@" + entryFunc)
		if !fn.IsNil() {
			entryFunc = "@" + entryFunc
		}
	}

	// Rename existing main to avoid collision with C main wrapper
	atMain := comp.Mod.NamedFunction("@main")
	if !atMain.IsNil() {
		atMain.SetName("yak_internal_atmain")
		if entryFunc == "@main" {
			entryFunc = "yak_internal_atmain"
		}
	}

	plainMain := comp.Mod.NamedFunction("main")
	if !plainMain.IsNil() {
		plainMain.SetName("yak_internal_main")
		if entryFunc == "main" {
			entryFunc = "yak_internal_main"
		}
	}

	// Regenerate IR because we modified the module (renamed function)
	ir = comp.Mod.String()

	ir = addMainWrapper(ir, entryFunc)

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
		if err := CompileLLVMToAsm(tmpLL.Name(), outputFile); err != nil {
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
		if err := CompileLLVMToObject(tmpLL.Name(), outputFile); err != nil {
			return err
		}
		log.Infof("Object file written to: %s", outputFile)
		return nil
	}

	if err := CompileLLVMToBinary(tmpLL.Name(), outputFile); err != nil {
		return err
	}

	log.Infof("Executable written to: %s", outputFile)
	return nil
}

func addMainWrapper(ir, entryFunc string) string {
	// Construct call target name
	// If entryFunc is "check", target is "@check"
	// If entryFunc is "@main", target is @"@main" (quoted because of @)
	callTarget := "@" + entryFunc
	if entryFunc == "@main" {
		callTarget = "@\"@main\""
	}

	gcDecl := ""
	if !strings.Contains(ir, "@yak_runtime_gc") {
		gcDecl = "\ndeclare void @yak_runtime_gc()\n"
	}

	mainWrapper := fmt.Sprintf(`%s
define i32 @main() {
entry:
  %%result = call i64 %s()
  call void @yak_runtime_gc()
  %%exit_code = trunc i64 %%result to i32
  ret i32 %%exit_code
}
`, gcDecl, callTarget)
	return ir + mainWrapper
}

func replaceExt(filename, newExt string) string {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	return base + newExt
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
