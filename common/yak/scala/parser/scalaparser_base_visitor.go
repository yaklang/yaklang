// Code generated from ./ScalaParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package scalaparser // ScalaParser
import "github.com/yaklang/antlr/v4"

type BaseScalaParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseScalaParserVisitor) VisitColonTripleOpen(ctx *ColonTripleOpenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTripleOpen(ctx *TripleOpenContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTripleClose(ctx *TripleCloseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitIndent(ctx *IndentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitOutdent(ctx *OutdentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnd_of_stat(ctx *End_of_statContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitCompilationUnit(ctx *CompilationUnitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTopStats(ctx *TopStatsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTopStat(ctx *TopStatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPackaging(ctx *PackagingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPackageObject(ctx *PackageObjectContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEndMarker(ctx *EndMarkerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEndMarkerTag(ctx *EndMarkerTagContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitImportDecl(ctx *ImportDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExportDecl(ctx *ExportDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitImportExpr(ctx *ImportExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitImportSpec(ctx *ImportSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitNamedSelector(ctx *NamedSelectorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitWildCardSelector(ctx *WildCardSelectorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitImportSelectors(ctx *ImportSelectorsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitImportSelector(ctx *ImportSelectorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimpleRef(ctx *SimpleRefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitQualId(ctx *QualIdContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitIds(ctx *IdsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClassQualifier(ctx *ClassQualifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimpleLiteral(ctx *SimpleLiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitNegation(ctx *NegationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitType_(ctx *Type_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitFunType(ctx *FunTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitFunTypeArgs(ctx *FunTypeArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitFunParamClause(ctx *FunParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypedFunParam(ctx *TypedFunParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitMatchType_(ctx *MatchType_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitInfixType(ctx *InfixTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitRefinedType(ctx *RefinedTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitAnnotType(ctx *AnnotTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimpleType_(ctx *SimpleType_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSingleton(ctx *SingletonContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitNameAndType(ctx *NameAndTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitFunArgType(ctx *FunArgTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitFunArgTypes(ctx *FunArgTypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitParamType(ctx *ParamTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitParamValueType(ctx *ParamValueTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypeArgs(ctx *TypeArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitRefinement(ctx *RefinementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypeBounds(ctx *TypeBoundsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypeAndCtxBounds(ctx *TypeAndCtxBoundsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitContextBounds(ctx *ContextBoundsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitContextBound(ctx *ContextBoundContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypes(ctx *TypesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypeCaseClauses(ctx *TypeCaseClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypeCaseClause(ctx *TypeCaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExpr(ctx *ExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitBlockResult(ctx *BlockResultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitFunParams(ctx *FunParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExpr1(ctx *Expr1Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitAscription(ctx *AscriptionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitCatches(ctx *CatchesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPostfixExpr(ctx *PostfixExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitInfixExpr(ctx *InfixExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitMatchClause(ctx *MatchClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPrefixExpr(ctx *PrefixExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPrefixOperator(ctx *PrefixOperatorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimpleExpr(ctx *SimpleExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimpleExprPrimary(ctx *SimpleExprPrimaryContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimpleExprSuffix(ctx *SimpleExprSuffixContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitColonArgument(ctx *ColonArgumentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitLambdaStart(ctx *LambdaStartContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitQuoted(ctx *QuotedContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExprsInParens(ctx *ExprsInParensContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExprInParens(ctx *ExprInParensContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitNamedExprInParens(ctx *NamedExprInParensContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitParArgumentExprs(ctx *ParArgumentExprsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitArgumentExprs(ctx *ArgumentExprsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitBlockExpr(ctx *BlockExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitBlockStat(ctx *BlockStatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitForExpr(ctx *ForExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumerators0(ctx *Enumerators0Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumerators(ctx *EnumeratorsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumerator(ctx *EnumeratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGenerator(ctx *GeneratorContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGuard_(ctx *Guard_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitCaseClauses(ctx *CaseClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitCaseClause(ctx *CaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExprCaseClause(ctx *ExprCaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPattern(ctx *PatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPattern1(ctx *Pattern1Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPattern2(ctx *Pattern2Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitInfixPattern(ctx *InfixPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimplePattern(ctx *SimplePatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSimplePattern1(ctx *SimplePattern1Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPatVar(ctx *PatVarContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitNamedPattern(ctx *NamedPatternContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPatterns(ctx *PatternsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitArgumentPatterns(ctx *ArgumentPatternsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClsTypeParamClause(ctx *ClsTypeParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClsTypeParam(ctx *ClsTypeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefTypeParamClause(ctx *DefTypeParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefTypeParam(ctx *DefTypeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypTypeParamClause(ctx *TypTypeParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTypTypeParam(ctx *TypTypeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitHkTypeParamClause(ctx *HkTypeParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitHkTypeParam(ctx *HkTypeParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitVariance(ctx *VarianceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClsParamClauses(ctx *ClsParamClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClsParamClause(ctx *ClsParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClsParams(ctx *ClsParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClsParam(ctx *ClsParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefParamClauses(ctx *DefParamClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefParamClause(ctx *DefParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitConstrParamClauses(ctx *ConstrParamClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitConstrParamClause(ctx *ConstrParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefTermParamClause(ctx *DefTermParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitUsingParamClause(ctx *UsingParamClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefImplicitClause(ctx *DefImplicitClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefTermParams(ctx *DefTermParamsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefTermParam(ctx *DefTermParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitParam(ctx *ParamContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitBindings(ctx *BindingsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitBinding(ctx *BindingContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitModifier(ctx *ModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitLocalModifier(ctx *LocalModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitAccessModifier(ctx *AccessModifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitAccessQualifier(ctx *AccessQualifierContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitAnnotation(ctx *AnnotationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitRefineDcl(ctx *RefineDclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitValDcl(ctx *ValDclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefDcl(ctx *DefDclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDef_(ctx *Def_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitPatDef(ctx *PatDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefDef(ctx *DefDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitDefSig(ctx *DefSigContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTmplDef(ctx *TmplDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClassDef(ctx *ClassDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitClassConstr(ctx *ClassConstrContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitConstrMods(ctx *ConstrModsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitObjectDef(ctx *ObjectDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumDef(ctx *EnumDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGivenDef(ctx *GivenDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGivenSig(ctx *GivenSigContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGivenImpl(ctx *GivenImplContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGivenConditional(ctx *GivenConditionalContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitGivenType(ctx *GivenTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitOldGivenDef(ctx *OldGivenDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitOldGivenSig(ctx *OldGivenSigContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitStructuralInstance(ctx *StructuralInstanceContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExtension_(ctx *Extension_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExtMethods(ctx *ExtMethodsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitExtMethod(ctx *ExtMethodContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTemplate(ctx *TemplateContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitInheritClauses(ctx *InheritClausesContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitConstrApps(ctx *ConstrAppsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitConstrApp(ctx *ConstrAppContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitConstrExpr(ctx *ConstrExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSelfInvocation(ctx *SelfInvocationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitWithTemplateBody(ctx *WithTemplateBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTemplateBody(ctx *TemplateBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitTemplateStat(ctx *TemplateStatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitSelfType(ctx *SelfTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumBody(ctx *EnumBodyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumStat(ctx *EnumStatContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitEnumCase(ctx *EnumCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseScalaParserVisitor) VisitId(ctx *IdContext) interface{} {
	return v.VisitChildren(ctx)
}
