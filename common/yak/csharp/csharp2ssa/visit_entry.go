package csharp2ssa

import (
	"strings"

	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitCompilationUnit(raw csharpparser.ICompilation_unitContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Compilation_unitContext)
	if !ok || i == nil {
		return
	}
	for _, using := range i.AllUsing_directive() {
		b.VisitUsingDirective(using)
	}
	// 与 java2ssa 一致：只在 PreHandler 阶段登记类型骨架，方法体等通过 lazy builder 延迟构建
	if !b.PreHandler() {
		return
	}
	// C# top-level statements execute in source order in the application's main
	// function. Register one lazy task so all statements share the same evolving
	// block/scope after the AST root is released.
	statements := make([]csharpparser.IStatementContext, 0, len(i.AllGlobal_statement()))
	for _, rawStatement := range i.AllGlobal_statement() {
		statement, _ := rawStatement.(*csharpparser.Global_statementContext)
		if statement != nil && statement.Statement() != nil {
			statements = append(statements, ssa.DetachAST(statement.Statement()))
		}
	}
	if len(statements) > 0 && b.Function != nil {
		b.lazyBuild(b.Function.AddLazyBuilder, func() {
			localFunctions := make([]*csharpparser.Local_function_declarationContext, 0)
			for _, statement := range statements {
				if declaration := localFunctionFromStatement(statement); declaration != nil {
					localFunctions = append(localFunctions, declaration)
					b.predeclareLocalFunction(declaration)
				}
			}
			for _, statement := range statements {
				if b.IsBlockFinish() {
					break
				}
				b.VisitStatement(statement)
			}
			// Top-level statements form the implicit Main body, so their local
			// functions have the same whole-scope visibility as declarations in an
			// ordinary statement list. Build any shell left behind by an abrupt
			// statement as well, keeping its body available to interprocedural flow.
			for _, declaration := range localFunctions {
				if shell := b.localFunctionShells[declaration]; shell != nil && !shell.IsFinished() {
					b.VisitLocalFunctionDeclaration(declaration)
				}
			}
		})
	}
	for _, member := range i.AllNamespace_member_declaration() {
		b.VisitNamespaceMemberDeclaration(member)
	}
}

func (b *singleFileBuilder) VisitUsingDirective(raw csharpparser.IUsing_directiveContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Using_directiveContext)
	if !ok || i == nil {
		return
	}
	isGlobal := i.KW_GLOBAL() != nil
	switch {
	case i.Using_namespace_directive() != nil:
		ns, _ := i.Using_namespace_directive().(*csharpparser.Using_namespace_directiveContext)
		if ns != nil && ns.Namespace_name() != nil {
			path := ns.Namespace_name().GetText()
			if isGlobal {
				b.globalUsings.addNamespace(path)
			} else {
				b.recordUsing(path)
			}
		}
	case i.Using_alias_directive() != nil:
		alias, _ := i.Using_alias_directive().(*csharpparser.Using_alias_directiveContext)
		if alias == nil {
			return
		}
		name := identText(alias.Identifier())
		target := ""
		if alias.Namespace_or_type_name() != nil {
			target = alias.Namespace_or_type_name().GetText()
		}
		if name == "" || target == "" {
			return
		}
		aliasNode := ssa.DetachAST(alias.Namespace_or_type_name())
		if isGlobal {
			b.globalUsings.addAlias(name, target, aliasNode)
		} else {
			b.usingAlias[name] = target
			b.usingAliasTypes[name] = aliasNode
		}
		// alias 指向已存在的类型时导出，方便 `Alias.Member` 直接解析
		short := lastDotSegment(stripGenericSuffix(target))
		if bp := b.GetBluePrint(short); bp != nil {
			b.GetProgram().SetExportType(name, bp)
		}
	case i.Using_static_directive() != nil:
		st, _ := i.Using_static_directive().(*csharpparser.Using_static_directiveContext)
		if st != nil && st.Type_name() != nil {
			path := st.Type_name().GetText()
			if isGlobal {
				b.globalUsings.addStatic(path)
			} else {
				b.recordUsingStatic(path)
			}
		}
	}
}

func (b *singleFileBuilder) recordUsingStatic(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, exist := range b.usingStatics {
		if exist == path {
			return
		}
	}
	b.usingStatics = append(b.usingStatics, path)
}

func (b *singleFileBuilder) recordUsing(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	for _, exist := range b.usings {
		if exist == path {
			return
		}
	}
	b.usings = append(b.usings, path)
}

func (b *singleFileBuilder) VisitNamespaceMemberDeclaration(raw csharpparser.INamespace_member_declarationContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Namespace_member_declarationContext)
	if !ok || i == nil {
		return
	}
	if ns := i.Namespace_declaration(); ns != nil {
		b.VisitNamespaceDeclaration(ns)
		return
	}
	if td := i.Type_declaration(); td != nil {
		b.VisitTypeDeclaration(td, nil)
	}
}

func (b *singleFileBuilder) VisitNamespaceDeclaration(raw csharpparser.INamespace_declarationContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Namespace_declarationContext)
	if !ok || i == nil {
		return
	}
	pkgPath := append([]string(nil), b.selfPkgPath...)
	if qi := i.Qualified_identifier(); qi != nil {
		q, _ := qi.(*csharpparser.Qualified_identifierContext)
		if q != nil {
			for _, id := range q.AllIdentifier() {
				pkgPath = append(pkgPath, identText(id))
			}
		}
	}
	pkgName := strings.Join(pkgPath, ".")
	prog := b.GetProgram()
	lib, _ := prog.GetLibrary(pkgName)
	if lib == nil {
		lib = prog.NewLibrary(pkgName, pkgPath)
	}
	lib.PushEditor(prog.GetApplication().GetCurrentEditor())
	builder := lib.GetAndCreateFunctionBuilder(pkgName, string(ssa.MainFunctionName))
	if builder != nil {
		builder.SetEditor(prog.GetApplication().GetCurrentEditor())
		builder.SetBuildSupport(b.FunctionBuilder)
		current := b.FunctionBuilder
		b.FunctionBuilder = builder
		defer func() { b.FunctionBuilder = current }()
	}
	prevPkg := append([]string(nil), b.selfPkgPath...)
	prevUsings := append([]string(nil), b.usings...)
	prevStatics := append([]string(nil), b.usingStatics...)
	prevAlias := cloneUsingAliases(b.usingAlias)
	prevAliasTypes := cloneUsingAliasTypes(b.usingAliasTypes)
	b.selfPkgPath = append([]string(nil), pkgPath...)
	b.usings = append([]string(nil), prevUsings...)
	b.usingStatics = append([]string(nil), prevStatics...)
	b.usingAlias = cloneUsingAliases(prevAlias)
	b.usingAliasTypes = cloneUsingAliasTypes(prevAliasTypes)
	defer func() {
		b.selfPkgPath = append([]string(nil), prevPkg...)
		b.usings = append([]string(nil), prevUsings...)
		b.usingStatics = append([]string(nil), prevStatics...)
		b.usingAlias = cloneUsingAliases(prevAlias)
		b.usingAliasTypes = cloneUsingAliasTypes(prevAliasTypes)
	}()

	if body, _ := i.Namespace_body().(*csharpparser.Namespace_bodyContext); body != nil {
		for _, using := range body.AllUsing_directive() {
			b.VisitUsingDirective(using)
		}
		for _, member := range body.AllNamespace_member_declaration() {
			b.VisitNamespaceMemberDeclaration(member)
		}
		return
	}
	// C# 10 file-scoped namespace: using directives and declarations are direct
	// children of namespace_declaration rather than a namespace_body.
	for _, using := range i.AllUsing_directive() {
		b.VisitUsingDirective(using)
	}
	for _, member := range i.AllNamespace_member_declaration() {
		b.VisitNamespaceMemberDeclaration(member)
	}
}

// VisitTypeDeclaration dispatches class/struct/interface/enum/delegate.
// outer 非空时表示嵌套类型，蓝图名会带上外部类前缀。
func (b *singleFileBuilder) VisitTypeDeclaration(raw csharpparser.IType_declarationContext, outer *ssa.Blueprint) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Type_declarationContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Class_declaration() != nil:
		b.VisitClassDeclaration(i.Class_declaration(), outer)
	case i.Struct_declaration() != nil:
		b.VisitStructDeclaration(i.Struct_declaration(), outer)
	case i.Interface_declaration() != nil:
		b.VisitInterfaceDeclaration(i.Interface_declaration(), outer)
	case i.Enum_declaration() != nil:
		b.VisitEnumDeclaration(i.Enum_declaration(), outer)
	case i.Delegate_declaration() != nil:
		b.VisitDelegateDeclaration(i.Delegate_declaration(), outer)
	}
}

func lastDotSegment(s string) string {
	if idx := strings.LastIndex(s, "."); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// stripGenericSuffix 去掉 `List<string>` 中的泛型参数，仅保留 `List`
func stripGenericSuffix(s string) string {
	if idx := strings.Index(s, "<"); idx >= 0 {
		return s[:idx]
	}
	return s
}
