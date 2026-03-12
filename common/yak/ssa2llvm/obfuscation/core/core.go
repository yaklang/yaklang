package core

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type SSAObfuscator interface {
	Name() string
	Run(*ssa.Program) error
}

type LLVMObfuscator interface {
	Name() string
	Run(llvm.Module) error
}

var Default = struct {
	SSA  map[string]SSAObfuscator
	LLVM map[string]LLVMObfuscator
}{
	SSA:  make(map[string]SSAObfuscator),
	LLVM: make(map[string]LLVMObfuscator),
}

func RegisterSSA(obfuscator SSAObfuscator) {
	if obfuscator == nil {
		log.Warnf("skip nil SSA obfuscator registration")
		return
	}

	name := normalizeName(obfuscator.Name())
	if name == "" {
		log.Warnf("skip SSA obfuscator registration with empty name")
		return
	}
	if _, exists := Default.SSA[name]; exists {
		log.Warnf("skip duplicate SSA obfuscator registration %q", name)
		return
	}

	Default.SSA[name] = obfuscator
}

func RegisterLLVM(obfuscator LLVMObfuscator) {
	if obfuscator == nil {
		log.Warnf("skip nil LLVM obfuscator registration")
		return
	}

	name := normalizeName(obfuscator.Name())
	if name == "" {
		log.Warnf("skip LLVM obfuscator registration with empty name")
		return
	}
	if _, exists := Default.LLVM[name]; exists {
		log.Warnf("skip duplicate LLVM obfuscator registration %q", name)
		return
	}

	Default.LLVM[name] = obfuscator
}

func ApplySSA(program *ssa.Program, names []string) error {
	resolved, err := expandNames("ssa", names, sortedKeys(Default.SSA))
	if err != nil {
		return err
	}
	for _, name := range resolved {
		if err := Default.SSA[name].Run(program); err != nil {
			return fmt.Errorf("ssa obfuscator %q failed: %w", name, err)
		}
	}
	return nil
}

func ApplyLLVM(module llvm.Module, names []string) error {
	resolved, err := expandNames("llvm", names, sortedKeys(Default.LLVM))
	if err != nil {
		return err
	}
	for _, name := range resolved {
		if err := Default.LLVM[name].Run(module); err != nil {
			return fmt.Errorf("llvm obfuscator %q failed: %w", name, err)
		}
	}
	return nil
}

func ListSSA() []string {
	return sortedKeys(Default.SSA)
}

func ListLLVM() []string {
	return sortedKeys(Default.LLVM)
}

func NormalizeNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		for _, part := range strings.Split(name, ",") {
			normalized := normalizeName(part)
			if normalized == "" {
				continue
			}
			out = append(out, normalized)
		}
	}
	return out
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func expandNames(stage string, patterns []string, available []string) ([]string, error) {
	normalizedPatterns := NormalizeNames(patterns)
	if len(normalizedPatterns) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(available))
	out := make([]string, 0, len(normalizedPatterns))
	for _, patternText := range normalizedPatterns {
		matched := false
		for _, candidate := range available {
			ok, err := path.Match(patternText, candidate)
			if err != nil {
				return nil, fmt.Errorf("invalid %s obfuscator pattern %q: %w", stage, patternText, err)
			}
			if !ok {
				continue
			}
			matched = true
			if _, exists := seen[candidate]; exists {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
		if !matched {
			return nil, unknownObfuscatorError(stage, patternText, available)
		}
	}
	return out, nil
}

func unknownObfuscatorError(stage, name string, available []string) error {
	if len(available) == 0 {
		return fmt.Errorf("unknown %s obfuscator/pattern %q (no %s obfuscators registered)", stage, name, stage)
	}
	return fmt.Errorf(
		"unknown %s obfuscator/pattern %q (available: %s; glob patterns like '*' are supported)",
		stage,
		name,
		strings.Join(available, ", "),
	)
}

func sortedKeys[T any](items map[string]T) []string {
	out := make([]string, 0, len(items))
	for name := range items {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
