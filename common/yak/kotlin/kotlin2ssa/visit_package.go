package kotlin2ssa

import (
	kotlinparser "github.com/yaklang/yaklang/common/yak/kotlin/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitKotlinFile(raw kotlinparser.IKotlinFileContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	file, ok := raw.(*kotlinparser.KotlinFileContext)
	if !ok || file == nil {
		return
	}
	if header := file.PackageHeader(); header != nil {
		if h, ok := header.(*kotlinparser.PackageHeaderContext); ok && h.Identifier() != nil {
			b.GetProgram().SetPackageName(h.Identifier().GetText())
		}
	}
	if imports := file.ImportList(); imports != nil {
		if list, ok := imports.(*kotlinparser.ImportListContext); ok && list != nil {
			for _, rawHeader := range list.AllImportHeader() {
				if header, ok := rawHeader.(*kotlinparser.ImportHeaderContext); ok && header.Identifier() != nil {
					b.imports = append(b.imports, header.Identifier().GetText())
				}
			}
		}
	}
	for _, rawObject := range file.AllTopLevelObject() {
		b.visitTopLevelObject(rawObject)
		if b.IsStop() {
			return
		}
	}
}

func (b *singleFileBuilder) visitTopLevelObject(raw kotlinparser.ITopLevelObjectContext) {
	object, ok := raw.(*kotlinparser.TopLevelObjectContext)
	if !ok || object == nil || object.Declaration() == nil {
		return
	}
	b.visitDeclaration(object.Declaration())
}

func (b *singleFileBuilder) visitDeclaration(raw kotlinparser.IDeclarationContext) {
	declaration, ok := raw.(*kotlinparser.DeclarationContext)
	if !ok || declaration == nil {
		return
	}
	switch {
	case declaration.FunctionDeclaration() != nil:
		b.visitFunctionDeclaration(declaration.FunctionDeclaration())
	case declaration.PropertyDeclaration() != nil:
		b.visitPropertyDeclaration(declaration.PropertyDeclaration())
	case declaration.ClassDeclaration() != nil:
		b.visitClassDeclaration(declaration.ClassDeclaration())
	case declaration.ObjectDeclaration() != nil:
		b.visitObjectDeclaration(declaration.ObjectDeclaration())
	}
}

func (b *singleFileBuilder) visitFunctionDeclaration(raw kotlinparser.IFunctionDeclarationContext) {
	fnCtx, ok := raw.(*kotlinparser.FunctionDeclarationContext)
	if !ok || fnCtx == nil || fnCtx.SimpleIdentifier() == nil {
		return
	}
	name := fnCtx.SimpleIdentifier().GetText()
	fn := b.NewFunc(name)
	previous := b.FunctionBuilder
	b.FunctionBuilder = b.PushFunction(fn)
	b.LoadGlobalVariable()
	for name, value := range b.classFields {
		b.AssignVariable(b.CreateVariable(name), value)
	}
	if params := fnCtx.FunctionValueParameters(); params != nil {
		b.addFunctionValueParameters(params)
	}
	if body := fnCtx.FunctionBody(); body != nil {
		b.visitFunctionBody(body)
	}
	b.Finish()
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	b.AssignVariable(b.CreateVariable(name), fn)
	b.GetProgram().SetExportValue(name, fn)
	b.AddGlobalVariable(name, func() ssa.Value { return fn })
}

func (b *singleFileBuilder) addFunctionValueParameters(raw kotlinparser.IFunctionValueParametersContext) {
	params, ok := raw.(*kotlinparser.FunctionValueParametersContext)
	if !ok || params == nil {
		return
	}
	for _, rawParam := range params.AllFunctionValueParameter() {
		param, ok := rawParam.(*kotlinparser.FunctionValueParameterContext)
		if !ok || param == nil || param.Parameter() == nil {
			continue
		}
		parameter, ok := param.Parameter().(*kotlinparser.ParameterContext)
		if !ok || parameter == nil || parameter.SimpleIdentifier() == nil {
			continue
		}
		b.NewParam(parameter.SimpleIdentifier().GetText())
	}
}

func (b *singleFileBuilder) visitFunctionBody(raw kotlinparser.IFunctionBodyContext) {
	body, ok := raw.(*kotlinparser.FunctionBodyContext)
	if !ok || body == nil {
		return
	}
	if body.Block() != nil {
		b.visitBlock(body.Block())
		return
	}
	if body.Expression() != nil {
		value := b.VisitExpression(body.Expression())
		if value != nil {
			b.EmitReturn([]ssa.Value{value})
		}
	}
}

func (b *singleFileBuilder) visitPropertyDeclaration(raw kotlinparser.IPropertyDeclarationContext) {
	property, ok := raw.(*kotlinparser.PropertyDeclarationContext)
	if !ok || property == nil {
		return
	}
	var value ssa.Value = b.EmitConstInstNil()
	if property.Expression() != nil {
		if v := b.VisitExpression(property.Expression()); v != nil {
			value = v
		}
	}
	if variable := property.VariableDeclaration(); variable != nil {
		if v, ok := variable.(*kotlinparser.VariableDeclarationContext); ok && v.SimpleIdentifier() != nil {
			name := v.SimpleIdentifier().GetText()
			b.AssignVariable(b.CreateVariable(name), value)
			b.GetProgram().SetExportValue(name, value)
			if b.classFields == nil {
				b.AddGlobalVariable(name, func() ssa.Value { return value })
			}
			if b.classFields != nil {
				b.classFields[name] = value
			}
		}
		return
	}
	if multi := property.MultiVariableDeclaration(); multi != nil {
		if m, ok := multi.(*kotlinparser.MultiVariableDeclarationContext); ok && m != nil {
			for _, rawVariable := range m.AllVariableDeclaration() {
				if v, ok := rawVariable.(*kotlinparser.VariableDeclarationContext); ok && v.SimpleIdentifier() != nil {
					b.AssignVariable(b.CreateVariable(v.SimpleIdentifier().GetText()), value)
				}
			}
		}
	}
}

func (b *singleFileBuilder) visitClassDeclaration(raw kotlinparser.IClassDeclarationContext) {
	class, ok := raw.(*kotlinparser.ClassDeclarationContext)
	if !ok || class == nil || class.ClassBody() == nil {
		return
	}
	b.visitClassBody(class.ClassBody())
}

func (b *singleFileBuilder) visitObjectDeclaration(raw kotlinparser.IObjectDeclarationContext) {
	object, ok := raw.(*kotlinparser.ObjectDeclarationContext)
	if !ok || object == nil || object.ClassBody() == nil {
		return
	}
	b.visitClassBody(object.ClassBody())
}

func (b *singleFileBuilder) visitClassBody(raw kotlinparser.IClassBodyContext) {
	body, ok := raw.(*kotlinparser.ClassBodyContext)
	if !ok || body == nil || body.ClassMemberDeclarations() == nil {
		return
	}
	members, ok := body.ClassMemberDeclarations().(*kotlinparser.ClassMemberDeclarationsContext)
	if !ok || members == nil {
		return
	}
	previousFields := b.classFields
	b.classFields = make(map[string]ssa.Value)
	defer func() { b.classFields = previousFields }()
	for _, rawMember := range members.AllClassMemberDeclaration() {
		member, ok := rawMember.(*kotlinparser.ClassMemberDeclarationContext)
		if !ok || member == nil || member.Declaration() == nil {
			continue
		}
		b.visitDeclaration(member.Declaration())
	}
}
