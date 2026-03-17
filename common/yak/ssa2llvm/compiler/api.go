package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/embed"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

type CompileConfig struct {
	SourceFile        string
	SourceCode        string
	Language          string
	OutputFile        string
	// FinalOutputFile, when set, copies the built artifact to this path (preserving mode).
	// Useful when building into a cached work dir but the user requested -o <path>.
	FinalOutputFile   string
	// FinalOutputAuto selects the default output path based on source file and mode.
	// (e.g. foo.yak -> foo.ll when EmitLLVM, else a.out)
	FinalOutputAuto bool
	WorkDir           string
	EntryFunctionName string
	EmitLLVM          bool
	EmitAsm           bool
	CompileOnly       bool
	PrintIR           bool
	ExternBindings    map[string]ExternBinding
	ExtraLinkArgs     []string
	SkipRuntimeLink   bool
	RuntimeArchive    string
	PrintEntryResult  bool
	SSAObfuscators    []string
	LLVMObfuscators   []string
	StdlibCompile     bool

	// CacheEnabled uses a deterministic work dir under $TMP and reuses existing artifacts.
	CacheEnabled bool
	// Force removes any existing work dir and rebuilds (like `go build -a`).
	Force bool
	// Trace prints WORK=... and external commands (like `go build -x`).
	Trace bool
}

type CompileOption func(*CompileConfig)

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

// WithCompileFinalOutputFile copies the produced artifact to dst on success.
func WithCompileFinalOutputFile(dst string) CompileOption {
	return func(c *CompileConfig) { c.FinalOutputFile = dst }
}

func WithCompileFinalOutputAuto(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.FinalOutputAuto = enabled }
}

func WithCompileWorkDir(dir string) CompileOption {
	return func(c *CompileConfig) { c.WorkDir = dir }
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

func WithCompileRuntimeArchive(path string) CompileOption {
	return func(c *CompileConfig) { c.RuntimeArchive = path }
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

func WithCompileStdlibCompile(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.StdlibCompile = enabled }
}

func WithCompileCacheEnabled(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.CacheEnabled = enabled }
}

func WithCompileForceRebuild(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.Force = enabled }
}

func WithCompileTrace(enabled bool) CompileOption {
	return func(c *CompileConfig) { c.Trace = enabled }
}

func WithCompileConfig(cfg CompileConfig) CompileOption {
	return func(c *CompileConfig) {
		c.SourceFile = cfg.SourceFile
		c.SourceCode = cfg.SourceCode
		c.Language = cfg.Language
		c.OutputFile = cfg.OutputFile
		c.FinalOutputFile = cfg.FinalOutputFile
		c.FinalOutputAuto = cfg.FinalOutputAuto
		c.WorkDir = cfg.WorkDir
		c.EntryFunctionName = cfg.EntryFunctionName
		c.EmitLLVM = cfg.EmitLLVM
		c.EmitAsm = cfg.EmitAsm
		c.CompileOnly = cfg.CompileOnly
		c.PrintIR = cfg.PrintIR
		c.SkipRuntimeLink = cfg.SkipRuntimeLink
		c.RuntimeArchive = cfg.RuntimeArchive
		c.PrintEntryResult = cfg.PrintEntryResult
		c.SSAObfuscators = append(c.SSAObfuscators, cfg.SSAObfuscators...)
		c.LLVMObfuscators = append(c.LLVMObfuscators, cfg.LLVMObfuscators...)
		c.StdlibCompile = cfg.StdlibCompile
		c.CacheEnabled = cfg.CacheEnabled
		c.Force = cfg.Force
		c.Trace = cfg.Trace
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

type CompileResult struct {
	WorkDir   string
	Artifact  string
	CacheHit  bool
	RuntimeA  string
	ExtraLink []string
}

func CompileToExecutable(opts ...CompileOption) (CompileResult, error) {
	cfg := defaultCompileConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	res, err := compileWithConfig(cfg)
	if err != nil {
		return CompileResult{}, err
	}
	return res, nil
}

func compileWithConfig(cfg *CompileConfig) (CompileResult, error) {
	if cfg == nil {
		return CompileResult{}, utils.Errorf("compile failed: nil config")
	}

	// If requested, enable go-build-like trace for WORK and command lines.
	trace.SetEnabled(cfg.Trace)

	linking := !cfg.EmitLLVM && !cfg.EmitAsm && !cfg.CompileOnly && !cfg.SkipRuntimeLink

	// Build into a deterministic cached work dir when enabled and WorkDir isn't explicit.
	if cfg.CacheEnabled && strings.TrimSpace(cfg.WorkDir) == "" {
		key, keyErr := cachedWorkKeyFromConfig(cfg)
		if keyErr != nil {
			return CompileResult{}, keyErr
		}
		cfg.WorkDir = cachedWorkDirFromKey(key)
	}
	// When linking (or extracting embedded runtime), we need a writable directory.
	if linking && strings.TrimSpace(cfg.WorkDir) == "" {
		tmp, err := os.MkdirTemp("", "yakssa-work-*")
		if err != nil {
			return CompileResult{}, utils.Errorf("prepare work dir failed: %v", err)
		}
		cfg.WorkDir = tmp
	}
	if cfg.WorkDir != "" {
		if cfg.Force {
			_ = os.RemoveAll(cfg.WorkDir)
		}
		if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
			return CompileResult{}, utils.Errorf("prepare work dir failed: %v", err)
		}
		if cfg.Trace {
			trace.PrintWorkDir(cfg.WorkDir)
		}
	}

	artifactPath := strings.TrimSpace(cfg.OutputFile)
	if cfg.CacheEnabled && strings.TrimSpace(cfg.WorkDir) != "" {
		artifactPath = cachedArtifactPath(cfg.WorkDir, cfg)
		cfg.OutputFile = artifactPath
	}
	if artifactPath != "" && cfg.CacheEnabled && !cfg.Force {
		if info, err := os.Stat(artifactPath); err == nil && !info.IsDir() && info.Size() > 0 {
			// Cache hit: optionally still copy to final output.
			if dst := finalOutputPath(cfg); strings.TrimSpace(dst) != "" {
				if err := CopyFilePreserveMode(artifactPath, dst); err != nil {
					return CompileResult{}, err
				}
			}
			return CompileResult{
				WorkDir:  cfg.WorkDir,
				Artifact: artifactPath,
				CacheHit: true,
			}, nil
		}
	}

	// Prepare runtime archive when linking and no archive is explicitly provided.
	runtimeArchive := strings.TrimSpace(cfg.RuntimeArchive)
	extraLinkArgs := append([]string{}, cfg.ExtraLinkArgs...)
	if linking && runtimeArchive == "" {
		if cfg.StdlibCompile {
			archivePath, gcLibDir, buildErr := embed.BuildRuntimeArchiveFromEmbeddedSource(cfg.WorkDir)
			if buildErr != nil {
				return CompileResult{}, buildErr
			}
			runtimeArchive = archivePath
			cfg.RuntimeArchive = archivePath
			if strings.TrimSpace(gcLibDir) != "" {
				extraLinkArgs = append(extraLinkArgs, "-L"+gcLibDir)
			}
		} else {
			if archivePath, extractErr := embed.ExtractLibyakToDir(cfg.WorkDir); extractErr == nil {
				runtimeArchive = archivePath
				cfg.RuntimeArchive = archivePath
			} else if extractErr != embed.ErrNoEmbeddedRuntime {
				return CompileResult{}, extractErr
			}

			if _, gcErr := embed.ExtractLibgcToDir(cfg.WorkDir); gcErr == nil {
				// Extracted libgc.a into the work dir; clang will use -L$WORK -lgc.
				extraLinkArgs = append(extraLinkArgs, "-L"+cfg.WorkDir)
			} else if gcErr != embed.ErrNoEmbeddedRuntime {
				return CompileResult{}, gcErr
			}
		}
	}
	cfg.ExtraLinkArgs = extraLinkArgs

	_, comp, ir, err := compileInput(
		cfg.SourceFile,
		cfg.SourceCode,
		cfg.Language,
		cfg.ExternBindings,
		cfg.SSAObfuscators,
		cfg.LLVMObfuscators,
	)
	if err != nil {
		return CompileResult{}, err
	}
	defer comp.Dispose()

		entryFunc, _, err := resolveEntryFunction(comp.Mod, cfg.EntryFunctionName)
		if err != nil {
			return CompileResult{}, err
		}
		entryFunc = renameConflictingMainFunctions(comp.Mod, entryFunc)

		if err := comp.addMainWrapperToModule(entryFunc, cfg.PrintEntryResult); err != nil {
			return CompileResult{}, err
		}

		// Verify again after emitting the wrapper entrypoint.
		if err := llvm.VerifyModule(comp.Mod, llvm.PrintMessageAction); err != nil {
			return CompileResult{}, utils.Errorf("LLVM verification failed after adding main wrapper: %v", err)
		}

		// Regenerate IR because we modified the module (renamed function + wrapper).
		ir = comp.Mod.String()

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
			return CompileResult{}, utils.Errorf("failed to write LLVM IR: %v", err)
		}
		log.Infof("LLVM IR written to: %s", outputFile)
		if dst := finalOutputPath(cfg); strings.TrimSpace(dst) != "" {
			if err := CopyFilePreserveMode(outputFile, dst); err != nil {
				return CompileResult{}, err
			}
		}
		return CompileResult{WorkDir: cfg.WorkDir, Artifact: outputFile, CacheHit: false}, nil
	}

	tmpLL, err := os.CreateTemp(cfg.WorkDir, "ssa2llvm-*.ll")
	if err != nil {
		return CompileResult{}, utils.Errorf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpLL.Name())

	if _, err := tmpLL.Write([]byte(ir)); err != nil {
		return CompileResult{}, utils.Errorf("failed to write temp IR: %v", err)
	}
	tmpLL.Close()

	if cfg.EmitAsm {
		if outputFile == cfg.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(cfg.SourceFile, ".s")
		}
		if err := CompileLLVMToAsm(tmpLL.Name(), outputFile); err != nil {
			return CompileResult{}, err
		}
		log.Infof("Assembly written to: %s", outputFile)
		if dst := finalOutputPath(cfg); strings.TrimSpace(dst) != "" {
			if err := CopyFilePreserveMode(outputFile, dst); err != nil {
				return CompileResult{}, err
			}
		}
		return CompileResult{WorkDir: cfg.WorkDir, Artifact: outputFile, CacheHit: false}, nil
	}

	if cfg.CompileOnly {
		if outputFile == cfg.OutputFile && outputFile != "" {
		} else {
			outputFile = replaceExt(cfg.SourceFile, ".o")
		}
		if err := CompileLLVMToObject(tmpLL.Name(), outputFile); err != nil {
			return CompileResult{}, err
		}
		log.Infof("Object file written to: %s", outputFile)
		if dst := finalOutputPath(cfg); strings.TrimSpace(dst) != "" {
			if err := CopyFilePreserveMode(outputFile, dst); err != nil {
				return CompileResult{}, err
			}
		}
		return CompileResult{WorkDir: cfg.WorkDir, Artifact: outputFile, CacheHit: false}, nil
	}

	if err := CompileLLVMToBinary(tmpLL.Name(), outputFile, !cfg.SkipRuntimeLink, cfg.RuntimeArchive, cfg.ExtraLinkArgs...); err != nil {
		return CompileResult{}, err
	}

	log.Infof("Executable written to: %s", outputFile)
	if dst := finalOutputPath(cfg); strings.TrimSpace(dst) != "" {
		if err := CopyFilePreserveMode(outputFile, dst); err != nil {
			return CompileResult{}, err
		}
	}
	return CompileResult{
		WorkDir:   cfg.WorkDir,
		Artifact:  outputFile,
		CacheHit:  false,
		RuntimeA:  runtimeArchive,
		ExtraLink: extraLinkArgs,
	}, nil
}

func finalOutputPath(cfg *CompileConfig) string {
	if cfg == nil {
		return ""
	}
	if strings.TrimSpace(cfg.FinalOutputFile) != "" {
		return strings.TrimSpace(cfg.FinalOutputFile)
	}
	if !cfg.FinalOutputAuto {
		return ""
	}
	// Follow the CLI defaults.
	switch {
	case cfg.EmitLLVM:
		return replaceExt(cfg.SourceFile, ".ll")
	case cfg.EmitAsm:
		return replaceExt(cfg.SourceFile, ".s")
	case cfg.CompileOnly:
		return replaceExt(cfg.SourceFile, ".o")
	default:
		if runtime.GOOS == "windows" {
			return "a.exe"
		}
		return "a.out"
	}
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
