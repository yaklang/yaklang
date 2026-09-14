package csharp2ssa

import (
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// 标识符解析（Java 风格）：
//  1. 局部变量 / 参数（PeekValue）
//  2. 当前类：静态方法、实例成员(this.x)、静态成员、常量、实例方法
//  3. 外层类（嵌套类型）成员
//  4. 类级常量 constMap
//  5. import 值 / 已声明的蓝图（当前命名空间、外层命名空间、using）
//  6. PascalCase 兜底为类型蓝图，其余走 ReadValue（Undefined）

// ReadIdentifierValue resolves an identifier in value position.
func (b *singleFileBuilder) ReadIdentifierValue(name string, token ssa.CanStartStopToken) ssa.Value {
	if b == nil || name == "" {
		return nil
	}
	if v := b.PeekValue(name); !utils.IsNil(v) {
		return v
	}
	if v := b.readClassMemberValue(name); !utils.IsNil(v) {
		return v
	}
	if v, ok := b.constMap[name]; ok && !utils.IsNil(v) {
		return v
	}
	if v := b.readUsingStaticMember(name); !utils.IsNil(v) {
		return v
	}
	if bp := b.lookupBlueprint(name); bp != nil {
		return bp.Container()
	}
	prog := b.GetProgram()
	if prog != nil {
		if v, ok := prog.ReadImportValue(name); ok && !utils.IsNil(v) {
			return v
		}
		if typ, ok := prog.ReadImportType(name); ok {
			if bp, ok := ssa.ToBluePrintType(typ); ok {
				return bp.Container()
			}
		}
	}
	if looksLikeTypeName(name) {
		bp := b.blueprintByName([]string{name}, token)
		if bp != nil {
			return bp.Container()
		}
	}
	return b.ReadValue(name)
}

// readUsingStaticMember resolves a bare identifier imported through
// `using static Namespace.Type`. Declared types expose their registered static
// members directly. Unknown framework types use an any-typed receiver so the
// member remains queryable without being mistaken for an instance method.
func (b *singleFileBuilder) readUsingStaticMember(name string) ssa.Value {
	if name == "" {
		return nil
	}
	statics := b.visibleUsingStatics()
	// Search all declared imports before synthesizing a framework fallback. A
	// later unresolved import (for example System.Console) must not hide a real
	// member from an earlier source-defined type.
	for idx := len(statics) - 1; idx >= 0; idx-- {
		target := stripGenericSuffix(statics[idx])
		segments := strings.Split(target, ".")
		if bp := b.lookupBlueprintByPath(segments); bp != nil {
			if method := bp.GetStaticMethod(name); !utils.IsNil(method) {
				return method
			}
			if member := bp.GetStaticMember(name); !utils.IsNil(member) {
				return member
			}
		}
	}
	for idx := len(statics) - 1; idx >= 0; idx-- {
		target := stripGenericSuffix(statics[idx])
		segments := strings.Split(target, ".")
		if b.lookupBlueprintByPath(segments) != nil {
			continue
		}
		receiverName := lastDotSegment(target)
		if receiverName == "" {
			continue
		}
		receiver := b.EmitUndefined(receiverName)
		member := b.ReadMemberCallValue(receiver, b.EmitConstInstPlaceholder(name))
		if !utils.IsNil(member) {
			return member
		}
	}
	return nil
}

// readClassMemberValue 尝试把裸标识符解释为当前类（含父类/外层类）的成员。
func (b *singleFileBuilder) readClassMemberValue(name string) ssa.Value {
	class := b.MarkedThisClassBlueprint
	if class == nil {
		return nil
	}
	this := b.PeekValue("this")
	for bp := class; bp != nil; bp = b.outerBlueprintOf(bp) {
		if method := bp.GetStaticMethod(name); !utils.IsNil(method) {
			return method
		}
		normalMember := bp.GetNormalMember(name)
		if !utils.IsNil(normalMember) || !utils.IsNil(bp.GetNormalMethod(name)) {
			if this != nil && bp == class {
				if !utils.IsNil(normalMember) {
					if getter, ok := b.emitDeclaredAccessor(this, "get_"+name, nil); ok {
						return getter
					}
				}
				if v := b.ReadMemberCallValue(this, b.EmitConstInstPlaceholder(name)); !utils.IsNil(v) {
					return v
				}
			}
		}
		if v := bp.GetStaticMember(name); !utils.IsNil(v) {
			if scoped := b.ReadMemberCallValueByName(bp.Container(), name); !utils.IsNil(scoped) {
				return scoped
			}
			if getter, ok := b.emitDeclaredAccessor(bp.Container(), "get_"+name, nil); ok {
				return getter
			}
			return v
		}
		if v := bp.GetConstMember(name); !utils.IsNil(v) {
			return v
		}
		if v := b.readSelfMemberOf(bp, name); !utils.IsNil(v) {
			return v
		}
	}
	return nil
}

// readSelfMemberOf 等价于 FunctionBuilder.ReadSelfMember，但可指定蓝图。
func (b *singleFileBuilder) readSelfMemberOf(class *ssa.Blueprint, name string) ssa.Value {
	if class == nil {
		return nil
	}
	variable := b.GetStaticMember(class, name)
	if v := b.PeekValueByVariable(variable); !utils.IsNil(v) {
		return v
	}
	if values := class.Read(name); len(values) > 0 {
		return values[len(values)-1]
	}
	return nil
}

// CreateIdentifierVariable resolves an identifier in lvalue position.
func (b *singleFileBuilder) CreateIdentifierVariable(name string, token ssa.CanStartStopToken) *ssa.Variable {
	if b == nil || name == "" {
		return nil
	}
	// 已存在局部变量 / 参数：直接写
	if v := b.PeekValueInThisFunction(name); !utils.IsNil(v) {
		if _, isParamMember := ssa.ToParameterMember(v); !isParamMember {
			// Keep an uninitialized declaration local only while assigning in the
			// same lexical scope. In a nested branch CreateVariable must capture
			// the outer slot so the CFG can merge it into a phi.
			if b.CurrentBlock != nil && b.CurrentBlock.ScopeTable != nil {
				if current := ssa.GetFristVariableFromScope(b.CurrentBlock.ScopeTable, name); current != nil && current.GetLocal() {
					return b.CreateLocalVariable(name)
				}
			}
			return b.CreateVariable(name, token)
		}
	}
	if b.PeekValue(name) != nil && b.MarkedThisClassBlueprint == nil {
		return b.CreateVariable(name, token)
	}
	if class := b.MarkedThisClassBlueprint; class != nil {
		this := b.PeekValue("this")
		for bp := class; bp != nil; bp = b.outerBlueprintOf(bp) {
			if !utils.IsNil(bp.GetNormalMember(name)) {
				if this != nil {
					return b.CreateMemberCallVariable(this, b.EmitConstInstPlaceholder(name))
				}
			}
			if !utils.IsNil(bp.GetStaticMember(name)) {
				return b.CreateMemberCallVariable(bp.Container(), b.EmitConstInstPlaceholder(name))
			}
		}
	}
	return b.CreateVariable(name, token)
}

// outerBlueprintOf 通过命名约定 Outer_Inner 找到外层类。
func (b *singleFileBuilder) outerBlueprintOf(bp *ssa.Blueprint) *ssa.Blueprint {
	if bp == nil {
		return nil
	}
	idx := strings.LastIndex(bp.Name, nestedTypeSplit)
	if idx <= 0 {
		return nil
	}
	outer := b.GetBluePrint(bp.Name[:idx])
	if outer == bp {
		return nil
	}
	return outer
}

// lookupBlueprint 只查找「已经存在」的蓝图，不会创建。
// 顺序：当前程序（含 import）→ 当前命名空间及其外层 → using 命名空间 → 全部库。
func (b *singleFileBuilder) lookupBlueprint(name string) *ssa.Blueprint {
	if b == nil || name == "" {
		return nil
	}
	if bp := b.GetBluePrint(name); bp != nil {
		return bp
	}
	// 嵌套类型：Outer_Inner
	if class := b.MarkedThisClassBlueprint; class != nil {
		for bp := class; bp != nil; bp = b.outerBlueprintOf(bp) {
			if inner := b.GetBluePrint(bp.Name + nestedTypeSplit + name); inner != nil {
				return inner
			}
		}
	}
	prog := b.GetProgram()
	if prog == nil {
		return nil
	}
	app := prog.GetApplication()
	if app == nil {
		app = prog
	}
	tryLib := func(lib *ssa.Program) *ssa.Blueprint {
		if lib == nil {
			return nil
		}
		if t, ok := lib.GetExportType(name); ok {
			if bp, ok := ssa.ToBluePrintType(t); ok {
				if !lib.PreHandler() {
					bp.Build()
				}
				return bp
			}
		}
		return nil
	}
	if bp := tryLib(app); bp != nil {
		return bp
	}
	for i := len(b.selfPkgPath); i > 0; i-- {
		ns := strings.Join(b.selfPkgPath[:i], ".")
		if lib, _ := app.GetLibrary(ns); lib != nil {
			if bp := tryLib(lib); bp != nil {
				return bp
			}
		}
	}
	for _, u := range b.visibleUsings() {
		if lib, _ := app.GetLibrary(u); lib != nil {
			if bp := tryLib(lib); bp != nil {
				return bp
			}
		}
	}
	var found *ssa.Blueprint
	b.forEachLibrary(func(lib *ssa.Program) bool {
		if bp := tryLib(lib); bp != nil {
			found = bp
			return false
		}
		return true
	})
	return found
}

// lookupBlueprintByPath 处理 `Ns.Sub.Type`：优先按命名空间库精确找，找不到时退化为按简单名找。
func (b *singleFileBuilder) lookupBlueprintByPath(segments []string) *ssa.Blueprint {
	if len(segments) == 0 {
		return nil
	}
	name := segments[len(segments)-1]
	if len(segments) > 1 {
		prog := b.GetProgram()
		if prog != nil {
			app := prog.GetApplication()
			if app == nil {
				app = prog
			}
			ns := strings.Join(segments[:len(segments)-1], ".")
			if lib, _ := app.GetLibrary(ns); lib != nil {
				if t, ok := lib.GetExportType(name); ok {
					if bp, ok := ssa.ToBluePrintType(t); ok {
						if !lib.PreHandler() {
							bp.Build()
						}
						return bp
					}
				}
			}
			// 相对当前命名空间：namespace A { ... B.C } => A.B.C
			for i := len(b.selfPkgPath); i > 0; i-- {
				full := strings.Join(append(append([]string(nil), b.selfPkgPath[:i]...), segments[:len(segments)-1]...), ".")
				if lib, _ := app.GetLibrary(full); lib != nil {
					if t, ok := lib.GetExportType(name); ok {
						if bp, ok := ssa.ToBluePrintType(t); ok {
							return bp
						}
					}
				}
			}
			// 嵌套类型 Outer.Inner
			if outer := b.lookupBlueprint(segments[len(segments)-2]); outer != nil {
				if inner := b.GetBluePrint(outer.Name + nestedTypeSplit + name); inner != nil {
					return inner
				}
			}
		}
	}
	return b.lookupBlueprint(name)
}

func looksLikeTypeName(name string) bool {
	if name == "" {
		return false
	}
	r := []rune(name)[0]
	return unicode.IsUpper(r)
}

// isNamespacePrefix 判断标识符是否是某个已知命名空间（库）的首段，
// 用于 `System.Text.Encoding.UTF8` 这类链式访问。
func (b *singleFileBuilder) isNamespacePrefix(name string) bool {
	if b == nil || name == "" {
		return false
	}
	prog := b.GetProgram()
	if prog == nil {
		return false
	}
	app := prog.GetApplication()
	if app == nil || app.UpStream == nil {
		return false
	}
	for _, key := range app.UpStream.Keys() {
		if key == name || strings.HasPrefix(key, name+".") {
			return true
		}
	}
	for _, u := range b.visibleUsings() {
		if strings.HasPrefix(u, name+".") || u == name {
			return true
		}
	}
	return false
}
