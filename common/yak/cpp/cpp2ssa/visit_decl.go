package cpp2ssa

import (
	"fmt"
	"strings"

	cppparser "github.com/yaklang/yaklang/common/yak/cpp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// paramInfo is one formal parameter: a name (may be empty) and its type.
type paramInfo struct {
	name string
	typ  ssa.Type
}

// declaratorInfo is the meaning of a C++ declarator: the declared name, its
// class scope when qualified, the resolved type, and the parameter list when the
// declarator is a function.
type declaratorInfo struct {
	name       string
	scope      []string
	typ        ssa.Type
	params     []paramInfo
	isFunction bool
	isStatic   bool
}

func (b *singleFileBuilder) visitTranslationUnit(raw cppparser.ITranslationUnitContext) {
	tu, ok := raw.(*cppparser.TranslationUnitContext)
	if !ok || tu == nil {
		return
	}
	b.visitDeclarationSeq(tu.DeclarationSeq())
}

func (b *singleFileBuilder) visitDeclarationSeq(raw cppparser.IDeclarationSeqContext) {
	seq, ok := raw.(*cppparser.DeclarationSeqContext)
	if !ok || seq == nil {
		return
	}
	for _, declaration := range seq.AllDeclaration() {
		b.visitDeclaration(declaration)
		if b.IsStop() {
			return
		}
	}
}

func (b *singleFileBuilder) visitDeclaration(raw cppparser.IDeclarationContext) {
	declaration, ok := raw.(*cppparser.DeclarationContext)
	if !ok || declaration == nil {
		return
	}
	switch {
	case declaration.BlockDeclaration() != nil:
		b.visitBlockDeclaration(declaration.BlockDeclaration())
	case declaration.FunctionDefinition() != nil:
		b.visitFunctionDefinition(declaration.FunctionDefinition(), b.class)
	case declaration.TemplateDeclaration() != nil:
		b.visitTemplateDeclaration(declaration.TemplateDeclaration())
	case declaration.ExplicitInstantiation() != nil:
		if explicit, ok := declaration.ExplicitInstantiation().(*cppparser.ExplicitInstantiationContext); ok && explicit != nil {
			b.visitDeclaration(explicit.Declaration())
		}
	case declaration.ExplicitSpecialization() != nil:
		if explicit, ok := declaration.ExplicitSpecialization().(*cppparser.ExplicitSpecializationContext); ok && explicit != nil {
			b.visitDeclaration(explicit.Declaration())
		}
	case declaration.NamespaceDefinition() != nil:
		b.visitNamespaceDefinition(declaration.NamespaceDefinition())
	case declaration.LinkageSpecification() != nil:
		b.visitLinkageSpecification(declaration.LinkageSpecification())
	}
}

func (b *singleFileBuilder) visitBlockDeclaration(raw cppparser.IBlockDeclarationContext) {
	declaration, ok := raw.(*cppparser.BlockDeclarationContext)
	if !ok || declaration == nil {
		return
	}
	switch {
	case declaration.SimpleDeclaration() != nil:
		b.visitSimpleDeclaration(declaration.SimpleDeclaration())
	case declaration.UsingDeclaration() != nil:
		b.visitUsingDeclaration(declaration.UsingDeclaration())
	case declaration.UsingDirective() != nil:
		b.visitUsingDirective(declaration.UsingDirective())
	case declaration.AliasDeclaration() != nil:
		b.visitAliasDeclaration(declaration.AliasDeclaration())
	case declaration.NamespaceAliasDefinition() != nil:
		b.visitNamespaceAlias(declaration.NamespaceAliasDefinition())
	}
}

// visitSimpleDeclaration handles variable declarations, typedefs, prototypes and
// inline class/struct definitions such as `struct Point { int x; } p;`.
func (b *singleFileBuilder) visitSimpleDeclaration(raw cppparser.ISimpleDeclarationContext) {
	declaration, ok := raw.(*cppparser.SimpleDeclarationContext)
	if !ok || declaration == nil {
		return
	}
	info := b.typeSpecifierInfo(declaration.DeclSpecifierSeq())
	list := declaration.InitDeclaratorList()
	if list == nil {
		return
	}
	declarators, ok := list.(*cppparser.InitDeclaratorListContext)
	if !ok || declarators == nil {
		return
	}
	for _, rawInit := range declarators.AllInitDeclarator() {
		init, ok := rawInit.(*cppparser.InitDeclaratorContext)
		if !ok || init == nil || init.Declarator() == nil {
			continue
		}
		resolved := b.parseDeclarator(init.Declarator(), info.typ)
		resolved.isStatic = info.isStatic
		if info.isTypedef {
			b.recordTypeAlias(resolved.name, info)
			continue
		}
		if resolved.isFunction {
			b.declareFunction(resolved, b.class, info)
			continue
		}
		value := b.declareValue(resolved, info)
		if initializer := init.Initializer(); initializer != nil {
			if assigned := b.visitInitializer(initializer, resolved.typ); assigned != nil {
				value = assigned
				value.SetType(resolved.typ)
			}
		}
		b.AssignVariable(b.CreateVariable(resolved.name), value)
		if info.isStatic && b.class != nil {
			b.class.RegisterStaticMember(resolved.name, value)
		}
	}
}

func (b *singleFileBuilder) recordTypeAlias(name string, info declTypeInfo) {
	if name == "" {
		return
	}
	if info.className != "" {
		b.typeAliases[name] = info.className
		return
	}
	b.typeAliases[name] = info.text
}

// declareValue creates an undefined value for a declaration without initializer.
func (b *singleFileBuilder) declareValue(resolved declaratorInfo, info declTypeInfo) ssa.Value {
	value := b.EmitUndefined(resolved.name)
	value.SetType(resolved.typ)
	if resolved.typ != nil {
		value.SetVerboseName(fmt.Sprintf("%s %s", info.text, resolved.name))
	}
	return value
}

// visitInitializer evaluates `= expr`, `(expr)`, `{...}` and `{x, y}`.
func (b *singleFileBuilder) visitInitializer(raw cppparser.IInitializerContext, typ ssa.Type) ssa.Value {
	initializer, ok := raw.(*cppparser.InitializerContext)
	if !ok || initializer == nil {
		return nil
	}
	if braceOrEqual := initializer.BraceOrEqualInitializer(); braceOrEqual != nil {
		return b.visitBraceOrEqualInitializer(braceOrEqual, typ)
	}
	if list := initializer.ExpressionList(); list != nil {
		values := b.visitExpressionList(list)
		if len(values) > 0 {
			return values[0]
		}
	}
	return nil
}

func (b *singleFileBuilder) visitBraceOrEqualInitializer(raw cppparser.IBraceOrEqualInitializerContext, typ ssa.Type) ssa.Value {
	initializer, ok := raw.(*cppparser.BraceOrEqualInitializerContext)
	if !ok || initializer == nil {
		return nil
	}
	if clause := initializer.InitializerClause(); clause != nil {
		return b.evalNode(clause)
	}
	if braced := initializer.BracedInitList(); braced != nil {
		return b.visitBracedInitList(braced, typ)
	}
	return nil
}

// visitBracedInitList lowers a brace initializer. A single element keeps its
// value and type; multiple elements become a slice value.
func (b *singleFileBuilder) visitBracedInitList(raw cppparser.IBracedInitListContext, typ ssa.Type) ssa.Value {
	braced, ok := raw.(*cppparser.BracedInitListContext)
	if !ok || braced == nil {
		return b.EmitConstInstNil()
	}
	list := braced.InitializerList()
	if list == nil {
		value := b.EmitUndefined("braced-init")
		if typ != nil {
			value.SetType(typ)
		}
		return value
	}
	values := b.visitInitializerList(list)
	if len(values) == 1 {
		return values[0]
	}
	sliceType := ssa.NewSliceType(ssa.CreateAnyType())
	if typ != nil {
		sliceType = ssa.NewSliceType(typ)
	}
	value := b.EmitUndefined("braced-init-list")
	value.SetType(sliceType)
	_ = values
	return value
}

func (b *singleFileBuilder) visitInitializerList(raw cppparser.IInitializerListContext) []ssa.Value {
	list, ok := raw.(*cppparser.InitializerListContext)
	if !ok || list == nil {
		return nil
	}
	values := make([]ssa.Value, 0, len(list.AllInitializerClause()))
	for _, rawClause := range list.AllInitializerClause() {
		if value := b.evalNode(rawClause); value != nil {
			values = append(values, value)
		}
	}
	return values
}

// visitNamespaceDefinition opens a namespace scope. The namespace path prefixes
// every function and class declared inside so qualified lookups stay distinct.
func (b *singleFileBuilder) visitNamespaceDefinition(raw cppparser.INamespaceDefinitionContext) {
	definition, ok := raw.(*cppparser.NamespaceDefinitionContext)
	if !ok || definition == nil {
		return
	}
	// namespace a::b { } contributes one scope per name.
	var names []string
	for _, identifier := range definition.AllIdentifier() {
		if identifier != nil {
			names = append(names, identifier.GetText())
		}
	}
	if len(names) > 0 {
		b.namespace = append(b.namespace, names...)
	}
	b.visitDeclarationSeq(definition.DeclarationSeq())
	if len(names) > 0 {
		b.namespace = b.namespace[:len(b.namespace)-len(names)]
	}
}

// visitLinkageSpecification builds the declarations of `extern "C" { ... }`.
func (b *singleFileBuilder) visitLinkageSpecification(raw cppparser.ILinkageSpecificationContext) {
	specification, ok := raw.(*cppparser.LinkageSpecificationContext)
	if !ok || specification == nil {
		return
	}
	b.visitDeclarationSeq(specification.DeclarationSeq())
	if declaration := specification.Declaration(); declaration != nil {
		b.visitDeclaration(declaration)
	}
}

func (b *singleFileBuilder) visitUsingDirective(raw cppparser.IUsingDirectiveContext) {
	directive, ok := raw.(*cppparser.UsingDirectiveContext)
	if !ok || directive == nil {
		return
	}
	name := ""
	if namespaceName := directive.NamespaceName(); namespaceName != nil {
		name = strings.TrimPrefix(namespaceName.GetText(), "::")
	}
	if name != "" {
		b.usingNamespaces = append(b.usingNamespaces, name)
	}
}

func (b *singleFileBuilder) visitUsingDeclaration(raw cppparser.IUsingDeclarationContext) {
	declaration, ok := raw.(*cppparser.UsingDeclarationContext)
	if !ok || declaration == nil {
		return
	}
	unqualified := declaration.UnqualifiedId()
	if unqualified == nil {
		return
	}
	name := unqualified.GetText()
	if name == "" {
		return
	}
	full := name
	if nested := declaration.NestedNameSpecifier(); nested != nil {
		full = nested.GetText() + name
	}
	b.typeAliases[name] = full
}

func (b *singleFileBuilder) visitAliasDeclaration(raw cppparser.IAliasDeclarationContext) {
	declaration, ok := raw.(*cppparser.AliasDeclarationContext)
	if !ok || declaration == nil || declaration.Identifier() == nil {
		return
	}
	name := declaration.Identifier().GetText()
	if typeID := declaration.TheTypeId(); typeID != nil {
		b.typeAliases[name] = typeID.GetText()
	}
}

func (b *singleFileBuilder) visitNamespaceAlias(raw cppparser.INamespaceAliasDefinitionContext) {
	alias, ok := raw.(*cppparser.NamespaceAliasDefinitionContext)
	if !ok || alias == nil || alias.Identifier() == nil {
		return
	}
	if target := alias.QualifiedNamespaceSpecifier(); target != nil {
		b.typeAliases[alias.Identifier().GetText()] = strings.TrimPrefix(target.GetText(), "::")
	}
}

// visitTemplateDeclaration records the template parameters and then builds the
// declaration they parameterize. Template instantiation is not performed: the
// declared function or class is built once with placeholder parameter types.
func (b *singleFileBuilder) visitTemplateDeclaration(raw cppparser.ITemplateDeclarationContext) {
	declaration, ok := raw.(*cppparser.TemplateDeclarationContext)
	if !ok || declaration == nil {
		return
	}
	saved := b.templateTypes
	b.templateTypes = make(map[string]ssa.Type, len(saved)+2)
	for name, typ := range saved {
		b.templateTypes[name] = typ
	}
	if list, ok := declaration.TemplateParameterList().(*cppparser.TemplateParameterListContext); ok && list != nil {
		for _, rawParameter := range list.AllTemplateParameter() {
			parameter, ok := rawParameter.(*cppparser.TemplateParameterContext)
			if !ok || parameter == nil {
				continue
			}
			switch {
			case parameter.TypeParameter() != nil:
				if typeParameter, ok := parameter.TypeParameter().(*cppparser.TypeParameterContext); ok && typeParameter != nil && typeParameter.Identifier() != nil {
					b.templateTypes[typeParameter.Identifier().GetText()] = ssa.CreateAnyType()
				}
			case parameter.ParameterDeclaration() != nil:
				if declaration, ok := parameter.ParameterDeclaration().(*cppparser.ParameterDeclarationContext); ok && declaration != nil {
					resolved := b.parseParameterDeclaration(declaration)
					if resolved.name != "" {
						b.templateTypes[resolved.name] = resolved.typ
					}
				}
			}
		}
	}
	if inner := declaration.Declaration(); inner != nil {
		b.visitDeclaration(inner)
	}
	b.templateTypes = saved
}

// visitClassSpecifier builds a class, struct or union blueprint and its members,
// and relates base classes through the blueprint parent list.
func (b *singleFileBuilder) visitClassSpecifier(raw cppparser.IClassSpecifierContext) *ssa.Blueprint {
	specifier, ok := raw.(*cppparser.ClassSpecifierContext)
	if !ok || specifier == nil {
		return nil
	}
	head, ok := specifier.ClassHead().(*cppparser.ClassHeadContext)
	if !ok || head == nil {
		return nil
	}
	name := ""
	if headName, ok := head.ClassHeadName().(*cppparser.ClassHeadNameContext); ok && headName != nil {
		name = normalizeTypeName(headName.GetText())
	}
	if name == "" {
		name = fmt.Sprintf("anonymous_%d", specifier.GetStart().GetStart())
	}
	if b.class != nil && !strings.Contains(name, "::") {
		name = b.class.Name + "::" + name
	}
	blueprint := b.GetBluePrint(name)
	if blueprint == nil {
		blueprint = b.CreateBlueprint(name)
	}
	blueprint.SetKind(ssa.BlueprintClass)
	b.GetProgram().SetExportType(name, blueprint)

	if baseClause, ok := head.BaseClause().(*cppparser.BaseClauseContext); ok && baseClause != nil {
		b.linkBaseClasses(blueprint, baseClause)
	}

	savedClass := b.class
	b.class = blueprint
	b.MarkedThisClassBlueprint = blueprint
	b.PushBlueprint(blueprint)
	if members, ok := specifier.MemberSpecification().(*cppparser.MemberSpecificationContext); ok && members != nil {
		for _, rawMember := range members.AllMemberDeclaration() {
			b.visitMemberDeclaration(rawMember, blueprint)
		}
	}
	b.PopBlueprint()
	b.MarkedThisClassBlueprint = nil
	b.class = savedClass
	return blueprint
}

func (b *singleFileBuilder) linkBaseClasses(blueprint *ssa.Blueprint, clause *cppparser.BaseClauseContext) {
	list, ok := clause.BaseSpecifierList().(*cppparser.BaseSpecifierListContext)
	if !ok || list == nil {
		return
	}
	for _, rawSpecifier := range list.AllBaseSpecifier() {
		specifier, ok := rawSpecifier.(*cppparser.BaseSpecifierContext)
		if !ok || specifier == nil || specifier.BaseTypeSpecifier() == nil {
			continue
		}
		parentName := normalizeTypeName(specifier.BaseTypeSpecifier().GetText())
		if parentName == "" {
			continue
		}
		parent := b.GetBluePrint(parentName)
		if parent == nil {
			parent = b.CreateBlueprint(parentName)
		}
		parent.SetKind(ssa.BlueprintClass)
		blueprint.AddParentBlueprint(parent)
	}
}

func (b *singleFileBuilder) visitMemberDeclaration(raw cppparser.IMemberDeclarationContext, blueprint *ssa.Blueprint) {
	member, ok := raw.(*cppparser.MemberDeclarationContext)
	if !ok || member == nil {
		return
	}
	switch {
	case member.FunctionDefinition() != nil:
		b.visitFunctionDefinition(member.FunctionDefinition(), blueprint)
	case member.TemplateDeclaration() != nil:
		b.visitTemplateDeclaration(member.TemplateDeclaration())
	case member.UsingDeclaration() != nil:
		b.visitUsingDeclaration(member.UsingDeclaration())
	case member.AliasDeclaration() != nil:
		b.visitAliasDeclaration(member.AliasDeclaration())
	}
	list, ok := member.MemberDeclaratorList().(*cppparser.MemberDeclaratorListContext)
	if !ok || list == nil {
		return
	}
	info := b.typeSpecifierInfo(member.DeclSpecifierSeq())
	for _, rawDeclarator := range list.AllMemberDeclarator() {
		declarator, ok := rawDeclarator.(*cppparser.MemberDeclaratorContext)
		if !ok || declarator == nil || declarator.Declarator() == nil {
			continue
		}
		if constant := declarator.ConstantExpression(); constant != nil {
			continue
		}
		resolved := b.parseDeclarator(declarator.Declarator(), info.typ)
		resolved.isStatic = info.isStatic
		if resolved.name == "" {
			continue
		}
		if resolved.isFunction {
			b.declareFunction(resolved, blueprint, info)
			continue
		}
		value := b.declareValue(resolved, info)
		if initializer := declarator.BraceOrEqualInitializer(); initializer != nil {
			if assigned := b.visitBraceOrEqualInitializer(initializer, resolved.typ); assigned != nil {
				value = assigned
				value.SetType(resolved.typ)
			}
		}
		if info.isStatic {
			blueprint.RegisterStaticMember(resolved.name, value)
		} else {
			blueprint.RegisterNormalMember(resolved.name, value)
		}
	}
}

// visitFunctionDefinition builds one function or member function body.
func (b *singleFileBuilder) visitFunctionDefinition(raw cppparser.IFunctionDefinitionContext, class *ssa.Blueprint) *ssa.Function {
	definition, ok := raw.(*cppparser.FunctionDefinitionContext)
	if !ok || definition == nil || definition.Declarator() == nil {
		return nil
	}
	info := b.typeSpecifierInfo(definition.DeclSpecifierSeq())
	resolved := b.parseDeclarator(definition.Declarator(), info.typ)
	resolved.isStatic = info.isStatic
	if target := b.classFromScope(resolved.scope); target != nil {
		class = target
	}
	return b.buildFunction(resolved, class, definition.FunctionBody())
}

func (b *singleFileBuilder) classFromScope(scope []string) *ssa.Blueprint {
	if len(scope) == 0 {
		return nil
	}
	name := normalizeTypeName(scope[len(scope)-1])
	if name == "" {
		return nil
	}
	return b.GetBluePrint(name)
}

// declareFunction registers a prototype without building a body, so calls to it
// still bind to a callable.
func (b *singleFileBuilder) declareFunction(resolved declaratorInfo, class *ssa.Blueprint, info declTypeInfo) *ssa.Function {
	fullName := b.functionFullName(resolved, class)
	fn := b.NewFunc(fullName)
	fn.SetMethodName(resolved.name)
	b.registerFunction(fn, resolved.name, class, resolved.isStatic || info.isStatic)
	b.GetProgram().SetExportValue(fullName, fn)
	return fn
}

// functionFullName prefers the declarator's own scope over the namespace stack,
// so an out-of-namespace definition `void ns::f()` keeps ns::f.
func (b *singleFileBuilder) functionFullName(resolved declaratorInfo, class *ssa.Blueprint) string {
	if class != nil {
		return class.Name + "::" + resolved.name
	}
	if len(resolved.scope) > 0 {
		return strings.Join(resolved.scope, "::") + "::" + resolved.name
	}
	return b.qualifiedName(resolved.name)
}

func (b *singleFileBuilder) registerFunction(fn *ssa.Function, name string, class *ssa.Blueprint, isStatic bool) {
	if class == nil {
		return
	}
	switch {
	case name == class.Name:
		class.RegisterMagicMethod(ssa.Constructor, fn)
	case strings.HasPrefix(name, "~"):
		class.RegisterMagicMethod(ssa.Destructor, fn)
	case isStatic:
		class.RegisterStaticMethod(name, fn)
	default:
		class.RegisterNormalMethod(name, fn)
	}
}

// buildFunction builds the body of a function or method and binds its name.
func (b *singleFileBuilder) buildFunction(resolved declaratorInfo, class *ssa.Blueprint, body cppparser.IFunctionBodyContext) *ssa.Function {
	fullName := b.functionFullName(resolved, class)
	fn := b.NewFunc(fullName)
	fn.SetMethodName(resolved.name)
	b.registerFunction(fn, resolved.name, class, resolved.isStatic)

	previous := b.FunctionBuilder
	b.FunctionBuilder = b.PushFunction(fn)
	savedClass := b.class
	b.class = class
	if class != nil {
		b.MarkedThisClassBlueprint = class
		this := b.NewParam("this")
		this.SetType(class)
	}
	for _, parameter := range resolved.params {
		param := b.NewParam(parameter.name)
		if parameter.typ != nil {
			param.SetType(parameter.typ)
		}
	}
	b.visitFunctionBody(body, class)
	b.Finish()
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	b.class = savedClass
	b.MarkedThisClassBlueprint = nil

	b.GetProgram().SetExportValue(fullName, fn)
	if class == nil {
		b.AssignVariable(b.CreateVariable(fullName), fn)
		if resolved.name != fullName {
			b.AssignVariable(b.CreateVariable(resolved.name), fn)
		}
	}
	return fn
}

func (b *singleFileBuilder) visitFunctionBody(raw cppparser.IFunctionBodyContext, class *ssa.Blueprint) {
	body, ok := raw.(*cppparser.FunctionBodyContext)
	if !ok || body == nil {
		b.EmitReturn(nil)
		return
	}
	if initializer := body.ConstructorInitializer(); initializer != nil {
		b.visitConstructorInitializer(initializer, class)
	}
	if block := body.CompoundStatement(); block != nil {
		b.visitCompoundStatement(block)
		return
	}
	if tryBlock := body.FunctionTryBlock(); tryBlock != nil {
		if functionTry, ok := tryBlock.(*cppparser.FunctionTryBlockContext); ok && functionTry != nil {
			if block := functionTry.CompoundStatement(); block != nil {
				b.visitCompoundStatement(block)
			}
			return
		}
	}
	// `= default` and `= delete` have no body.
	b.EmitReturn(nil)
}

// visitConstructorInitializer lowers `: member(value), Base(arg)` into member
// writes. A base-class initializer is kept as a constructor call.
func (b *singleFileBuilder) visitConstructorInitializer(raw cppparser.IConstructorInitializerContext, class *ssa.Blueprint) {
	initializer, ok := raw.(*cppparser.ConstructorInitializerContext)
	if !ok || initializer == nil {
		return
	}
	list, ok := initializer.MemInitializerList().(*cppparser.MemInitializerListContext)
	if !ok || list == nil {
		return
	}
	thisValue := b.ReadValue("this")
	for _, rawItem := range list.AllMemInitializer() {
		item, ok := rawItem.(*cppparser.MemInitializerContext)
		if !ok || item == nil || item.MemInitializerId() == nil {
			continue
		}
		name := normalizeTypeName(item.MemInitializerId().GetText())
		values := b.visitMemInitializerArguments(item)
		if len(values) == 0 {
			continue
		}
		if class != nil && name == class.Name {
			continue
		}
		if thisValue != nil {
			variable := b.CreateMemberCallVariable(thisValue, b.EmitConstInst(name), true)
			b.AssignVariable(variable, values[0])
			continue
		}
		b.AssignVariable(b.CreateVariable(name), values[0])
	}
}

func (b *singleFileBuilder) visitMemInitializerArguments(item *cppparser.MemInitializerContext) []ssa.Value {
	if list := item.ExpressionList(); list != nil {
		return b.visitExpressionList(list)
	}
	if braced := item.BracedInitList(); braced != nil {
		if value := b.visitBracedInitList(braced, nil); value != nil {
			return []ssa.Value{value}
		}
	}
	return nil
}

// parseDeclarator reads a C++ declarator: pointer and reference depth, array
// extents, a function parameter list, and the declared name with its scope.
func (b *singleFileBuilder) parseDeclarator(raw cppparser.IDeclaratorContext, base ssa.Type) declaratorInfo {
	info := declaratorInfo{typ: base}
	declarator, ok := raw.(*cppparser.DeclaratorContext)
	if !ok || declarator == nil {
		return info
	}
	if pointer, ok := declarator.PointerDeclarator().(*cppparser.PointerDeclaratorContext); ok && pointer != nil {
		return b.parsePointerDeclarator(pointer, info)
	}
	if noPointer := declarator.NoPointerDeclarator(); noPointer != nil {
		info = b.parseNoPointerDeclarator(noPointer, info)
	}
	if qualifiers := declarator.ParametersAndQualifiers(); qualifiers != nil {
		info.isFunction = true
		info.params = b.parseParametersAndQualifiers(qualifiers)
	}
	if trailing := declarator.TrailingReturnType(); trailing != nil {
		if returnType, ok := trailing.(*cppparser.TrailingReturnTypeContext); ok && returnType != nil {
			info.typ = b.typeFromText(returnType.TrailingTypeSpecifierSeq().GetText())
		}
	}
	return info
}

func (b *singleFileBuilder) parsePointerDeclarator(pointer *cppparser.PointerDeclaratorContext, info declaratorInfo) declaratorInfo {
	depth := countPointerOperators(pointer.AllPointerOperator())
	if noPointer, ok := pointer.NoPointerDeclarator().(*cppparser.NoPointerDeclaratorContext); ok && noPointer != nil {
		info = b.parseNoPointerDeclarator(noPointer, info)
	}
	info.typ = wrapPointerType(info.typ, depth)
	return info
}

func (b *singleFileBuilder) parseNoPointerDeclarator(raw cppparser.INoPointerDeclaratorContext, info declaratorInfo) declaratorInfo {
	declarator, ok := raw.(*cppparser.NoPointerDeclaratorContext)
	if !ok || declarator == nil {
		return info
	}
	switch {
	case declarator.NoPointerDeclarator() != nil:
		info = b.parseNoPointerDeclarator(declarator.NoPointerDeclarator(), info)
	case declarator.DeclaratorId() != nil:
		name, scope := splitQualifiedName(declarator.DeclaratorId().GetText())
		info.name = normalizeOperatorName(name)
		info.scope = scope
	case declarator.PointerDeclarator() != nil:
		if pointer, ok := declarator.PointerDeclarator().(*cppparser.PointerDeclaratorContext); ok && pointer != nil {
			info = b.parsePointerDeclarator(pointer, info)
		}
	}
	if declarator.ParametersAndQualifiers() != nil {
		info.isFunction = true
		info.params = b.parseParametersAndQualifiers(declarator.ParametersAndQualifiers())
	}
	if declarator.LeftBracket() != nil {
		info.typ = ssa.NewSliceType(info.typ)
	}
	return info
}

func (b *singleFileBuilder) parseParametersAndQualifiers(raw cppparser.IParametersAndQualifiersContext) []paramInfo {
	qualifiers, ok := raw.(*cppparser.ParametersAndQualifiersContext)
	if !ok || qualifiers == nil {
		return nil
	}
	clause, ok := qualifiers.ParameterDeclarationClause().(*cppparser.ParameterDeclarationClauseContext)
	if !ok || clause == nil {
		return nil
	}
	list, ok := clause.ParameterDeclarationList().(*cppparser.ParameterDeclarationListContext)
	if !ok || list == nil {
		return nil
	}
	params := make([]paramInfo, 0, len(list.AllParameterDeclaration()))
	for _, rawDeclaration := range list.AllParameterDeclaration() {
		declaration, ok := rawDeclaration.(*cppparser.ParameterDeclarationContext)
		if !ok || declaration == nil {
			continue
		}
		params = append(params, b.parseParameterDeclaration(declaration))
	}
	return params
}

func (b *singleFileBuilder) parseParameterDeclaration(raw *cppparser.ParameterDeclarationContext) paramInfo {
	info := b.typeSpecifierInfo(raw.DeclSpecifierSeq())
	param := paramInfo{typ: info.typ}
	if declarator := raw.Declarator(); declarator != nil {
		resolved := b.parseDeclarator(declarator, info.typ)
		param.name = resolved.name
		param.typ = resolved.typ
	} else if abstract := raw.AbstractDeclarator(); abstract != nil {
		param.typ = b.applyAbstractDeclarator(abstract, info.typ)
	}
	return param
}

// applyAbstractDeclarator applies pointer/array/reference markers of an unnamed
// parameter, such as `const std::string&`.
func (b *singleFileBuilder) applyAbstractDeclarator(raw cppparser.IAbstractDeclaratorContext, base ssa.Type) ssa.Type {
	abstract, ok := raw.(*cppparser.AbstractDeclaratorContext)
	if !ok || abstract == nil {
		return base
	}
	if pointer := abstract.PointerAbstractDeclarator(); pointer != nil {
		if concrete, ok := pointer.(*cppparser.PointerAbstractDeclaratorContext); ok && concrete != nil {
			return wrapPointerType(base, countPointerOperators(concrete.AllPointerOperator()))
		}
	}
	if abstract.ParametersAndQualifiers() != nil {
		return ssa.NewFunctionType("function", nil, base, false)
	}
	return base
}

func countPointerOperators(operators []cppparser.IPointerOperatorContext) int {
	count := 0
	for _, operator := range operators {
		if operator == nil {
			continue
		}
		if strings.Contains(operator.GetText(), "*") || strings.Contains(operator.GetText(), "&") {
			count++
		}
	}
	return count
}

func wrapPointerType(base ssa.Type, depth int) ssa.Type {
	if depth <= 0 {
		return base
	}
	if base == nil {
		base = ssa.CreateAnyType()
	}
	pointer := ssa.NewPointerType()
	pointer.FieldType = base
	current := ssa.Type(pointer)
	for i := 1; i < depth; i++ {
		next := ssa.NewPointerType()
		next.FieldType = current
		current = next
	}
	return current
}

// splitQualifiedName splits `Foo::bar` into ("bar", ["Foo"]).
func splitQualifiedName(text string) (string, []string) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "::")
	if text == "" {
		return "", nil
	}
	parts := strings.Split(text, "::")
	name := parts[len(parts)-1]
	scope := parts[:len(parts)-1]
	return name, scope
}

// normalizeOperatorName makes overloaded operator names stable.
func normalizeOperatorName(text string) string {
	text = strings.ReplaceAll(text, " ", "")
	if strings.HasPrefix(text, "operator") {
		return text
	}
	return text
}
