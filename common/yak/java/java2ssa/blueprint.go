package java2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *singleFileBuilder) GetBluePrint(name string) *ssa.Blueprint {
	if y == nil {
		return nil
	}
	// try to get inner class firstly
	if y.MarkedThisClassBlueprint != nil {
		n := y.MarkedThisClassBlueprint.Name + INNER_CLASS_SPLIT + name
		bp := y.FunctionBuilder.GetBluePrint(n)
		if bp != nil {
			return bp
		}
	}
	return y.FunctionBuilder.GetBluePrint(name)
}

func (y *singleFileBuilder) getClassNameCandidates(parts ...string) []string {
	if len(parts) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(parts)+1)
	result := make([]string, 0, len(parts)+1)
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	if len(parts) > 1 {
		for start := len(parts) - 2; start >= 0; start-- {
			prefix := strings.Join(parts[:start], ".")
			suffix := strings.Join(parts[start:], INNER_CLASS_SPLIT)
			if prefix == "" {
				add(suffix)
			} else {
				add(prefix + "." + suffix)
			}
		}
	}
	add(strings.Join(parts, "."))
	return result
}

func (y *singleFileBuilder) resolveImportedNestedBlueprint(parts ...string) *ssa.Blueprint {
	if y == nil || len(parts) < 2 {
		return nil
	}
	prog := y.GetProgram()
	if prog == nil {
		return nil
	}
	for prefixEnd := len(parts) - 1; prefixEnd >= 1; prefixEnd-- {
		prefixName := strings.Join(parts[:prefixEnd], ".")
		importType, ok := prog.ReadImportType(prefixName)
		if !ok || importType == nil {
			continue
		}
		bp, ok := ssa.ToClassBluePrintType(importType)
		if !ok || bp == nil {
			continue
		}
		if !y.PreHandler() {
			bp.Build()
		}

		current := bp
		traverseOk := true
		for _, seg := range parts[prefixEnd:] {
			member := current.GetStaticMember(seg)
			if utils.IsNil(member) {
				traverseOk = false
				break
			}
			next, ok := ssa.ToClassBluePrintType(member.GetType())
			if !ok || next == nil {
				traverseOk = false
				break
			}
			if !y.PreHandler() {
				next.Build()
			}
			current = next
		}
		if traverseOk {
			return current
		}
	}
	return nil
}
