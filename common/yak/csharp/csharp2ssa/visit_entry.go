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
	if !b.PreHandler() {
		return
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
	switch {
	case i.Using_namespace_directive() != nil:
		ns := i.Using_namespace_directive().(*csharpparser.Using_namespace_directiveContext)
		if ns.Namespace_name() != nil {
			b.recordUsing(ns.Namespace_name().GetText())
		}
	case i.Using_alias_directive() != nil:
		alias := i.Using_alias_directive().(*csharpparser.Using_alias_directiveContext)
		name := identText(alias.Identifier())
		target := ""
		if alias.Namespace_or_type_name() != nil {
			target = alias.Namespace_or_type_name().GetText()
		}
		if name != "" {
			b.recordUsing(name)
			if bp := b.GetBluePrint(target); bp != nil {
				b.GetProgram().SetExportType(name, bp)
			}
		}
	case i.Using_static_directive() != nil:
		st := i.Using_static_directive().(*csharpparser.Using_static_directiveContext)
		if st.Type_name() != nil {
			b.recordUsing(st.Type_name().GetText())
		}
	}
}

func (b *singleFileBuilder) recordUsing(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	parts := strings.Split(path, ".")
	b.selfPkgPath = append([]string{}, b.selfPkgPath...)
	_ = parts
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
		b.VisitTypeDeclaration(td)
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
	pkgPath := []string{}
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
	prev := b.selfPkgPath
	b.selfPkgPath = pkgPath
	defer func() { b.selfPkgPath = prev }()

	body, _ := i.Namespace_body().(*csharpparser.Namespace_bodyContext)
	if body == nil {
		return
	}
	for _, using := range body.AllUsing_directive() {
		b.VisitUsingDirective(using)
	}
	for _, member := range body.AllNamespace_member_declaration() {
		b.VisitNamespaceMemberDeclaration(member)
	}
}

func (b *singleFileBuilder) VisitTypeDeclaration(raw csharpparser.IType_declarationContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	i, ok := raw.(*csharpparser.Type_declarationContext)
	if !ok || i == nil {
		return
	}
	switch {
	case i.Class_declaration() != nil:
		b.VisitClassDeclaration(i.Class_declaration(), nil)
	case i.Struct_declaration() != nil:
		b.VisitStructDeclaration(i.Struct_declaration())
	case i.Interface_declaration() != nil:
		b.VisitInterfaceDeclaration(i.Interface_declaration())
	case i.Enum_declaration() != nil:
		b.VisitEnumDeclaration(i.Enum_declaration())
	}
}
