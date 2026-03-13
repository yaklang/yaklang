package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/clibuild"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/compiler"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/obfuscation"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtimeembed"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/trace"
)

const buildCacheVersion = "ssa2llvm-cache-v1"

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
	outputFile string
	language   string
	function   string
	printIR    bool
	ssaObf     []string
	llvmObf    []string
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

Use the printed names with --ssa-obf or --llvm-obf on the compile/run commands.
Names can be repeated, passed as comma-separated lists, or selected with glob patterns.`,
	Action: listObfuscatorsAction,
}

func compileAction(c *cli.Context) error {
	cfg, err := newBuildCommandConfig(c)
	if err != nil {
		return err
	}
	maybeEnableInfoLoggingForTrace(cfg.trace)

	finalOutput := strings.TrimSpace(c.String("output"))
	emitLLVM := c.Bool("emit-llvm")
	emitAsm := c.Bool("emit-asm")
	compileOnly := c.Bool("c")
	if finalOutput == "" {
		finalOutput = defaultCompileOutputPath(cfg.sourceFile, emitLLVM, emitAsm, compileOnly)
	}

	workKey, err := buildWorkKey(cfg, buildWorkKeyOptions{
		emitLLVM:    emitLLVM,
		emitAsm:     emitAsm,
		compileOnly: compileOnly,
	})
	if err != nil {
		return err
	}

	buildDir := buildWorkDirFromKey(workKey)
	if cfg.force {
		if cfg.trace {
			trace.SetEnabled(true)
		}
		_ = os.RemoveAll(buildDir)
	}
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return fmt.Errorf("prepare work dir failed: %w", err)
	}
	trace.SetEnabled(cfg.trace)
	trace.PrintWorkDir(buildDir)

	tempOutput := buildArtifactPath(buildDir, emitLLVM, emitAsm, compileOnly)
	if info, err := os.Stat(tempOutput); err == nil && !info.IsDir() && info.Size() > 0 && !cfg.force {
		trace.Printf("# cache hit\n")
		return copyFilePreserveMode(tempOutput, finalOutput)
	}

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
	return copyFilePreserveMode(tempOutput, finalOutput)
}

func runAction(c *cli.Context) error {
	cfg, err := newBuildCommandConfig(c)
	if err != nil {
		return err
	}
	maybeEnableInfoLoggingForTrace(cfg.trace)

	workKey, err := buildWorkKey(cfg, buildWorkKeyOptions{})
	if err != nil {
		return err
	}
	buildDir := buildWorkDirFromKey(workKey)
	if cfg.force {
		if cfg.trace {
			trace.SetEnabled(true)
		}
		_ = os.RemoveAll(buildDir)
	}
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		return fmt.Errorf("prepare work dir failed: %w", err)
	}
	trace.SetEnabled(cfg.trace)
	trace.PrintWorkDir(buildDir)

	finalOutput := strings.TrimSpace(c.String("output"))
	tempOutput := ""
	runPath := ""
	if finalOutput != "" {
		tempOutput = buildArtifactPath(buildDir, false, false, false)
		runPath = finalOutput
	} else {
		tempOutput = buildArtifactPath(buildDir, false, false, false)
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

	if info, err := os.Stat(tempOutput); err != nil || info.IsDir() || info.Size() == 0 || cfg.force {
		if err := compiler.CompileToExecutable(options...); err != nil {
			return err
		}
	} else {
		trace.Printf("# cache hit\n")
	}

	if finalOutput != "" {
		if err := copyFilePreserveMode(tempOutput, finalOutput); err != nil {
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
		trace:      c.Bool("x"),
		force:      c.Bool("a"),
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

type buildWorkKeyOptions struct {
	emitLLVM    bool
	emitAsm     bool
	compileOnly bool
}

func buildWorkKey(cfg *buildCommandConfig, opts buildWorkKeyOptions) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("compute work key failed: nil config")
	}
	srcPath := strings.TrimSpace(cfg.sourceFile)
	if srcPath == "" {
		return "", fmt.Errorf("compute work key failed: empty source file")
	}
	code, err := os.ReadFile(srcPath)
	if err != nil {
		return "", fmt.Errorf("compute work key failed: read source file: %w", err)
	}

	ssaObf := append([]string{}, cfg.ssaObf...)
	llvmObf := append([]string{}, cfg.llvmObf...)
	sort.Strings(ssaObf)
	sort.Strings(llvmObf)

	h := sha256.New()
	write := func(s string) {
		_, _ = io.WriteString(h, s)
		_, _ = io.WriteString(h, "\n")
	}
	write(buildCacheVersion)
	write("goos=" + runtime.GOOS)
	write("goarch=" + runtime.GOARCH)
	write("lang=" + strings.TrimSpace(cfg.language))
	write("entry=" + strings.TrimSpace(cfg.function))
	write(fmt.Sprintf("emitLLVM=%t", opts.emitLLVM))
	write(fmt.Sprintf("emitAsm=%t", opts.emitAsm))
	write(fmt.Sprintf("compileOnly=%t", opts.compileOnly))
	write(fmt.Sprintf("printIR=%t", cfg.printIR))
	write(fmt.Sprintf("stdlibCompile=%t", cfg.stdlibComp))
	write("ssaObf=" + strings.Join(ssaObf, ","))
	write("llvmObf=" + strings.Join(llvmObf, ","))
	_, _ = h.Write(code)

	sum := h.Sum(nil)
	return hex.EncodeToString(sum), nil
}

func buildWorkDirFromKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) > 32 {
		key = key[:32]
	}
	if key == "" {
		key = "unknown"
	}
	return filepath.Join(os.TempDir(), "yakssa-compile-"+key)
}

func buildArtifactPath(workDir string, emitLLVM, emitAsm, compileOnly bool) string {
	switch {
	case emitLLVM:
		return filepath.Join(workDir, "cache.ll")
	case emitAsm:
		return filepath.Join(workDir, "cache.s")
	case compileOnly:
		return filepath.Join(workDir, "cache.o")
	default:
		if isWindows() {
			return filepath.Join(workDir, "cache.exe")
		}
		return filepath.Join(workDir, "cache.bin")
	}
}

func copyFilePreserveMode(src, dst string) error {
	if strings.TrimSpace(dst) == "" {
		return fmt.Errorf("copy output failed: empty destination path")
	}
	if src == dst {
		return nil
	}
	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("copy output failed: stat source: %w", err)
	}
	if srcInfo.IsDir() {
		return fmt.Errorf("copy output failed: source is a directory: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("copy output failed: create output dir: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy output failed: open source: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("copy output failed: create destination: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy output failed: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("copy output failed: close destination: %w", closeErr)
	}
	_ = os.Chmod(dst, srcInfo.Mode())
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
