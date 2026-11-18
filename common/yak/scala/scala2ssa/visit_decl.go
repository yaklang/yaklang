package scala2ssa

import (
	scalaparser "github.com/yaklang/yaklang/common/yak/scala/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitCompilationUnit(raw scalaparser.ICompilationUnitContext) {
	unit, ok := raw.(*scalaparser.CompilationUnitContext)
	if !ok || unit == nil {
		return
	}
	if ids := unit.AllQualId(); len(ids) > 0 && ids[0] != nil {
		b.GetProgram().SetPackageName(ids[0].GetText())
	}
	if stats := unit.TopStats(); stats != nil {
		b.visitTopStats(stats)
	}
}

func (b *singleFileBuilder) visitTopStats(raw scalaparser.ITopStatsContext) {
	stats, ok := raw.(*scalaparser.TopStatsContext)
	if !ok || stats == nil {
		return
	}
	for _, rawStat := range stats.AllTopStat() {
		stat, ok := rawStat.(*scalaparser.TopStatContext)
		if !ok || stat == nil {
			continue
		}
		b.visitTopStat(stat)
		if b.IsStop() {
			return
		}
	}
}

func (b *singleFileBuilder) visitTopStat(stat *scalaparser.TopStatContext) {
	if stat == nil {
		return
	}
	if stat.Def_() != nil {
		b.visitDef(stat.Def_())
	}
}

func (b *singleFileBuilder) visitDef(raw scalaparser.IDef_Context) {
	def, ok := raw.(*scalaparser.Def_Context)
	if !ok || def == nil {
		return
	}
	switch {
	case def.VAL() != nil || def.VAR() != nil:
		b.visitPatDef(def.PatDef())
	case def.DEF() != nil:
		b.visitDefDef(def.DefDef())
	case def.TmplDef() != nil:
		b.visitTmplDef(def.TmplDef())
	}
}

func (b *singleFileBuilder) visitPatDef(raw scalaparser.IPatDefContext) {
	pat, ok := raw.(*scalaparser.PatDefContext)
	if !ok || pat == nil {
		return
	}
	var value ssa.Value = b.EmitConstInstNil()
	if pat.Expr() != nil {
		if v := b.VisitExpr(pat.Expr()); v != nil {
			value = v
		}
	}
	if ids := pat.Ids(); ids != nil {
		if names, ok := ids.(*scalaparser.IdsContext); ok && names != nil {
			for i := 0; i < len(names.AllId()); i++ {
				name := names.Id(i).GetText()
				b.AssignVariable(b.CreateVariable(name), value)
				b.GetProgram().SetExportValue(name, value)
			}
		}
	}
}

func (b *singleFileBuilder) visitDefDef(raw scalaparser.IDefDefContext) {
	def, ok := raw.(*scalaparser.DefDefContext)
	if !ok || def == nil || def.DefSig() == nil {
		return
	}
	sig, ok := def.DefSig().(*scalaparser.DefSigContext)
	if !ok || sig == nil || sig.Id() == nil {
		return
	}
	name := sig.Id().GetText()
	fn := b.NewFunc(name)
	previous := b.FunctionBuilder
	b.FunctionBuilder = b.PushFunction(fn)
	if clauses := sig.DefParamClauses(); clauses != nil {
		b.addDefParamClauses(clauses)
	}
	if def.Expr() != nil {
		if value := b.VisitExpr(def.Expr()); value != nil {
			b.EmitReturn([]ssa.Value{value})
		}
	} else if def.Block() != nil {
		if value := b.visitBlockResult(def.Block()); value != nil {
			b.EmitReturn([]ssa.Value{value})
		}
	}
	b.Finish()
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	b.AssignVariable(b.CreateVariable(name), fn)
	b.GetProgram().SetExportValue(name, fn)
}

func (b *singleFileBuilder) addDefParamClauses(raw scalaparser.IDefParamClausesContext) {
	clauses, ok := raw.(*scalaparser.DefParamClausesContext)
	if !ok || clauses == nil {
		return
	}
	for _, rawClause := range clauses.AllDefParamClause() {
		clause, ok := rawClause.(*scalaparser.DefParamClauseContext)
		if !ok || clause == nil || clause.DefTermParamClause() == nil {
			continue
		}
		term, ok := clause.DefTermParamClause().(*scalaparser.DefTermParamClauseContext)
		if !ok || term == nil || term.DefTermParams() == nil {
			continue
		}
		params, ok := term.DefTermParams().(*scalaparser.DefTermParamsContext)
		if !ok || params == nil {
			continue
		}
		for _, rawParam := range params.AllDefTermParam() {
			param, ok := rawParam.(*scalaparser.DefTermParamContext)
			if !ok || param == nil || param.Param() == nil {
				continue
			}
			parameter, ok := param.Param().(*scalaparser.ParamContext)
			if !ok || parameter == nil || parameter.Id() == nil {
				continue
			}
			b.NewParam(parameter.Id().GetText())
		}
	}
}

func (b *singleFileBuilder) visitTmplDef(raw scalaparser.ITmplDefContext) {
	tmpl, ok := raw.(*scalaparser.TmplDefContext)
	if !ok || tmpl == nil {
		return
	}
	switch {
	case tmpl.ClassDef() != nil:
		if class, ok := tmpl.ClassDef().(*scalaparser.ClassDefContext); ok && class != nil && class.Template() != nil {
			b.visitTemplate(class.Template())
		}
	case tmpl.ObjectDef() != nil:
		if object, ok := tmpl.ObjectDef().(*scalaparser.ObjectDefContext); ok && object != nil && object.Template() != nil {
			b.visitTemplate(object.Template())
		}
	case tmpl.EnumDef() != nil:
		// enum bodies are not lowered yet
	}
}

func (b *singleFileBuilder) visitTemplate(raw scalaparser.ITemplateContext) {
	template, ok := raw.(*scalaparser.TemplateContext)
	if !ok || template == nil || template.TemplateBody() == nil {
		return
	}
	body, ok := template.TemplateBody().(*scalaparser.TemplateBodyContext)
	if !ok || body == nil {
		return
	}
	for _, rawStat := range body.AllTemplateStat() {
		stat, ok := rawStat.(*scalaparser.TemplateStatContext)
		if !ok || stat == nil {
			continue
		}
		if stat.Def_() != nil {
			b.visitDef(stat.Def_())
		} else if stat.Expr1() != nil {
			b.VisitExpr1(stat.Expr1())
		}
	}
}

func (b *singleFileBuilder) visitBlock(raw scalaparser.IBlockContext) {
	b.visitBlockResult(raw)
}

func (b *singleFileBuilder) visitBlockResult(raw scalaparser.IBlockContext) ssa.Value {
	block, ok := raw.(*scalaparser.BlockContext)
	if !ok || block == nil {
		return nil
	}
	var result ssa.Value
	stats := block.AllBlockStat()
	for _, rawStat := range stats {
		stat, ok := rawStat.(*scalaparser.BlockStatContext)
		if !ok || stat == nil {
			continue
		}
		if stat.Def_() != nil {
			b.visitDef(stat.Def_())
			result = nil
		} else if stat.Expr1() != nil {
			result = b.VisitExpr1(stat.Expr1())
		}
	}
	return result
}
