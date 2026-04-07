package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/pack"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/plugin"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/runner"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/llvminterop/verify"
)

func applyLLVMInterop(cfg *CompileConfig, inputIR string) (string, []string, error) {
	if cfg == nil {
		return inputIR, nil, nil
	}

	descs, err := cfg.llvmInteropDescriptors()
	if err != nil {
		return "", nil, err
	}
	if len(descs) == 0 {
		return inputIR, nil, nil
	}

	_, capabilities, err := detectLLVMInteropEnvironment(cfg)
	if err != nil {
		return "", nil, err
	}

	current := inputIR
	temps := make([]string, 0, len(descs))
	for i, desc := range descs {
		if err := validatePluginCapabilities(desc, capabilities); err != nil {
			return "", temps, err
		}

		next, err := os.CreateTemp(cfg.workDirForTemps(), fmt.Sprintf("ssa2llvm-interop-%02d-*.ll", i))
		if err != nil {
			return "", temps, fmt.Errorf("llvm interop: create temp output: %w", err)
		}
		nextPath := next.Name()
		next.Close()
		temps = append(temps, nextPath)

		result, runErr := runner.Run(&runner.Config{
			OptBinary:  cfg.LLVMOptBinary,
			Plugin:     desc,
			InputFile:  current,
			OutputFile: nextPath,
			Verbose:    cfg.Trace,
		})
		if runErr != nil {
			return "", temps, fmt.Errorf("llvm interop: run %q failed: %w", desc.Name, runErr)
		}
		if result.ExitCode != 0 {
			diagnostics := verify.DiagnoseFailure(result.Stderr, result.ExitCode)
			return "", temps, fmt.Errorf("llvm interop: %q failed: %s", desc.Name, strings.Join(diagnostics, "; "))
		}
		if outputCheck := verify.CheckOutputFile(nextPath); !outputCheck.Valid {
			return "", temps, fmt.Errorf("llvm interop: %q produced invalid output: %s", desc.Name, strings.Join(outputCheck.Errors, "; "))
		}

		data, err := os.ReadFile(nextPath)
		if err != nil {
			return "", temps, fmt.Errorf("llvm interop: read %q output: %w", desc.Name, err)
		}
		if irCheck := verify.CheckIRValidity(string(data)); !irCheck.Valid {
			return "", temps, fmt.Errorf("llvm interop: %q produced invalid IR: %s", desc.Name, strings.Join(irCheck.Errors, "; "))
		}

		current = nextPath
	}

	return current, temps, nil
}

func detectLLVMInteropEnvironment(cfg *CompileConfig) (*runner.VersionInfo, *runner.Capabilities, error) {
	version, err := runner.DetectVersion(cfg.LLVMOptBinary)
	if err != nil {
		return nil, nil, err
	}
	return version, runner.DetectCapabilities(version), nil
}

func validatePluginCapabilities(desc *plugin.Descriptor, capabilities *runner.Capabilities) error {
	if desc == nil {
		return fmt.Errorf("llvm interop: nil descriptor")
	}
	switch desc.Kind {
	case plugin.KindNewPM:
		if capabilities != nil && !capabilities.SupportsLoadPassPlugin {
			return fmt.Errorf("llvm interop: plugin %q requires new-pm support", desc.Name)
		}
	case plugin.KindLegacy:
		if capabilities != nil && !capabilities.HasLegacyPM {
			return fmt.Errorf("llvm interop: plugin %q requires legacy PM support", desc.Name)
		}
	}
	return nil
}

func loadLLVMPackManifest(packRef string) (*pack.Manifest, error) {
	packRef = strings.TrimSpace(packRef)
	if packRef == "" {
		return nil, nil
	}
	if manifest, ok := pack.LookupBuiltin(packRef); ok && manifest != nil {
		return manifest, nil
	}
	return pack.LoadManifest(packRef)
}

func parseLLVMPluginKind(kind string) (plugin.Kind, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "new-pm", "newpm", "new_pm":
		return plugin.KindNewPM, nil
	case "legacy", "legacy-pm", "legacy_pm":
		return plugin.KindLegacy, nil
	case "tool":
		return plugin.KindTool, nil
	default:
		return plugin.KindNewPM, fmt.Errorf("llvm interop: unsupported plugin kind %q", kind)
	}
}

func clonePluginDescriptor(in plugin.Descriptor) *plugin.Descriptor {
	out := in
	out.Args = append([]string{}, in.Args...)
	out.Passes = append([]string{}, in.Passes...)
	return &out
}

func (cfg *CompileConfig) llvmInteropDescriptors() ([]*plugin.Descriptor, error) {
	if cfg == nil {
		return nil, nil
	}
	if strings.TrimSpace(cfg.LLVMPack) != "" && strings.TrimSpace(cfg.LLVMPluginPath) != "" {
		return nil, fmt.Errorf("llvm interop: --llvm-pack and --llvm-plugin cannot be used together")
	}

	if strings.TrimSpace(cfg.LLVMPack) != "" {
		manifest, err := loadLLVMPackManifest(cfg.LLVMPack)
		if err != nil {
			return nil, fmt.Errorf("llvm interop: load pack manifest %q: %w", cfg.LLVMPack, err)
		}
		if err := manifest.Validate(); err != nil {
			return nil, err
		}
		version, _, err := detectLLVMInteropEnvironment(cfg)
		if err != nil {
			return nil, err
		}
		if !manifest.Compatible(version.Major) {
			return nil, fmt.Errorf("llvm interop: pack %q is incompatible with LLVM %d", manifest.Name, version.Major)
		}
		descs := make([]*plugin.Descriptor, 0, len(manifest.Plugins))
		for _, entry := range manifest.Plugins {
			descs = append(descs, clonePluginDescriptor(entry))
		}
		return descs, nil
	}

	if strings.TrimSpace(cfg.LLVMPluginPath) == "" {
		if len(cfg.LLVMPasses) > 0 {
			return nil, fmt.Errorf("llvm interop: --llvm-passes requires --llvm-plugin")
		}
		return nil, nil
	}

	kind, err := parseLLVMPluginKind(cfg.LLVMPluginKind)
	if err != nil {
		return nil, err
	}
	desc := &plugin.Descriptor{
		Name:   filepath.Base(cfg.LLVMPluginPath),
		Kind:   kind,
		Path:   strings.TrimSpace(cfg.LLVMPluginPath),
		Passes: append([]string{}, cfg.LLVMPasses...),
	}
	if err := desc.Validate(); err != nil {
		return nil, err
	}
	return []*plugin.Descriptor{desc}, nil
}
