package php2ssa

import (
	"strings"

	"github.com/yaklang/antlr/v4"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// phpNamespacePass2 holds detached nodes required inside a namespace for pass2 only.
type phpNamespacePass2 struct {
	name       string
	path       []string
	uses       []antlr.Tree
	functions  []antlr.Tree
	classes    []antlr.Tree
	globals    []antlr.Tree
	statements []antlr.Tree
	enums      []antlr.Tree
}

func (n *phpNamespacePass2) empty() bool {
	if n == nil {
		return true
	}
	return len(n.uses) == 0 &&
		len(n.functions) == 0 &&
		len(n.classes) == 0 &&
		len(n.globals) == 0 &&
		len(n.statements) == 0 &&
		len(n.enums) == 0
}

func (n *phpNamespacePass2) release() {
	if n == nil {
		return
	}
	n.name = ""
	n.path = nil
	n.uses = nil
	n.functions = nil
	n.classes = nil
	n.globals = nil
	n.statements = nil
	n.enums = nil
}

// phpPass2Capture holds detached AST nodes required for pass2 top-level work only.
// Function/class skeletons are emitted in pass1 and must not be retained here.
type phpPass2Capture struct {
	namespaces     []phpNamespacePass2
	namespaceDecls []antlr.Tree
	uses           []antlr.Tree
	globals        []antlr.Tree
	statements     []antlr.Tree
	enums          []antlr.Tree
}

func (c *phpPass2Capture) empty() bool {
	if c == nil {
		return true
	}
	if len(c.uses) > 0 || len(c.globals) > 0 || len(c.statements) > 0 || len(c.enums) > 0 {
		return false
	}
	for i := range c.namespaces {
		if !c.namespaces[i].empty() {
			return false
		}
	}
	return true
}

func (c *phpPass2Capture) release() {
	if c == nil {
		return
	}
	for i := range c.namespaces {
		c.namespaces[i].release()
	}
	c.namespaces = nil
	c.namespaceDecls = nil
	c.uses = nil
	c.globals = nil
	c.statements = nil
	c.enums = nil
}

func collectPHPFilePass2Capture(ast phpparser.IHtmlDocumentContext) *phpPass2Capture {
	html, ok := ast.(*phpparser.HtmlDocumentContext)
	if !ok || html == nil {
		return nil
	}
	capture := &phpPass2Capture{}
	var walk func(antlr.Tree)
	walk = func(node antlr.Tree) {
		if node == nil {
			return
		}
		if block, ok := node.(*phpparser.PhpBlockContext); ok {
			appendPHPBlockPass2Nodes(capture, block)
			return
		}
		rule, ok := node.(antlr.ParserRuleContext)
		if !ok {
			return
		}
		for _, child := range rule.GetChildren() {
			if tree, ok := child.(antlr.Tree); ok {
				walk(tree)
			}
		}
	}
	for _, child := range html.GetChildren() {
		if tree, ok := child.(antlr.Tree); ok {
			walk(tree)
		}
	}
	if capture.empty() {
		return nil
	}
	return capture
}

func appendPHPBlockPass2Nodes(capture *phpPass2Capture, block *phpparser.PhpBlockContext) {
	if capture == nil || block == nil {
		return
	}
	for _, ns := range block.AllNamespaceDeclaration() {
		appendNamespacePass2Nodes(capture, ns)
	}
	for _, useDecl := range block.AllUseDeclaration() {
		if useDecl != nil {
			capture.uses = append(capture.uses, ssa.DetachAST(useDecl))
		}
	}
	for _, global := range block.AllGlobalConstantDeclaration() {
		if global != nil {
			capture.globals = append(capture.globals, ssa.DetachAST(global))
		}
	}
	for _, stmt := range block.AllStatement() {
		if stmt != nil {
			capture.statements = append(capture.statements, ssa.DetachAST(stmt))
		}
	}
	for _, enumDecl := range block.AllEnumDeclaration() {
		if enumDecl != nil {
			capture.enums = append(capture.enums, ssa.DetachAST(enumDecl))
		}
	}
}

func appendNamespacePass2Nodes(capture *phpPass2Capture, ns phpparser.INamespaceDeclarationContext) {
	if capture == nil || ns == nil {
		return
	}
	n, ok := ns.(*phpparser.NamespaceDeclarationContext)
	if !ok || n == nil {
		return
	}
	entry := phpNamespacePass2{
		path: extractNamespacePath(n.NamespacePath()),
	}
	entry.name = strings.Join(entry.path, ".")
	appendNamespaceStatementPass2Nodes(&entry, n.AllNamespaceStatement())
	if entry.empty() {
		return
	}
	capture.namespaces = append(capture.namespaces, entry)
	capture.namespaceDecls = append(capture.namespaceDecls, ssa.DetachAST(ns))
}

func appendNamespaceStatementPass2Nodes(entry *phpNamespacePass2, stmts []phpparser.INamespaceStatementContext) {
	if entry == nil {
		return
	}
	// Unnamed namespaces replay declareStatement in pass2; named ones only need statements.
	captureDecl := entry.name == ""
	for _, stmt := range stmts {
		nsc, ok := stmt.(*phpparser.NamespaceStatementContext)
		if !ok {
			continue
		}
		if useDecl := nsc.UseDeclaration(); useDecl != nil {
			entry.uses = append(entry.uses, ssa.DetachAST(useDecl))
		}
		if captureDecl {
			if fn := nsc.FunctionDeclaration(); fn != nil {
				entry.functions = append(entry.functions, ssa.DetachAST(fn))
			}
			if cls := nsc.ClassDeclaration(); cls != nil {
				entry.classes = append(entry.classes, ssa.DetachAST(cls))
			}
		}
		if global := nsc.GlobalConstantDeclaration(); global != nil {
			entry.globals = append(entry.globals, ssa.DetachAST(global))
		}
		if statement := nsc.Statement(); statement != nil {
			entry.statements = append(entry.statements, ssa.DetachAST(statement))
		}
		if enumDecl := nsc.EnumDeclaration(); enumDecl != nil {
			entry.enums = append(entry.enums, ssa.DetachAST(enumDecl))
		}
	}
}

func extractNamespacePath(raw phpparser.INamespacePathContext) []string {
	if raw == nil {
		return nil
	}
	pathCtx, ok := raw.(*phpparser.NamespacePathContext)
	if !ok || pathCtx == nil {
		return nil
	}
	ids := pathCtx.AllIdentifier()
	path := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			continue
		}
		path = append(path, id.GetText())
	}
	return path
}

func visitPHPFilePass2Capture(functionBuilder *ssa.FunctionBuilder, callbackBuilder *ssa.FunctionBuilder, capture *phpPass2Capture) {
	if capture == nil || capture.empty() {
		return
	}
	build := newPHPFileBuilder(functionBuilder, callbackBuilder)
	prog := build.GetProgram()
	if prog != nil && prog.CurrentIncludingStack.Len() <= 0 {
		for _, raw := range capture.namespaceDecls {
			if ns, ok := raw.(phpparser.INamespaceDeclarationContext); ok {
				build.VisitNamespaceOnlyUse(ns)
			}
		}
		for _, raw := range capture.namespaceDecls {
			if ns, ok := raw.(phpparser.INamespaceDeclarationContext); ok {
				build.VisitNamespaceDeclaration(ns)
			}
		}
	}
	for _, raw := range capture.uses {
		if useDecl, ok := raw.(phpparser.IUseDeclarationContext); ok {
			build.VisitUseDeclaration(useDecl)
		}
	}
	for _, raw := range capture.globals {
		if global, ok := raw.(phpparser.IGlobalConstantDeclarationContext); ok {
			build.VisitGlobalConstantDeclaration(global)
		}
	}
	for _, raw := range capture.statements {
		if stmt, ok := raw.(phpparser.IStatementContext); ok {
			build.VisitStatement(stmt)
		}
	}
	for _, raw := range capture.enums {
		if enumDecl, ok := raw.(phpparser.IEnumDeclarationContext); ok {
			build.VisitEnumDeclaration(enumDecl)
		}
	}
	build.Finish()
}

func (y *builder) visitNamespacePass2OnlyUse(entry *phpNamespacePass2) {
	if y == nil || entry == nil || entry.empty() {
		return
	}
	if len(entry.uses) == 0 {
		return
	}
	visitUses := func() {
		for _, raw := range entry.uses {
			if useDecl, ok := raw.(phpparser.IUseDeclarationContext); ok {
				y.VisitUseDeclaration(useDecl)
			}
		}
	}
	if entry.name == "" {
		visitUses()
		return
	}
	prog := y.GetProgram().GetApplication()
	library, ok := prog.GetLibrary(entry.name)
	if !ok {
		return
	}
	functionBuilder := library.GetAndCreateFunctionBuilder(entry.name, string(ssa.InitFunctionName))
	currentBuilder := y.FunctionBuilder
	y.FunctionBuilder = functionBuilder
	defer func() {
		y.FunctionBuilder = currentBuilder
	}()
	visitUses()
}

func (y *builder) visitNamespacePass2Declaration(entry *phpNamespacePass2) {
	if y == nil || entry == nil || entry.empty() {
		return
	}
	hasName := len(entry.path) > 0
	prog := y.GetProgram().GetApplication()

	switch {
	case hasName:
		namespaceName := entry.name
		library, _ := prog.GetLibrary(namespaceName)
		if library == nil {
			library = prog.NewLibrary(namespaceName, []string{prog.Loader.GetBasePath()})
		}
		library.PushEditor(prog.GetCurrentEditor())
		functionBuilder := library.GetAndCreateFunctionBuilder(namespaceName, string(ssa.InitFunctionName))
		functionBuilder.SetEditor(y.FunctionBuilder.GetEditor())
		functionBuilder.SetBuildSupport(y.FunctionBuilder)
		currentBuilder := y.FunctionBuilder
		y.FunctionBuilder = functionBuilder
		defer func() {
			y.FunctionBuilder = currentBuilder
		}()

		currentProg := y.GetProgram()
		y.SetProgram(library)
		defer y.SetProgram(currentProg)

		for _, raw := range entry.statements {
			if stmt, ok := raw.(phpparser.IStatementContext); ok {
				y.VisitStatement(stmt)
			}
		}
	default:
		prog.PkgName = entry.name
		for _, raw := range entry.functions {
			if fn, ok := raw.(phpparser.IFunctionDeclarationContext); ok {
				y.VisitFunctionDeclaration(fn)
			}
		}
		for _, raw := range entry.classes {
			if cls, ok := raw.(phpparser.IClassDeclarationContext); ok {
				y.VisitClassDeclaration(cls)
			}
		}
		for _, raw := range entry.globals {
			if global, ok := raw.(phpparser.IGlobalConstantDeclarationContext); ok {
				y.VisitGlobalConstantDeclaration(global)
			}
		}
		for _, raw := range entry.enums {
			if enumDecl, ok := raw.(phpparser.IEnumDeclarationContext); ok {
				y.VisitEnumDeclaration(enumDecl)
			}
		}
		for _, raw := range entry.statements {
			if stmt, ok := raw.(phpparser.IStatementContext); ok {
				y.VisitStatement(stmt)
			}
		}
	}
}
