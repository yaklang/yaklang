// Code generated from ./ScalaParser.g4 by ANTLR 4.13.2. DO NOT EDIT.

package scalaparser // ScalaParser
import "github.com/yaklang/antlr/v4"

// A complete Visitor for a parse tree produced by ScalaParser.
type ScalaParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by ScalaParser#colonTripleOpen.
	VisitColonTripleOpen(ctx *ColonTripleOpenContext) interface{}

	// Visit a parse tree produced by ScalaParser#tripleOpen.
	VisitTripleOpen(ctx *TripleOpenContext) interface{}

	// Visit a parse tree produced by ScalaParser#tripleClose.
	VisitTripleClose(ctx *TripleCloseContext) interface{}

	// Visit a parse tree produced by ScalaParser#indent.
	VisitIndent(ctx *IndentContext) interface{}

	// Visit a parse tree produced by ScalaParser#outdent.
	VisitOutdent(ctx *OutdentContext) interface{}

	// Visit a parse tree produced by ScalaParser#end_of_stat.
	VisitEnd_of_stat(ctx *End_of_statContext) interface{}

	// Visit a parse tree produced by ScalaParser#compilationUnit.
	VisitCompilationUnit(ctx *CompilationUnitContext) interface{}

	// Visit a parse tree produced by ScalaParser#topStats.
	VisitTopStats(ctx *TopStatsContext) interface{}

	// Visit a parse tree produced by ScalaParser#topStat.
	VisitTopStat(ctx *TopStatContext) interface{}

	// Visit a parse tree produced by ScalaParser#packaging.
	VisitPackaging(ctx *PackagingContext) interface{}

	// Visit a parse tree produced by ScalaParser#packageObject.
	VisitPackageObject(ctx *PackageObjectContext) interface{}

	// Visit a parse tree produced by ScalaParser#endMarker.
	VisitEndMarker(ctx *EndMarkerContext) interface{}

	// Visit a parse tree produced by ScalaParser#endMarkerTag.
	VisitEndMarkerTag(ctx *EndMarkerTagContext) interface{}

	// Visit a parse tree produced by ScalaParser#importDecl.
	VisitImportDecl(ctx *ImportDeclContext) interface{}

	// Visit a parse tree produced by ScalaParser#exportDecl.
	VisitExportDecl(ctx *ExportDeclContext) interface{}

	// Visit a parse tree produced by ScalaParser#importExpr.
	VisitImportExpr(ctx *ImportExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#importSpec.
	VisitImportSpec(ctx *ImportSpecContext) interface{}

	// Visit a parse tree produced by ScalaParser#namedSelector.
	VisitNamedSelector(ctx *NamedSelectorContext) interface{}

	// Visit a parse tree produced by ScalaParser#wildCardSelector.
	VisitWildCardSelector(ctx *WildCardSelectorContext) interface{}

	// Visit a parse tree produced by ScalaParser#importSelectors.
	VisitImportSelectors(ctx *ImportSelectorsContext) interface{}

	// Visit a parse tree produced by ScalaParser#simpleRef.
	VisitSimpleRef(ctx *SimpleRefContext) interface{}

	// Visit a parse tree produced by ScalaParser#qualId.
	VisitQualId(ctx *QualIdContext) interface{}

	// Visit a parse tree produced by ScalaParser#ids.
	VisitIds(ctx *IdsContext) interface{}

	// Visit a parse tree produced by ScalaParser#classQualifier.
	VisitClassQualifier(ctx *ClassQualifierContext) interface{}

	// Visit a parse tree produced by ScalaParser#simpleLiteral.
	VisitSimpleLiteral(ctx *SimpleLiteralContext) interface{}

	// Visit a parse tree produced by ScalaParser#negation.
	VisitNegation(ctx *NegationContext) interface{}

	// Visit a parse tree produced by ScalaParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by ScalaParser#type_.
	VisitType_(ctx *Type_Context) interface{}

	// Visit a parse tree produced by ScalaParser#funType.
	VisitFunType(ctx *FunTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#funTypeArgs.
	VisitFunTypeArgs(ctx *FunTypeArgsContext) interface{}

	// Visit a parse tree produced by ScalaParser#funParamClause.
	VisitFunParamClause(ctx *FunParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#typedFunParam.
	VisitTypedFunParam(ctx *TypedFunParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#matchType_.
	VisitMatchType_(ctx *MatchType_Context) interface{}

	// Visit a parse tree produced by ScalaParser#infixType.
	VisitInfixType(ctx *InfixTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#refinedType.
	VisitRefinedType(ctx *RefinedTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#annotType.
	VisitAnnotType(ctx *AnnotTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#simpleType_.
	VisitSimpleType_(ctx *SimpleType_Context) interface{}

	// Visit a parse tree produced by ScalaParser#singleton.
	VisitSingleton(ctx *SingletonContext) interface{}

	// Visit a parse tree produced by ScalaParser#nameAndType.
	VisitNameAndType(ctx *NameAndTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#funArgType.
	VisitFunArgType(ctx *FunArgTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#funArgTypes.
	VisitFunArgTypes(ctx *FunArgTypesContext) interface{}

	// Visit a parse tree produced by ScalaParser#paramType.
	VisitParamType(ctx *ParamTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#paramValueType.
	VisitParamValueType(ctx *ParamValueTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#typeArgs.
	VisitTypeArgs(ctx *TypeArgsContext) interface{}

	// Visit a parse tree produced by ScalaParser#refinement.
	VisitRefinement(ctx *RefinementContext) interface{}

	// Visit a parse tree produced by ScalaParser#typeBounds.
	VisitTypeBounds(ctx *TypeBoundsContext) interface{}

	// Visit a parse tree produced by ScalaParser#typeAndCtxBounds.
	VisitTypeAndCtxBounds(ctx *TypeAndCtxBoundsContext) interface{}

	// Visit a parse tree produced by ScalaParser#contextBounds.
	VisitContextBounds(ctx *ContextBoundsContext) interface{}

	// Visit a parse tree produced by ScalaParser#contextBound.
	VisitContextBound(ctx *ContextBoundContext) interface{}

	// Visit a parse tree produced by ScalaParser#types.
	VisitTypes(ctx *TypesContext) interface{}

	// Visit a parse tree produced by ScalaParser#typeCaseClauses.
	VisitTypeCaseClauses(ctx *TypeCaseClausesContext) interface{}

	// Visit a parse tree produced by ScalaParser#typeCaseClause.
	VisitTypeCaseClause(ctx *TypeCaseClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#expr.
	VisitExpr(ctx *ExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#blockResult.
	VisitBlockResult(ctx *BlockResultContext) interface{}

	// Visit a parse tree produced by ScalaParser#funParams.
	VisitFunParams(ctx *FunParamsContext) interface{}

	// Visit a parse tree produced by ScalaParser#expr1.
	VisitExpr1(ctx *Expr1Context) interface{}

	// Visit a parse tree produced by ScalaParser#ascription.
	VisitAscription(ctx *AscriptionContext) interface{}

	// Visit a parse tree produced by ScalaParser#catches.
	VisitCatches(ctx *CatchesContext) interface{}

	// Visit a parse tree produced by ScalaParser#postfixExpr.
	VisitPostfixExpr(ctx *PostfixExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#infixExpr.
	VisitInfixExpr(ctx *InfixExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#matchClause.
	VisitMatchClause(ctx *MatchClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#prefixExpr.
	VisitPrefixExpr(ctx *PrefixExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#prefixOperator.
	VisitPrefixOperator(ctx *PrefixOperatorContext) interface{}

	// Visit a parse tree produced by ScalaParser#simpleExpr.
	VisitSimpleExpr(ctx *SimpleExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#colonArgument.
	VisitColonArgument(ctx *ColonArgumentContext) interface{}

	// Visit a parse tree produced by ScalaParser#lambdaStart.
	VisitLambdaStart(ctx *LambdaStartContext) interface{}

	// Visit a parse tree produced by ScalaParser#quoted.
	VisitQuoted(ctx *QuotedContext) interface{}

	// Visit a parse tree produced by ScalaParser#exprsInParens.
	VisitExprsInParens(ctx *ExprsInParensContext) interface{}

	// Visit a parse tree produced by ScalaParser#exprInParens.
	VisitExprInParens(ctx *ExprInParensContext) interface{}

	// Visit a parse tree produced by ScalaParser#namedExprInParens.
	VisitNamedExprInParens(ctx *NamedExprInParensContext) interface{}

	// Visit a parse tree produced by ScalaParser#parArgumentExprs.
	VisitParArgumentExprs(ctx *ParArgumentExprsContext) interface{}

	// Visit a parse tree produced by ScalaParser#argumentExprs.
	VisitArgumentExprs(ctx *ArgumentExprsContext) interface{}

	// Visit a parse tree produced by ScalaParser#blockExpr.
	VisitBlockExpr(ctx *BlockExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by ScalaParser#blockStat.
	VisitBlockStat(ctx *BlockStatContext) interface{}

	// Visit a parse tree produced by ScalaParser#forExpr.
	VisitForExpr(ctx *ForExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#enumerators0.
	VisitEnumerators0(ctx *Enumerators0Context) interface{}

	// Visit a parse tree produced by ScalaParser#enumerators.
	VisitEnumerators(ctx *EnumeratorsContext) interface{}

	// Visit a parse tree produced by ScalaParser#enumerator.
	VisitEnumerator(ctx *EnumeratorContext) interface{}

	// Visit a parse tree produced by ScalaParser#generator.
	VisitGenerator(ctx *GeneratorContext) interface{}

	// Visit a parse tree produced by ScalaParser#guard_.
	VisitGuard_(ctx *Guard_Context) interface{}

	// Visit a parse tree produced by ScalaParser#caseClauses.
	VisitCaseClauses(ctx *CaseClausesContext) interface{}

	// Visit a parse tree produced by ScalaParser#caseClause.
	VisitCaseClause(ctx *CaseClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#exprCaseClause.
	VisitExprCaseClause(ctx *ExprCaseClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#pattern.
	VisitPattern(ctx *PatternContext) interface{}

	// Visit a parse tree produced by ScalaParser#pattern1.
	VisitPattern1(ctx *Pattern1Context) interface{}

	// Visit a parse tree produced by ScalaParser#pattern2.
	VisitPattern2(ctx *Pattern2Context) interface{}

	// Visit a parse tree produced by ScalaParser#infixPattern.
	VisitInfixPattern(ctx *InfixPatternContext) interface{}

	// Visit a parse tree produced by ScalaParser#simplePattern.
	VisitSimplePattern(ctx *SimplePatternContext) interface{}

	// Visit a parse tree produced by ScalaParser#simplePattern1.
	VisitSimplePattern1(ctx *SimplePattern1Context) interface{}

	// Visit a parse tree produced by ScalaParser#patVar.
	VisitPatVar(ctx *PatVarContext) interface{}

	// Visit a parse tree produced by ScalaParser#namedPattern.
	VisitNamedPattern(ctx *NamedPatternContext) interface{}

	// Visit a parse tree produced by ScalaParser#patterns.
	VisitPatterns(ctx *PatternsContext) interface{}

	// Visit a parse tree produced by ScalaParser#argumentPatterns.
	VisitArgumentPatterns(ctx *ArgumentPatternsContext) interface{}

	// Visit a parse tree produced by ScalaParser#clsTypeParamClause.
	VisitClsTypeParamClause(ctx *ClsTypeParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#clsTypeParam.
	VisitClsTypeParam(ctx *ClsTypeParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#defTypeParamClause.
	VisitDefTypeParamClause(ctx *DefTypeParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#defTypeParam.
	VisitDefTypeParam(ctx *DefTypeParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#typTypeParamClause.
	VisitTypTypeParamClause(ctx *TypTypeParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#typTypeParam.
	VisitTypTypeParam(ctx *TypTypeParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#hkTypeParamClause.
	VisitHkTypeParamClause(ctx *HkTypeParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#hkTypeParam.
	VisitHkTypeParam(ctx *HkTypeParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#variance.
	VisitVariance(ctx *VarianceContext) interface{}

	// Visit a parse tree produced by ScalaParser#clsParamClauses.
	VisitClsParamClauses(ctx *ClsParamClausesContext) interface{}

	// Visit a parse tree produced by ScalaParser#clsParamClause.
	VisitClsParamClause(ctx *ClsParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#clsParams.
	VisitClsParams(ctx *ClsParamsContext) interface{}

	// Visit a parse tree produced by ScalaParser#clsParam.
	VisitClsParam(ctx *ClsParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#defParamClauses.
	VisitDefParamClauses(ctx *DefParamClausesContext) interface{}

	// Visit a parse tree produced by ScalaParser#defParamClause.
	VisitDefParamClause(ctx *DefParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#constrParamClauses.
	VisitConstrParamClauses(ctx *ConstrParamClausesContext) interface{}

	// Visit a parse tree produced by ScalaParser#constrParamClause.
	VisitConstrParamClause(ctx *ConstrParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#defTermParamClause.
	VisitDefTermParamClause(ctx *DefTermParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#usingParamClause.
	VisitUsingParamClause(ctx *UsingParamClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#defImplicitClause.
	VisitDefImplicitClause(ctx *DefImplicitClauseContext) interface{}

	// Visit a parse tree produced by ScalaParser#defTermParams.
	VisitDefTermParams(ctx *DefTermParamsContext) interface{}

	// Visit a parse tree produced by ScalaParser#defTermParam.
	VisitDefTermParam(ctx *DefTermParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#param.
	VisitParam(ctx *ParamContext) interface{}

	// Visit a parse tree produced by ScalaParser#bindings.
	VisitBindings(ctx *BindingsContext) interface{}

	// Visit a parse tree produced by ScalaParser#binding.
	VisitBinding(ctx *BindingContext) interface{}

	// Visit a parse tree produced by ScalaParser#modifier.
	VisitModifier(ctx *ModifierContext) interface{}

	// Visit a parse tree produced by ScalaParser#localModifier.
	VisitLocalModifier(ctx *LocalModifierContext) interface{}

	// Visit a parse tree produced by ScalaParser#accessModifier.
	VisitAccessModifier(ctx *AccessModifierContext) interface{}

	// Visit a parse tree produced by ScalaParser#accessQualifier.
	VisitAccessQualifier(ctx *AccessQualifierContext) interface{}

	// Visit a parse tree produced by ScalaParser#annotation.
	VisitAnnotation(ctx *AnnotationContext) interface{}

	// Visit a parse tree produced by ScalaParser#refineDcl.
	VisitRefineDcl(ctx *RefineDclContext) interface{}

	// Visit a parse tree produced by ScalaParser#valDcl.
	VisitValDcl(ctx *ValDclContext) interface{}

	// Visit a parse tree produced by ScalaParser#defDcl.
	VisitDefDcl(ctx *DefDclContext) interface{}

	// Visit a parse tree produced by ScalaParser#def_.
	VisitDef_(ctx *Def_Context) interface{}

	// Visit a parse tree produced by ScalaParser#patDef.
	VisitPatDef(ctx *PatDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#defDef.
	VisitDefDef(ctx *DefDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#defSig.
	VisitDefSig(ctx *DefSigContext) interface{}

	// Visit a parse tree produced by ScalaParser#tmplDef.
	VisitTmplDef(ctx *TmplDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#classDef.
	VisitClassDef(ctx *ClassDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#classConstr.
	VisitClassConstr(ctx *ClassConstrContext) interface{}

	// Visit a parse tree produced by ScalaParser#constrMods.
	VisitConstrMods(ctx *ConstrModsContext) interface{}

	// Visit a parse tree produced by ScalaParser#objectDef.
	VisitObjectDef(ctx *ObjectDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#enumDef.
	VisitEnumDef(ctx *EnumDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#givenDef.
	VisitGivenDef(ctx *GivenDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#givenSig.
	VisitGivenSig(ctx *GivenSigContext) interface{}

	// Visit a parse tree produced by ScalaParser#givenImpl.
	VisitGivenImpl(ctx *GivenImplContext) interface{}

	// Visit a parse tree produced by ScalaParser#givenConditional.
	VisitGivenConditional(ctx *GivenConditionalContext) interface{}

	// Visit a parse tree produced by ScalaParser#givenType.
	VisitGivenType(ctx *GivenTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#oldGivenDef.
	VisitOldGivenDef(ctx *OldGivenDefContext) interface{}

	// Visit a parse tree produced by ScalaParser#oldGivenSig.
	VisitOldGivenSig(ctx *OldGivenSigContext) interface{}

	// Visit a parse tree produced by ScalaParser#structuralInstance.
	VisitStructuralInstance(ctx *StructuralInstanceContext) interface{}

	// Visit a parse tree produced by ScalaParser#extension_.
	VisitExtension_(ctx *Extension_Context) interface{}

	// Visit a parse tree produced by ScalaParser#extMethods.
	VisitExtMethods(ctx *ExtMethodsContext) interface{}

	// Visit a parse tree produced by ScalaParser#extMethod.
	VisitExtMethod(ctx *ExtMethodContext) interface{}

	// Visit a parse tree produced by ScalaParser#template.
	VisitTemplate(ctx *TemplateContext) interface{}

	// Visit a parse tree produced by ScalaParser#inheritClauses.
	VisitInheritClauses(ctx *InheritClausesContext) interface{}

	// Visit a parse tree produced by ScalaParser#constrApps.
	VisitConstrApps(ctx *ConstrAppsContext) interface{}

	// Visit a parse tree produced by ScalaParser#constrApp.
	VisitConstrApp(ctx *ConstrAppContext) interface{}

	// Visit a parse tree produced by ScalaParser#constrExpr.
	VisitConstrExpr(ctx *ConstrExprContext) interface{}

	// Visit a parse tree produced by ScalaParser#selfInvocation.
	VisitSelfInvocation(ctx *SelfInvocationContext) interface{}

	// Visit a parse tree produced by ScalaParser#withTemplateBody.
	VisitWithTemplateBody(ctx *WithTemplateBodyContext) interface{}

	// Visit a parse tree produced by ScalaParser#templateBody.
	VisitTemplateBody(ctx *TemplateBodyContext) interface{}

	// Visit a parse tree produced by ScalaParser#templateStat.
	VisitTemplateStat(ctx *TemplateStatContext) interface{}

	// Visit a parse tree produced by ScalaParser#selfType.
	VisitSelfType(ctx *SelfTypeContext) interface{}

	// Visit a parse tree produced by ScalaParser#enumBody.
	VisitEnumBody(ctx *EnumBodyContext) interface{}

	// Visit a parse tree produced by ScalaParser#enumStat.
	VisitEnumStat(ctx *EnumStatContext) interface{}

	// Visit a parse tree produced by ScalaParser#enumCase.
	VisitEnumCase(ctx *EnumCaseContext) interface{}

	// Visit a parse tree produced by ScalaParser#id.
	VisitId(ctx *IdContext) interface{}
}
