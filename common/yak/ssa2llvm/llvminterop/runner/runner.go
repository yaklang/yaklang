// Package runner executes external LLVM tools (opt, clang, custom adapters)
// as subprocesses and feeds them IR produced by the ssa2llvm compiler.
package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
)

// Config holds parameters for a single external LLVM pass invocation.
type Config struct {
	// OptBinary is the path to the `opt` executable.
	// If empty, "opt" is resolved via PATH.
	OptBinary string

	// Plugin describes the external plugin/tool to load.
	Plugin *plugin.Descriptor

	// InputFile is the path to the .ll or .bc file to process.
	InputFile string

	// OutputFile is the path where the transformed IR is written.
	OutputFile string

	// Verbose enables printing the full command line.
	Verbose bool
}

// Result captures the outcome of an external LLVM tool invocation.
type Result struct {
	// Command is the full command line that was executed.
	Command string

	// Stdout is the standard output.
	Stdout string

	// Stderr is the standard error.
	Stderr string

	// ExitCode is the process exit code.
	ExitCode int

	// OutputFile is the path to the produced artifact.
	OutputFile string
}

// Run invokes the external LLVM tool described by cfg and returns the result.
func Run(cfg *Config) (*Result, error) {
	if cfg == nil {
		return nil, fmt.Errorf("runner: nil config")
	}
	if cfg.Plugin == nil {
		return nil, fmt.Errorf("runner: nil plugin descriptor")
	}
	if err := cfg.Plugin.Validate(); err != nil {
		return nil, fmt.Errorf("runner: %w", err)
	}
	if cfg.InputFile == "" {
		return nil, fmt.Errorf("runner: InputFile must not be empty")
	}
	if cfg.OutputFile == "" {
		return nil, fmt.Errorf("runner: OutputFile must not be empty")
	}

	// Verify input file exists.
	if _, err := os.Stat(cfg.InputFile); err != nil {
		return nil, fmt.Errorf("runner: input file: %w", err)
	}

	switch cfg.Plugin.Kind {
	case plugin.KindNewPM, plugin.KindLegacy:
		return runOpt(cfg)
	case plugin.KindTool:
		return runTool(cfg)
	default:
		return nil, fmt.Errorf("runner: unsupported plugin kind %q", cfg.Plugin.Kind)
	}
}

func runOpt(cfg *Config) (*Result, error) {
	optBin := cfg.OptBinary
	if optBin == "" {
		optBin = "opt"
	}

	args := make([]string, 0, 16)

	switch cfg.Plugin.Kind {
	case plugin.KindNewPM:
		args = append(args, "--load-pass-plugin="+cfg.Plugin.Path)
	case plugin.KindLegacy:
		args = append(args, "-load="+cfg.Plugin.Path)
	}

	for _, pass := range cfg.Plugin.Passes {
		args = append(args, "--passes="+pass)
	}

	args = append(args, cfg.Plugin.Args...)
	args = append(args, "-S", "-o", cfg.OutputFile, cfg.InputFile)

	return executeCommand(optBin, args, cfg.Verbose)
}

func runTool(cfg *Config) (*Result, error) {
	args := make([]string, 0, 16)
	args = append(args, cfg.Plugin.Args...)
	args = append(args, "-o", cfg.OutputFile, cfg.InputFile)

	return executeCommand(cfg.Plugin.Path, args, cfg.Verbose)
}

func executeCommand(binary string, args []string, verbose bool) (*Result, error) {
	cmd := exec.Command(binary, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmdLine := binary + " " + strings.Join(args, " ")

	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("runner: failed to execute %q: %w", binary, err)
		}
	}

	result := &Result{
		Command:    cmdLine,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		ExitCode:   exitCode,
		OutputFile: "",
	}

	if exitCode == 0 {
		result.OutputFile = args[len(args)-2] // -o <output>
		// Find the -o flag output
		for i, a := range args {
			if a == "-o" && i+1 < len(args) {
				result.OutputFile = args[i+1]
				break
			}
		}
	}

	return result, nil
}
