// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package gol // GoParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

type BaseGoParserVisitor struct {
	*antlr.BaseParseTreeVisitor
}

func (v *BaseGoParserVisitor) VisitSourceFile(ctx *SourceFileContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitPackageClause(ctx *PackageClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitPackageName(ctx *PackageNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitImportDecl(ctx *ImportDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitImportSpec(ctx *ImportSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitImportPath(ctx *ImportPathContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitDeclaration(ctx *DeclarationContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitConstDecl(ctx *ConstDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitConstSpec(ctx *ConstSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitIdentifierList(ctx *IdentifierListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitExpressionList(ctx *ExpressionListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeDecl(ctx *TypeDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeSpec(ctx *TypeSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitAliasDecl(ctx *AliasDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeDef(ctx *TypeDefContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeParameters(ctx *TypeParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeParameterDecl(ctx *TypeParameterDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeElement(ctx *TypeElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeTerm(ctx *TypeTermContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitFunctionDecl(ctx *FunctionDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitMethodDecl(ctx *MethodDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitReceiver(ctx *ReceiverContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitVarDecl(ctx *VarDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitVarSpec(ctx *VarSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitBlock(ctx *BlockContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitStatementList(ctx *StatementListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitStatement(ctx *StatementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSimpleStmt(ctx *SimpleStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitAssignment(ctx *AssignmentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitAssign_op(ctx *Assign_opContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitExpressionStmt(ctx *ExpressionStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSendStmt(ctx *SendStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitIncDecStmt(ctx *IncDecStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitShortVarDecl(ctx *ShortVarDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitLabeledStmt(ctx *LabeledStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitReturnStmt(ctx *ReturnStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitBreakStmt(ctx *BreakStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitContinueStmt(ctx *ContinueStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitGotoStmt(ctx *GotoStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitFallthroughStmt(ctx *FallthroughStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitDeferStmt(ctx *DeferStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitIfStmt(ctx *IfStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSwitchStmt(ctx *SwitchStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitExprSwitchStmt(ctx *ExprSwitchStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitExprCaseClause(ctx *ExprCaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitExprSwitchCase(ctx *ExprSwitchCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeSwitchStmt(ctx *TypeSwitchStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeSwitchGuard(ctx *TypeSwitchGuardContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeCaseClause(ctx *TypeCaseClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeSwitchCase(ctx *TypeSwitchCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeList(ctx *TypeListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSelectStmt(ctx *SelectStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitCommClause(ctx *CommClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitCommCase(ctx *CommCaseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitRecvStmt(ctx *RecvStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitForStmt(ctx *ForStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitForClause(ctx *ForClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitRangeClause(ctx *RangeClauseContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitGoStmt(ctx *GoStmtContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitType_(ctx *Type_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeArgs(ctx *TypeArgsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeName(ctx *TypeNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeLit(ctx *TypeLitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitArrayType(ctx *ArrayTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitArrayLength(ctx *ArrayLengthContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitElementType(ctx *ElementTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitPointerType(ctx *PointerTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitInterfaceType(ctx *InterfaceTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSliceType(ctx *SliceTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitMapType(ctx *MapTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitChannelType(ctx *ChannelTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitMethodSpec(ctx *MethodSpecContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitFunctionType(ctx *FunctionTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSignature(ctx *SignatureContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitResult(ctx *ResultContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitParameters(ctx *ParametersContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitParameterDecl(ctx *ParameterDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitExpression(ctx *ExpressionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitPrimaryExpr(ctx *PrimaryExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitConversion(ctx *ConversionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitOperand(ctx *OperandContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitLiteral(ctx *LiteralContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitBasicLit(ctx *BasicLitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitInteger(ctx *IntegerContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitOperandName(ctx *OperandNameContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitQualifiedIdent(ctx *QualifiedIdentContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitCompositeLit(ctx *CompositeLitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitLiteralType(ctx *LiteralTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitLiteralValue(ctx *LiteralValueContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitElementList(ctx *ElementListContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitKeyedElement(ctx *KeyedElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitKey(ctx *KeyContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitElement(ctx *ElementContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitStructType(ctx *StructTypeContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitFieldDecl(ctx *FieldDeclContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitString_(ctx *String_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitChar_(ctx *Char_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitEmbeddedField(ctx *EmbeddedFieldContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitFunctionLit(ctx *FunctionLitContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitIndex(ctx *IndexContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitSlice_(ctx *Slice_Context) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitTypeAssertion(ctx *TypeAssertionContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitArguments(ctx *ArgumentsContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitMethodExpr(ctx *MethodExprContext) interface{} {
	return v.VisitChildren(ctx)
}

func (v *BaseGoParserVisitor) VisitEos(ctx *EosContext) interface{} {
	return v.VisitChildren(ctx)
}
