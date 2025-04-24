package js2ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
)
import "github.com/yaklang/yaklang/common/yak/ssa"

func (b *builder) GetRecoverRange(sourcefile *ast.SourceFile, node *core.TextRange, text string) func() {
	startLine, startCol := scanner.GetLineAndCharacterOfPosition(sourcefile, node.Pos())
	endLine, endCol := scanner.GetLineAndCharacterOfPosition(sourcefile, node.End())
	return b.SetRangeWithCommonTokenLoc(ssa.NewCommonTokenLoc(text, startLine, startCol, endLine, endCol))
}

func (b *builder) VisitSourceFile(sourcefile *ast.SourceFile) interface{} {
	if sourcefile == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(sourcefile, &sourcefile.Loc, sourcefile.Text())
	defer recoverRange()

	b.VisitStatements(sourcefile.Statements)
	return nil
}

func (b *builder) VisitStatements(stmtList *ast.NodeList) interface{} {
	if stmtList == nil || len(stmtList.Nodes) == 0 {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &stmtList.Loc, "")
	defer recoverRange()

	for _, stmt := range stmtList.Nodes {
		b.VisitStatement(stmt)
	}
	return nil
}

// ===== Statement =====

// VisitStatement 处理Statement相关
func (b *builder) VisitStatement(node *ast.Node) interface{} {
	if node == nil {
		return nil
	}
	if b.IsBlockFinish() {
		return nil
	}
	b.AppendBlockRange()

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	switch node.Kind {
	case ast.KindVariableStatement:
		b.VisitVariableStatement(node.AsVariableStatement())
	case ast.KindExpressionStatement:
		b.VisitExpressionStatement(node.AsExpressionStatement())
	case ast.KindIfStatement:
		b.VisitIfStatement(node.AsIfStatement())
	case ast.KindBlock:
		b.VisitBlock(node.AsBlock())
	case ast.KindDoStatement:
		b.VisitDoStatement(node.AsDoStatement())
	case ast.KindWhileStatement:
		b.VisitWhileStatement(node.AsWhileStatement())
	case ast.KindForStatement:
		b.VisitForStatement(node.AsForStatement())
	case ast.KindForInStatement, ast.KindForOfStatement:
		b.VisitForInOrOfStatement(node.AsForInOrOfStatement())
	case ast.KindFunctionDeclaration:
		b.VisitFunctionDeclaration(node.AsFunctionDeclaration())
	case ast.KindReturnStatement:
		b.VisitReturnStatement(node.AsReturnStatement())
	case ast.KindBreakStatement:
		b.VisitBreakStatement(node.AsBreakStatement())
	case ast.KindContinueStatement:
		b.VisitContinueStatement(node.AsContinueStatement())
	case ast.KindLabeledStatement:
		b.VisitLabeledStatement(node.AsLabeledStatement())
	case ast.KindTryStatement:
		b.VisitTryStatement(node.AsTryStatement())
	case ast.KindSwitchStatement:
		b.VisitSwitchStatement(node.AsSwitchStatement())
	case ast.KindThrowStatement:
		b.VisitThrowStatement(node.AsThrowStatement())
	case ast.KindEmptyStatement:
		b.VisitEmptyStatement(node.AsEmptyStatement())
	case ast.KindDebuggerStatement:
		b.VisitDebuggerStatement(node.AsDebuggerStatement())
	case ast.KindWithStatement:
		b.VisitWithStatement(node.AsWithStatement())
	case ast.KindClassDeclaration:
		b.VisitClassDeclaration(node.AsClassDeclaration())
	case ast.KindInterfaceDeclaration:
		b.VisitInterfaceDeclaration(node.AsInterfaceDeclaration())
	case ast.KindTypeAliasDeclaration:
		b.VisitTypeAliasDeclaration(node.AsTypeAliasDeclaration())
	case ast.KindEnumDeclaration:
		b.VisitEnumDeclaration(node.AsEnumDeclaration())
	case ast.KindModuleDeclaration:
		b.VisitModuleDeclaration(node.AsModuleDeclaration())
	case ast.KindImportEqualsDeclaration:
		b.VisitImportEqualsDeclaration(node.AsImportEqualsDeclaration())
	case ast.KindImportDeclaration:
		b.VisitImportDeclaration(node.AsImportDeclaration())
	case ast.KindExportAssignment:
		b.VisitExportAssignment(node.AsExportAssignment())
	case ast.KindNamespaceExportDeclaration:
		b.VisitNamespaceExportDeclaration(node.AsNamespaceExportDeclaration())
	case ast.KindExportDeclaration:
		b.VisitExportDeclaration(node.AsExportDeclaration())
	case ast.KindNotEmittedStatement:
		b.VisitNotEmittedStatement(node)
	default:
		panic("Unhandled Statement")
	}
	return nil
}

func (b *builder) VisitVariableStatement(node *ast.VariableStatement) interface{} {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	if decList := node.DeclarationList; decList != nil {
		b.VisitVariableDeclarationList(decList)
	}
	return nil
}

// VisitExpressionStatement 访问表达式语句
func (b *builder) VisitExpressionStatement(node *ast.ExpressionStatement) interface{} {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 处理表达式语句
	if node.Expression != nil {
		return b.VisitExpression(node.Expression)
	}
	return nil
}

// VisitIdentifier 访问标识符
func (b *builder) VisitIdentifier(node *ast.Identifier) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, node.Text)
	defer recoverRange()

	// 查找变量并返回
	if variable := b.GetVariable(node.Text); variable != nil {
		return variable.GetValue()
	}
	return b.CreateVariable(node.Text).Value
}

// VisitStringLiteral 访问字符串字面量
func (b *builder) VisitStringLiteral(node *ast.StringLiteral) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, node.Text)
	defer recoverRange()

	// 创建字符串常量
	return b.EmitConstInst(node.Text)
}

// VisitNumericLiteral 访问数字字面量
func (b *builder) VisitNumericLiteral(node *ast.NumericLiteral) ssa.Value {
	if node == nil {
		return nil
	}

	text := node.Text
	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, text)
	defer recoverRange()

	// 创建数字常量
	return b.EmitConstInst(utils.InterfaceToInt(text))
}

// VisitBooleanLiteral 访问布尔字面量
func (b *builder) VisitBooleanLiteral(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	var boolValue bool
	if node.Kind == ast.KindTrueKeyword {
		boolValue = true
	} else {
		boolValue = false
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建布尔常量
	return b.EmitConstInst(boolValue)
}

// VisitNullLiteral 访问null字面量
func (b *builder) VisitNullLiteral(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "null")
	defer recoverRange()

	// 创建null常量
	return b.EmitConstInstNil()
}

// VisitUndefinedLiteral 访问undefined字面量
func (b *builder) VisitUndefinedLiteral(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "undefined")
	defer recoverRange()

	// 创建undefined常量
	return b.EmitUndefined("")
}

// VisitIfStatement 访问if语句
func (b *builder) VisitIfStatement(node *ast.IfStatement) interface{} { return nil }

// VisitBlock 访问代码块
func (b *builder) VisitBlock(node *ast.Block) interface{} { return nil }

// VisitDoStatement 访问do-while语句
func (b *builder) VisitDoStatement(node *ast.DoStatement) interface{} { return nil }

// VisitWhileStatement 访问while语句
func (b *builder) VisitWhileStatement(node *ast.WhileStatement) interface{} { return nil }

// VisitForStatement 访问for语句
func (b *builder) VisitForStatement(node *ast.ForStatement) interface{} { return nil }

// VisitForInOrOfStatement 访问for-in和for-of语句
func (b *builder) VisitForInOrOfStatement(node *ast.ForInOrOfStatement) interface{} { return nil }

// VisitFunctionDeclaration 访问函数声明
func (b *builder) VisitFunctionDeclaration(node *ast.FunctionDeclaration) interface{} { return nil }

// VisitReturnStatement 访问return语句
func (b *builder) VisitReturnStatement(node *ast.ReturnStatement) interface{} { return nil }

// VisitBreakStatement 访问break语句
func (b *builder) VisitBreakStatement(node *ast.BreakStatement) interface{} { return nil }

// VisitContinueStatement 访问continue语句
func (b *builder) VisitContinueStatement(node *ast.ContinueStatement) interface{} { return nil }

// VisitLabeledStatement 访问带标签的语句
func (b *builder) VisitLabeledStatement(node *ast.LabeledStatement) interface{} { return nil }

// VisitTryStatement 访问try语句
func (b *builder) VisitTryStatement(node *ast.TryStatement) interface{} { return nil }

// VisitCatchClause 访问catch子句
func (b *builder) VisitCatchClause(node *ast.CatchClause) interface{} { return nil }

// VisitSwitchStatement 访问switch语句
func (b *builder) VisitSwitchStatement(node *ast.SwitchStatement) interface{} { return nil }

// VisitCaseBlock 访问case块
func (b *builder) VisitCaseBlock(node *ast.CaseBlock) interface{} { return nil }

// VisitCaseClause 访问case子句
func (b *builder) VisitCaseClause(node *ast.CaseOrDefaultClause) interface{} { return nil }

// VisitDefaultClause 访问default子句
func (b *builder) VisitDefaultClause(node *ast.CaseOrDefaultClause) interface{} { return nil }

// VisitThrowStatement 访问throw语句
func (b *builder) VisitThrowStatement(node *ast.ThrowStatement) interface{} { return nil }

// VisitEmptyStatement 访问空语句
func (b *builder) VisitEmptyStatement(node *ast.EmptyStatement) interface{} { return nil }

// VisitDebuggerStatement 访问debugger语句
func (b *builder) VisitDebuggerStatement(node *ast.DebuggerStatement) interface{} { return nil }

// VisitWithStatement 访问with语句
func (b *builder) VisitWithStatement(node *ast.WithStatement) interface{} { return nil }

// VisitClassDeclaration 访问类声明
func (b *builder) VisitClassDeclaration(node *ast.ClassDeclaration) interface{} { return nil }

// VisitHeritageClause 访问继承子句
func (b *builder) VisitHeritageClause(node *ast.HeritageClause) interface{} { return nil }

// VisitClassElement 访问类成员
func (b *builder) VisitClassElement(node *ast.Node) interface{} { return nil }

// ===== Declaration =====

// VisitVariableDeclarationList 访问变量声明列表
func (b *builder) VisitVariableDeclarationList(node *ast.VariableDeclarationListNode) interface{} {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	declList := node.AsVariableDeclarationList()
	for _, varDecl := range declList.Declarations.Nodes {
		b.VisitVariableDeclaration(varDecl, declList.Flags)
	}
	return nil
}

// VisitVariableDeclaration 访问变量声明
func (b *builder) VisitVariableDeclaration(node *ast.Node, declType ast.NodeFlags) interface{} {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	decl := node.AsVariableDeclaration()
	// 变量应该有名字
	if decl.Name() == nil {
		//b.NewError()
		return nil
	}
	declName := decl.Name().AsIdentifier().Text
	// 考虑js声明关键词let const var
	if declType != ast.NodeFlagsLet && declType != ast.NodeFlagsConst && declType != ast.NodeFlagsNone {
		//b.NewError()
		return nil
	}
	// const修饰的变量必须在声明时提供initializer
	if declType == ast.NodeFlagsConst && decl.Initializer == nil {
		//b.NewError()
		return nil
	}

	if decl.Initializer != nil { // 定义变量
		variable := b.CreateVariable(declName)
		value := b.VisitExpression(decl.Initializer)
		b.AssignVariable(variable, value)
	} else { // 声明变量
		newVariable := b.CreateVariable(declName)
		value := b.EmitValueOnlyDeclare(declName)
		b.AssignVariable(newVariable, value)

	}
	return nil
}

// VisitPropertyDeclaration 访问属性声明
func (b *builder) VisitPropertyDeclaration(node *ast.PropertyDeclaration) interface{} { return nil }

// VisitMethodDeclaration 访问方法声明
func (b *builder) VisitMethodDeclaration(node *ast.MethodDeclaration) interface{} { return nil }

// VisitConstructorDeclaration 访问构造函数声明
func (b *builder) VisitConstructorDeclaration(node *ast.ConstructorDeclaration) interface{} {
	return nil
}

// VisitGetAccessorDeclaration 访问getter声明
func (b *builder) VisitGetAccessorDeclaration(node *ast.GetAccessorDeclaration) interface{} {
	return nil
}

// VisitSetAccessorDeclaration 访问setter声明
func (b *builder) VisitSetAccessorDeclaration(node *ast.SetAccessorDeclaration) interface{} {
	return nil
}

// VisitClassStaticBlockDeclaration 访问静态代码块声明
func (b *builder) VisitClassStaticBlockDeclaration(node *ast.ClassStaticBlockDeclaration) interface{} {
	return nil
}

// VisitInterfaceDeclaration 访问接口声明
func (b *builder) VisitInterfaceDeclaration(node *ast.InterfaceDeclaration) interface{} { return nil }

// VisitPropertySignatureDeclaration 访问属性签名声明
func (b *builder) VisitPropertySignatureDeclaration(node *ast.PropertySignatureDeclaration) interface{} {
	return nil
}

// VisitMethodSignatureDeclaration 访问方法签名声明
func (b *builder) VisitMethodSignatureDeclaration(node *ast.MethodSignatureDeclaration) interface{} {
	return nil
}

// VisitIndexSignatureDeclaration 访问索引签名声明
func (b *builder) VisitIndexSignatureDeclaration(node *ast.IndexSignatureDeclaration) interface{} {
	return nil
}

// VisitCallSignatureDeclaration 访问调用签名声明
func (b *builder) VisitCallSignatureDeclaration(node *ast.CallSignatureDeclaration) interface{} {
	return nil
}

// VisitConstructSignatureDeclaration 访问构造签名声明
func (b *builder) VisitConstructSignatureDeclaration(node *ast.ConstructSignatureDeclaration) interface{} {
	return nil
}

// VisitTypeAliasDeclaration 访问类型别名声明
func (b *builder) VisitTypeAliasDeclaration(node *ast.TypeAliasDeclaration) interface{} { return nil }

// VisitEnumDeclaration 访问枚举声明
func (b *builder) VisitEnumDeclaration(node *ast.EnumDeclaration) interface{} { return nil }

// VisitEnumMember 访问枚举成员
func (b *builder) VisitEnumMember(node *ast.EnumMember) interface{} { return nil }

// VisitModuleDeclaration 访问模块声明
func (b *builder) VisitModuleDeclaration(node *ast.ModuleDeclaration) interface{} { return nil }

// VisitModuleBlock 访问模块块
func (b *builder) VisitModuleBlock(node *ast.ModuleBlock) interface{} { return nil }

// VisitImportDeclaration 访问导入声明
func (b *builder) VisitImportDeclaration(node *ast.ImportDeclaration) interface{} { return nil }

// VisitImportClause 访问导入子句
func (b *builder) VisitImportClause(node *ast.ImportClause) interface{} { return nil }

// VisitNamespaceImport 访问命名空间导入
func (b *builder) VisitNamespaceImport(node *ast.NamespaceImport) interface{} { return nil }

// VisitNamedImports 访问命名导入
func (b *builder) VisitNamedImports(node *ast.NamedImports) interface{} { return nil }

// VisitImportSpecifier 访问导入说明符
func (b *builder) VisitImportSpecifier(node *ast.ImportSpecifier) interface{} { return nil }

// VisitExportDeclaration 访问导出声明
func (b *builder) VisitExportDeclaration(node *ast.ExportDeclaration) interface{} { return nil }

// VisitNamedExports 访问命名导出
func (b *builder) VisitNamedExports(node *ast.NamedExports) interface{} { return nil }

// VisitExportSpecifier 访问导出说明符
func (b *builder) VisitExportSpecifier(node *ast.ExportSpecifier) interface{} { return nil }

// VisitExportAssignment 访问导出赋值
func (b *builder) VisitExportAssignment(node *ast.ExportAssignment) interface{} { return nil }

// VisitNamespaceExportDeclaration 访问命名空间导出声明
func (b *builder) VisitNamespaceExportDeclaration(node *ast.NamespaceExportDeclaration) interface{} {
	return nil
}

// VisitImportEqualsDeclaration 访问导入等号声明
func (b *builder) VisitImportEqualsDeclaration(node *ast.ImportEqualsDeclaration) interface{} {
	return nil
}

// VisitExternalModuleReference 访问外部模块引用
func (b *builder) VisitExternalModuleReference(node *ast.ExternalModuleReference) interface{} {
	return nil
}

// VisitNotEmittedStatement 访问不发出的语句
func (b *builder) VisitNotEmittedStatement(node *ast.Node) interface{} { return nil }

// =====Expression=====

// VisitExpression 访问表达式相关的访问函数
func (b *builder) VisitExpression(node *ast.Expression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	switch node.Kind {
	case ast.KindIdentifier:
		return b.VisitIdentifier(node.AsIdentifier())
	case ast.KindStringLiteral:
		return b.VisitStringLiteral(node.AsStringLiteral())
	case ast.KindNumericLiteral:
		return b.VisitNumericLiteral(node.AsNumericLiteral())
	case ast.KindBigIntLiteral:
		return b.VisitBigIntLiteral(node.AsBigIntLiteral())
	case ast.KindRegularExpressionLiteral:
		return b.VisitRegularExpressionLiteral(node.AsRegularExpressionLiteral())
	case ast.KindNoSubstitutionTemplateLiteral:
		return b.VisitNoSubstitutionTemplateLiteral(node.AsNoSubstitutionTemplateLiteral())
	case ast.KindTrueKeyword, ast.KindFalseKeyword:
		return b.VisitBooleanLiteral(node)
	case ast.KindNullKeyword:
		return b.VisitNullLiteral(node)
	case ast.KindUndefinedKeyword:
		return b.VisitUndefinedLiteral(node)
	case ast.KindThisKeyword:
		return b.VisitThisExpression(node)
	case ast.KindSuperKeyword:
		return b.VisitSuperExpression(node)
	case ast.KindObjectLiteralExpression:
		return b.VisitObjectLiteralExpression(node.AsObjectLiteralExpression())
	case ast.KindArrayLiteralExpression:
		return b.VisitArrayLiteralExpression(node.AsArrayLiteralExpression())
	case ast.KindBinaryExpression:
		return b.VisitBinaryExpression(node.AsBinaryExpression())
	case ast.KindPrefixUnaryExpression:
		return b.VisitPrefixUnaryExpression(node.AsPrefixUnaryExpression())
	case ast.KindPostfixUnaryExpression:
		return b.VisitPostfixUnaryExpression(node.AsPostfixUnaryExpression())
	case ast.KindCallExpression:
		return b.VisitCallExpression(node.AsCallExpression())
	case ast.KindPropertyAccessExpression:
		return b.VisitPropertyAccessExpression(node.AsPropertyAccessExpression())
	case ast.KindElementAccessExpression:
		return b.VisitElementAccessExpression(node.AsElementAccessExpression())
	case ast.KindNewExpression:
		return b.VisitNewExpression(node.AsNewExpression())
	case ast.KindParenthesizedExpression:
		return b.VisitParenthesizedExpression(node.AsParenthesizedExpression())
	case ast.KindFunctionExpression:
		return b.VisitFunctionExpression(node.AsFunctionExpression())
	case ast.KindArrowFunction:
		return b.VisitArrowFunction(node.AsArrowFunction())
	case ast.KindConditionalExpression:
		return b.VisitConditionalExpression(node.AsConditionalExpression())
	case ast.KindTemplateExpression:
		return b.VisitTemplateExpression(node.AsTemplateExpression())
	case ast.KindTaggedTemplateExpression:
		return b.VisitTaggedTemplateExpression(node.AsTaggedTemplateExpression())
	case ast.KindDeleteExpression:
		return b.VisitDeleteExpression(node.AsDeleteExpression())
	case ast.KindTypeOfExpression:
		return b.VisitTypeOfExpression(node.AsTypeOfExpression())
	case ast.KindVoidExpression:
		return b.VisitVoidExpression(node.AsVoidExpression())
	case ast.KindAwaitExpression:
		return b.VisitAwaitExpression(node.AsAwaitExpression())
	case ast.KindYieldExpression:
		return b.VisitYieldExpression(node.AsYieldExpression())
	case ast.KindSpreadElement:
		return b.VisitSpreadElement(node.AsSpreadElement())
	case ast.KindClassExpression:
		return b.VisitClassExpression(node.AsClassExpression())
	case ast.KindOmittedExpression:
		return b.VisitOmittedExpression(node)
	case ast.KindExpressionWithTypeArguments:
		return b.VisitExpressionWithTypeArguments(node.AsExpressionWithTypeArguments())
	case ast.KindAsExpression:
		return b.VisitAsExpression(node.AsAsExpression())
	case ast.KindNonNullExpression:
		return b.VisitNonNullExpression(node.AsNonNullExpression())
	case ast.KindMetaProperty:
		return b.VisitMetaProperty(node.AsMetaProperty())
	case ast.KindSyntheticExpression:
		return b.VisitSyntheticExpression(node)
	case ast.KindPartiallyEmittedExpression:
		return b.VisitPartiallyEmittedExpression(node.AsPartiallyEmittedExpression())
	case ast.KindCommaListExpression:
		return b.VisitCommaListExpression(node)
	case ast.KindJsxElement:
		return b.VisitJsxElement(node.AsJsxElement())
	case ast.KindJsxSelfClosingElement:
		return b.VisitJsxSelfClosingElement(node.AsJsxSelfClosingElement())
	case ast.KindJsxFragment:
		return b.VisitJsxFragment(node.AsJsxFragment())
	case ast.KindSatisfiesExpression:
		return b.VisitSatisfiesExpression(node.AsSatisfiesExpression())
	case ast.KindTypeAssertionExpression:
		return b.VisitTypeAssertion(node.AsTypeAssertion())
	case ast.KindComputedPropertyName:
		return b.VisitComputedPropertyName(node.AsComputedPropertyName())
	case ast.KindJsxSpreadAttribute:
		return b.VisitJsxSpreadAttribute(node.AsJsxSpreadAttribute())
	case ast.KindTemplateSpan:
		return b.VisitTemplateSpan(node.AsTemplateSpan())
	default:
		// 未处理的表达式类型
		panic("Unhandled Exp type")
	}
}

// VisitBinaryExpression 访问二元表达式
func (b *builder) VisitBinaryExpression(node *ast.BinaryExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	left := b.VisitExpression(node.Left)
	right := b.VisitExpression(node.Right)

	// 根据操作符类型生成不同的二元操作
	switch node.OperatorToken.Kind {
	case ast.KindPlusToken:
		return b.EmitBinOp(ssa.OpAdd, left, right)
	case ast.KindMinusToken:
		return b.EmitBinOp(ssa.OpSub, left, right)
	case ast.KindAsteriskToken:
		return b.EmitBinOp(ssa.OpMul, left, right)
	case ast.KindSlashToken:
		return b.EmitBinOp(ssa.OpDiv, left, right)
	case ast.KindPercentToken:
		return b.EmitBinOp(ssa.OpMod, left, right)
	case ast.KindAsteriskAsteriskToken:
		return b.EmitBinOp(ssa.OpPow, left, right)
	case ast.KindLessThanToken:
		return b.EmitBinOp(ssa.OpLt, left, right)
	case ast.KindGreaterThanToken:
		return b.EmitBinOp(ssa.OpGt, left, right)
	case ast.KindLessThanEqualsToken:
		return b.EmitBinOp(ssa.OpLtEq, left, right)
	case ast.KindGreaterThanEqualsToken:
		return b.EmitBinOp(ssa.OpGtEq, left, right)
	case ast.KindEqualsEqualsToken, ast.KindEqualsEqualsEqualsToken:
		return b.EmitBinOp(ssa.OpEq, left, right)
	case ast.KindExclamationEqualsToken, ast.KindExclamationEqualsEqualsToken:
		return b.EmitBinOp(ssa.OpNotEq, left, right)
	case ast.KindAmpersandToken:
		return b.EmitBinOp(ssa.OpAnd, left, right)
	case ast.KindBarToken:
		return b.EmitBinOp(ssa.OpOr, left, right)
	case ast.KindCaretToken: // ^
		return b.EmitBinOp(ssa.OpXor, left, right)
	case ast.KindLessThanLessThanToken:
		return b.EmitBinOp(ssa.OpShl, left, right)
	case ast.KindGreaterThanGreaterThanToken:
		return b.EmitBinOp(ssa.OpShr, left, right)
	//case ast.KindGreaterThanGreaterThanGreaterThanToken:
	//	return b.EmitBinOp(ssa.OpShr, left, right)
	case ast.KindAmpersandAmpersandToken:
		return b.EmitBinOp(ssa.OpLogicAnd, left, right)
	case ast.KindBarBarToken:
		return b.EmitBinOp(ssa.OpLogicOr, left, right)
	//case ast.KindEqualsToken:
	//return b.Emit(left, right)
	// 处理其他运算符...
	default:
		// 未实现的操作符处理
		return b.EmitUndefined("")
	}
}

// VisitCallExpression 处理函数调用表达式
func (b *builder) VisitCallExpression(node *ast.CallExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 处理参数列表
	var args []ssa.Value
	if node.ArgumentList() != nil && len(node.Arguments.Nodes) > 0 {
		for _, argNode := range node.Arguments.Nodes {
			argValue := b.VisitExpression(argNode)
			if argValue != nil {
				args = append(args, argValue)
			} else {
				// 如果参数无法解析，使用undefined代替
				args = append(args, b.EmitUndefined(""))
			}
		}
	}

	// 处理不同类型的函数调用
	switch node.Expression.Kind {
	case ast.KindIdentifier:
		// 处理普通函数调用
		funcName := node.Expression.AsIdentifier().Text
		return b.EmitCall(b.NewCall(b.ReadValue(funcName), args))

	default:
		// 其他类型的调用表达式
		panic("Unhandled call type")
	}
}

// VisitObjectLiteralExpression 访问对象字面量表达式
func (b *builder) VisitObjectLiteralExpression(node *ast.ObjectLiteralExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitArrayLiteralExpression 访问数组字面量表达式
func (b *builder) VisitArrayLiteralExpression(node *ast.ArrayLiteralExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitPrefixUnaryExpression 访问前缀一元表达式
func (b *builder) VisitPrefixUnaryExpression(node *ast.PrefixUnaryExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitPostfixUnaryExpression 访问后缀一元表达式
func (b *builder) VisitPostfixUnaryExpression(node *ast.PostfixUnaryExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitPropertyAccessExpression 访问属性访问表达式
func (b *builder) VisitPropertyAccessExpression(node *ast.PropertyAccessExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitElementAccessExpression 访问元素访问表达式
func (b *builder) VisitElementAccessExpression(node *ast.ElementAccessExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitNewExpression 访问new表达式
func (b *builder) VisitNewExpression(node *ast.NewExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitParenthesizedExpression 访问带括号的表达式
func (b *builder) VisitParenthesizedExpression(node *ast.ParenthesizedExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitFunctionExpression 访问函数表达式
func (b *builder) VisitFunctionExpression(node *ast.FunctionExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitArrowFunction 访问箭头函数
func (b *builder) VisitArrowFunction(node *ast.ArrowFunction) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitConditionalExpression 访问条件表达式
func (b *builder) VisitConditionalExpression(node *ast.ConditionalExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitTemplateExpression 访问模板表达式
func (b *builder) VisitTemplateExpression(node *ast.TemplateExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitNoSubstitutionTemplateLiteral 访问无替换模板字面量
func (b *builder) VisitNoSubstitutionTemplateLiteral(node *ast.NoSubstitutionTemplateLiteral) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitTaggedTemplateExpression 访问标记模板表达式
func (b *builder) VisitTaggedTemplateExpression(node *ast.TaggedTemplateExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitSpreadElement 访问展开元素
func (b *builder) VisitSpreadElement(node *ast.SpreadElement) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitDeleteExpression 访问delete表达式
func (b *builder) VisitDeleteExpression(node *ast.DeleteExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitTypeOfExpression 访问typeof表达式
func (b *builder) VisitTypeOfExpression(node *ast.TypeOfExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitVoidExpression 访问void表达式
func (b *builder) VisitVoidExpression(node *ast.VoidExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitAwaitExpression 访问await表达式
func (b *builder) VisitAwaitExpression(node *ast.AwaitExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitYieldExpression 访问yield表达式
func (b *builder) VisitYieldExpression(node *ast.YieldExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitTypeAssertion 访问类型断言
func (b *builder) VisitTypeAssertion(node *ast.TypeAssertion) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitAsExpression 访问as表达式
func (b *builder) VisitAsExpression(node *ast.AsExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitSatisfiesExpression 访问satisfies表达式
func (b *builder) VisitSatisfiesExpression(node *ast.SatisfiesExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitNonNullExpression 访问非空表达式
func (b *builder) VisitNonNullExpression(node *ast.NonNullExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitMetaProperty 访问元属性
func (b *builder) VisitMetaProperty(node *ast.MetaProperty) ssa.Value { return b.EmitUndefined("") }

// VisitPropertyAssignment 访问属性赋值
func (b *builder) VisitPropertyAssignment(node *ast.PropertyAssignment) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitShorthandPropertyAssignment 访问简写属性赋值
func (b *builder) VisitShorthandPropertyAssignment(node *ast.ShorthandPropertyAssignment) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitSpreadAssignment 访问展开赋值
func (b *builder) VisitSpreadAssignment(node *ast.SpreadAssignment) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitJsxElement 访问JSX元素
func (b *builder) VisitJsxElement(node *ast.JsxElement) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitTemplateSpan 访问模板跨度
func (b *builder) VisitTemplateSpan(node *ast.TemplateSpan) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitBigIntLiteral 访问BigInt字面量
func (b *builder) VisitBigIntLiteral(node *ast.BigIntLiteral) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitRegularExpressionLiteral 访问正则表达式字面量
func (b *builder) VisitRegularExpressionLiteral(node *ast.RegularExpressionLiteral) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitThisExpression 访问this表达式
func (b *builder) VisitThisExpression(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	return nil
}

// VisitSuperExpression 访问super表达式
func (b *builder) VisitSuperExpression(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "super")
	defer recoverRange()

	return nil
}

// VisitClassExpression 访问类表达式
func (b *builder) VisitClassExpression(node *ast.ClassExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitOmittedExpression 访问省略表达式
func (b *builder) VisitOmittedExpression(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitExpressionWithTypeArguments 访问带类型参数的表达式
func (b *builder) VisitExpressionWithTypeArguments(node *ast.ExpressionWithTypeArguments) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitSyntheticExpression 访问合成表达式
func (b *builder) VisitSyntheticExpression(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitPartiallyEmittedExpression 访问部分发出的表达式
func (b *builder) VisitPartiallyEmittedExpression(node *ast.PartiallyEmittedExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 部分发出表达式包装了另一个表达式，实际上应该处理其包含的表达式
	if node.Expression != nil {
		return b.VisitExpression(node.Expression)
	}

	return b.EmitUndefined("")
}

// VisitCommaListExpression 访问逗号列表表达式
func (b *builder) VisitCommaListExpression(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxSelfClosingElement 访问JSX自闭合元素
func (b *builder) VisitJsxSelfClosingElement(node *ast.JsxSelfClosingElement) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxFragment 访问JSX片段
func (b *builder) VisitJsxFragment(node *ast.JsxFragment) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxAttributes 访问JSX属性
func (b *builder) VisitJsxAttributes(node *ast.JsxAttributes) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxAttributeValue 访问JSX属性值
func (b *builder) VisitJsxAttributeValue(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxChild 访问JSX子元素
func (b *builder) VisitJsxChild(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitComputedPropertyName 访问计算属性名
// 处理对象字面量或类中使用计算属性名的情况，如 { [expr]: value }
func (b *builder) VisitComputedPropertyName(node *ast.ComputedPropertyName) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 处理计算属性表达式
	if node.Expression != nil {
		return b.VisitExpression(node.Expression)
	}

	return b.EmitUndefined("")
}

// VisitJsxSpreadAttribute 访问JSX展开属性
func (b *builder) VisitJsxSpreadAttribute(node *ast.JsxSpreadAttribute) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// ===== MISC =====

// VisitTypeElement 访问类型元素
func (b *builder) VisitTypeElement(node *ast.Node) interface{} { return nil }
