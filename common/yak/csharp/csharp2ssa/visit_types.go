package csharp2ssa

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// 常见 BCL 集合类型到 SSA 容器类型的映射，其它类型统一走蓝图。
var (
	csharpSliceLikeTypes = map[string]bool{
		"List": true, "IList": true, "IEnumerable": true, "ICollection": true,
		"IReadOnlyList": true, "IReadOnlyCollection": true, "HashSet": true, "ISet": true,
		"Queue": true, "Stack": true, "LinkedList": true, "ArrayList": true,
		"Span": true, "ReadOnlySpan": true, "Memory": true, "ReadOnlyMemory": true,
		"ArraySegment": true, "ImmutableList": true, "ImmutableArray": true,
		"IEnumerator": true, "ConcurrentBag": true, "ConcurrentQueue": true, "ConcurrentStack": true,
		"IAsyncEnumerable": true, "SortedSet": true, "Collection": true, "ObservableCollection": true,
	}
	csharpMapLikeTypes = map[string]bool{
		"Dictionary": true, "IDictionary": true, "IReadOnlyDictionary": true,
		"ConcurrentDictionary": true, "SortedDictionary": true, "SortedList": true,
		"Hashtable": true, "ImmutableDictionary": true, "NameValueCollection": true,
	}
	// 包装类型，语义上直接取第一个泛型参数
	csharpUnwrapTypes = map[string]bool{
		"Task": true, "ValueTask": true, "Nullable": true,
	}
	// Container/wrapper lowering is valid only when the name denotes the BCL
	// type. Matching solely on the final segment would turn source declarations
	// such as Vendor.List<T> and Vendor.Task<T> into slices/scalars.
	csharpBCLTypeNamespaces = map[string][]string{
		"List":                 {"System.Collections.Generic"},
		"IList":                {"System.Collections.Generic", "System.Collections"},
		"IEnumerable":          {"System.Collections.Generic", "System.Collections"},
		"ICollection":          {"System.Collections.Generic", "System.Collections"},
		"IReadOnlyList":        {"System.Collections.Generic"},
		"IReadOnlyCollection":  {"System.Collections.Generic"},
		"HashSet":              {"System.Collections.Generic"},
		"ISet":                 {"System.Collections.Generic"},
		"Queue":                {"System.Collections.Generic", "System.Collections"},
		"Stack":                {"System.Collections.Generic", "System.Collections"},
		"LinkedList":           {"System.Collections.Generic"},
		"IEnumerator":          {"System.Collections.Generic", "System.Collections"},
		"IAsyncEnumerable":     {"System.Collections.Generic"},
		"SortedSet":            {"System.Collections.Generic"},
		"Dictionary":           {"System.Collections.Generic"},
		"IDictionary":          {"System.Collections.Generic"},
		"IReadOnlyDictionary":  {"System.Collections.Generic"},
		"SortedDictionary":     {"System.Collections.Generic"},
		"SortedList":           {"System.Collections.Generic", "System.Collections"},
		"ArrayList":            {"System.Collections"},
		"Hashtable":            {"System.Collections"},
		"Span":                 {"System"},
		"ReadOnlySpan":         {"System"},
		"Memory":               {"System"},
		"ReadOnlyMemory":       {"System"},
		"ArraySegment":         {"System"},
		"Nullable":             {"System"},
		"ImmutableList":        {"System.Collections.Immutable"},
		"ImmutableArray":       {"System.Collections.Immutable"},
		"ImmutableDictionary":  {"System.Collections.Immutable"},
		"ConcurrentBag":        {"System.Collections.Concurrent"},
		"ConcurrentQueue":      {"System.Collections.Concurrent"},
		"ConcurrentStack":      {"System.Collections.Concurrent"},
		"ConcurrentDictionary": {"System.Collections.Concurrent"},
		"Collection":           {"System.Collections.ObjectModel"},
		"ObservableCollection": {"System.Collections.ObjectModel"},
		"NameValueCollection":  {"System.Collections.Specialized"},
		"Task":                 {"System.Threading.Tasks"},
		"ValueTask":            {"System.Threading.Tasks"},
	}
	csharpPredefinedTypeNames = map[string]func() ssa.Type{
		"bool": ssa.CreateBooleanType, "Boolean": ssa.CreateBooleanType,
		"string": ssa.CreateStringType, "String": ssa.CreateStringType,
		"char": ssa.CreateStringType, "Char": ssa.CreateStringType,
		"byte": ssa.CreateByteType, "sbyte": ssa.CreateByteType, "Byte": ssa.CreateByteType, "SByte": ssa.CreateByteType,
		"short": ssa.CreateNumberType, "ushort": ssa.CreateNumberType, "int": ssa.CreateNumberType,
		"uint": ssa.CreateNumberType, "long": ssa.CreateNumberType, "ulong": ssa.CreateNumberType,
		"float": ssa.CreateNumberType, "double": ssa.CreateNumberType, "decimal": ssa.CreateNumberType,
		"nint": ssa.CreateNumberType, "nuint": ssa.CreateNumberType,
		"Int16": ssa.CreateNumberType, "UInt16": ssa.CreateNumberType, "Int32": ssa.CreateNumberType,
		"UInt32": ssa.CreateNumberType, "Int64": ssa.CreateNumberType, "UInt64": ssa.CreateNumberType,
		"Single": ssa.CreateNumberType, "Double": ssa.CreateNumberType, "Decimal": ssa.CreateNumberType,
		"object": ssa.CreateAnyType, "Object": ssa.CreateAnyType, "dynamic": ssa.CreateAnyType,
		"var": ssa.CreateAnyType, "void": ssa.CreateAnyType, "Void": ssa.CreateAnyType,
		"Func": ssa.CreateAnyType, "Action": ssa.CreateAnyType, "Predicate": ssa.CreateAnyType, "Delegate": ssa.CreateAnyType,
	}
)

// VisitType resolves a C# `type_` into an SSA type.
func (b *singleFileBuilder) VisitType(raw csharpparser.IType_Context) ssa.Type {
	if b == nil || raw == nil {
		return ssa.CreateAnyType()
	}
	i, ok := raw.(*csharpparser.Type_Context)
	if !ok || i == nil {
		return ssa.CreateAnyType()
	}
	switch {
	case i.Type_parameter() != nil:
		return ssa.CreateAnyType()
	case i.Value_type() != nil:
		return b.visitValueType(i.Value_type())
	case i.Reference_type() != nil:
		return b.visitReferenceType(i.Reference_type())
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitValueType(raw csharpparser.IValue_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Value_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	if i.Non_nullable_value_type() != nil {
		return b.visitNonNullableValueType(i.Non_nullable_value_type())
	}
	if nv, _ := i.Nullable_value_type().(*csharpparser.Nullable_value_typeContext); nv != nil {
		return b.visitNonNullableValueType(nv.Non_nullable_value_type())
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitNonNullableValueType(raw csharpparser.INon_nullable_value_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Non_nullable_value_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	if st, _ := i.Struct_type().(*csharpparser.Struct_typeContext); st != nil {
		switch {
		case st.Type_name() != nil:
			return b.visitTypeName(st.Type_name())
		case st.Simple_type() != nil:
			return b.predefinedType(st.Simple_type().GetText())
		case st.Tuple_type() != nil:
			return ssa.CreateAnyType()
		}
	}
	if et, _ := i.Enum_type().(*csharpparser.Enum_typeContext); et != nil {
		return b.visitTypeName(et.Type_name())
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitReferenceType(raw csharpparser.IReference_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Reference_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	if i.Non_nullable_reference_type() != nil {
		return b.visitNonNullableReferenceType(i.Non_nullable_reference_type())
	}
	if nr, _ := i.Nullable_reference_type().(*csharpparser.Nullable_reference_typeContext); nr != nil {
		return b.visitNonNullableReferenceType(nr.Non_nullable_reference_type())
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitNonNullableReferenceType(raw csharpparser.INon_nullable_reference_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Non_nullable_reference_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	switch {
	case i.Delegate_type() != nil:
		dt, _ := i.Delegate_type().(*csharpparser.Delegate_typeContext)
		if dt != nil {
			return b.visitTypeName(dt.Type_name())
		}
	case i.Interface_type() != nil:
		return b.visitInterfaceType(i.Interface_type())
	case i.Class_type() != nil:
		return b.visitClassType(i.Class_type())
	case i.Array_type() != nil:
		return b.visitArrayType(i.Array_type())
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitInterfaceType(raw csharpparser.IInterface_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Interface_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	return b.visitTypeName(i.Type_name())
}

func (b *singleFileBuilder) visitClassType(raw csharpparser.IClass_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Class_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	switch {
	case i.Type_name() != nil:
		return b.visitTypeName(i.Type_name())
	case i.KW_STRING() != nil:
		return ssa.CreateStringType()
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitArrayType(raw csharpparser.IArray_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Array_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	elem := b.visitNonArrayType(i.Non_array_type())
	ranks := 0
	for _, rawRank := range i.AllRank_specifier() {
		rank, _ := rawRank.(*csharpparser.Rank_specifierContext)
		if rank == nil {
			continue
		}
		// One rank specifier contributes one dimension plus each comma:
		// [,] is two-dimensional, [,,] is three-dimensional, while two
		// separate [] specifiers still represent a two-level jagged array.
		ranks += 1 + len(rank.AllTK_COMMA())
	}
	if ranks == 0 {
		ranks = 1
	}
	var typ ssa.Type = elem
	for idx := 0; idx < ranks; idx++ {
		typ = ssa.NewSliceType(typ)
	}
	return typ
}

func (b *singleFileBuilder) visitNonArrayType(raw csharpparser.INon_array_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Non_array_typeContext)
	if i == nil {
		return ssa.CreateAnyType()
	}
	switch {
	case i.Value_type() != nil:
		return b.visitValueType(i.Value_type())
	case i.Class_type() != nil:
		return b.visitClassType(i.Class_type())
	case i.Interface_type() != nil:
		return b.visitInterfaceType(i.Interface_type())
	case i.Delegate_type() != nil:
		dt, _ := i.Delegate_type().(*csharpparser.Delegate_typeContext)
		if dt != nil {
			return b.visitTypeName(dt.Type_name())
		}
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) predefinedType(name string) ssa.Type {
	if f, ok := csharpPredefinedTypeNames[name]; ok {
		return f()
	}
	return ssa.CreateAnyType()
}

func (b *singleFileBuilder) visitTypeArgumentList(raw csharpparser.IType_argument_listContext) []ssa.Type {
	list, _ := raw.(*csharpparser.Type_argument_listContext)
	if list == nil {
		return nil
	}
	types := make([]ssa.Type, 0, len(list.AllType_argument()))
	for _, rawArgument := range list.AllType_argument() {
		argument, _ := rawArgument.(*csharpparser.Type_argumentContext)
		if argument == nil || argument.Type_() == nil {
			types = append(types, ssa.CreateAnyType())
			continue
		}
		types = append(types, b.VisitType(argument.Type_()))
	}
	return types
}

// namespaceOrTypeNameParts walks direct grammar children so generic arguments
// stay attached to the identifier segment they follow. AllType_argument_list
// alone cannot distinguish Outer<int>.List from Outer.List<int>.
func (b *singleFileBuilder) namespaceOrTypeNameParts(raw csharpparser.INamespace_or_type_nameContext) (segments []string, lastTypeArgs []ssa.Type) {
	n, _ := raw.(*csharpparser.Namespace_or_type_nameContext)
	if n == nil {
		return nil, nil
	}
	if qa, _ := n.Qualified_alias_member().(*csharpparser.Qualified_alias_memberContext); qa != nil {
		ids := qa.AllIdentifier()
		if len(ids) > 1 {
			alias := identText(ids[0])
			// global:: starts at the compilation root; named aliases must remain
			// in the path so resolveNamedType can expand them.
			if alias != "" && alias != "global" {
				segments = append(segments, alias)
			}
			segments = append(segments, identText(ids[1]))
			lastTypeArgs = b.visitTypeArgumentList(qa.Type_argument_list())
		}
	}
	for _, child := range n.GetChildren() {
		switch current := child.(type) {
		case *csharpparser.IdentifierContext:
			segments = append(segments, identText(current))
			lastTypeArgs = nil
		case *csharpparser.Type_argument_listContext:
			lastTypeArgs = b.visitTypeArgumentList(current)
		}
	}
	return segments, lastTypeArgs
}

// typeNameParts 把 `A.B.C<T1, T2>` 拆成段列表与最后一段的泛型参数列表。
func (b *singleFileBuilder) typeNameParts(raw csharpparser.IType_nameContext) (segments []string, typeArgs []ssa.Type, token csharpparser.IType_nameContext) {
	i, _ := raw.(*csharpparser.Type_nameContext)
	if i == nil {
		return nil, nil, raw
	}
	segments, typeArgs = b.namespaceOrTypeNameParts(i.Namespace_or_type_name())
	return segments, typeArgs, raw
}

func (b *singleFileBuilder) visitTypeName(raw csharpparser.IType_nameContext) ssa.Type {
	segments, typeArgs, token := b.typeNameParts(raw)
	if len(segments) == 0 {
		return ssa.CreateAnyType()
	}
	return b.resolveNamedType(segments, typeArgs, token)
}

// resolveNamedType 是所有「按名字找类型」的统一入口。
func (b *singleFileBuilder) resolveNamedType(segments []string, typeArgs []ssa.Type, token ssa.CanStartStopToken) ssa.Type {
	if len(segments) == 0 {
		return ssa.CreateAnyType()
	}
	// using alias 展开
	if target, aliasNode, ok := b.lookupUsingAlias(segments[0]); ok {
		expanded := strings.Split(stripGenericSuffix(target), ".")
		aliasArgs := []ssa.Type(nil)
		if aliasNode != nil {
			if structured, args := b.namespaceOrTypeNameParts(aliasNode); len(structured) > 0 {
				expanded, aliasArgs = structured, args
			}
		}
		aliasOnly := len(segments) == 1
		segments = append(expanded, segments[1:]...)
		if aliasOnly && len(typeArgs) == 0 {
			typeArgs = aliasArgs
		}
	}
	base := segments[len(segments)-1]
	// Source-declared qualified/nested types win over BCL short-name
	// conveniences. In particular Outer<int>.List is not List<T>.
	if declared := b.lookupBlueprintByPathStrict(segments); declared != nil && b.isDeclaredBlueprint(declared.Name) {
		return declared
	}
	// An unqualified source type imported with `using Namespace;` shadows our
	// convenience lowering for well-known BCL containers. Do this lookup without
	// the all-library fallback used for value identifiers: an unrelated namespace
	// must not make an otherwise-unresolved short name visible.
	if len(segments) == 1 {
		if declared := b.lookupVisibleSourceBlueprint(base); declared != nil {
			return declared
		}
	}
	if len(segments) == 1 {
		if f, ok := csharpPredefinedTypeNames[base]; ok {
			return f()
		}
	} else if segments[0] == "System" && len(segments) == 2 {
		if f, ok := csharpPredefinedTypeNames[base]; ok {
			return f()
		}
	}
	argOr := func(idx int) ssa.Type {
		if idx < len(typeArgs) && typeArgs[idx] != nil {
			return typeArgs[idx]
		}
		return ssa.CreateAnyType()
	}
	bclShorthand := b.isBCLTypeName(segments)
	switch {
	case csharpUnwrapTypes[base] && bclShorthand:
		if len(typeArgs) == 0 {
			return ssa.CreateAnyType()
		}
		return argOr(0)
	case csharpSliceLikeTypes[base] && bclShorthand:
		return ssa.NewSliceType(argOr(0))
	case csharpMapLikeTypes[base] && bclShorthand:
		return ssa.NewMapType(argOr(0), argOr(1))
	}
	return b.blueprintByName(segments, token)
}

// isBCLTypeName reports whether a known shorthand is fully qualified with its
// canonical System namespace or made visible by an explicit using directive.
func (b *singleFileBuilder) isBCLTypeName(segments []string) bool {
	if len(segments) == 0 {
		return false
	}
	base := segments[len(segments)-1]
	namespaces, ok := csharpBCLTypeNamespaces[base]
	if !ok {
		return false
	}
	matches := func(candidate string) bool {
		for _, namespace := range namespaces {
			if candidate == namespace {
				return true
			}
		}
		return false
	}
	if len(segments) > 1 {
		return matches(strings.Join(segments[:len(segments)-1], "."))
	}
	for _, namespace := range b.visibleUsings() {
		if matches(namespace) {
			return true
		}
	}
	return false
}

// lookupVisibleSourceBlueprint resolves only declarations visible in the
// current namespace, an enclosing namespace, or an explicit using namespace.
// Unlike lookupBlueprint it deliberately does not search every project library.
func (b *singleFileBuilder) lookupVisibleSourceBlueprint(name string) *ssa.Blueprint {
	if b == nil || name == "" {
		return nil
	}
	if class := b.MarkedThisClassBlueprint; class != nil {
		for outer := class; outer != nil; outer = b.outerBlueprintOf(outer) {
			if inner := b.GetBluePrint(outer.Name + nestedTypeSplit + name); inner != nil {
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
	fromExport := func(lib *ssa.Program) *ssa.Blueprint {
		if lib == nil {
			return nil
		}
		typ, ok := lib.GetExportType(name)
		if !ok {
			return nil
		}
		bp, ok := ssa.ToBluePrintType(typ)
		if !ok {
			return nil
		}
		if !lib.PreHandler() {
			bp.Build()
		}
		return bp
	}
	if bp := fromExport(prog); bp != nil {
		return bp
	}
	if app != prog {
		if bp := fromExport(app); bp != nil {
			return bp
		}
	}
	for idx := len(b.selfPkgPath); idx > 0; idx-- {
		if lib, _ := app.GetLibrary(strings.Join(b.selfPkgPath[:idx], ".")); lib != nil {
			if bp := fromExport(lib); bp != nil {
				return bp
			}
		}
	}
	for _, namespace := range b.visibleUsings() {
		if lib, _ := app.GetLibrary(namespace); lib != nil {
			if bp := fromExport(lib); bp != nil {
				return bp
			}
		}
	}
	return nil
}

// blueprintByName 找到或创建蓝图，并补齐 full type name。
func (b *singleFileBuilder) blueprintByName(segments []string, token ssa.CanStartStopToken) *ssa.Blueprint {
	base := segments[len(segments)-1]
	// A qualified type name identifies a namespace/library before it identifies a
	// short blueprint name.  Looking up the short name first makes `A.Input`
	// resolve to an unrelated `B.Input` in the current namespace and then pollutes
	// that blueprint with `A.Input` as an additional full name.
	if len(segments) > 1 {
		if bp := b.lookupBlueprintByPathStrict(segments); bp != nil {
			return bp
		}
		// Keep unresolved qualified types distinct. Keying both A.Widget and
		// B.Widget as merely "Widget" merges their members and dataflow even
		// though their C# type identities differ.
		full := strings.Join(segments, ".")
		if bp := b.GetBluePrint(full); bp != nil {
			return bp
		}
		var bp *ssa.Blueprint
		if token != nil {
			bp = b.CreateBlueprint(full, token)
		} else {
			bp = b.CreateBlueprint(full)
		}
		if bp != nil {
			b.ensureBlueprintConstructorSlot(bp)
			bp.AddFullTypeName(full)
		}
		return bp
	}
	bp := b.GetBluePrint(base)
	if bp == nil {
		if token != nil {
			bp = b.CreateBlueprint(base, token)
		} else {
			bp = b.CreateBlueprint(base)
		}
		b.ensureBlueprintConstructorSlot(bp)
	}
	if bp == nil {
		return nil
	}
	if len(segments) > 1 {
		bp.AddFullTypeName(strings.Join(segments, "."))
	} else if !b.isDeclaredBlueprint(base) {
		b.addGuessedFullTypeNames(bp, base)
	}
	return bp
}

// lookupBlueprintByPathStrict 只查找、不创建：
//   - 单段：按名字查当前程序及 include 栈；
//   - 多段：末段蓝图必须已存在，且其 full type name 含完整路径，或前缀是已知库/命名空间。
func (b *singleFileBuilder) lookupBlueprintByPathStrict(segments []string) *ssa.Blueprint {
	if len(segments) == 0 {
		return nil
	}
	base := segments[len(segments)-1]
	if !looksLikeTypeName(base) {
		return nil
	}
	if len(segments) == 1 {
		return b.GetBluePrint(base)
	}
	full := strings.Join(segments, ".")
	prefix := strings.Join(segments[:len(segments)-1], ".")
	// Unknown qualified types are stored under their full path so two namespaces
	// with the same final segment remain distinct.
	if exact := b.GetBluePrint(full); exact != nil {
		return exact
	}
	if prog := b.GetProgram(); prog != nil {
		app := prog.GetApplication()
		if app == nil {
			app = prog
		}
		// Nested blueprints use '$' internally. Try every split between the
		// namespace prefix and the nested type chain, longest namespace first.
		for split := len(segments) - 2; split >= 0; split-- {
			nestedName := strings.Join(segments[split:], nestedTypeSplit)
			if split == 0 {
				if nested := b.GetBluePrint(nestedName); nested != nil {
					return nested
				}
				continue
			}
			namespace := strings.Join(segments[:split], ".")
			if lib, _ := app.GetLibrary(namespace); lib != nil {
				if nested, ok := lib.Blueprint.Get(nestedName); ok && nested != nil {
					return nested
				}
			}
		}
		if lib, _ := app.GetLibrary(prefix); lib != nil {
			if libBp, ok := lib.Blueprint.Get(base); ok && libBp != nil {
				return libBp
			}
		}
		// Namespace names may be relative to the namespace containing the use.
		for idx := len(b.selfPkgPath); idx > 0; idx-- {
			relativePrefix := strings.Join(append(append([]string(nil), b.selfPkgPath[:idx]...), segments[:len(segments)-1]...), ".")
			if lib, _ := app.GetLibrary(relativePrefix); lib != nil {
				if libBp, ok := lib.Blueprint.Get(base); ok && libBp != nil {
					return libBp
				}
			}
		}
	}
	bp := b.GetBluePrint(base)
	if bp == nil {
		return nil
	}
	for _, n := range bp.GetFullTypeNames() {
		if n == full {
			return bp
		}
	}
	// Outer.Inner：Outer 是已声明蓝图时，Inner 视为其嵌套类型
	if outer := b.GetBluePrint(segments[len(segments)-2]); outer != nil && b.isDeclaredBlueprint(base) {
		return bp
	}
	return nil
}

// isDeclaredBlueprint 判断蓝图是否由源码声明（声明时会 SetExportType）。
func (b *singleFileBuilder) isDeclaredBlueprint(name string) bool {
	prog := b.GetProgram()
	if prog == nil {
		return false
	}
	if _, ok := prog.GetExportType(name); ok {
		return true
	}
	if app := prog.GetApplication(); app != nil && app != prog {
		if _, ok := app.GetExportType(name); ok {
			return true
		}
	}
	found := false
	b.forEachLibrary(func(lib *ssa.Program) bool {
		if _, ok := lib.GetExportType(name); ok {
			found = true
			return false
		}
		return true
	})
	return found
}

func (b *singleFileBuilder) forEachLibrary(fn func(*ssa.Program) bool) {
	prog := b.GetProgram()
	if prog == nil {
		return
	}
	app := prog.GetApplication()
	if app == nil || app.UpStream == nil {
		return
	}
	for _, lib := range app.UpStream.Values() {
		if lib == nil {
			continue
		}
		if !fn(lib) {
			return
		}
	}
}

// addGuessedFullTypeNames 对未声明（BCL / 第三方）类型，按 using 与当前命名空间猜测全名。
func (b *singleFileBuilder) addGuessedFullTypeNames(bp *ssa.Blueprint, name string) {
	if bp == nil || name == "" {
		return
	}
	exist := bp.GetFullTypeNames()
	add := func(full string) {
		if full == "" || utils.StringArrayContains(exist, full) {
			return
		}
		bp.AddFullTypeName(full)
		exist = append(exist, full)
	}
	for _, u := range b.visibleUsings() {
		add(u + "." + name)
	}
	if len(b.selfPkgPath) > 0 {
		add(strings.Join(b.selfPkgPath, ".") + "." + name)
	}
}

// declaredFullTypeName 给源码声明的类型登记 `namespace.Name`。
func (b *singleFileBuilder) declaredFullTypeName(bp *ssa.Blueprint) {
	if bp == nil {
		return
	}
	if len(b.selfPkgPath) > 0 {
		bp.AddFullTypeName(strings.Join(b.selfPkgPath, ".") + "." + bp.Name)
	}
}

// applyDeclaredType applies a declaration's compile-time type while preserving
// the initializer as an operand. In particular, `Base x = new Derived()` must
// resolve overloads against Base, then retain the Derived constructor through
// the cast for data-flow and virtual dispatch.
func (b *singleFileBuilder) applyDeclaredType(value ssa.Value, typ ssa.Type) ssa.Value {
	if utils.IsNil(value) || typ == nil {
		return value
	}
	if typ.GetTypeKind() == ssa.AnyTypeKind {
		return value
	}
	cur := value.GetType()
	if cur == nil {
		value.SetType(typ)
		return value
	}
	switch cur.GetTypeKind() {
	case ssa.AnyTypeKind, ssa.NullTypeKind:
		value.SetType(typ)
		return value
	}
	if declared, isBlueprint := ssa.ToBluePrintType(typ); isBlueprint {
		if current, currentIsBlueprint := ssa.ToBluePrintType(cur); currentIsBlueprint {
			if current != declared {
				if cast := b.EmitTypeCast(value, typ); !utils.IsNil(cast) {
					return cast
				}
			}
		} else {
			value.SetType(typ)
		}
	}
	return value
}

// rememberDeclaredVariableType associates an explicit source type with its
// lexical variable slot. SSA assignments create new Variable versions, so the
// type cannot live on one *ssa.Variable alone; scope plus source name identifies
// the declaration while still allowing an inner declaration to shadow it.
func (b *singleFileBuilder) rememberDeclaredVariableType(variable *ssa.Variable, typ ssa.Type) {
	if b == nil || variable == nil || typ == nil || typ.GetTypeKind() == ssa.AnyTypeKind {
		return
	}
	scope := variable.GetScope()
	if scope == nil {
		return
	}
	if b.declaredVariableTypes == nil {
		b.declaredVariableTypes = make(map[ssa.ScopeIF]map[string]ssa.Type)
	}
	types := b.declaredVariableTypes[scope]
	if types == nil {
		types = make(map[string]ssa.Type)
		b.declaredVariableTypes[scope] = types
	}
	types[variable.GetName()] = typ
}

func (b *singleFileBuilder) rememberDeclaredParameterType(parameter *ssa.Parameter, typ ssa.Type) {
	if parameter == nil || typ == nil {
		return
	}
	parameter.SetType(typ)
	b.rememberDeclaredVariableType(parameter.GetLastVariable(), typ)
}

func (b *singleFileBuilder) registerDeclaredMemberType(owner *ssa.Blueprint, name string, static bool, typ ssa.Type) {
	if b == nil || b.declaredTypes == nil || owner == nil || name == "" || typ == nil {
		return
	}
	state := b.declaredTypes
	state.Lock()
	defer state.Unlock()
	if state.members == nil {
		state.members = make(map[csharpDeclaredMemberSlot]ssa.Type)
	}
	state.members[csharpDeclaredMemberSlot{owner: owner, name: name, static: static}] = typ
}

func (b *singleFileBuilder) declaredMemberSlotForReceiver(receiver ssa.Value, name string) (csharpDeclaredMemberSlot, ssa.Type, bool) {
	if b == nil || b.declaredTypes == nil || utils.IsNil(receiver) || name == "" {
		return csharpDeclaredMemberSlot{}, nil, false
	}
	class, ok := ssa.ToBluePrintType(receiver.GetType())
	if !ok || class == nil {
		return csharpDeclaredMemberSlot{}, nil, false
	}
	static := false
	if container := class.Container(); !utils.IsNil(container) && container.GetId() == receiver.GetId() {
		static = true
	}
	state := b.declaredTypes
	state.RLock()
	defer state.RUnlock()
	for current := class; current != nil; current = current.GetSuperBlueprint() {
		slot := csharpDeclaredMemberSlot{owner: current, name: name, static: static}
		if typ := state.members[slot]; typ != nil {
			return slot, typ, true
		}
	}
	return csharpDeclaredMemberSlot{}, nil, false
}

func (b *singleFileBuilder) declaredMemberSlot(variable *ssa.Variable) (csharpDeclaredMemberSlot, ssa.Type, bool) {
	if variable == nil || !variable.IsMemberCall() {
		return csharpDeclaredMemberSlot{}, nil, false
	}
	object, key := variable.GetMemberCall()
	constant, ok := ssa.ToConstInst(key)
	if !ok || constant == nil {
		return csharpDeclaredMemberSlot{}, nil, false
	}
	return b.declaredMemberSlotForReceiver(object, constant.VarString())
}

func (b *singleFileBuilder) rememberDeclaredMemberWriter(value ssa.Value, slot csharpDeclaredMemberSlot) {
	if b == nil || b.declaredTypes == nil || utils.IsNil(value) || slot.owner == nil {
		return
	}
	key := csharpDeclaredValueKey{program: value.GetProgram(), id: value.GetId()}
	state := b.declaredTypes
	state.Lock()
	defer state.Unlock()
	if state.writers == nil {
		state.writers = make(map[csharpDeclaredValueKey]csharpDeclaredMemberSlot)
	}
	state.writers[key] = slot
}

func (b *singleFileBuilder) declaredMemberWriter(value ssa.Value) (csharpDeclaredMemberSlot, bool) {
	if b == nil || b.declaredTypes == nil || utils.IsNil(value) {
		return csharpDeclaredMemberSlot{}, false
	}
	seen := make(map[int64]struct{})
	for !utils.IsNil(value) {
		if _, duplicate := seen[value.GetId()]; duplicate {
			return csharpDeclaredMemberSlot{}, false
		}
		seen[value.GetId()] = struct{}{}
		key := csharpDeclaredValueKey{program: value.GetProgram(), id: value.GetId()}
		b.declaredTypes.RLock()
		slot, ok := b.declaredTypes.writers[key]
		b.declaredTypes.RUnlock()
		if ok {
			return slot, true
		}
		if cast, ok := ssa.ToTypeCast(value); ok && cast != nil {
			next, exists := cast.GetValueById(cast.Value)
			if !exists {
				return csharpDeclaredMemberSlot{}, false
			}
			value = next
			continue
		}
		if sideEffect, ok := ssa.ToSideEffect(value); ok && sideEffect != nil {
			next, exists := sideEffect.GetValueById(sideEffect.Value)
			if !exists {
				return csharpDeclaredMemberSlot{}, false
			}
			value = next
			continue
		}
		return csharpDeclaredMemberSlot{}, false
	}
	return csharpDeclaredMemberSlot{}, false
}

// declaredVariableType finds the nearest explicit declaration visible from an
// assignment target. Branch scopes therefore inherit the outer slot's type,
// while an inner local with the same name wins until that lexical scope ends.
func (b *singleFileBuilder) declaredVariableType(variable *ssa.Variable) ssa.Type {
	if b == nil || variable == nil {
		return nil
	}
	if variable.IsMemberCall() {
		_, typ, _ := b.declaredMemberSlot(variable)
		return typ
	}
	for scope := variable.GetScope(); scope != nil; scope = scope.GetParent() {
		if types := b.declaredVariableTypes[scope]; types != nil {
			if typ := types[variable.GetName()]; typ != nil {
				return typ
			}
		}
	}
	return nil
}

func (b *singleFileBuilder) applyVariableDeclaredType(variable *ssa.Variable, value ssa.Value) ssa.Value {
	if slot, typ, ok := b.declaredMemberSlot(variable); ok {
		b.rememberDeclaredMemberWriter(value, slot)
		value = b.applyDeclaredType(value, typ)
		b.rememberDeclaredMemberWriter(value, slot)
		return value
	}
	return b.applyDeclaredType(value, b.declaredVariableType(variable))
}

// typeOfReturnType handles `return_type: ref_return_type | 'void'`.
func (b *singleFileBuilder) VisitReturnType(raw csharpparser.IReturn_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Return_typeContext)
	if i == nil || i.KW_VOID() != nil {
		return ssa.CreateAnyType()
	}
	rt, _ := i.Ref_return_type().(*csharpparser.Ref_return_typeContext)
	if rt == nil {
		return ssa.CreateAnyType()
	}
	return b.VisitType(rt.Type_())
}

// VisitLocalVariableType handles `local_variable_type: 'var' | type_`; returns nil for var.
func (b *singleFileBuilder) VisitLocalVariableType(raw csharpparser.ILocal_variable_typeContext) ssa.Type {
	i, _ := raw.(*csharpparser.Local_variable_typeContext)
	if i == nil || i.KW_VAR() != nil || i.Type_() == nil {
		return nil
	}
	return b.VisitType(i.Type_())
}
