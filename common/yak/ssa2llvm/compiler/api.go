package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"unsafe"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type CompileConfig struct {
	SourceFile        string
	SourceCode        string
	Language          string
	OutputFile        string
	EntryFunctionName string
	EmitLLVM          bool
	EmitAsm           bool
	CompileOnly       bool
	PrintIR           bool
	ExternBindings    map[string]ExternBinding
	ExtraLinkArgs     []string
	SkipRuntimeLink   bool
	PrintEntryResult  bool
	SSAObfuscators    []string
	LLVMObfuscators   []string
}

type CompileOption func(*CompileConfig)

type RunConfig struct {
	SourceFile      string
	SourceCode      string
	Language        string
	FunctionName    string
	Args            []uint64
	PrintIR         bool
	ExternalHooks   map[string]unsafe.Pointer
	ExternBindings  map[string]ExternBinding
	SSAObfuscators  []string
	LLVMObfuscators []string
}

type RunOption func(*RunConfig)

func defaultRunConfig() *RunConfig {
	return &RunConfig{
		FunctionName:  "check",
		ExternalHooks: make(map[string]unsafe.Pointer),
	}
}

func defaultCompileConfig() *CompileConfig {
	return &CompileConfig{}
}

func WithCompileSourceFile(path string) CompileOption {
	return func(c *CompileConfig) { c.SourceFile = path }
}

func WithCompileSourceCode(code string) CompileOption {
	return func(c *CompileConfig) { c.SourceCode = code }
}

func WithCompileLanguage(language string) CompileOption {
	return func(c *CompileConfig) { c.Language = language }
}

func WithCompileOutputFile(path string) CompileOption {
	return func(c *CompileConfig) { c.OutputFile = path }
}

func WithCompileEntryFunction(name string) CompileOption {
	return func(c *CompileConfig) { c.EntryFunctionName = name }
}

func WithCompileEmitLLVM(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.EmitLLVM = enabled }
}

func WithCompileEmitAsm(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.EmitAsm = enabled }
}

func WithCompileOnly(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.CompileOnly = enabled }
}

func WithCompilePrintIR(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.PrintIR = enabled }
}

func WithCompileExternBindings(bindings map[string]ExternBinding) CompileOption {
	return func(c *CompileConfig) {
		if len(bindings) == 0 {
			return
		}
		if c.ExternBindings == nil {
			c.ExternBindings = make(map[string]ExternBinding, len(bindings))
		}
		for name, binding := range bindings {
			c.ExternBindings[name] = binding
		}
	}
}

func WithCompileExtraLinkArgs(args ...string) CompileOption {
	return func(c *CompileConfig) {
		c.ExtraLinkArgs = append(c.ExtraLinkArgs, args...)
	}
}

func WithCompileSkipRuntimeLink(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.SkipRuntimeLink = enabled }
}

func WithCompilePrintEntryResult(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.PrintEntryResult = enabled }
}

func WithCompileSSAObfuscators(names ...string) CompileOption {
	return func(c *CompileConfig) {
		c.SSAObfuscators = appendObfuscatorNames(c.SSAObfuscators, names...)
	}
}

func WithCompileLLVMObfuscators(names ...string) CompileOption {
	return func(c *CompileConfig) {
		c.LLVMObfuscators = appendObfuscatorNames(c.LLVMObfuscators, names...)
	}
}

func WithCompileConfig(cfg CompileConfig) CompileOption {
	return func(c *CompileConfig) {
		c.SourceFile = cfg.SourceFile
		c.SourceCode = cfg.SourceCode
		c.Language = cfg.Language
		c.OutputFile = cfg.OutputFile
		c.EntryFunctionName = cfg.EntryFunctionName
		c.EmitLLVM = cfg.EmitLLVM
		c.EmitAsm = cfg.EmitAsm
		c.CompileOnly = cfg.CompileOnly
		c.PrintIR = cfg.PrintIR
		c.SkipRuntimeLink = cfg.SkipRuntimeLink
		c.PrintEntryResult = cfg.PrintEntryResult
		c.SSAObfuscators = append(c.SSAObfuscators, cfg.SSAObfuscators...)
		c.LLVMObfuscators = append(c.LLVMObfuscators, cfg.LLVMObfuscators...)
		if len(cfg.ExtraLinkArgs) > 0 {
			c.ExtraLinkArgs = append(c.ExtraLinkArgs, cfg.ExtraLinkArgs...)
		}
		if len(cfg.ExternBindings) > 0 {
			if c.ExternBindings == nil {
				c.ExternBindings = make(map[string]ExternBinding, len(cfg.ExternBindings))
			}
			for name, binding := range cfg.ExternBindings {
				c.ExternBindings[name] = binding
			}
		}
	}
}

func WithRunSourceFile(path string) RunOption {
	return func(c *RunConfig) { c.SourceFile = path }
}

func WithRunSourceCode(code string) RunOption {
	return func(c *RunConfig) { c.SourceCode = code }
}

func WithRunLanguage(language string) RunOption {
	return func(c *RunConfig) { c.Language = language }
}

func WithRunFunction(name string) RunOption {
	return func(c *RunConfig) { c.FunctionName = name }
}

func WithRunArgs(args ...uint64) RunOption {
	return func(c *RunConfig) {
		c.Args = append(c.Args[:0], args...)
	}
}

func WithRunPrintIR(enabled bool) RunOption {
	return func(c *RunConfig) { c.PrintIR = enabled }
}

func WithRunExternalHooks(hooks map[string]unsafe.Pointer) RunOption {
	return func(c *RunConfig) {
		if c.ExternalHooks == nil {
			c.ExternalHooks = make(map[string]unsafe.Pointer, len(hooks))
		}
		for name, addr := range hooks {
			c.ExternalHooks[name] = addr
		}
	}
}

func WithRunExternBindings(bindings map[string]ExternBinding) RunOption {
	return func(c *RunConfig) { c.ExternBindings = bindings }
}

func WithRunSSAObfuscators(names ...string) RunOption {
	return func(c *RunConfig) {
		c.SSAObfuscators = appendObfuscatorNames(c.SSAObfuscators, names...)
	}
}

func WithRunLLVMObfuscators(names ...string) RunOption {
	return func(c *RunConfig) {
		c.LLVMObfuscators = appendObfuscatorNames(c.LLVMObfuscators, names...)
	}
}

func WithRunConfig(cfg RunConfig) RunOption {
	return func(c *RunConfig) {
		c.SourceFile = cfg.SourceFile
		c.SourceCode = cfg.SourceCode
		c.Language = cfg.Language
		if cfg.FunctionName != "" {
			c.FunctionName = cfg.FunctionName
		}
		c.Args = append(c.Args[:0], cfg.Args...)
		c.PrintIR = cfg.PrintIR
		c.ExternBindings = cfg.ExternBindings
		c.SSAObfuscators = append(c.SSAObfuscators, cfg.SSAObfuscators...)
		c.LLVMObfuscators = append(c.LLVMObfuscators, cfg.LLVMObfuscators...)
		if c.ExternalHooks == nil {
			c.ExternalHooks = make(map[string]unsafe.Pointer, len(cfg.ExternalHooks))
		}
		for name, addr := range cfg.ExternalHooks {
			c.ExternalHooks[name] = addr
		}
	}
}

// RunViaJIT compiles and executes the code using LLVM JIT.
func RunViaJIT(opts ...RunOption) (int64, error) {
	cfg := defaultRunConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	_, comp, _, err := compileInput(
		cfg.SourceFile,
		cfg.SourceCode,
		cfg.Language,
		cfg.ExternBindings,
		cfg.SSAObfuscators,
		cfg.LLVMObfuscators,
	)
	if err != nil {
		return 0, err
	}

	if cfg.PrintIR {
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
	for name, addr := range cfg.ExternalHooks {
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

	functionName, fn, err := resolveEntryFunction(comp.Mod, cfg.FunctionName)
	if err != nil {
		return 0, err
	}

	// Prepare arguments
	llvmArgs := make([]llvm.GenericValue, len(cfg.Args))
	for i, arg := range cfg.Args {
		llvmArgs[i] = llvm.NewGenericValueFromInt(comp.LLVMCtx.Int64Type(), arg, false)
	}

	log.Infof("executing function: %s()", functionName)
	result := engine.RunFunction(fn, llvmArgs)

	return int64(result.Int(true)), nil
}

func compileInput(
	sourceFile, sourceCode, language string,
	externBindings map[string]ExternBinding,
	ssaObfuscators []string,
	llvmObfuscators []string,
) (*ssaapi.Program, *Compiler, string, error) {
	code, sourceLabel, language, err := resolveCompileInput(sourceFile, sourceCode, language)
	if err != nil {
		return nil, nil, "", err
	}

	ctx := context.Background()
	opts := buildSSAOptions(language)
	progBundle, err := ssaapi.Parse(code, opts...)
	if err != nil {
		return nil, nil, "", utils.Errorf("SSA parse failed: %v", err)
	}
	if err := obfuscation.ApplySSA(progBundle.Program, ssaObfuscators); err != nil {
		return nil, nil, "", utils.Errorf("SSA obfuscation failed: %v", err)
	}

	llvm.InitializeNativeTarget()
	llvm.InitializeNativeAsmPrinter()

	log.Infof("compiling %s (%s)", sourceLabel, language)

	comp := NewCompiler(ctx, progBundle.Program, WithExternBindings(externBindings))
	if err := comp.Compile(); err != nil {
		comp.Dispose()
		return nil, nil, "", utils.Errorf("LLVM compilation failed: %v", err)
	}
	if err := obfuscation.ApplyLLVM(comp.Mod, llvmObfuscators); err != nil {
		comp.Dispose()
		return nil, nil, "", utils.Errorf("LLVM obfuscation failed: %v", err)
	}

	if err := llvm.VerifyModule(comp.Mod, llvm.PrintMessageAction); err != nil {
		comp.Dispose()
		return nil, nil, "", utils.Errorf("LLVM verification failed: %v", err)
	}

	return progBundle, comp, comp.Mod.String(), nil
}

func resolveCompileInput(sourceFile, sourceCode, language string) (string, string, string, error) {
	if sourceCode != "" {
		if language == "" {
			language = "yak"
		}
		return sourceCode, "<memory>", language, nil
	}
	if sourceFile == "" {
		return "", "", "", utils.Errorf("no source file or source code provided")
	}

	code, err := os.ReadFile(sourceFile)
	if err != nil {
		return "", "", "", utils.Errorf("failed to read source file: %v", err)
	}
	if language == "" {
		language = detectLanguageFromExt(sourceFile)
	}
	return string(code), sourceFile, language, nil
}

func resolveEntryFunction(mod llvm.Module, requested string) (string, llvm.Value, error) {
	candidates := entryFunctionCandidates(requested)
	for _, candidate := range candidates {
		fn := mod.NamedFunction(candidate)
		if !fn.IsNil() {
			return candidate, fn, nil
		}
	}
	if requested == "" {
		requested = "check"
	}
	return "", llvm.Value{}, utils.Errorf("function %q not found in module", requested)
}

func entryFunctionCandidates(requested string) []string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		requested = "check"
	}

	seen := make(map[string]struct{}, 4)
	add := func(name string, out *[]string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		*out = append(*out, name)
	}

	out := make([]string, 0, 4)
	add(requested, &out)
	if strings.HasPrefix(requested, "@") {
		add(strings.TrimPrefix(requested, "@"), &out)
	} else {
		add("@"+requested, &out)
	}
	if requested == "check" {
		add("main", &out)
		add("@main", &out)
	}
	return out
}

func renameConflictingMainFunctions(mod llvm.Module, entryFunc string) string {
	atMain := mod.NamedFunction("@main")
	if !atMain.IsNil() {
		atMain.SetName("yak_internal_atmain")
		if entryFunc == "@main" {
			entryFunc = "yak_internal_atmain"
		}
	}

	plainMain := mod.NamedFunction("main")
	if !plainMain.IsNil() {
		plainMain.SetName("yak_internal_main")
		if entryFunc == "main" {
			entryFunc = "yak_internal_main"
		}
	}

	return entryFunc
}

func CompileToExecutable(opts ...CompileOption) error {
	cfg := defaultCompileConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	_, comp, ir, err := compileInput(
		cfg.SourceFile,
		cfg.SourceCode,
		cfg.Language,
		cfg.ExternBindings,
		cfg.SSAObfuscators,
		cfg.LLVMObfuscators,
	)
	if err != nil {
		return err
	}
	defer comp.Dispose()

	entryFunc, _, err := resolveEntryFunction(comp.Mod, cfg.EntryFunctionName)
	if err != nil {
		return err
	}
	entryFunc = renameConflictingMainFunctions(comp.Mod, entryFunc)

	// Regenerate IR because we modified the module (renamed function)
	ir = comp.Mod.String()

	ir = addMainWrapper(ir, entryFunc, cfg.PrintEntryResult)

	if cfg.PrintIR {
		fmt.Println(ir)
	}

	outputFile := cfg.OutputFile
	if outputFile == "" {
		if runtime.GOOS == "windows" {
			outputFile = "a.exe"
		} else {
			outputFile = "a.out"
		}
	}

	if cfg.EmitLLVM {
		if outputFile == cfg.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(cfg.SourceFile, ".ll")
		}
		if err := os.WriteFile(outputFile, []byte(ir), 0644); err != nil {
			return utils.Errorf("failed to write LLVM IR: %v", err)
		}
		log.Infof("LLVM IR written to: %s", outputFile)
		return nil
	}

	tmpLL, err := os.CreateTemp("", "ssa2llvm-*.ll")
	if err != nil {
		return utils.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpLL.Name())

	if _, err := tmpLL.Write([]byte(ir)); err != nil {
		return utils.Errorf("failed to write temp IR: %v", err)
	}
	tmpLL.Close()

	if cfg.EmitAsm {
		if outputFile == cfg.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(cfg.SourceFile, ".s")
		}
		if err := CompileLLVMToAsm(tmpLL.Name(), outputFile); err != nil {
			return err
		}
		log.Infof("Assembly written to: %s", outputFile)
		return nil
	}

	if cfg.CompileOnly {
		if outputFile == cfg.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(cfg.SourceFile, ".o")
		}
		if err := CompileLLVMToObject(tmpLL.Name(), outputFile); err != nil {
			return err
		}
		log.Infof("Object file written to: %s", outputFile)
		return nil
	}

	if err := CompileLLVMToBinary(tmpLL.Name(), outputFile, !cfg.SkipRuntimeLink, cfg.ExtraLinkArgs...); err != nil {
		return err
	}

	log.Infof("Executable written to: %s", outputFile)
	return nil
}

func addMainWrapper(ir, entryFunc string, printEntryResult bool) string {
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
	printDecl := ""
	printCall := ""
	if printEntryResult {
		if !strings.Contains(ir, "@yak_internal_print_int") {
			printDecl = "declare void @yak_internal_print_int(i64)\n"
		}
		printCall = "  call void @yak_internal_print_int(i64 %result)\n"
	}

	mainWrapper := fmt.Sprintf(`%s%s
define i32 @main() {
entry:
  %%result = call i64 %s()
%s  call void @yak_runtime_gc()
  %%exit_code = trunc i64 %%result to i32
  ret i32 %%exit_code
}
`, gcDecl, printDecl, callTarget, printCall)
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

func appendObfuscatorNames(dst []string, names ...string) []string {
	return append(dst, obfuscation.NormalizeNames(names)...)
}
