package java2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// registerJavaHierarchyTask relates extends/implements after every skeleton and
// before any library body. The class lazy task stays separate and is the one
// that declares a supertype nobody has written: that declaration has to be
// emitted where the reference is, which may be a branch of a field initializer.
// Sharing one closure between the two passes consumed the declaration early and
// appended it after the branch terminator.
// switchToLiveDeclarationBlock moves a missing-supertype container out of the
// function entry and out of a block that already has a terminator. The entry
// is where a later field initializer emits its branch; sharing that block
// appends the declaration under the terminator. The caller's block is restored.
func (y *singleFileBuilder) switchToLiveDeclarationBlock() func() {
	nop := func() {}
	if y == nil || y.FunctionBuilder == nil || y.CurrentBlock == nil {
		return nop
	}
	finished := y.IsBlockFinish()
	inEntry := y.EnterBlock != 0 && y.CurrentBlock.GetId() == y.EnterBlock
	if !finished && !inEntry {
		return nop
	}
	previous := y.CurrentBlock
	block := y.NewBasicBlock("declare")
	y.CurrentBlock = block
	return func() {
		y.CurrentBlock = previous
	}
}

func (y *singleFileBuilder) registerJavaHierarchyTask(blueprint *ssa.Blueprint, relateExisting func()) {
	if y == nil || blueprint == nil || relateExisting == nil {
		return
	}
	prog := y.GetProgram()
	if prog == nil {
		return
	}
	prog.RegisterDeferredBuild(ssa.DeferredBuildKindHierarchy, blueprint.Name, relateExisting)
}

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
