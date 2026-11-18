package rust2ssa

import (
	rustparser "github.com/yaklang/yaklang/common/yak/rust/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitCrate(raw rustparser.ICrateContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	crate, ok := raw.(*rustparser.CrateContext)
	if !ok || crate == nil {
		return
	}
	for _, item := range crate.AllItem() {
		b.visitItem(item)
		if b.IsStop() {
			return
		}
	}
}

func (b *singleFileBuilder) visitItem(raw rustparser.IItemContext) {
	if b == nil || raw == nil || b.IsStop() {
		return
	}
	item, ok := raw.(*rustparser.ItemContext)
	if !ok || item == nil {
		return
	}
	recoverRange := b.SetRange(item)
	defer recoverRange()

	if vis := item.VisItem(); vis != nil {
		b.visitVisItem(vis)
	}
}

func (b *singleFileBuilder) visitVisItem(raw rustparser.IVisItemContext) {
	item, ok := raw.(*rustparser.VisItemContext)
	if !ok || item == nil {
		return
	}
	switch {
	case item.Function_() != nil:
		b.visitFunction(item.Function_())
	case item.Module() != nil:
		b.visitModule(item.Module())
	case item.UseDeclaration() != nil:
		b.visitUse(item.UseDeclaration())
	case item.ConstantItem() != nil:
		b.visitConstant(item.ConstantItem())
	case item.StaticItem() != nil:
		b.visitStatic(item.StaticItem())
	}
}

func (b *singleFileBuilder) visitFunction(raw rustparser.IFunction_Context) {
	fnCtx, ok := raw.(*rustparser.Function_Context)
	if !ok || fnCtx == nil {
		return
	}
	name := "anonymous"
	if fnCtx.Identifier() != nil {
		name = fnCtx.Identifier().GetText()
	}
	body := fnCtx.BlockExpression()
	if body == nil {
		return
	}

	fn := b.NewFunc(name)
	previous := b.FunctionBuilder
	b.FunctionBuilder = b.PushFunction(fn)
	if params := fnCtx.FunctionParameters(); params != nil {
		b.addFunctionParams(params)
	}
	b.visitBlockExpression(body, true)
	b.Finish()
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous

	b.AssignVariable(b.CreateVariable(name), fn)
	b.GetProgram().SetExportValue(name, fn)
}

func (b *singleFileBuilder) addFunctionParams(raw rustparser.IFunctionParametersContext) {
	params, ok := raw.(*rustparser.FunctionParametersContext)
	if !ok || params == nil {
		return
	}
	if params.SelfParam() != nil {
		b.NewParam("self")
	}
	for _, rawParam := range params.AllFunctionParam() {
		param, ok := rawParam.(*rustparser.FunctionParamContext)
		if !ok || param == nil {
			continue
		}
		if param.DOTDOTDOT() != nil {
			b.NewParam("...")
			continue
		}
		pattern := param.FunctionParamPattern()
		if pattern == nil {
			continue
		}
		p, ok := pattern.(*rustparser.FunctionParamPatternContext)
		if !ok || p == nil {
			continue
		}
		if name := rustPatternName(p.Pattern()); name != "" {
			b.NewParam(name)
		}
	}
}

func (b *singleFileBuilder) visitModule(raw rustparser.IModuleContext) {
	module, ok := raw.(*rustparser.ModuleContext)
	if !ok || module == nil {
		return
	}
	name := "module"
	if module.Identifier() != nil {
		name = module.Identifier().GetText()
	}
	if len(module.AllItem()) == 0 {
		b.modules = append(b.modules, name)
		b.GetProgram().SetExportValue(name, b.EmitUndefined("mod "+name))
		return
	}
	for _, item := range module.AllItem() {
		b.visitItem(item)
	}
}

func (b *singleFileBuilder) visitUse(raw rustparser.IUseDeclarationContext) {
	use, ok := raw.(*rustparser.UseDeclarationContext)
	if !ok || use == nil || use.UseTree() == nil {
		return
	}
	b.imports = append(b.imports, use.UseTree().GetText())
}

func (b *singleFileBuilder) visitConstant(raw rustparser.IConstantItemContext) {
	item, ok := raw.(*rustparser.ConstantItemContext)
	if !ok || item == nil || item.Identifier() == nil {
		return
	}
	name := item.Identifier().GetText()
	var value ssa.Value = b.EmitConstInstNil()
	if item.Expression() != nil {
		if v := b.VisitExpression(item.Expression()); v != nil {
			value = v
		}
	}
	b.AssignVariable(b.CreateVariable(name), value)
	b.GetProgram().SetExportValue(name, value)
}

func (b *singleFileBuilder) visitStatic(raw rustparser.IStaticItemContext) {
	item, ok := raw.(*rustparser.StaticItemContext)
	if !ok || item == nil || item.Identifier() == nil {
		return
	}
	name := item.Identifier().GetText()
	var value ssa.Value = b.EmitConstInstNil()
	if item.Expression() != nil {
		if v := b.VisitExpression(item.Expression()); v != nil {
			value = v
		}
	}
	b.AssignVariable(b.CreateVariable(name), value)
	b.GetProgram().SetExportValue(name, value)
}

func rustPatternName(raw rustparser.IPatternContext) string {
	ctx, ok := raw.(*rustparser.PatternContext)
	if !ok || ctx == nil || len(ctx.AllPatternNoTopAlt()) == 0 {
		return ""
	}
	pattern, ok := ctx.PatternNoTopAlt(0).(*rustparser.PatternNoTopAltContext)
	if !ok || pattern == nil || pattern.PatternWithoutRange() == nil {
		return ""
	}
	withoutRange, ok := pattern.PatternWithoutRange().(*rustparser.PatternWithoutRangeContext)
	if !ok || withoutRange == nil || withoutRange.IdentifierPattern() == nil {
		return ""
	}
	identifier, ok := withoutRange.IdentifierPattern().(*rustparser.IdentifierPatternContext)
	if !ok || identifier == nil || identifier.Identifier() == nil {
		return ""
	}
	return identifier.Identifier().GetText()
}

func rustPatternNoTopAltName(raw rustparser.IPatternNoTopAltContext) string {
	pattern, ok := raw.(*rustparser.PatternNoTopAltContext)
	if !ok || pattern == nil || pattern.PatternWithoutRange() == nil {
		return ""
	}
	withoutRange, ok := pattern.PatternWithoutRange().(*rustparser.PatternWithoutRangeContext)
	if !ok || withoutRange == nil || withoutRange.IdentifierPattern() == nil {
		return ""
	}
	identifier, ok := withoutRange.IdentifierPattern().(*rustparser.IdentifierPatternContext)
	if !ok || identifier == nil || identifier.Identifier() == nil {
		return ""
	}
	return identifier.Identifier().GetText()
}
