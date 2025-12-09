// Code generated from java-escape by ANTLR 4.11.1. DO NOT EDIT.

package gol // GoParser
import "github.com/antlr/antlr4/runtime/Go/antlr/v4"

// A complete Visitor for a parse tree produced by GoParser.
type GoParserVisitor interface {
	antlr.ParseTreeVisitor

	// Visit a parse tree produced by GoParser#sourceFile.
	VisitSourceFile(ctx *SourceFileContext) interface{}

	// Visit a parse tree produced by GoParser#packageClause.
	VisitPackageClause(ctx *PackageClauseContext) interface{}

	// Visit a parse tree produced by GoParser#packageName.
	VisitPackageName(ctx *PackageNameContext) interface{}

	// Visit a parse tree produced by GoParser#importDecl.
	VisitImportDecl(ctx *ImportDeclContext) interface{}

	// Visit a parse tree produced by GoParser#importSpec.
	VisitImportSpec(ctx *ImportSpecContext) interface{}

	// Visit a parse tree produced by GoParser#importPath.
	VisitImportPath(ctx *ImportPathContext) interface{}

	// Visit a parse tree produced by GoParser#declaration.
	VisitDeclaration(ctx *DeclarationContext) interface{}

	// Visit a parse tree produced by GoParser#constDecl.
	VisitConstDecl(ctx *ConstDeclContext) interface{}

	// Visit a parse tree produced by GoParser#constSpec.
	VisitConstSpec(ctx *ConstSpecContext) interface{}

	// Visit a parse tree produced by GoParser#identifierList.
	VisitIdentifierList(ctx *IdentifierListContext) interface{}

	// Visit a parse tree produced by GoParser#expressionList.
	VisitExpressionList(ctx *ExpressionListContext) interface{}

	// Visit a parse tree produced by GoParser#typeDecl.
	VisitTypeDecl(ctx *TypeDeclContext) interface{}

	// Visit a parse tree produced by GoParser#typeSpec.
	VisitTypeSpec(ctx *TypeSpecContext) interface{}

	// Visit a parse tree produced by GoParser#aliasDecl.
	VisitAliasDecl(ctx *AliasDeclContext) interface{}

	// Visit a parse tree produced by GoParser#typeDef.
	VisitTypeDef(ctx *TypeDefContext) interface{}

	// Visit a parse tree produced by GoParser#typeParameters.
	VisitTypeParameters(ctx *TypeParametersContext) interface{}

	// Visit a parse tree produced by GoParser#typeParameterDecl.
	VisitTypeParameterDecl(ctx *TypeParameterDeclContext) interface{}

	// Visit a parse tree produced by GoParser#typeElement.
	VisitTypeElement(ctx *TypeElementContext) interface{}

	// Visit a parse tree produced by GoParser#typeTerm.
	VisitTypeTerm(ctx *TypeTermContext) interface{}

	// Visit a parse tree produced by GoParser#functionDecl.
	VisitFunctionDecl(ctx *FunctionDeclContext) interface{}

	// Visit a parse tree produced by GoParser#methodDecl.
	VisitMethodDecl(ctx *MethodDeclContext) interface{}

	// Visit a parse tree produced by GoParser#receiver.
	VisitReceiver(ctx *ReceiverContext) interface{}

	// Visit a parse tree produced by GoParser#varDecl.
	VisitVarDecl(ctx *VarDeclContext) interface{}

	// Visit a parse tree produced by GoParser#varSpec.
	VisitVarSpec(ctx *VarSpecContext) interface{}

	// Visit a parse tree produced by GoParser#block.
	VisitBlock(ctx *BlockContext) interface{}

	// Visit a parse tree produced by GoParser#statementList.
	VisitStatementList(ctx *StatementListContext) interface{}

	// Visit a parse tree produced by GoParser#statement.
	VisitStatement(ctx *StatementContext) interface{}

	// Visit a parse tree produced by GoParser#simpleStmt.
	VisitSimpleStmt(ctx *SimpleStmtContext) interface{}

	// Visit a parse tree produced by GoParser#assignment.
	VisitAssignment(ctx *AssignmentContext) interface{}

	// Visit a parse tree produced by GoParser#assign_op.
	VisitAssign_op(ctx *Assign_opContext) interface{}

	// Visit a parse tree produced by GoParser#expressionStmt.
	VisitExpressionStmt(ctx *ExpressionStmtContext) interface{}

	// Visit a parse tree produced by GoParser#sendStmt.
	VisitSendStmt(ctx *SendStmtContext) interface{}

	// Visit a parse tree produced by GoParser#incDecStmt.
	VisitIncDecStmt(ctx *IncDecStmtContext) interface{}

	// Visit a parse tree produced by GoParser#shortVarDecl.
	VisitShortVarDecl(ctx *ShortVarDeclContext) interface{}

	// Visit a parse tree produced by GoParser#labeledStmt.
	VisitLabeledStmt(ctx *LabeledStmtContext) interface{}

	// Visit a parse tree produced by GoParser#returnStmt.
	VisitReturnStmt(ctx *ReturnStmtContext) interface{}

	// Visit a parse tree produced by GoParser#breakStmt.
	VisitBreakStmt(ctx *BreakStmtContext) interface{}

	// Visit a parse tree produced by GoParser#continueStmt.
	VisitContinueStmt(ctx *ContinueStmtContext) interface{}

	// Visit a parse tree produced by GoParser#gotoStmt.
	VisitGotoStmt(ctx *GotoStmtContext) interface{}

	// Visit a parse tree produced by GoParser#fallthroughStmt.
	VisitFallthroughStmt(ctx *FallthroughStmtContext) interface{}

	// Visit a parse tree produced by GoParser#deferStmt.
	VisitDeferStmt(ctx *DeferStmtContext) interface{}

	// Visit a parse tree produced by GoParser#ifStmt.
	VisitIfStmt(ctx *IfStmtContext) interface{}

	// Visit a parse tree produced by GoParser#switchStmt.
	VisitSwitchStmt(ctx *SwitchStmtContext) interface{}

	// Visit a parse tree produced by GoParser#exprSwitchStmt.
	VisitExprSwitchStmt(ctx *ExprSwitchStmtContext) interface{}

	// Visit a parse tree produced by GoParser#exprCaseClause.
	VisitExprCaseClause(ctx *ExprCaseClauseContext) interface{}

	// Visit a parse tree produced by GoParser#exprSwitchCase.
	VisitExprSwitchCase(ctx *ExprSwitchCaseContext) interface{}

	// Visit a parse tree produced by GoParser#typeSwitchStmt.
	VisitTypeSwitchStmt(ctx *TypeSwitchStmtContext) interface{}

	// Visit a parse tree produced by GoParser#typeSwitchGuard.
	VisitTypeSwitchGuard(ctx *TypeSwitchGuardContext) interface{}

	// Visit a parse tree produced by GoParser#typeCaseClause.
	VisitTypeCaseClause(ctx *TypeCaseClauseContext) interface{}

	// Visit a parse tree produced by GoParser#typeSwitchCase.
	VisitTypeSwitchCase(ctx *TypeSwitchCaseContext) interface{}

	// Visit a parse tree produced by GoParser#typeList.
	VisitTypeList(ctx *TypeListContext) interface{}

	// Visit a parse tree produced by GoParser#selectStmt.
	VisitSelectStmt(ctx *SelectStmtContext) interface{}

	// Visit a parse tree produced by GoParser#commClause.
	VisitCommClause(ctx *CommClauseContext) interface{}

	// Visit a parse tree produced by GoParser#commCase.
	VisitCommCase(ctx *CommCaseContext) interface{}

	// Visit a parse tree produced by GoParser#recvStmt.
	VisitRecvStmt(ctx *RecvStmtContext) interface{}

	// Visit a parse tree produced by GoParser#forStmt.
	VisitForStmt(ctx *ForStmtContext) interface{}

	// Visit a parse tree produced by GoParser#forClause.
	VisitForClause(ctx *ForClauseContext) interface{}

	// Visit a parse tree produced by GoParser#rangeClause.
	VisitRangeClause(ctx *RangeClauseContext) interface{}

	// Visit a parse tree produced by GoParser#goStmt.
	VisitGoStmt(ctx *GoStmtContext) interface{}

	// Visit a parse tree produced by GoParser#type_.
	VisitType_(ctx *Type_Context) interface{}

	// Visit a parse tree produced by GoParser#typeArgs.
	VisitTypeArgs(ctx *TypeArgsContext) interface{}

	// Visit a parse tree produced by GoParser#typeName.
	VisitTypeName(ctx *TypeNameContext) interface{}

	// Visit a parse tree produced by GoParser#typeLit.
	VisitTypeLit(ctx *TypeLitContext) interface{}

	// Visit a parse tree produced by GoParser#arrayType.
	VisitArrayType(ctx *ArrayTypeContext) interface{}

	// Visit a parse tree produced by GoParser#arrayLength.
	VisitArrayLength(ctx *ArrayLengthContext) interface{}

	// Visit a parse tree produced by GoParser#elementType.
	VisitElementType(ctx *ElementTypeContext) interface{}

	// Visit a parse tree produced by GoParser#pointerType.
	VisitPointerType(ctx *PointerTypeContext) interface{}

	// Visit a parse tree produced by GoParser#interfaceType.
	VisitInterfaceType(ctx *InterfaceTypeContext) interface{}

	// Visit a parse tree produced by GoParser#sliceType.
	VisitSliceType(ctx *SliceTypeContext) interface{}

	// Visit a parse tree produced by GoParser#mapType.
	VisitMapType(ctx *MapTypeContext) interface{}

	// Visit a parse tree produced by GoParser#channelType.
	VisitChannelType(ctx *ChannelTypeContext) interface{}

	// Visit a parse tree produced by GoParser#methodSpec.
	VisitMethodSpec(ctx *MethodSpecContext) interface{}

	// Visit a parse tree produced by GoParser#functionType.
	VisitFunctionType(ctx *FunctionTypeContext) interface{}

	// Visit a parse tree produced by GoParser#signature.
	VisitSignature(ctx *SignatureContext) interface{}

	// Visit a parse tree produced by GoParser#result.
	VisitResult(ctx *ResultContext) interface{}

	// Visit a parse tree produced by GoParser#parameters.
	VisitParameters(ctx *ParametersContext) interface{}

	// Visit a parse tree produced by GoParser#parameterDecl.
	VisitParameterDecl(ctx *ParameterDeclContext) interface{}

	// Visit a parse tree produced by GoParser#expression.
	VisitExpression(ctx *ExpressionContext) interface{}

	// Visit a parse tree produced by GoParser#primaryExpr.
	VisitPrimaryExpr(ctx *PrimaryExprContext) interface{}

	// Visit a parse tree produced by GoParser#conversion.
	VisitConversion(ctx *ConversionContext) interface{}

	// Visit a parse tree produced by GoParser#operand.
	VisitOperand(ctx *OperandContext) interface{}

	// Visit a parse tree produced by GoParser#literal.
	VisitLiteral(ctx *LiteralContext) interface{}

	// Visit a parse tree produced by GoParser#basicLit.
	VisitBasicLit(ctx *BasicLitContext) interface{}

	// Visit a parse tree produced by GoParser#integer.
	VisitInteger(ctx *IntegerContext) interface{}

	// Visit a parse tree produced by GoParser#operandName.
	VisitOperandName(ctx *OperandNameContext) interface{}

	// Visit a parse tree produced by GoParser#qualifiedIdent.
	VisitQualifiedIdent(ctx *QualifiedIdentContext) interface{}

	// Visit a parse tree produced by GoParser#compositeLit.
	VisitCompositeLit(ctx *CompositeLitContext) interface{}

	// Visit a parse tree produced by GoParser#literalType.
	VisitLiteralType(ctx *LiteralTypeContext) interface{}

	// Visit a parse tree produced by GoParser#literalValue.
	VisitLiteralValue(ctx *LiteralValueContext) interface{}

	// Visit a parse tree produced by GoParser#elementList.
	VisitElementList(ctx *ElementListContext) interface{}

	// Visit a parse tree produced by GoParser#keyedElement.
	VisitKeyedElement(ctx *KeyedElementContext) interface{}

	// Visit a parse tree produced by GoParser#key.
	VisitKey(ctx *KeyContext) interface{}

	// Visit a parse tree produced by GoParser#element.
	VisitElement(ctx *ElementContext) interface{}

	// Visit a parse tree produced by GoParser#structType.
	VisitStructType(ctx *StructTypeContext) interface{}

	// Visit a parse tree produced by GoParser#fieldDecl.
	VisitFieldDecl(ctx *FieldDeclContext) interface{}

	// Visit a parse tree produced by GoParser#string_.
	VisitString_(ctx *String_Context) interface{}

	// Visit a parse tree produced by GoParser#char_.
	VisitChar_(ctx *Char_Context) interface{}

	// Visit a parse tree produced by GoParser#embeddedField.
	VisitEmbeddedField(ctx *EmbeddedFieldContext) interface{}

	// Visit a parse tree produced by GoParser#functionLit.
	VisitFunctionLit(ctx *FunctionLitContext) interface{}

	// Visit a parse tree produced by GoParser#index.
	VisitIndex(ctx *IndexContext) interface{}

	// Visit a parse tree produced by GoParser#slice_.
	VisitSlice_(ctx *Slice_Context) interface{}

	// Visit a parse tree produced by GoParser#typeAssertion.
	VisitTypeAssertion(ctx *TypeAssertionContext) interface{}

	// Visit a parse tree produced by GoParser#arguments.
	VisitArguments(ctx *ArgumentsContext) interface{}

	// Visit a parse tree produced by GoParser#methodExpr.
	VisitMethodExpr(ctx *MethodExprContext) interface{}

	// Visit a parse tree produced by GoParser#eos.
	VisitEos(ctx *EosContext) interface{}
}
