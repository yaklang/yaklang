package js2ssa

import (
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"strconv"
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
		return b.VisitRightValueExpression(node.Expression)
	}
	return nil
}

// VisitIdentifier 访问标识符
func (b *builder) VisitIdentifier(node *ast.Identifier) string {
	if node == nil {
		return ""
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, node.Text)
	defer recoverRange()

	// 查找变量并返回
	return node.Text
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
	return b.EmitConstInst(codec.Atoi(text))
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
		b.VisitVariableDeclaration(varDecl.AsVariableDeclaration(), declList.Flags)
	}
	return nil
}

// VisitVariableDeclaration 访问变量声明
func (b *builder) VisitVariableDeclaration(decl *ast.VariableDeclaration, declType ast.NodeFlags) interface{} {
	if decl == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &decl.Loc, "")
	defer recoverRange()
	// === Fast Fail Start ===
	if decl.Name() == nil {
		b.NewError(ssa.Error, TAG, NoDeclaraionName())
		return nil
	}

	// 考虑js声明关键词let const var
	if declType != ast.NodeFlagsLet && declType != ast.NodeFlagsConst && declType != ast.NodeFlagsNone {
		b.NewError(ssa.Error, TAG, UnexpectedVariableDeclarationModifierError(strconv.Itoa(int(declType))))
		return nil
	}

	// const修饰的变量必须在声明时提供initializer
	if declType == ast.NodeFlagsConst && decl.Initializer == nil {
		b.NewError(ssa.Error, TAG, ConstDeclarationWithoutInitializer())
		return nil
	}
	// === Fast Fail End ===

	/*
		let x = 1;                    // Identifier
		let [a, b] = [1, 2];          // ArrayBindingPattern
		let { x: y, z } = obj;        // ObjectBindingPattern
	*/

	// 获取声明的初始化值（如果有的话）
	var initValue ssa.Value
	if decl.Initializer != nil {
		initValue = b.VisitRightValueExpression(decl.Initializer)
	}

	// 处理变量名(BindingPattern | Identifier)
	name := decl.Name()

	switch {
	case ast.IsIdentifier(name): // 简单变量: let x = value
		identifier := b.VisitIdentifier(name.AsIdentifier())
		variable := b.CreateVariable(identifier)

		if decl.Initializer != nil { // 定义变量
			b.AssignVariable(variable, initValue)
		} else { // 仅声明变量
			undefinedValue := b.EmitValueOnlyDeclare(identifier)
			b.AssignVariable(variable, undefinedValue)
		}

	case ast.IsBindingPattern(name): // 解构模式: let {a, b} = obj 或 let [x, y] = arr
		// 检查是否有初始化值
		if initValue == nil {
			b.NewError(ssa.Error, TAG, BindPatternDeclarationWithoutInitializer())
			return nil
		}

		// 根据绑定模式类型处理
		if ast.IsObjectBindingPattern(name) {
			// 对象解构: let {a, b} = obj
			b.ProcessObjectBindingPattern(name.AsBindingPattern(), initValue)
		} else if ast.IsArrayBindingPattern(name) {
			// 数组解构: let [x, y] = arr
			b.ProcessArrayBindingPattern(name.AsBindingPattern(), initValue)
		}

	default:
		panic("unexpected variable declaration type")
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
// VisitExpression 返回L-Val和R-Val分别对应返回类型*ssa.Variable和ssa.Value
func (b *builder) VisitExpression(node *ast.Expression, isLval bool) (*ssa.Variable, ssa.Value) {
	if node == nil {
		return nil, nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	switch node.Kind {
	// 需要区分L/R Value的类型
	case ast.KindIdentifier:
		identifierName := b.VisitIdentifier(node.AsIdentifier())
		if isLval {
			return b.CreateVariable(identifierName), nil
		}
		if identifierName == "undefined" {
			return nil, b.EmitUndefined("")
		}
		if identifierName == "null" {
			return nil, b.EmitConstInstNil()
		}
		return nil, b.ReadValue(identifierName)
	case ast.KindPropertyAccessExpression:
		b.VisitPropertyAccessExpression(node.AsPropertyAccessExpression())
		if isLval {
			return nil, nil
		}
		return nil, nil
	case ast.KindElementAccessExpression:
		b.VisitElementAccessExpression(node.AsElementAccessExpression())
		if isLval {
			return nil, nil
		}
		return nil, nil

	// 只会是RValue
	case ast.KindStringLiteral:
		return nil, b.VisitStringLiteral(node.AsStringLiteral())
	case ast.KindNumericLiteral:
		return nil, b.VisitNumericLiteral(node.AsNumericLiteral())
	case ast.KindBigIntLiteral:
		return nil, b.VisitBigIntLiteral(node.AsBigIntLiteral())
	case ast.KindRegularExpressionLiteral:
		return nil, b.VisitRegularExpressionLiteral(node.AsRegularExpressionLiteral())
	case ast.KindNoSubstitutionTemplateLiteral:
		return nil, b.VisitNoSubstitutionTemplateLiteral(node.AsNoSubstitutionTemplateLiteral())
	case ast.KindTrueKeyword, ast.KindFalseKeyword:
		return nil, b.VisitBooleanLiteral(node)
	case ast.KindNullKeyword:
		return nil, b.VisitNullLiteral(node)
	case ast.KindUndefinedKeyword:
		return nil, b.VisitUndefinedLiteral(node)
	case ast.KindThisKeyword:
		return nil, b.VisitThisExpression(node)
	case ast.KindSuperKeyword:
		return nil, b.VisitSuperExpression(node)
	case ast.KindObjectLiteralExpression:
		return nil, b.VisitObjectLiteralExpression(node.AsObjectLiteralExpression())
	case ast.KindArrayLiteralExpression:
		return nil, b.VisitArrayLiteralExpression(node.AsArrayLiteralExpression())
	case ast.KindBinaryExpression:
		return nil, b.VisitBinaryExpression(node.AsBinaryExpression())
	case ast.KindPrefixUnaryExpression:
		return nil, b.VisitPrefixUnaryExpression(node.AsPrefixUnaryExpression())
	case ast.KindPostfixUnaryExpression:
		return nil, b.VisitPostfixUnaryExpression(node.AsPostfixUnaryExpression())
	case ast.KindCallExpression:
		return nil, b.VisitCallExpression(node.AsCallExpression())

	case ast.KindNewExpression:
		return nil, b.VisitNewExpression(node.AsNewExpression())
	case ast.KindParenthesizedExpression:
		return nil, b.VisitParenthesizedExpression(node.AsParenthesizedExpression())
	case ast.KindFunctionExpression:
		return nil, b.VisitFunctionExpression(node.AsFunctionExpression())
	case ast.KindArrowFunction:
		return nil, b.VisitArrowFunction(node.AsArrowFunction())
	case ast.KindConditionalExpression:
		return nil, b.VisitConditionalExpression(node.AsConditionalExpression())
	case ast.KindTemplateExpression:
		return nil, b.VisitTemplateExpression(node.AsTemplateExpression())
	case ast.KindTaggedTemplateExpression:
		return nil, b.VisitTaggedTemplateExpression(node.AsTaggedTemplateExpression())
	case ast.KindDeleteExpression:
		return nil, b.VisitDeleteExpression(node.AsDeleteExpression())
	case ast.KindTypeOfExpression:
		return nil, b.VisitTypeOfExpression(node.AsTypeOfExpression())
	case ast.KindVoidExpression:
		return nil, b.VisitVoidExpression(node.AsVoidExpression())
	case ast.KindAwaitExpression:
		return nil, b.VisitAwaitExpression(node.AsAwaitExpression())
	case ast.KindYieldExpression:
		return nil, b.VisitYieldExpression(node.AsYieldExpression())
	case ast.KindSpreadElement:
		return nil, b.VisitSpreadElement(node.AsSpreadElement())
	case ast.KindClassExpression:
		return nil, b.VisitClassExpression(node.AsClassExpression())
	case ast.KindOmittedExpression:
		return nil, b.VisitOmittedExpression(node)
	case ast.KindExpressionWithTypeArguments:
		return nil, b.VisitExpressionWithTypeArguments(node.AsExpressionWithTypeArguments())
	case ast.KindAsExpression:
		return nil, b.VisitAsExpression(node.AsAsExpression())
	case ast.KindNonNullExpression:
		return nil, b.VisitNonNullExpression(node.AsNonNullExpression())
	case ast.KindMetaProperty:
		return nil, b.VisitMetaProperty(node.AsMetaProperty())
	case ast.KindSyntheticExpression:
		return nil, b.VisitSyntheticExpression(node)
	case ast.KindPartiallyEmittedExpression:
		return nil, b.VisitPartiallyEmittedExpression(node.AsPartiallyEmittedExpression())
	case ast.KindCommaListExpression:
		return nil, b.VisitCommaListExpression(node)
	case ast.KindJsxElement:
		return nil, b.VisitJsxElement(node.AsJsxElement())
	case ast.KindJsxSelfClosingElement:
		return nil, b.VisitJsxSelfClosingElement(node.AsJsxSelfClosingElement())
	case ast.KindJsxFragment:
		return nil, b.VisitJsxFragment(node.AsJsxFragment())
	case ast.KindSatisfiesExpression:
		return nil, b.VisitSatisfiesExpression(node.AsSatisfiesExpression())
	case ast.KindTypeAssertionExpression:
		return nil, b.VisitTypeAssertion(node.AsTypeAssertion())
	case ast.KindComputedPropertyName:
		return nil, b.VisitComputedPropertyName(node.AsComputedPropertyName())
	case ast.KindJsxSpreadAttribute:
		return nil, b.VisitJsxSpreadAttribute(node.AsJsxSpreadAttribute())
	case ast.KindTemplateSpan:
		return nil, b.VisitTemplateSpan(node.AsTemplateSpan())
	default:
		// 未处理的表达式类型
		panic("Unhandled Exp type")
	}
}

// VisitBinaryExpression 访问二元表达式
// in和instanceof 还没处理
func (b *builder) VisitBinaryExpression(node *ast.BinaryExpression) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	left := b.VisitRightValueExpression(node.Left)
	right := b.VisitRightValueExpression(node.Right)

	// 根据操作符类型生成不同的二元操作
	switch node.OperatorToken.Kind {
	// Arithmetic PLUS + MINUS - MUL * DIV / MOD % POW **
	case ast.KindPlusToken, ast.KindMinusToken, ast.KindAsteriskToken, ast.KindSlashToken, ast.KindPercentToken, ast.KindAsteriskAsteriskToken:
		binOp, ok := arithmeticBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedArithmeticOP())
			return nil
		}
		return b.EmitBinOp(binOp, left, right)
	// TODO: !=. !== 这两个可能需要额外处理?
	// TODO: ==, === 这两个可能需要额外处理?
	// Comparison < > <= >= == === != !==
	case ast.KindLessThanToken, ast.KindGreaterThanToken, ast.KindLessThanEqualsToken, ast.KindGreaterThanEqualsToken, ast.KindEqualsEqualsToken, ast.KindEqualsEqualsEqualsToken, ast.KindExclamationEqualsToken, ast.KindExclamationEqualsEqualsToken:
		binOp, ok := comparisonBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedComparisonOP())
			return nil
		}
		return b.EmitBinOp(binOp, left, right)
	// Bitwise AND & OR | XOR ^ Left Shift << Right Shift >> Bitwise Unsigned Right Shift
	case ast.KindAmpersandToken, ast.KindBarToken, ast.KindCaretToken, ast.KindLessThanLessThanToken, ast.KindGreaterThanGreaterThanToken, ast.KindGreaterThanGreaterThanGreaterThanToken:
		binOp, ok := bitwiseBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedBinaryBitWiseOP())
			return nil
		}
		return b.EmitBinOp(binOp, left, right)
	// Logical AND && OR ||
	case ast.KindAmpersandAmpersandToken, ast.KindBarBarToken:
		binOp, ok := logicalBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedLogicalOP())
			return nil
		}
		return b.EmitBinOp(binOp, left, right)
	// Nullish Coalescing ??
	case ast.KindQuestionQuestionToken:
		if b.IsNullishValue(left) {
			return right
		}
		return left
	// Arithmetic Assignment += -= *= /= %= **=
	case ast.KindPlusEqualsToken, ast.KindMinusEqualsToken, ast.KindAsteriskEqualsToken, ast.KindSlashEqualsToken, ast.KindPercentEqualsToken, ast.KindAsteriskAsteriskEqualsToken:
		variable := b.VisitLeftValueExpression(node.Left)
		binOp, ok := arithmeticBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedArithmeticOP())
			return nil
		}
		newVal := b.EmitBinOp(binOp, left, right)
		b.AssignVariable(variable, newVal)
		return newVal
	// Bitwise Assignment <<= >>= >>>= &= ^= |=
	case ast.KindLessThanLessThanEqualsToken, ast.KindGreaterThanGreaterThanEqualsToken, ast.KindGreaterThanGreaterThanGreaterThanEqualsToken, ast.KindAmpersandEqualsToken, ast.KindCaretEqualsToken, ast.KindBarEqualsToken:
		variable := b.VisitLeftValueExpression(node.Left)
		binOp, ok := bitwiseBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedBinaryBitWiseOP())
			return nil
		}
		newVal := b.EmitBinOp(binOp, left, right)
		b.AssignVariable(variable, newVal)
		return newVal
	// Logical Assignment &&= ||=
	// ECMAScript 2021 introduce Logical Assignment
	case ast.KindAmpersandAmpersandEqualsToken, ast.KindBarBarEqualsToken:
		variable := b.VisitLeftValueExpression(node.Left)
		binOp, ok := logicalBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedLogicalOP())
			return nil
		}
		newVal := b.EmitBinOp(binOp, left, right)
		b.AssignVariable(variable, newVal)
		return newVal
	// Logical Assignment ??=
	case ast.KindQuestionQuestionEqualsToken:
		if b.IsNullishValue(left) {
			variable := b.VisitLeftValueExpression(node.Left)
			b.AssignVariable(variable, right)
			return right
		}
		return left
	// Assignment =
	case ast.KindEqualsToken:
		switch node.Left.Kind {
		case ast.KindArrayLiteralExpression: // arrayLiteral as binding pattern
			// TODO: 这里 [a, b] = exp 还不知道怎么处理
			return nil
		case ast.KindObjectLiteralExpression:
			return nil
		// TODO: 这里 {a, b} = exp 还不知道怎么处理
		default:
			/*
				let x;
				let y = (x = 42);  // y 的值是 42
				console.log(x);    // 42
				console.log(y);    // 42
			*/
			variable := b.VisitLeftValueExpression(node.Left)
			b.AssignVariable(variable, right)
			return right
		}
	// 处理其他运算符...
	default:
		// 未实现的操作符处理
		panic("unhandled bin op")
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
			argValue := b.VisitRightValueExpression(argNode)
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
func (b *builder) VisitObjectLiteralExpression(objLiteral *ast.ObjectLiteralExpression) ssa.Value {
	if objLiteral == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &objLiteral.Loc, "")
	defer recoverRange()

	// 没有属性的情况下，创建一个空对象
	if objLiteral.Properties == nil || len(objLiteral.Properties.Nodes) == 0 {
		return b.EmitMakeWithoutType(nil, nil)
	}

	var values []ssa.Value
	var keys []ssa.Value
	hasNamedProperty := false

	// 处理所有属性
	for i, prop := range objLiteral.Properties.Nodes {
		if prop == nil {
			continue
		}

		// 处理展开属性
		/*
			你可以在 ... 后放的	举例	说明
			变量名	{ ...obj }	常见，展开已有对象
			函数调用	{ ...getProps() }	动态获取要展开的对象
			三元表达式	{ ...(flag ? a : b) }	根据条件决定展开哪个对象
			字面量	{ ...{ a: 1 } }	直接展开临时对象
			组合多个	{ ...a, ...b, ...c }	顺序合并，后者覆盖前者
		*/
		if ast.IsSpreadAssignment(prop) {
			spreadAssignment := prop.AsSpreadAssignment()
			if spreadAssignment.Expression != nil {
				expressionValue := b.VisitRightValueExpression(spreadAssignment.Expression)
				if expressionValue != nil {
					// TODO: 处理展开运算符
					// 将展开对象的属性合并到当前对象中
					// 这需要运行时支持
					b.NewErrorWithPos(ssa.Warn, TAG, b.CurrentRange, "Spread properties not fully implemented yet")
				}
			}
			continue
		}

		// 处理属性赋值
		if ast.IsPropertyAssignment(prop) {
			if i == 0 {
				hasNamedProperty = true
			}

			if !hasNamedProperty {
				b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, "Unexpected token ':'")
				return nil
			}

			propAssignment := prop.AsPropertyAssignment()
			var key ssa.Value
			var value ssa.Value

			// 处理属性名
			if propAssignment.Name() != nil {
				propertyName := propAssignment.Name()
				key = b.VisitPropertyName(propertyName)
			}

			// 处理属性值
			if propAssignment.Initializer != nil {
				value = b.VisitRightValueExpression(propAssignment.Initializer)
			}

			if key != nil && value != nil {
				keys = append(keys, key)
				values = append(values, value)
			}
		} else if ast.IsShorthandPropertyAssignment(prop) {
			// 处理简写属性 { x } 等价于 { x: x }
			if hasNamedProperty {
				b.NewErrorWithPos(ssa.Error, "TS2ssa", b.CurrentRange, "Unexpected token ':'")
				return nil
			}

			shorthand := prop.AsShorthandPropertyAssignment()

			// 属性名和变量名相同
			if shorthand.Name() != nil {
				propertyName := shorthand.Name()
				key := b.VisitPropertyName(propertyName)

				// 对于简写属性，值就是与属性名同名的变量
				var value ssa.Value

				// 如果有默认值
				if shorthand.ObjectAssignmentInitializer != nil {
					value = b.VisitRightValueExpression(shorthand.ObjectAssignmentInitializer)
				} else {
					// 在作用域中查找同名变量
					variableName := ""
					if idNode := propertyName.AsIdentifier(); idNode != nil {
						variableName = idNode.Text
					} else {
						variableName = propertyName.Text()
					}

					value = b.PeekValue(variableName)
					if value == nil {
						value = b.EmitUndefined(variableName)
					}
				}

				if key != nil && value != nil {
					keys = append(keys, key)
					values = append(values, value)
				}
			}
		} else if ast.IsMethodDeclaration(prop) {
			// 处理方法声明
			if hasNamedProperty {
				b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, "Unexpected token ':'")
				return nil
			}

			methodDecl := prop.AsMethodDeclaration()

			var methodName string
			if methodDecl.Name() != nil {
				propertyName := methodDecl.Name()
				if idNode := propertyName.AsIdentifier(); idNode != nil {
					methodName = idNode.Text
				} else {
					methodName = propertyName.Text()
				}

				// 创建函数值
				functionName := methodName
				// TODO: 处理生成器
				if methodDecl.AsteriskToken != nil {
					continue
				}

				// 创建一个新的函数
				newFunc := b.EmitValueOnlyDeclare(functionName)

				// 函数的参数和函数体应该在这里处理
				// 这需要创建一个新的函数构建器并设置参数和函数体
				// 目前为了简化，我们仅创建函数但不实现其内部逻辑

				if methodName != "" {
					key := b.EmitConstInst(methodName)
					keys = append(keys, key)
					values = append(values, newFunc)
				}
			}
		} else if ast.IsGetAccessorDeclaration(prop) || ast.IsSetAccessorDeclaration(prop) {
			// 处理getter和setter
			var accessorName string
			accessorDecl := prop.AsGetAccessorDeclaration()

			if accessorDecl.Name() != nil {
				propertyName := accessorDecl.Name()
				if idNode := propertyName.AsIdentifier(); idNode != nil {
					accessorName = idNode.Text
				} else {
					accessorName = propertyName.Text()
				}
			}

			if accessorName != "" {
				// 创建函数值
				functionName := accessorName

				// 创建一个新的函数代表getter/setter
				newFunc := b.EmitValueOnlyDeclare(functionName)

				// 标记这是一个getter或setter
				// 目前为了简化，我们仅创建函数但不实现其内部逻辑

				key := b.EmitConstInst(accessorName)
				keys = append(keys, key)
				values = append(values, newFunc)
			}
		}
	}

	// 创建对象
	if len(keys) == 0 {
		// 没有命名属性，使用数组方式创建
		return b.CreateObjectWithSlice(values)
	} else {
		// 有命名属性，使用map方式创建
		return b.CreateObjectWithMap(keys, values)
	}
}

// VisitArrayLiteralExpression 访问数组字面量表达式
func (b *builder) VisitArrayLiteralExpression(arrayLiteral *ast.ArrayLiteralExpression) ssa.Value {
	if arrayLiteral == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &arrayLiteral.Loc, "")
	defer recoverRange()

	// 没有元素的空数组
	if arrayLiteral.Elements == nil || len(arrayLiteral.Elements.Nodes) == 0 {
		return b.EmitMakeWithoutType(nil, nil)
	}

	// 收集数组的所有元素值
	var elementValues []ssa.Value

	for _, element := range arrayLiteral.Elements.Nodes {
		if element == nil {
			// 数组中的空位，用 undefined 填充
			elementValues = append(elementValues, b.EmitUndefined(""))
			continue
		}

		if ast.IsOmittedExpression(element) {
			// 显式的空位，例如 [1,,3]
			elementValues = append(elementValues, b.EmitUndefined(""))
			continue
		}

		if ast.IsSpreadElement(element) {
			// 处理展开元素，例如 [...arr]
			spreadElement := element.AsSpreadElement()
			if spreadElement.Expression != nil {
				expressionValue := b.VisitRightValueExpression(spreadElement.Expression)
				if expressionValue != nil {
					// TODO: 完整实现展开运算符
					// 当前简化处理：将展开元素作为一个整体添加到数组中
					b.NewErrorWithPos(ssa.Warn, TAG, b.CurrentRange, "Spread elements in arrays not fully implemented yet")
					elementValues = append(elementValues, expressionValue)
				}
			}
			continue
		}

		// 处理普通元素
		elementValue := b.VisitRightValueExpression(element)
		if elementValue != nil {
			elementValues = append(elementValues, elementValue)
		} else {
			// 如果元素值无法解析，用undefined代替
			elementValues = append(elementValues, b.EmitUndefined(""))
		}
	}
	return b.CreateObjectWithSlice(elementValues)
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
	// 括号表达式直接访问其内部表达式
	// 括号在AST中只是一个标记，不影响执行结果
	// 但在某些情况下可能影响优先级或类型推断
	if node.Expression == nil {
		return b.EmitUndefined("")
	}

	// 括号表达式的值就是内部表达式的值
	return b.VisitRightValueExpression(node.Expression)
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

	return b.EmitConstInst(node.Text)
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

	return b.EmitConstInst(codec.Atoi64(node.Text))
}

// VisitRegularExpressionLiteral 访问正则表达式字面量
func (b *builder) VisitRegularExpressionLiteral(node *ast.RegularExpressionLiteral) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()
	// TODO: 正则表达式处理 目前当作字符串
	return b.EmitConstInst(node.Text)
}

// VisitThisExpression 访问this表达式
func (b *builder) VisitThisExpression(node *ast.Node) ssa.Value {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "this")
	defer recoverRange()

	// 尝试从当前作用域获取已存在的this
	if thisValue := b.PeekValue("this"); thisValue != nil {
		return thisValue
	}

	// 如果不存在，才创建一个新的
	thisParam := ssa.NewParam("this", false, b.FunctionBuilder)

	// 可能还需要设置thisParam的类型
	// 如果在类方法中，设置为当前类的类型
	// 如果在全局上下文中，设置为全局对象的类型

	return thisParam
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
		return b.VisitRightValueExpression(node.Expression)
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
		return b.VisitRightValueExpression(node.Expression)
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

func (b *builder) VisitPropertyName(propertyName *ast.PropertyName) ssa.Value {
	switch propertyName.Kind {
	case ast.KindIdentifier:
		return b.EmitConstInst(propertyName.AsIdentifier().Text)
	case ast.KindPrivateIdentifier:
		return b.EmitConstInst(propertyName.AsPrivateIdentifier().Text)
	case ast.KindStringLiteral:
		return b.VisitStringLiteral(propertyName.AsStringLiteral())
	case ast.KindNoSubstitutionTemplateLiteral:
		return b.VisitNoSubstitutionTemplateLiteral(propertyName.AsNoSubstitutionTemplateLiteral())
	// 在ECMAScript 规范 (ECMA-262) 里 PropertyKey = String | Symbol 所以数字会被当成string处理
	// const obj = { 42: "the answer", "42": "not the answer" }; "42"会覆盖前面的
	case ast.KindNumericLiteral:
		return b.VisitNumericLiteral(propertyName.AsNumericLiteral())
	case ast.KindComputedPropertyName:
		return b.VisitComputedPropertyName(propertyName.AsComputedPropertyName())
	// A 'bigint' literal cannot be used as a property name.
	case ast.KindBigIntLiteral:
		return nil
	default:
		panic("unknown property name kind")
	}
}

// ProcessObjectBindingPattern 处理对象解构模式
func (b *builder) ProcessObjectBindingPattern(pattern *ast.BindingPattern, sourceObj ssa.Value) {
	if pattern == nil || sourceObj == nil {
		return
	}

	// 获取所有绑定元素
	elements := pattern.Elements.Nodes

	for _, element := range elements {
		if element == nil {
			// 跳过空元素
			continue
		}

		bindingElement := element.AsBindingElement()
		if bindingElement == nil {
			continue
		}

		// 检查是否是剩余元素: let { ...rest } = obj
		isRest := bindingElement.DotDotDotToken != nil

		if isRest {
			// 简化处理：直接将整个对象赋值给rest变量
			if bindingElement.Name() != nil && ast.IsIdentifier(bindingElement.Name()) {
				restName := bindingElement.Name().AsIdentifier().Text
				restVar := b.CreateVariable(restName)
				// 直接赋值整个对象
				b.AssignVariable(restVar, sourceObj)
			} else {
				b.NewError(ssa.Error, TAG, RestElementRequiresIdentifier())
			}
			continue
		}

		// 处理普通属性
		var propertyKey ssa.Value

		// 获取属性键
		if bindingElement.PropertyName != nil {
			// 显式属性名: let { a: b } = obj
			propertyKey = b.VisitPropertyName(bindingElement.PropertyName)
		} else if bindingElement.Name() != nil && ast.IsIdentifier(bindingElement.Name()) {
			// 简写形式: let { a } = obj
			propName := bindingElement.Name().AsIdentifier().Text
			propertyKey = b.EmitConstInst(propName)
		} else {
			// 没有有效的属性名
			b.NewError(ssa.Error, TAG, InvalidPropertyBinding())
			continue
		}

		// 如果没有绑定名称，跳过
		if bindingElement.Name() == nil {
			continue
		}

		// 从源对象获取属性值
		propValue := b.ReadMemberCallValue(sourceObj, propertyKey)

		// 应用默认值（如果有）
		if bindingElement.Initializer != nil {
			defaultValue := b.VisitRightValueExpression(bindingElement.Initializer)
			if propValue.IsUndefined() {
				propValue = defaultValue
			}
		}

		// 根据绑定名类型分配值
		switch {
		case ast.IsIdentifier(bindingElement.Name()):
			// 简单变量: let { a } = obj 或 let { a: b } = obj
			varName := bindingElement.Name().AsIdentifier().Text
			variable := b.CreateVariable(varName)
			b.AssignVariable(variable, propValue)

		case ast.IsBindingPattern(bindingElement.Name()):
			// 嵌套解构: let { a: { b, c } } = obj 或 let { a: [x, y] } = obj
			nestedPattern := bindingElement.Name()

			// 递归处理
			if ast.IsObjectBindingPattern(nestedPattern) {
				b.ProcessObjectBindingPattern(nestedPattern.AsBindingPattern(), propValue)
			} else if ast.IsArrayBindingPattern(nestedPattern) {
				b.ProcessArrayBindingPattern(nestedPattern.AsBindingPattern(), propValue)
			}
		}
	}
}

// ProcessArrayBindingPattern 处理数组解构模式
func (b *builder) ProcessArrayBindingPattern(pattern *ast.BindingPattern, sourceArr ssa.Value) {
	if pattern == nil || sourceArr == nil {
		return
	}

	// 获取所有绑定元素
	elements := pattern.Elements.Nodes

	for i, element := range elements {
		if element == nil || ast.IsOmittedExpression(element) {
			continue // 跳过空位: let [a, , b] = arr
		}

		bindingElement := element.AsBindingElement()
		if bindingElement == nil {
			continue
		}

		// 如果没有绑定名，跳过
		if bindingElement.Name() == nil {
			continue
		}

		// 检查是否是剩余元素: let [a, ...rest] = arr
		isRest := bindingElement.DotDotDotToken != nil

		var elementValue ssa.Value

		if isRest {
			// 简化处理：直接将整个数组赋值给rest变量
			if ast.IsIdentifier(bindingElement.Name()) {
				restName := bindingElement.Name().AsIdentifier().Text
				restVar := b.CreateVariable(restName)
				b.AssignVariable(restVar, sourceArr)
			}
			continue
		} else {
			// 普通元素: arr[i]
			indexValue := b.EmitConstInst(i)
			elementValue = b.ReadMemberCallValue(sourceArr, indexValue)
		}

		// 应用默认值（如果有）
		if bindingElement.Initializer != nil {
			defaultValue := b.VisitRightValueExpression(bindingElement.Initializer)
			if elementValue.IsUndefined() {
				elementValue = defaultValue
			}
		}

		// 分配值
		switch {
		case ast.IsIdentifier(bindingElement.Name()):
			// 标识符: let [a] = arr
			varName := bindingElement.Name().AsIdentifier().Text
			variable := b.CreateVariable(varName)
			b.AssignVariable(variable, elementValue)

		case ast.IsBindingPattern(bindingElement.Name()):
			// 嵌套解构: let [[a, b], {c}] = arr
			nestedPattern := bindingElement.Name()

			// 递归处理
			if ast.IsObjectBindingPattern(nestedPattern) {
				b.ProcessObjectBindingPattern(nestedPattern.AsBindingPattern(), elementValue)
			} else if ast.IsArrayBindingPattern(nestedPattern) {
				b.ProcessArrayBindingPattern(nestedPattern.AsBindingPattern(), elementValue)
			}
		}
	}
}
