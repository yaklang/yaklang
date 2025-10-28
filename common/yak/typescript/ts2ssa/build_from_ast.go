//go:build !no_language
// +build !no_language

package ts2ssa

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (b *builder) GetRecoverRange(sourcefile *ast.SourceFile, node *core.TextRange, text string) func() {
	pos := node.Pos()
	pos = scanner.SkipTrivia(sourcefile.Text(), pos)
	startLine, startCol := scanner.GetLineAndCharacterOfPosition(sourcefile, pos)
	endLine, endCol := scanner.GetLineAndCharacterOfPosition(sourcefile, node.End())
	return b.SetRangeWithCommonTokenLoc(ssa.NewCommonTokenLoc(text, startLine, startCol, endLine, endCol))
}

func (b *builder) VisitSourceFile(sourcefile *ast.SourceFile) interface{} {
	if sourcefile == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(sourcefile, &sourcefile.Loc, sourcefile.Text())
	defer recoverRange()

	prog := b.GetProgram()
	application := prog.Application
	fileName := b.GetEditor().GetFilename()
	folderPath := b.GetEditor().GetFolderPath()
	fileUrl := path.Join([]string{folderPath, fileName}...)

	ssaGlobal := application.GlobalVariablesBlueprint.Container()
	if ssaGlobal == nil {
		return nil
	}

	if b.PreHandler() {
		lib, _ := prog.GetLibrary(fileUrl)
		if lib == nil {
			lib = prog.NewLibrary(fileUrl, []string{fileUrl})
			variable := b.CreateMemberCallVariable(ssaGlobal, b.EmitConstInstPlaceholder(fileUrl))
			emptyContainer := b.EmitEmptyContainer()
			b.AssignVariable(variable, emptyContainer)
			if lib.GlobalVariablesBlueprint != nil {
				moduleScope := b.ReadMemberCallValue(ssaGlobal, b.EmitConstInstPlaceholder(fileUrl))
				lib.GlobalVariablesBlueprint.InitializeWithContainer(moduleScope)
			}
		}
		defer func() {
			lib.VisitAst(sourcefile)
		}()
		lib.PushEditor(prog.GetCurrentEditor())

		subBuilder := lib.GetAndCreateFunctionBuilder(fileName, string(ssa.MainFunctionName))

		if subBuilder != nil {
			subBuilder.SetBuildSupport(b.FunctionBuilder)
			subBuilder.SetEditor(prog.GetApplication().GetCurrentEditor())
			currentBuilder := b.FunctionBuilder
			b.FunctionBuilder = subBuilder
			defer func() {
				for _, e := range subBuilder.GetProgram().GetErrors() {
					currentBuilder.GetProgram().AddError(e)
				}
				b.FunctionBuilder = currentBuilder
			}()
		}

		if sourcefile.Statements != nil {
			for _, statement := range sourcefile.Statements.Nodes {
				if ast.IsPrologueDirective(statement) && statement.AsExpressionStatement().Expression.Text() == "use strict" {
					b.useStrict = true
				}
				if ast.IsImportDeclaration(statement) {
					b.VisitStatement(statement)
				}
			}
			for _, statement := range sourcefile.Statements.Nodes {
				if ast.IsFunctionDeclaration(statement) || ast.IsVariableStatement(statement) || ast.IsEnumDeclaration(statement) || ast.IsClassDeclaration(statement) || ast.IsInterfaceDeclaration(statement) {
					b.VisitStatement(statement)
				}

			}
		}

		for exportValueName, exportedObjectName := range b.namedValueExports {
			exportedObjectVal := b.PeekValue(exportedObjectName)
			if exportedObjectVal != nil {
				lib.SetExportValue(exportValueName, exportedObjectVal)
			}
		}

		for exportTypeName, exportedTypeName := range b.namedValueExports {
			exportedObjectVal := b.PeekValue(exportedTypeName)
			if exportedObjectVal != nil {
				lib.SetExportType(exportTypeName, exportedObjectVal.GetType())
			}
		}
		return nil
	}

	lib, _ := prog.GetLibrary(fileUrl)

	if lib == nil {
		b.NewError(ssa.Error, TAG, "no library found for file %s", fileUrl)
		return nil
	}

	subBuilder := lib.GetAndCreateFunctionBuilder(fileName, string(ssa.MainFunctionName))

	if subBuilder != nil {
		subBuilder.SetBuildSupport(b.FunctionBuilder)
		subBuilder.SetEditor(prog.GetApplication().GetCurrentEditor())
		currentBuilder := b.FunctionBuilder
		b.FunctionBuilder = subBuilder
		defer func() {
			for _, e := range subBuilder.GetProgram().GetErrors() {
				currentBuilder.GetProgram().AddError(e)
			}
			b.FunctionBuilder = currentBuilder
		}()
	}

	defer func() {
		lib.VisitAst(sourcefile)
	}()
	if sourcefile.Statements != nil {
		for _, statement := range sourcefile.Statements.Nodes {
			if ast.IsPrologueDirective(statement) && statement.AsExpressionStatement().Expression.Text() == "use strict" {
				b.useStrict = true
				break
			}
		}
		for _, statement := range sourcefile.Statements.Nodes {
			b.VisitStatement(statement)
		}
	}

	return nil
}

func (b *builder) VisitStatements(stmtList *ast.NodeList) interface{} {
	if stmtList == nil || len(stmtList.Nodes) == 0 || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &stmtList.Loc, "")
	defer recoverRange()

	for _, stmt := range stmtList.Nodes {
		if b.IsStop() {
			return nil
		}
		b.VisitStatement(stmt)
	}
	return nil
}

// ===== Statement =====

// VisitStatement 处理Statement相关
func (b *builder) VisitStatement(node *ast.Node) interface{} {
	if node == nil || b.IsStop() {
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
	case ast.KindImportDeclaration:
		b.VisitImportDeclaration(node.AsImportDeclaration())
	case ast.KindExportAssignment:
		b.VisitExportAssignment(node.AsExportAssignment())
	case ast.KindEnumDeclaration:
		b.VisitEnumDeclaration(node.AsEnumDeclaration())
	case ast.KindInterfaceDeclaration:
		b.VisitInterfaceDeclaration(node.AsInterfaceDeclaration())
	default:
		b.NewError(ssa.Error, TAG, UnhandledStatement())
	}
	return nil
}

func (b *builder) VisitVariableStatement(node *ast.VariableStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 使用 AST 提供的辅助方法检查修饰符
	nodePtr := node.AsNode()
	isExport := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsExport)

	if !ShouldVisit(b.PreHandler(), isExport) {
		return nil
	}

	if decList := node.DeclarationList; decList != nil {
		b.VisitVariableDeclarationList(decList, isExport)
	}
	return nil
}

// VisitExpressionStatement 访问表达式语句
func (b *builder) VisitExpressionStatement(node *ast.ExpressionStatement) interface{} {
	if node == nil || b.IsStop() {
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
	if node == nil || b.IsStop() {
		return ""
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, node.Text)
	defer recoverRange()

	// 查找变量并返回
	return node.Text
}

// VisitStringLiteral 访问字符串字面量
func (b *builder) VisitStringLiteral(node *ast.StringLiteral) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, node.Text)
	defer recoverRange()

	// 创建字符串常量
	return b.EmitConstInst(node.Text)
}

// VisitNumericLiteral 访问数字字面量
func (b *builder) VisitNumericLiteral(node *ast.NumericLiteral) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	text := node.Text
	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, text)
	defer recoverRange()

	// 创建数字常量
	num, err := strconv.ParseInt(text, 0, 64)
	if err == nil {
		return b.EmitConstInst(num)
	}
	float, err := strconv.ParseFloat(text, 64)
	if err == nil {
		return b.EmitConstInst(float)
	}
	return b.EmitConstInst(utils.InterfaceToFloat64(text))
}

// VisitBooleanLiteral 访问布尔字面量
func (b *builder) VisitBooleanLiteral(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	var boolValue bool
	if node.Kind == ast.KindTrueKeyword {
		boolValue = true
	} else {
		boolValue = false
	}

	// 创建布尔常量
	return b.EmitConstInst(boolValue)
}

// VisitNullLiteral 访问null字面量
func (b *builder) VisitNullLiteral(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建null常量
	return b.EmitConstInstNil()
}

// VisitUndefinedLiteral 访问undefined字面量
func (b *builder) VisitUndefinedLiteral(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建undefined常量
	return b.EmitUndefined("")
}

// VisitIfStatement 访问if语句
func (b *builder) VisitIfStatement(node *ast.IfStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建if构建器
	ifBuilder := b.CreateIfBuilder()

	// 辅助结构体
	type conditionBlock struct {
		condition func() ssa.Value
		block     func()
	}

	// 递归收集所有if-else if-else链
	var collectIfChain func(ifNode *ast.IfStatement) ([]conditionBlock, func())
	collectIfChain = func(ifNode *ast.IfStatement) ([]conditionBlock, func()) {
		var blocks []conditionBlock
		var elseFunc func()

		// 添加当前if条件和块
		blocks = append(blocks, conditionBlock{
			condition: func() ssa.Value {
				if ifNode.Expression != nil {
					return b.VisitRightValueExpression(ifNode.Expression)
				}
				return nil
			},
			block: func() {
				if ifNode.ThenStatement != nil {
					b.VisitStatement(ifNode.ThenStatement)
				}
			},
		})

		// 处理else部分
		if ifNode.ElseStatement != nil {
			if ifNode.ElseStatement.Kind == ast.KindIfStatement {
				// 是else-if，递归收集
				elseIfBlocks, nestedElse := collectIfChain(ifNode.ElseStatement.AsIfStatement())
				blocks = append(blocks, elseIfBlocks...)
				elseFunc = nestedElse
			} else {
				// 是纯else
				elseFunc = func() {
					b.VisitStatement(ifNode.ElseStatement)
				}
			}
		}

		return blocks, elseFunc
	}

	// 收集所有条件块
	blocks, elseBlock := collectIfChain(node)

	// 添加到if构建器
	for _, block := range blocks {
		ifBuilder.AppendItem(block.condition, block.block)
	}

	// 设置最终的else块
	if elseBlock != nil {
		ifBuilder.SetElse(elseBlock)
	}

	// 构建并执行if语句
	ifBuilder.Build()
	return nil
}

// VisitBlock 访问代码块
func (b *builder) VisitBlock(node *ast.Block) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 处理代码块中的语句列表
	if node.Statements != nil && len(node.Statements.Nodes) > 0 {
		// 使用语法块包装执行
		b.BuildSyntaxBlock(func() {
			// 逐个处理每个语句
			for _, stmt := range node.Statements.Nodes {
				b.VisitStatement(stmt)

				// 如果某个语句导致块结束(如return, break等)，提前返回
				if b.IsBlockFinish() {
					break
				}
			}
		})
	}

	return nil
}

// VisitDoStatement 访问do-while语句 - 至少执行一次的循环
func (b *builder) VisitDoStatement(node *ast.DoStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建一个无条件循环（条件永远为true）
	loop := b.CreateLoopBuilder()

	// 设置循环条件为true，这样循环体至少会执行一次
	loop.SetCondition(func() ssa.Value {
		return b.EmitConstInst(true)
	})

	// 设置循环体
	loop.SetBody(func() {
		// 执行循环体
		if node.Statement != nil {
			b.VisitStatement(node.Statement)
		}

		// 检查do-while的条件，如果条件为false则break
		if node.Expression != nil {
			// 创建条件分支，当条件为false时退出循环
			ifBuilder := b.CreateIfBuilder()
			ifBuilder.SetCondition(func() ssa.Value {
				condition := b.VisitRightValueExpression(node.Expression)
				if condition == nil {
					// 如果无法获取条件，可以选择继续循环或退出
					// 这里我们选择退出循环
					b.Break()
				}
				// 对条件取反，当原条件为false时进入if分支
				return b.EmitUnOp(ssa.OpNeg, condition)
			}, func() {
				b.Break() // 退出循环
			})
			ifBuilder.Build()
		}
	})

	// 完成循环构建
	loop.Finish()

	return nil
}

// VisitWhileStatement 访问while语句 - 简单条件循环
func (b *builder) VisitWhileStatement(node *ast.WhileStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建循环构建器
	loop := b.CreateLoopBuilder()

	// 设置循环条件
	loop.SetCondition(func() ssa.Value {
		if node.Expression != nil {
			condition := b.VisitRightValueExpression(node.Expression)
			if condition == nil {
				return b.EmitConstInst(true)
			}
			return condition
		}
		return b.EmitConstInst(true)
	})

	// 设置循环体
	loop.SetBody(func() {
		if node.Statement != nil {
			b.VisitStatement(node.Statement)
		}
	})

	// 完成循环构建
	loop.Finish()

	return nil
}

// VisitForStatement 访问for语句 - 经典三语句循环
func (b *builder) VisitForStatement(node *ast.ForStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	var loop *ssa.LoopBuilder
	// 创建循环构建器
	if len(b.contextLabelStack) > 0 {
		loop = b.CreateLoopBuilderWithLabelName(b.contextLabelStack[len(b.contextLabelStack)-1])
	} else {
		loop = b.CreateLoopBuilder()
	}

	// 设置初始化语句(first)
	if node.Initializer != nil {
		loop.SetFirst(func() []ssa.Value {
			var results []ssa.Value

			switch node.Initializer.Kind {
			case ast.KindVariableDeclarationList:
				// 变量声明初始化：for(let i = 0; ...)
				b.VisitVariableDeclarationList(node.Initializer.AsVariableDeclarationList().AsNode(), false)
				for _, varDecl := range node.Initializer.AsVariableDeclarationList().Declarations.Nodes {
					name := varDecl.AsVariableDeclaration().Name()
					switch {
					case ast.IsIdentifier(name): // 简单变量: let x = value
						results = append(results, b.ReadValue(name.AsIdentifier().Text))
					case ast.IsBindingPattern(name): // 解构模式: let {a, b} = obj 或 let [x, y] = arr
						bindingPattern := name.AsBindingPattern()
						for _, element := range bindingPattern.Elements.Nodes {
							if ast.IsIdentifier(element) {
								results = append(results, b.ReadValue(element.AsIdentifier().Text))
							}
						}
					}
				}
			default:
				// 表达式初始化：for(i = 0; ...)
				result := b.VisitRightValueExpression(node.Initializer)
				if result != nil {
					results = append(results, result)
				}
			}

			return results
		})
	}

	// 设置条件语句(condition)
	loop.SetCondition(func() ssa.Value {
		if node.Condition != nil {
			condition := b.VisitRightValueExpression(node.Condition)
			if condition == nil {
				return b.EmitConstInst(true)
			}
			return condition
		}
		// 没有条件表达式，默认为true（无限循环）
		return b.EmitConstInst(true)
	})

	// 设置增量语句(third/incrementor)
	if node.Incrementor != nil {
		loop.SetThird(func() []ssa.Value {
			result := b.VisitRightValueExpression(node.Incrementor)
			if result != nil {
				return []ssa.Value{result}
			}
			return nil
		})
	}

	// 设置循环体
	loop.SetBody(func() {
		if node.Statement != nil {
			b.VisitStatement(node.Statement)
		}
	})

	// 完成循环构建
	loop.Finish()

	return nil
}

// VisitForInOrOfStatement 访问for-in和for-of语句
func (b *builder) VisitForInOrOfStatement(node *ast.ForInOrOfStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 创建循环构建器
	loop := b.CreateLoopBuilder()

	// 设置循环条件
	loop.SetCondition(func() ssa.Value {
		var lv *ssa.Variable
		var iterableValue ssa.Value

		// 处理循环变量（左侧）
		if node.Initializer != nil {
			switch node.Initializer.Kind {
			case ast.KindVariableDeclarationList:
				// 变量声明形式：for(let x in/of obj)
				declList := node.Initializer.AsVariableDeclarationList()
				if declList != nil && declList.Declarations != nil && len(declList.Declarations.Nodes) > 0 {
					// 获取第一个声明的变量
					varDecl := declList.Declarations.Nodes[0].AsVariableDeclaration()
					if varDecl != nil && varDecl.Name() != nil && ast.IsIdentifier(varDecl.Name()) {
						varName := varDecl.Name().AsIdentifier().Text
						// 创建循环变量，但暂不赋值（值将在迭代中设置）
						if node.AsNode().Kind == ast.KindForInStatement {
							// for-in循环变量通常是块级作用域
							lv = b.CreateLocalVariable(varName)
						} else {
							// for-of循环变量同样是块级作用域
							lv = b.CreateLocalVariable(varName)
						}
					}
				}
			default:
				// 直接表达式形式：for(x in/of obj)
				lval, _ := b.VisitExpression(node.Initializer, true)
				lv = lval
			}
		}

		// 处理要迭代的对象（右侧）
		if node.Expression != nil {
			iterableValue = b.VisitRightValueExpression(node.Expression)
		}

		// 获取下一个迭代值
		if iterableValue != nil && lv != nil {
			if node.AsNode().Kind == ast.KindForInStatement {
				// for-in 循环获取对象的键
				key, _, ok := b.EmitNext(iterableValue, false)
				b.AssignVariable(lv, key)
				return ok
			} else {
				// for-of 循环获取迭代器的值
				_, value, ok := b.EmitNext(iterableValue, true)
				b.AssignVariable(lv, value)
				return ok
			}
		}

		// 如果无法设置迭代，返回false终止循环
		return b.EmitConstInst(false)
	})

	// 设置循环体
	loop.SetBody(func() {
		if node.Statement != nil {
			b.VisitStatement(node.Statement)
		}
	})

	// 完成循环构建
	loop.Finish()

	return nil
}

// VisitReturnStatement 访问return语句
func (b *builder) VisitReturnStatement(node *ast.ReturnStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 如果有返回表达式，处理它并返回
	if node.Expression != nil {
		returnValue := b.VisitRightValueExpression(node.Expression)
		if returnValue != nil {
			b.EmitReturn([]ssa.Value{returnValue})
		} else {
			b.EmitReturn([]ssa.Value{ssa.NewUndefined("ret")})
			_ = b.VisitRightValueExpression(node.Expression)
		}

	} else {
		// 如果没有返回表达式，返回undefined
		b.EmitReturn([]ssa.Value{b.EmitUndefined("")})
	}
	return nil
}

// VisitBreakStatement 访问break语句
func (b *builder) VisitBreakStatement(node *ast.BreakStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// if exist label, goto label
	if label := node.Label; label != nil {
		b.BreakWithLabelName(label.Text())
		return nil
	}

	if !b.Break() {
		b.NewError(ssa.Error, TAG, UnexpectedBreakStmt())
	}
	return nil
}

// VisitContinueStatement 访问continue语句
func (b *builder) VisitContinueStatement(node *ast.ContinueStatement) interface{} {
	if node == nil {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// if exist label, goto label
	if label := node.Label; label != nil {
		b.ContinueWithLabelName(label.Text())
		return nil
	}

	if !b.Continue() {
		b.NewError(ssa.Error, TAG, UnexpectedContinueStmt())
	}
	return nil
}

// VisitLabeledStatement 访问带标签的语句
func (b *builder) VisitLabeledStatement(node *ast.LabeledStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 对于JS Label语句结构分为Label名和语句两部分
	// outer: console.log("not a jump target") 这种标签语句合法但不会成为break和continue目标
	// 因为Label语句的语句部分不是Block或者是循环
	// 这里为了兼容label循环的情况统一套一个block

	// 获取标签名称
	labelName := ""
	if node.Label != nil {
		labelName = node.Label.Text()
	} else {
		b.NewError(ssa.Error, TAG, LabelNameEmptyNotAllowed())
		return nil
	}

	b.contextLabelStack = append(b.contextLabelStack, labelName)
	defer func() {
		b.contextLabelStack = b.contextLabelStack[:len(b.contextLabelStack)-1]
	}()

	label := b.CreateLabelBlockBuilder(labelName)

	label.SetLabelBlock(func() {
		if node.Statement != nil {
			b.VisitStatement(node.Statement)
		}
	})

	label.Finish()
	return nil
}

// VisitTryStatement 访问try语句
func (b *builder) VisitTryStatement(node *ast.TryStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	tryBuilder := b.BuildTry()
	tryBuilder.BuildTryBlock(func() {
		if node.TryBlock != nil {
			b.VisitBlock(node.TryBlock.AsBlock())
		}
	})

	if node.CatchClause != nil {
		catchClause := node.CatchClause.AsCatchClause()
		tryBuilder.BuildErrorCatch(func() string {
			if catchClause.VariableDeclaration != nil {
				varDecl := catchClause.VariableDeclaration.AsVariableDeclaration()
				varName := varDecl.Name()
				if varName.Kind == ast.KindIdentifier {
					return varName.AsIdentifier().Text
				} else { // BindingPattern in catch clause?
					return ""
				}
			}
			return ""
		}, func() {
			if catchClause.Block != nil {
				b.VisitBlock(catchClause.Block.AsBlock())
			}
		})
	}

	if node.FinallyBlock != nil {
		tryBuilder.BuildFinally(func() {
			b.VisitBlock(node.FinallyBlock.AsBlock())
		})
	}
	tryBuilder.Finish()

	return nil
}

// VisitSwitchStatement 访问switch语句
func (b *builder) VisitSwitchStatement(node *ast.SwitchStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	switchBuilder := b.BuildSwitch()
	switchBuilder.AutoBreak = false

	// 设置switch的表达式
	switchBuilder.BuildCondition(func() ssa.Value {
		exp := b.VisitRightValueExpression(node.Expression)
		return exp
	})

	// 计算case分支数量
	caseBlock := node.CaseBlock.AsCaseBlock()
	var caseCount int
	var commonCase []*ast.Node
	var defaultCase *ast.Node
	if caseBlock.Clauses != nil && caseBlock.Clauses.Nodes != nil {
		for _, caseClause := range caseBlock.Clauses.Nodes {
			if caseClause.AsCaseOrDefaultClause().Expression == nil {
				defaultCase = caseClause
			} else {
				commonCase = append(commonCase, caseClause)
			}
		}
		if defaultCase != nil {
			caseCount = len(caseBlock.Clauses.Nodes) - 1
		} else {
			caseCount = len(caseBlock.Clauses.Nodes)
		}
	} else {
		caseCount = 0
	}

	switchBuilder.BuildCaseSize(caseCount)

	switchBuilder.SetCase(func(i int) []ssa.Value {
		switchCase := commonCase[i].AsCaseOrDefaultClause()
		return []ssa.Value{b.VisitRightValueExpression(switchCase.Expression)}
	})

	switchBuilder.BuildBody(func(i int) {
		switchCase := commonCase[i].AsCaseOrDefaultClause()
		b.VisitStatements(switchCase.Statements)
	})

	if defaultCase != nil {
		switchBuilder.BuildDefault(func() {
			switchDefault := defaultCase.AsCaseOrDefaultClause()
			b.VisitStatements(switchDefault.Statements)
		})
	}

	switchBuilder.Finish()
	return nil
}

// VisitThrowStatement 访问throw语句
func (b *builder) VisitThrowStatement(node *ast.ThrowStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	value := b.VisitRightValueExpression(node.Expression)
	b.EmitReturn([]ssa.Value{value})
	return nil
}

// VisitEmptyStatement 访问空语句
func (b *builder) VisitEmptyStatement(node *ast.EmptyStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()
	return nil
}

// VisitDebuggerStatement 访问debugger语句
func (b *builder) VisitDebuggerStatement(node *ast.DebuggerStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()
	return nil
}

// VisitWithStatement 访问with语句
func (b *builder) VisitWithStatement(node *ast.WithStatement) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	b.VisitStatement(node.Statement)
	return nil
}

// VisitClassDeclaration 访问类声明
func (b *builder) VisitClassDeclaration(node *ast.ClassDeclaration) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	var (
		blueprint  *ssa.Blueprint
		extendName string
	)

	className := node.Name().AsIdentifier().Text

	// 使用 AST 提供的辅助方法检查修饰符
	nodePtr := node.AsNode()
	isExport := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsExport)
	isDefault := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsDefault)

	if !ShouldVisit(b.PreHandler(), isExport) {
		return nil
	}

	if isExport {
		if !isDefault {
			b.namedTypeExports[className] = className
			b.namedValueExports[className] = className
		} else {
			b.namedTypeExports["default"] = className
			b.namedValueExports["default"] = className
		}
	}

	blueprint = b.CreateBlueprint(className)
	blueprint.SetKind(ssa.BlueprintClass)

	if node.HeritageClauses != nil && len(node.HeritageClauses.Nodes) != 0 {
		parent := node.HeritageClauses.Nodes[0].AsHeritageClause()
		if parent.Types != nil && len(parent.Types.Nodes) != 0 {
			typedExp := parent.Types.Nodes[0].AsExpressionWithTypeArguments()
			if ast.IsIdentifier(typedExp.Expression) {
				extendName = typedExp.Expression.AsIdentifier().Text
			}
		}
	}

	/*
		该lazyBuilder顺序按照cls解析顺序
	*/
	store := b.StoreFunctionBuilder()
	blueprint.AddLazyBuilder(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()

		if extendName != "" {
			bp := b.GetBluePrint(extendName)
			if bp == nil {
				bp = b.CreateBlueprint(extendName)
			}
			bp.SetKind(ssa.BlueprintClass)
			blueprint.AddParentBlueprint(bp)
		}

	})

	container := blueprint.Container()
	b.MarkedThisClassBlueprint = blueprint
	defer func() {
		b.MarkedThisClassBlueprint = nil
	}()

	if node.Members != nil && len(node.Members.Nodes) > 0 {
		for _, memberNode := range node.Members.Nodes {
			b.ProcessClassMember(memberNode, blueprint)
		}
	}
	return container
}

// VisitHeritageClause 访问继承子句
func (b *builder) VisitHeritageClause(node *ast.HeritageClause) interface{} { return nil }

// ===== Declaration =====

// VisitVariableDeclarationList 访问变量声明列表
func (b *builder) VisitVariableDeclarationList(node *ast.VariableDeclarationListNode, isExport bool) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	declList := node.AsVariableDeclarationList()
	for _, varDecl := range declList.Declarations.Nodes {
		b.VisitVariableDeclaration(varDecl.AsVariableDeclaration(), declList.Flags, isExport)
	}
	return nil
}

// VisitVariableDeclaration 访问变量声明
func (b *builder) VisitVariableDeclaration(decl *ast.VariableDeclaration, declType ast.NodeFlags, isExport bool) interface{} {
	if decl == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &decl.Loc, "")
	defer recoverRange()
	// === Fast Fail Start ===
	if decl.Name() == nil {
		b.NewError(ssa.Error, TAG, NoDeclarationName())
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

	// 获取显式类型注解（如果有的话）
	var explicitType ssa.Type
	if decl.Type != nil {
		explicitType = b.VisitTypeNode(decl.Type)
	}

	// 获取声明的初始化值（如果有的话）
	var initValue ssa.Value
	if decl.Initializer != nil {
		initValue = b.VisitRightValueExpression(decl.Initializer)
	}

	// 处理变量名(BindingPattern | Identifier)
	name := decl.Name()

	var isLocal bool
	if declType == ast.NodeFlagsLet || declType == ast.NodeFlagsConst {
		isLocal = true
	} else {
		isLocal = false
	}

	switch {
	case ast.IsIdentifier(name): // 简单变量: let x = value
		identifier := b.VisitIdentifier(name.AsIdentifier())
		if isExport {
			b.namedValueExports[identifier] = identifier
		}
		var variable *ssa.Variable
		if isLocal {
			variable = b.CreateLocalVariable(identifier)
		} else {
			variable = b.CreateJSVariable(identifier)
		}

		if initValue == nil {
			initValue = b.EmitUndefined(identifier)
		}

		if decl.Initializer != nil { // 定义变量
			if explicitType != nil {
				mergedType := b.MergeTypeWithAnnotation(initValue.GetType(), explicitType)
				initValue.SetType(mergedType)
			}
			b.AssignVariable(variable, initValue)
		} else { // 仅声明变量
			// 仅声明，使用显式类型或默认类型
			var finalType ssa.Type
			if explicitType != nil {
				finalType = explicitType
			} else {
				finalType = ssa.CreateAnyType() // TypeScript的默认类型
			}

			undefinedValue := b.EmitUndefined(identifier)
			undefinedValue.SetType(finalType)
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
			b.ProcessObjectBindingPattern(name.AsBindingPattern(), initValue, isLocal, isExport)
		} else if ast.IsArrayBindingPattern(name) {
			// 数组解构: let [x, y] = arr
			b.ProcessArrayBindingPattern(name.AsBindingPattern(), initValue, isLocal, isExport)
		}

	default:
		b.NewError(ssa.Error, TAG, UnhandledVariableDeclarationType())
	}

	return nil
}

// VisitModuleBlock 访问模块块
func (b *builder) VisitModuleBlock(node *ast.ModuleBlock) interface{} { return nil }

// VisitImportDeclaration 访问导入声明
func (b *builder) VisitImportDeclaration(node *ast.ImportDeclaration) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 提取模块路径
	modulePath := b.extractModuleSpecifierText(node.ModuleSpecifier)
	if modulePath == "" {
		b.NewError(ssa.Warn, TAG, "empty import module specifier")
		return nil
	}
	// 解析导入路径
	resolvedPath, isExternal := b.resolveImportLibPath(modulePath)
	if resolvedPath == "" || isExternal {
		return nil
	}

	// 处理导入子句
	if node.ImportClause != nil {
		b.VisitImportClause(node.ImportClause.AsImportClause(), resolvedPath, isExternal)
	}

	return nil
}

// VisitImportClause 访问导入子句
func (b *builder) VisitImportClause(node *ast.ImportClause, resolvedPath string, isExternal bool) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 跳过类型导入
	if node.IsTypeOnly {
		return nil
	}

	// 处理默认导入: import React from 'react'
	if binding := node.Name(); binding != nil {
		localName := b.getDeclarationNameText(binding)
		if localName != "" {
			b.assignImportedValue(localName, "default", resolvedPath, isExternal)
		}
	}

	// 处理命名绑定
	if node.NamedBindings != nil {
		bindings := node.NamedBindings
		switch {
		case ast.IsNamespaceImport(bindings):
			// 命名空间导入: import * as fs from 'fs'
			namespace := bindings.AsNamespaceImport()
			if namespace != nil && namespace.Name() != nil {
				alias := b.getDeclarationNameText(namespace.Name())
				if alias != "" {
					b.bindNamespaceImport(alias, resolvedPath, isExternal)
				}
			}

		case ast.IsNamedImports(bindings):
			// 命名导入: import { a, b as c } from 'module'
			named := bindings.AsNamedImports()
			if named != nil && named.Elements != nil {
				for _, specNode := range named.Elements.Nodes {
					if specNode == nil {
						continue
					}
					spec := specNode.AsImportSpecifier()
					if spec == nil || spec.IsTypeOnly {
						continue
					}

					localName := b.getDeclarationNameText(spec.Name())
					if localName == "" {
						continue
					}

					// 获取原始导出名（如果有别名的话）
					originalName := localName
					if spec.PropertyName != nil {
						if ast.IsIdentifier(spec.PropertyName) {
							originalName = spec.PropertyName.AsIdentifier().Text
						} else if ast.IsStringLiteral(spec.PropertyName) {
							originalName = strings.Trim(spec.PropertyName.AsStringLiteral().Text, `"'`)
						}
					}

					b.assignImportedValue(localName, originalName, resolvedPath, isExternal)
				}
			}
		}
	}

	return nil
}

// VisitNamespaceImport 访问命名空间导入
func (b *builder) VisitNamespaceImport(node *ast.NamespaceImport) interface{} { return nil }

// VisitNamedImports 访问命名导入
func (b *builder) VisitNamedImports(node *ast.NamedImports) interface{} { return nil }

// VisitImportSpecifier 访问导入说明符
func (b *builder) VisitImportSpecifier(node *ast.ImportSpecifier) interface{} { return nil }

func (b *builder) VisitImportEqualsDeclaration(node *ast.ImportEqualsDeclaration) interface{} {
	return nil
}

// VisitExportAssignment 访问导出赋值
/*
A. export default <表达式>;（IsExportEquals == false） export default 42;
B. export = <实体名>;（IsExportEquals == true） 等价 CJS：module.exports = foo
   这是 TypeScript 专有的 export-equals，用于兼容 CommonJS/AMD。右侧严格是 实体名（EntityName）：标识符或限定名（A.B.C）。不是任意表达式。

不会命中的写法（列举以便区分）

导出声明族（ExportDeclaration）：

export { a, b as c };
export { x as default };      // 这是“把具名导出重新导出为默认”，仍是 ExportDeclaration
export * from "./mod";
export { a } from "./mod";
export { ChildNewApp as default } // convert named export to default export
export * as ns from "./mod";


带 default 的声明（声明节点本身）：

export default function F() {}
export default class C {}


这些是 FunctionDeclaration / ClassDeclaration，带 Export+Default 修饰，不走 ExportAssignment。

变量默认导出（不合法）：

export default const x = 1;   // 语法非法
*/
func (b *builder) VisitExportAssignment(node *ast.ExportAssignment) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	if !node.IsExportEquals { // export default
		if variable := b.VisitLeftValueExpression(node.Expression); variable != nil {
			b.namedValueExports["default"] = variable.GetName()
		}
	} else {
		if variable := b.VisitLeftValueExpression(node.Expression); variable != nil {
			b.cjsExport = variable.GetName()
		}
	}
	return nil
}

// VisitExportDeclaration 导出声明
/*
export { foo }
export { foo as bar }
export { foo } from './mod'
*/
func (b *builder) VisitExportDeclaration(decl *ast.ExportDeclaration) interface{} {
	if decl == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &decl.Loc, "")
	defer recoverRange()

	if exportClause := decl.ExportClause; exportClause != nil {
		if ast.IsNamedExports(exportClause) {
			namedExports := exportClause.AsNamedExports()
			b.VisitNamedExports(namedExports)
		} else if ast.IsNamespaceExport(exportClause) {
			namespaceExport := exportClause.AsNamespaceExport()
			_ = namespaceExport
			log.Warn("unimplemented Namespace export")
		}
	}
	return nil
}

// VisitNamedExports 访问命名导出
func (b *builder) VisitNamedExports(namedExports *ast.NamedExports) interface{} {
	if namedExports == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &namedExports.Loc, "")
	defer recoverRange()

	var exportName, exportedItemName string
	for _, element := range namedExports.Elements.Nodes {
		if aliasName := element.AsExportSpecifier().PropertyName; aliasName != nil {
			exportName = b.VisitModuleExportName(aliasName)
		}
		exportedItemName = b.VisitModuleExportName(element.AsExportSpecifier().Name())
		if exportName == "" {
			exportName = exportedItemName
		}
		if exportName == "default" { // export { ChildNewApp as default }
			b.namedValueExports["default"] = exportedItemName
		} else {
			b.namedValueExports[exportName] = exportedItemName
		}

	}
	return nil
}

func (b *builder) VisitModuleExportName(node *ast.ModuleExportName) string {
	if node == nil || b.IsStop() {
		return ""
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	if ast.IsIdentifier(node) {
		return node.AsIdentifier().Text
	}
	return node.AsStringLiteral().Text
}

// VisitExternalModuleReference 访问外部模块引用
func (b *builder) VisitExternalModuleReference(node *ast.ExternalModuleReference) interface{} {
	return nil
}

// =====Expression=====

// VisitExpression 访问表达式相关的访问函数
// VisitExpression 返回L-Val和R-Val分别对应返回类型*ssa.Variable和ssa.Value
func (b *builder) VisitExpression(node *ast.Expression, isLval bool) (*ssa.Variable, ssa.Value) {
	if node == nil || b.IsStop() {
		return nil, nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	switch node.Kind {
	// 需要区分L/R Value的类型
	case ast.KindIdentifier:
		class := b.MarkedThisClassBlueprint
		identifierName := b.VisitIdentifier(node.AsIdentifier())
		if isLval {
			if class == nil {
				return b.CreateJSVariable(identifierName), nil
			}
			if class.GetNormalMember(identifierName) != nil {
				obj := b.PeekValue("this")
				if obj != nil {
					return b.CreateMemberCallVariable(obj, b.EmitConstInstPlaceholder(identifierName), true), nil
				}
			}
			return b.CreateJSVariable(identifierName), nil
		}
		// undefined 是一个 identifier
		if identifierName == "undefined" {
			return nil, b.EmitUndefined("")
		}

		if class != nil {
			if method := class.GetStaticMethod(identifierName); !utils.IsNil(method) {
				return nil, method
			}
			if class.GetNormalMember(identifierName) != nil {
				obj := b.PeekValue("this")
				if obj != nil {
					if value := b.ReadMemberCallValue(obj, b.EmitConstInstPlaceholder(identifierName)); value != nil {
						return nil, value
					}
				}
			}
			value := b.ReadSelfMember(identifierName)
			if value != nil {
				return nil, value
			}
		}

		bp := b.GetBluePrint(identifierName)
		if bp != nil {
			bp.Build()
			return nil, bp.Container()
		}
		if importType, ok := b.GetProgram().TryReadImportDeclare(identifierName); ok {
			if blueprint, ok := ssa.ToClassBluePrintType(importType); ok {
				blueprint.Build()
				return nil, blueprint.Container()
			}
		}
		readValue := b.ReadValue(identifierName)
		if !utils.IsNil(readValue) {
			if function, ok := ssa.ToFunction(readValue); ok && !utils.IsNil(function.LazyBuilder) {
				function.Build()
			}
		}
		return nil, readValue
	case ast.KindPropertyAccessExpression:
		propertyAccessExp := node.AsPropertyAccessExpression()
		obj, propName := b.VisitPropertyAccessExpression(propertyAccessExp)
		var objName string
		if ast.IsIdentifier(propertyAccessExp.Expression) {
			objName = propertyAccessExp.Expression.AsIdentifier().Text
		}

		bp := b.GetBluePrint(objName) // 处理静态方法调用
		if importType, ok := b.GetProgram().TryReadImportDeclare(objName); ok {
			if blueprint, ok := ssa.ToClassBluePrintType(importType); ok {
				bp = blueprint
			}
		}
		if (obj == nil && bp == nil) || propName == "" {
			return nil, nil
		}
		name := b.EmitConstInstPlaceholder(propName)
		if isLval {
			return b.CreateMemberCallVariable(obj, name), nil
		}
		if obj == nil {
			val := bp.GetStaticMember(propName)
			if !utils.IsNil(val) {
				return nil, val
			}
			return nil, nil
		}
		if b.IsPromiseType(obj.GetType()) && (propName == "then" || propName == "catch" || propName == "finally") {
			return nil, obj // let call handle async call for Promise with Promise<T>.then()
		}
		return nil, b.ReadMemberCallMethodOrValue(obj, name)
	case ast.KindElementAccessExpression:
		obj, arg := b.VisitElementAccessExpression(node.AsElementAccessExpression())
		if obj == nil || arg == nil {
			return nil, nil
		}
		if isLval {
			return b.CreateMemberCallVariable(obj, arg), nil
		}
		return nil, b.ReadMemberCallValue(obj, arg)

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
	case ast.KindComputedPropertyName:
		return nil, b.VisitComputedPropertyName(node.AsComputedPropertyName())
	case ast.KindJsxSpreadAttribute:
		return nil, b.VisitJsxSpreadAttribute(node.AsJsxSpreadAttribute())
	case ast.KindTemplateSpan:
		return nil, b.VisitTemplateSpan(node.AsTemplateSpan())
	default:
		// 未处理的表达式类型
		b.NewError(ssa.Error, TAG, "Unhandled Exp type")
		return nil, nil
	}
}

// VisitBinaryExpression 访问二元表达式
func (b *builder) VisitBinaryExpression(node *ast.BinaryExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	handlerJumpExpression := func(
		cond func(string) ssa.Value,
		trueExpr, falseExpr func() ssa.Value,
		valueName string,
	) ssa.Value {
		// 为了聚合产生Phi指令
		id := valueName + "_" + uuid.NewString()
		variable := b.CreateLocalVariable(id)
		b.AssignVariable(variable, b.EmitValueOnlyDeclare(id))
		// 只需要使用b.WriteValue设置value到此ID，并最后调用b.ReadValue可聚合产生Phi指令，完成语句预期行为
		ifb := b.CreateIfBuilder()
		ifb.AppendItem(
			func() ssa.Value {
				return cond(id)
			},
			func() {
				v := trueExpr()
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
			},
		)
		ifb.SetElse(func() {
			v := falseExpr()
			variable := b.CreateVariable(id)
			b.AssignVariable(variable, v)
		})
		ifb.Build()
		// generator phi instruction
		v := b.ReadValue(id)
		v.SetName(scanner.GetSourceTextOfNodeFromSourceFile(b.sourceFile, node.AsNode(), true))
		return v
	}

	left := b.VisitRightValueExpression(node.Left)
	right := b.VisitRightValueExpression(node.Right)

	if left == nil || right == nil {
		b.NewError(ssa.Error, TAG, BinOPWithNilSSAValue())
		return b.EmitUndefined("bad-binary-op")
	}

	// 根据操作符类型生成不同的二元操作
	switch node.OperatorToken.Kind {
	// Arithmetic PLUS + MINUS - MUL * DIV / MOD % POW **
	case ast.KindPlusToken, ast.KindMinusToken, ast.KindAsteriskToken, ast.KindSlashToken, ast.KindPercentToken, ast.KindAsteriskAsteriskToken:
		binOp, ok := arithmeticBinOpTbl[node.OperatorToken.Kind]
		if !ok {
			b.NewError(ssa.Error, TAG, UnexpectedArithmeticOP())
			return b.EmitUndefined("")
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
	// Logical AND &&
	// `a && b` return a if a is falsy else return b
	case ast.KindAmpersandAmpersandToken:
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return left
			},
			func() ssa.Value {
				return right
			},
			func() ssa.Value {
				return left
			},
			ssa.AndExpressionVariable,
		)
	// Logical OR ||
	// `a || b` return a if a is truthy else return b
	case ast.KindBarBarToken:
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return left
			},
			func() ssa.Value {
				return left
			},
			func() ssa.Value {
				return right
			},
			ssa.OrExpressionVariable,
		)
	// Nullish Coalescing ??
	case ast.KindQuestionQuestionToken:
		return handlerJumpExpression(
			func(id string) ssa.Value {
				return b.EmitBinOp(ssa.OpLogicOr, b.EmitBinOp(ssa.OpEq, left, ssa.NewNil()), b.EmitBinOp(ssa.OpEq, left, ssa.NewUndefined("")))
			},
			func() ssa.Value {
				return right
			},
			func() ssa.Value {
				return left
			},
			ssa.AndExpressionVariable,
		)
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
	case ast.KindAmpersandAmpersandEqualsToken:
		variable := b.VisitLeftValueExpression(node.Left)
		newVal := handlerJumpExpression(
			func(id string) ssa.Value {
				return left
			},
			func() ssa.Value {
				return right
			},
			func() ssa.Value {
				return left
			},
			ssa.AndExpressionVariable,
		)
		b.AssignVariable(variable, newVal)
		return newVal
	case ast.KindBarBarEqualsToken:
		variable := b.VisitLeftValueExpression(node.Left)
		newVal := handlerJumpExpression(
			func(id string) ssa.Value {
				return left
			},
			func() ssa.Value {
				return left
			},
			func() ssa.Value {
				return right
			},
			ssa.OrExpressionVariable,
		)
		b.AssignVariable(variable, newVal)
		return newVal
	// Logical Assignment ??=
	case ast.KindQuestionQuestionEqualsToken:
		variable := b.VisitLeftValueExpression(node.Left)
		newVal := handlerJumpExpression(
			func(id string) ssa.Value {
				return b.EmitBinOp(ssa.OpLogicOr, b.EmitBinOp(ssa.OpEq, left, ssa.NewNil()), b.EmitBinOp(ssa.OpEq, left, ssa.NewUndefined("")))
			},
			func() ssa.Value {
				return right
			},
			func() ssa.Value {
				return left
			},
			ssa.AndExpressionVariable,
		)
		b.AssignVariable(variable, newVal)
		return newVal
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
	case ast.KindCommaToken:
		return right
	case ast.KindInKeyword:
		if left != nil && right != nil && (b.IsListLike(right) || b.IsMapLike(right) || b.IsObjectLike(right)) {
			_, ok := right.GetMember(left)
			return b.EmitConstInst(ok)
		}
		return b.EmitUndefined("")
	case ast.KindInstanceOfKeyword:
		if left != nil && right != nil {
			if right.GetType() == nil || left.GetType() == nil {
				return b.EmitConstInst(true)
			} else {
				if ssa.TypeCompare(left.GetType(), right.GetType()) {
					return b.EmitConstInst(true)
				} else {
					return b.EmitConstInst(false)
				}
			}
		}
		b.NewError(ssa.Error, TAG, InstanceOfGotNilValue())
		return b.EmitUndefined("")

	// 处理其他运算符...
	default:
		// 未实现的操作符处理
		b.NewError(ssa.Error, TAG, UnhandledBinOP())
	}
	return nil
}

// VisitCallExpression 处理函数调用表达式
func (b *builder) VisitCallExpression(node *ast.CallExpression) ssa.Value {
	if node == nil || b.IsStop() {
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

	var callee ssa.Value
	callee = b.VisitRightValueExpression(node.Expression)
	// 检查是否是 Promise 的特殊方法调用（then, catch, finally）
	if ast.IsPropertyAccessExpression(node.Expression) {
		propAccess := node.Expression.AsPropertyAccessExpression()
		if propAccess.Name() != nil {
			methodName := b.ProcessMemberName(propAccess.Name())
			if methodName == "then" || methodName == "catch" || methodName == "finally" {
				// 检查调用对象是否返回 Promise 类型
				if b.IsPromiseType(callee.GetType()) {
					// 确认是 Promise 类型，进行特殊处理
					return b.HandlePromiseMethod(callee, methodName, args)
				}
				// 如果不是 Promise 类型，按普通方法调用处理
			}
		}
	}

	// 处理callee（被调用函数）
	if callee == nil {
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, InvalidFunctionCallee())
		return b.EmitUndefined("")
	}
	if !utils.IsNil(callee.GetFunc()) {
		for len(callee.GetFunc().Params) > len(args) {
			args = append(args, b.EmitUndefined(""))
		}
		// 创建调用
		// TODO: 函数调用导致实参发生改变如何处理?
		call := b.EmitCall(b.NewCall(callee, args))

		// 根据被调用函数的类型设置 Call 指令的返回类型
		// 这对于 Promise 等返回类型的正确传递非常重要
		if funcType := callee.GetType(); funcType != nil {
			if funcType.GetTypeKind() == ssa.FunctionTypeKind {
				// 如果是函数类型，获取其返回类型
				if ft, ok := funcType.(*ssa.FunctionType); ok && ft.ReturnType != nil {
					call.SetType(ft.ReturnType)
				}
			}
		}

		return call
	}
	return nil
}

// VisitObjectLiteralExpression 访问对象字面量表达式
func (b *builder) VisitObjectLiteralExpression(objLiteral *ast.ObjectLiteralExpression) ssa.Value {
	if objLiteral == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &objLiteral.Loc, "")
	defer recoverRange()

	var values []ssa.Value
	var keys []ssa.Value
	hasNamedProperty := false

	// 没有属性的情况下，创建一个空对象
	if objLiteral.Properties == nil || len(objLiteral.Properties.Nodes) == 0 {
		b.CreateObjectWithMap(keys, values)
	}

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
				b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, "Unexpected token ':'")
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
					key := b.EmitConstInstPlaceholder(methodName)
					keys = append(keys, key)
					values = append(values, newFunc)
				}
			}
		} else if ast.IsGetAccessorDeclaration(prop) || ast.IsSetAccessorDeclaration(prop) {
			// 处理getter和setter
			var accessorName string
			var propertyName *ast.PropertyName

			// 根据实际类型选择正确的转换
			if ast.IsGetAccessorDeclaration(prop) {
				accessorDecl := prop.AsGetAccessorDeclaration()
				propertyName = accessorDecl.Name()
			} else { // 必定是SetAccessorDeclaration
				accessorDecl := prop.AsSetAccessorDeclaration()
				propertyName = accessorDecl.Name()
			}

			// 处理属性名
			if propertyName != nil {
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

				key := b.EmitConstInstPlaceholder(accessorName)
				keys = append(keys, key)
				values = append(values, newFunc)
			}
		}
	}

	// 创建对象
	//if len(keys) == 0 {
	//	// 没有命名属性，使用数组方式创建
	//	return b.CreateObjectWithSlice(values)
	//} else {
	// 有命名属性，使用map方式创建
	return b.CreateObjectWithMap(keys, values)
	//}
}

// VisitArrayLiteralExpression 访问数组字面量表达式
func (b *builder) VisitArrayLiteralExpression(arrayLiteral *ast.ArrayLiteralExpression) ssa.Value {
	if arrayLiteral == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &arrayLiteral.Loc, "")
	defer recoverRange()

	// 没有元素的空数组
	if arrayLiteral.Elements == nil || len(arrayLiteral.Elements.Nodes) == 0 {
		return b.CreateObjectWithSlice([]ssa.Value{})
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
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 处理操作数
	operand := b.VisitRightValueExpression(node.Operand)
	if operand == nil {
		b.NewError(ssa.Error, TAG, NoOperandFoundForPrefixUnaryExp())
		return b.EmitUndefined("")
	}

	// 根据操作符类型生成不同的一元操作
	switch node.Operator {
	// 一元加法(+)：尝试将操作数转换为数值
	case ast.KindPlusToken:
		return b.EmitUnOp(ssa.OpPlus, operand)

	// 一元减法(-)：对操作数取负
	case ast.KindMinusToken:
		return b.EmitUnOp(ssa.OpNeg, operand)

	// 按位取反(~)：对操作数进行按位取反
	case ast.KindTildeToken:
		return b.EmitUnOp(ssa.OpBitwiseNot, operand)

	// 逻辑非(!)：对操作数进行逻辑取反
	case ast.KindExclamationToken:
		return b.EmitUnOp(ssa.OpNot, operand)

	// 删除操作符(delete)：删除对象的属性
	case ast.KindDeleteKeyword:
		return b.EmitConstInst(true)
	// 类型查询操作符(typeof)：返回操作数的类型字符串
	case ast.KindTypeOfKeyword:
		return b.EmitConstInst("")

	// void操作符：执行操作数并返回undefined
	case ast.KindVoidKeyword:
		return b.EmitUndefined("")

	// 前缀自增(++x)：将操作数加1并返回新值 | 前缀自减(--x)：将操作数减1并返回新值
	case ast.KindPlusPlusToken, ast.KindMinusMinusToken:
		variable := b.VisitLeftValueExpression(node.Operand)
		if variable == nil {
			b.NewError(ssa.Error, TAG, NoViableOperandForPrefixUnaryExp())
			return nil
		}
		// 获取当前值，加1，更新变量，返回新值
		currentValue := b.PeekValueByVariable(variable)
		if currentValue == nil {
			b.NewError(ssa.Error, TAG, VariableIsNotDefined())
			return nil
		}
		var binOP ssa.BinaryOpcode
		if node.Operator == ast.KindPlusPlusToken {
			binOP = ssa.OpAdd
		} else {
			binOP = ssa.OpSub
		}
		newValue := b.EmitBinOp(binOP, currentValue, b.EmitConstInst(1))
		b.AssignVariable(variable, newValue)
		return newValue

	// await操作符：等待Promise解析
	// TODO: await
	case ast.KindAwaitKeyword:
		// 创建await操作
		return b.EmitUndefined("")

	default:
		// 未实现的操作符处理
		//panic("unhandled prefix unary expression")
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, UnexpectedUnaryOP())
		return nil
	}
}

// VisitPostfixUnaryExpression 访问后缀一元表达式
func (b *builder) VisitPostfixUnaryExpression(node *ast.PostfixUnaryExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 获取操作数
	// 注意：后缀操作符需要操作数是可赋值的表达式（左值）
	variable := b.VisitLeftValueExpression(node.Operand)
	if variable == nil {
		b.NewError(ssa.Error, TAG, NoViableOperandForPostfixUnaryExp())
		return b.EmitUndefined("")
	}

	// 获取操作数的当前值
	currentValue := b.PeekValueByVariable(variable)
	if currentValue == nil {
		b.NewError(ssa.Error, TAG, VariableIsNotDefined())
		return b.EmitUndefined("")
	}

	// 根据操作符类型处理
	switch node.Operator {
	// 后缀自增(x++)：将操作数加1，但返回原始值
	case ast.KindPlusPlusToken:
		// 计算新值：当前值加1
		newValue := b.EmitBinOp(ssa.OpAdd, currentValue, b.EmitConstInst(1))

		// 更新变量值
		b.AssignVariable(variable, newValue)

		// 返回操作前的原始值
		return currentValue

	// 后缀自减(x--)：将操作数减1，但返回原始值
	case ast.KindMinusMinusToken:
		// 计算新值：当前值减1
		newValue := b.EmitBinOp(ssa.OpSub, currentValue, b.EmitConstInst(1))

		// 更新变量值
		b.AssignVariable(variable, newValue)

		// 返回操作前的原始值
		return currentValue

	default:
		// 未实现的操作符处理
		//panic("unhandled postfix unary expression")
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, fmt.Sprintf("未支持的后缀一元操作符: %v", node.Operator))
		return nil
	}
}

// VisitPropertyAccessExpression 访问属性访问表达式
func (b *builder) VisitPropertyAccessExpression(node *ast.PropertyAccessExpression) (ssa.Value, string) {
	if node == nil || b.IsStop() {
		return nil, ""
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	obj := b.VisitRightValueExpression(node.Expression)
	if obj == nil {
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, UnexpectedRightValueForObjectPropertyAccess())
		return nil, ""
	}
	// 获取属性名
	propName := ""
	if node.Name() != nil {
		propName = b.ProcessMemberName(node.Name())
	}

	return obj, propName
}

// VisitElementAccessExpression 访问元素访问表达式
func (b *builder) VisitElementAccessExpression(node *ast.ElementAccessExpression) (ssa.Value, ssa.Value) {
	if node == nil {
		return nil, nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	obj := b.VisitRightValueExpression(node.Expression)
	if obj == nil {
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, UnexpectedRightValueForElementAccess())
		return nil, nil
	}
	// 获取下标参数
	var argument ssa.Value
	if node.ArgumentExpression != nil {
		argument = b.VisitRightValueExpression(node.ArgumentExpression)
	}

	return obj, argument
}

// VisitNewExpression 访问new表达式
func (b *builder) VisitNewExpression(node *ast.NewExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// TODO: js builtin support? Symbol Bigint
	className := ""
	switch node.Expression.Kind {
	case ast.KindIdentifier:
		className = node.Expression.AsIdentifier().Text
	default:
		b.NewError(ssa.Warn, TAG, NewExpressionOnlySupportIdentifierClassName())
		return nil
	}

	class := b.GetBluePrint(className)
	obj := b.EmitUndefined(className)
	if class == nil {
		class = b.CreateBlueprint(className)
		defaultCtorFunc := b.NewFunc(fmt.Sprintf("%s_%s", className, "default_ctor_func"))
		class.RegisterMagicMethod(ssa.Constructor, defaultCtorFunc)
		store := b.StoreFunctionBuilder()
		defaultCtorFunc.AddLazyBuilder(func() {
			switchHandler := b.SwitchFunctionBuilder(store)
			defer switchHandler()
			b.FunctionBuilder = b.PushFunction(defaultCtorFunc)
			{
				b.NewParam("$this")
				container := b.EmitEmptyContainer()
				variable := b.CreateVariable("this")
				b.AssignVariable(variable, container)
				container.SetType(class)

				b.EmitReturn([]ssa.Value{container})
				b.Finish()
			}
			b.FunctionBuilder = b.PopFunction()
		})
	}
	obj.SetType(class)
	args := []ssa.Value{obj}
	if node.Arguments != nil {
		for _, arg := range node.Arguments.Nodes {
			argVal := b.VisitRightValueExpression(arg)
			if argVal != nil {
				args = append(args, argVal)
			} else {
				args = append(args, b.EmitUndefined("bad arg when call new()"))
			}

		}
	}
	return b.ClassConstructorWithoutDeferDestructor(class, args)
}

// VisitParenthesizedExpression 访问带括号的表达式
func (b *builder) VisitParenthesizedExpression(node *ast.ParenthesizedExpression) ssa.Value {
	// 括号表达式直接访问其内部表达式
	// 括号在AST中只是一个标记，不影响执行结果
	// 但在某些情况下可能影响优先级或类型推断
	if node.Expression == nil || b.IsStop() {
		return b.EmitUndefined("")
	}

	// 括号表达式的值就是内部表达式的值
	return b.VisitRightValueExpression(node.Expression)
}

// 定义函数有多种方法 使用函数声明(函数语句)或者使用函数表达式

// VisitFunctionDeclaration 访问函数声明
// function name([param[, param[, ... param]]]) { statements }
func (b *builder) VisitFunctionDeclaration(node *ast.FunctionDeclaration) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 获取函数名
	funcName := ""
	if node.Name() != nil && node.Name().Kind == ast.KindIdentifier {
		funcName = node.Name().AsIdentifier().Text
	} else {
		// 函数声明必须有名称，如果没有名称，生成一个唯一名称
		funcName = "anonymous_func_" + uuid.NewString()
	}

	// 使用 AST 提供的辅助方法检查修饰符
	nodePtr := node.AsNode()
	isExport := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsExport)
	isDefault := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsDefault)
	isAsync := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsAsync)
	if !ShouldVisit(b.PreHandler(), isExport) {
		return nil
	}
	if isExport {
		if !isDefault {
			b.namedValueExports[funcName] = funcName
		} else {
			b.namedValueExports["default"] = funcName
		}

	}

	// 创建新的函数对象
	newFunc := b.NewFunc(funcName)
	store := b.StoreFunctionBuilder()
	log.Infof("add function funcName = %s", funcName)
	newFunc.AddLazyBuilder(func() {
		log.Infof("lazy-build function funcName = %s", funcName)
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)

		// 设置函数返回类型（如果有显式类型注解）
		funcLikeData := node.FunctionLikeData()
		if funcLikeData != nil && funcLikeData.Type != nil {
			returnType := b.VisitTypeNode(funcLikeData.Type)
			b.SetCurrentReturnType(returnType)
		} else if isAsync {
			// async 函数如果没有显式类型注解，自动包装为 Promise<any>
			promiseType := ssa.NewObjectType()
			promiseType.AddFullTypeName("Promise<any>")
			b.SetCurrentReturnType(promiseType)
		}

		// 处理函数参数
		if node.Parameters != nil && len(node.Parameters.Nodes) > 0 {
			b.ProcessFunctionParams(node.Parameters)
		}

		// 处理函数体
		// 只有箭头函数的函数体可以是表达式
		if node.Body != nil && node.Body.Kind == ast.KindBlock {
			blockNode := node.Body.AsBlock()
			if blockNode.Statements != nil {
				b.VisitStatements(blockNode.Statements)
			}
		}
		b.FunctionBuilder = b.PopFunction()
	})
	// 在当前作用域中创建函数变量
	variable := b.CreateJSVariable(funcName)
	b.AssignVariable(variable, newFunc)

	return nil
}

// VisitFunctionExpression 访问函数表达式
// var myFunction = function name([param[, param[, ... param]]]) { statements }
func (b *builder) VisitFunctionExpression(node *ast.FunctionExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 获取函数名（如果有）
	funcName := ""
	if node.Name() != nil {
		funcName = node.Name().AsIdentifier().Text
	} else {
		funcName = "anonymous_func_" + uuid.NewString()
	}

	// 使用 AST 提供的辅助方法检查是否是 async 函数
	nodePtr := node.AsNode()
	isAsync := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsAsync)

	// 创建新的函数对象
	newFunc := b.NewFunc(funcName)
	store := b.StoreFunctionBuilder()
	log.Infof("add function expression funcName = %s", funcName)

	newFunc.AddLazyBuilder(func() {
		log.Infof("lazy-build function expression funcName = %s", funcName)
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)

		// 设置函数返回类型（如果有显式类型注解）
		funcLikeData := node.FunctionLikeData()
		if funcLikeData != nil && funcLikeData.Type != nil {
			returnType := b.VisitTypeNode(funcLikeData.Type)
			b.SetCurrentReturnType(returnType)
		} else if isAsync {
			// async 函数如果没有显式类型注解，自动包装为 Promise<any>
			promiseType := ssa.NewObjectType()
			promiseType.AddFullTypeName("Promise<any>")
			b.SetCurrentReturnType(promiseType)
		}

		// 处理函数参数
		if node.Parameters != nil && len(node.Parameters.Nodes) > 0 {
			b.ProcessFunctionParams(node.Parameters)
		}

		// 处理函数体
		if node.Body != nil && node.Body.Kind == ast.KindBlock {
			blockNode := node.Body.AsBlock()
			if blockNode.Statements != nil {
				b.VisitStatements(blockNode.Statements)
			}
		}

		// 完成函数构建
		b.Finish()

		// 恢复原来的函数上下文
		b.FunctionBuilder = b.PopFunction()
	})

	// 如果函数有名称并且在当前作用域中可见，将其加入变量表
	if funcName != "" {
		variable := b.CreateJSVariable(funcName)
		b.AssignVariable(variable, newFunc)
	}

	return newFunc
}

// VisitArrowFunction 访问箭头函数
// ([param] [, param]) => { statements } param => expression
func (b *builder) VisitArrowFunction(node *ast.ArrowFunction) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 获取函数名（如果有）
	funcName := ""
	if node.Name() != nil {
		funcName = node.Name().AsIdentifier().Text
	} else {
		funcName = "arrow_func_" + uuid.NewString()
	}

	// 使用 AST 提供的辅助方法检查是否是 async 函数
	nodePtr := node.AsNode()
	isAsync := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsAsync)

	// 创建新的函数对象
	newFunc := b.NewFunc(funcName)
	store := b.StoreFunctionBuilder()
	log.Infof("add arrow function funcName = %s", funcName)

	newFunc.AddLazyBuilder(func() {
		log.Infof("lazy-build arrow function funcName = %s", funcName)
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)

		// 设置函数返回类型（如果有显式类型注解）
		funcLikeData := node.FunctionLikeData()
		if funcLikeData != nil && funcLikeData.Type != nil {
			returnType := b.VisitTypeNode(funcLikeData.Type)
			b.SetCurrentReturnType(returnType)
		} else if isAsync {
			// async 箭头函数如果没有显式类型注解，自动包装为 Promise<any>
			promiseType := ssa.NewObjectType()
			promiseType.AddFullTypeName("Promise<any>")
			b.SetCurrentReturnType(promiseType)
		}

		// 处理函数参数
		if node.Parameters != nil && len(node.Parameters.Nodes) > 0 {
			b.ProcessFunctionParams(node.Parameters)
		}

		// 处理函数体
		if node.Body != nil && node.Body.Kind == ast.KindBlock {
			b.VisitBlock(node.Body.AsBlock())
		} else if node.Body != nil {
			ret := b.VisitRightValueExpression(node.Body)
			if ret == nil {
				b.EmitReturn([]ssa.Value{b.EmitUndefined("bad ret val")})
			} else {
				b.EmitReturn([]ssa.Value{ret})
			}

		}
		// 完成函数构建
		b.Finish()
		// 恢复原来的函数上下文
		b.FunctionBuilder = b.PopFunction()
	})
	// 箭头函数接收者应该是作为参数或者被赋值给变量
	return newFunc
}

// VisitConditionalExpression 访问条件表达式
func (b *builder) VisitConditionalExpression(node *ast.ConditionalExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	handlerJumpExpression := func(
		cond func(string) ssa.Value,
		trueExpr, falseExpr func() ssa.Value,
		valueName string,
	) ssa.Value {
		// 为了聚合产生Phi指令
		id := valueName + "_" + uuid.NewString()
		variable := b.CreateLocalVariable(id)
		b.AssignVariable(variable, b.EmitValueOnlyDeclare(id))
		// 只需要使用b.WriteValue设置value到此ID，并最后调用b.ReadValue可聚合产生Phi指令，完成语句预期行为
		ifb := b.CreateIfBuilder()
		ifb.AppendItem(
			func() ssa.Value {
				return cond(id)
			},
			func() {
				v := trueExpr()
				variable := b.CreateVariable(id)
				b.AssignVariable(variable, v)
			},
		)
		ifb.SetElse(func() {
			v := falseExpr()
			variable := b.CreateVariable(id)
			b.AssignVariable(variable, v)
		})
		ifb.Build()
		// generator phi instruction
		v := b.ReadValue(id)
		v.SetName(scanner.GetSourceTextOfNodeFromSourceFile(b.sourceFile, node.AsNode(), true))
		return v
	}

	return handlerJumpExpression(
		func(id string) ssa.Value {
			return b.EmitBinOp(ssa.OpEq, b.EmitConstInst(true), b.VisitRightValueExpression(node.Condition))
		},
		func() ssa.Value {
			return b.VisitRightValueExpression(node.WhenTrue)
		},
		func() ssa.Value {
			return b.VisitRightValueExpression(node.WhenFalse)
		},
		"ternary",
	)
}

// VisitTemplateExpression 访问模板表达式
func (b *builder) VisitTemplateExpression(node *ast.TemplateExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	unescapeTemplate := func(s string) string {
		s = strings.ReplaceAll(s, "\\`", "`")
		s = strings.ReplaceAll(s, "\\$", "$")
		s = strings.ReplaceAll(s, "\\n", "\n")
		s = strings.ReplaceAll(s, "\\r", "\r")
		return s
	}

	getRawTemplateText := func(node *ast.Node) string {
		if node == nil {
			return ""
		}
		switch node.Kind {
		case ast.KindTemplateHead:
			return node.AsTemplateHead().Text
		case ast.KindTemplateMiddle:
			return node.AsTemplateMiddle().Text
		case ast.KindTemplateTail:
			return node.AsTemplateTail().Text
		default:
			b.NewError(ssa.Error, TAG, "Unknown template literal node type")
			return ""
		}
	}
	var result ssa.Value
	result = b.EmitConstInst("")

	// 处理 head 部分（`head ${...}`）
	if node.Head != nil {
		headText := getRawTemplateText(node.Head)
		unescaped := unescapeTemplate(headText)
		result = b.EmitBinOp(ssa.OpAdd, result, b.EmitConstInst(unescaped))
	}

	// 处理每个 TemplateSpan：${expr} + literal
	if node.TemplateSpans != nil {
		for _, spanNode := range node.TemplateSpans.Nodes {
			// 1. 表达式部分
			exprVal := b.VisitRightValueExpression(spanNode.AsTemplateSpan().Expression)
			exprStr := b.EmitTypeCast(exprVal, ssa.CreateStringType())
			result = b.EmitBinOp(ssa.OpAdd, result, exprStr)

			// 2. 字符串字面量部分（tail 或 middle）
			if spanNode.AsTemplateSpan().Literal != nil {
				text := getRawTemplateText(spanNode.AsTemplateSpan().Literal)
				unescaped := unescapeTemplate(text)
				// TS前端解析出的TemplateSpan包含一个空的TemplateTail其中的Text和RawText都为空
				if spanNode.AsTemplateSpan().Literal.Kind == ast.KindTemplateTail && unescaped == "" {
					continue
				}
				result = b.EmitBinOp(ssa.OpAdd, result, b.EmitConstInst(unescaped))
			}
		}
	}

	return result
}

// VisitNoSubstitutionTemplateLiteral 访问无替换模板字面量
func (b *builder) VisitNoSubstitutionTemplateLiteral(node *ast.NoSubstitutionTemplateLiteral) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return b.EmitConstInst(node.Text)
}

// VisitTaggedTemplateExpression 访问标记模板表达式
func (b *builder) VisitTaggedTemplateExpression(node *ast.TaggedTemplateExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return b.EmitConstInst("")
}

// VisitSpreadElement 访问展开元素
func (b *builder) VisitSpreadElement(node *ast.SpreadElement) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitDeleteExpression 访问delete表达式
func (b *builder) VisitDeleteExpression(node *ast.DeleteExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitTypeOfExpression 访问typeof表达式
func (b *builder) VisitTypeOfExpression(node *ast.TypeOfExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	value := b.VisitRightValueExpression(node.Expression)
	if value == nil {
		b.NewError(ssa.Error, TAG, TypeofValueIsNil())
		return nil
	}
	t := value.GetType()
	if t == nil {
		return b.EmitUndefined("")
	}
	switch t.GetTypeKind() {
	case ssa.NumberTypeKind:
		return b.EmitConstInst("number")
	case ssa.StringTypeKind:
		return b.EmitConstInst("string")
	case ssa.BooleanTypeKind:
		return b.EmitConstInst("boolean")
	case ssa.NullTypeKind:
		// js的特性
		return b.EmitConstInst("object")
	case ssa.UndefinedTypeKind:
		return b.EmitConstInst("undefined")
	case ssa.ObjectTypeKind:
		return b.EmitConstInst("object")
	case ssa.MapTypeKind:
		return b.EmitConstInst("object")
	case ssa.SliceTypeKind:
		return b.EmitConstInst("object")
	default:
		return b.EmitUndefined("")
	}
}

// VisitVoidExpression 访问void表达式
func (b *builder) VisitVoidExpression(node *ast.VoidExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	/*
		void操作符用于表达式前，表示该表达式的计算结果被忽略，返回 undefined
		常用于函数中阻止页面跳转(如：<a href="javascript:void(0)">)
		console.log(void 0);  // undefined
		console.log(void(1 + 2));  // undefined
	*/
	return b.EmitUndefined("")
}

// VisitAwaitExpression 访问await表达式
func (b *builder) VisitAwaitExpression(node *ast.AwaitExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 处理await表达式
	// await 会等待 Promise 完成，并返回其解析的值
	promiseValue := b.VisitRightValueExpression(node.Expression)
	if promiseValue == nil {
		return b.EmitUndefined("")
	}

	// 从 Promise<T> 中提取 T
	resolvedValue := b.ExtractPromiseResolvedValue(promiseValue)
	return resolvedValue
}

// VisitTypeNode 访问TypeScript类型节点
func (b *builder) VisitTypeNode(typeNode *ast.TypeNode) ssa.Type {
	if typeNode == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &typeNode.Loc, "")
	defer recoverRange()

	switch typeNode.Kind {
	case ast.KindNumberKeyword:
		return ssa.CreateNumberType()
	case ast.KindStringKeyword:
		return ssa.CreateStringType()
	case ast.KindBooleanKeyword:
		return ssa.CreateBooleanType()
	case ast.KindAnyKeyword:
		return ssa.CreateAnyType()
	case ast.KindVoidKeyword:
		return ssa.CreateUndefinedType()
	case ast.KindNullKeyword:
		return ssa.CreateNullType()
	case ast.KindUndefinedKeyword:
		return ssa.CreateUndefinedType()
	case ast.KindNeverKeyword:
		return ssa.CreateAnyType() // TypeScript的never类型，在运行时表现为any
	case ast.KindObjectKeyword:
		return ssa.NewObjectType()
	case ast.KindArrayType:
		// 处理数组类型: number[], string[]
		elementType := b.VisitTypeNode(typeNode.AsArrayTypeNode().ElementType)
		sliceType := ssa.NewSliceType(elementType)
		// 设置数组类型的完整类型名
		elementTypeNames := elementType.GetFullTypeNames()
		for _, name := range elementTypeNames {
			sliceType.AddFullTypeName(name + "[]")
		}
		return sliceType
	case ast.KindTypeReference:
		// 处理类型引用: MyClass, Array<string>, Promise<number>
		return b.VisitTypeReference(typeNode.AsTypeReferenceNode())
	case ast.KindUnionType:
		// 处理联合类型: string | number
		return b.VisitUnionType(typeNode.AsUnionTypeNode())
	case ast.KindIntersectionType:
		// 处理交叉类型: A & B
		return b.VisitIntersectionType(typeNode.AsIntersectionTypeNode())
	case ast.KindFunctionType:
		// 处理函数类型: (x: number) => string
		return b.VisitFunctionType(typeNode.AsFunctionTypeNode())
	case ast.KindLiteralType:
		// 处理字面量类型: "hello", 42, true
		return b.VisitLiteralType(typeNode.AsLiteralTypeNode())
	case ast.KindTupleType:
		// 处理元组类型: [string, number]
		return b.VisitTupleType(typeNode.AsTupleTypeNode())
	case ast.KindOptionalType:
		// 处理可选类型: string?
		baseType := b.VisitTypeNode(typeNode.AsOptionalTypeNode().Type)
		// 在TypeScript中，可选类型通常表示为联合类型 T | undefined
		undefinedType := ssa.CreateUndefinedType()
		return ssa.NewOrType(baseType, undefinedType)
	case ast.KindParenthesizedType:
		// 处理括号类型: (string | number)
		return b.VisitTypeNode(typeNode.AsParenthesizedTypeNode().Type)
	default:
		// 未处理的类型，返回any类型
		return ssa.CreateAnyType()
	}
}

// VisitTypeReference 访问类型引用
func (b *builder) VisitTypeReference(typeRef *ast.TypeReferenceNode) ssa.Type {
	if typeRef == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &typeRef.Loc, "")
	defer recoverRange()

	// 获取类型名称
	var typeName string
	switch typeRef.TypeName.Kind {
	case ast.KindQualifiedName:
		typeName = b.VisitQualifiedName(typeRef.TypeName.AsQualifiedName())
	default: // Identifier
		typeName = typeRef.TypeName.Text()
	}

	// 处理泛型参数
	var typeArgs []ssa.Type
	if typeRef.TypeArguments != nil {
		for _, arg := range typeRef.TypeArguments.Nodes {
			argType := b.VisitTypeNode(arg)
			typeArgs = append(typeArgs, argType)
		}
	}

	// 根据类型名称创建相应的类型
	switch typeName {
	case "Array":
		if len(typeArgs) > 0 {
			elementType := typeArgs[0]
			sliceType := ssa.NewSliceType(elementType)
			elementTypeNames := elementType.GetFullTypeNames()
			for _, name := range elementTypeNames {
				sliceType.AddFullTypeName("Array<" + name + ">")
			}
			return sliceType
		}
		return ssa.NewSliceType(ssa.CreateAnyType())
	case "Promise":
		if len(typeArgs) > 0 {
			// Promise<T> 类型
			returnType := typeArgs[0]
			promiseType := ssa.NewObjectType()
			promiseType.AddFullTypeName("Promise<" + returnType.String() + ">")
			return promiseType
		}
		return ssa.NewObjectType()
	case "Date":
		return ssa.NewObjectType()
	case "RegExp":
		return ssa.NewObjectType()
	case "Error":
		return ssa.NewObjectType()
	default:
		// 自定义类型或类
		objectType := ssa.NewObjectType()
		objectType.AddFullTypeName(typeName)
		return objectType
	}
}

// VisitUnionType 访问联合类型
func (b *builder) VisitUnionType(unionType *ast.UnionTypeNode) ssa.Type {
	if unionType == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &unionType.Loc, "")
	defer recoverRange()

	var types []ssa.Type
	for _, typeNode := range unionType.Types.Nodes {
		typ := b.VisitTypeNode(typeNode)
		types = append(types, typ)
	}

	if len(types) == 0 {
		return ssa.CreateAnyType()
	} else if len(types) == 1 {
		return types[0]
	} else {
		return ssa.NewOrType(types...)
	}
}

// VisitIntersectionType 访问交叉类型
func (b *builder) VisitIntersectionType(intersectionType *ast.IntersectionTypeNode) ssa.Type {
	if intersectionType == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &intersectionType.Loc, "")
	defer recoverRange()

	// 交叉类型在运行时通常表现为对象类型的合并
	// 这里简化为返回第一个类型，实际实现可能需要更复杂的合并逻辑
	if len(intersectionType.Types.Nodes) > 0 {
		return b.VisitTypeNode(intersectionType.Types.Nodes[0])
	}
	return ssa.NewObjectType()
}

// VisitFunctionType 访问函数类型
func (b *builder) VisitFunctionType(funcType *ast.FunctionTypeNode) ssa.Type {
	if funcType == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &funcType.Loc, "")
	defer recoverRange()

	// 处理参数类型
	var paramTypes []ssa.Type
	if funcType.Parameters != nil {
		for _, param := range funcType.Parameters.Nodes {
			paramType := b.VisitTypeNode(param)
			paramTypes = append(paramTypes, paramType)
		}
	}

	// 处理返回类型
	var returnType ssa.Type
	if funcType.Type != nil {
		returnType = b.VisitTypeNode(funcType.Type)
	} else {
		returnType = ssa.CreateAnyType()
	}

	// 创建函数类型
	functionType := ssa.NewFunctionType("", paramTypes, returnType, false)

	return functionType
}

// VisitLiteralType 访问字面量类型
func (b *builder) VisitLiteralType(literalType *ast.LiteralTypeNode) ssa.Type {
	if literalType == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &literalType.Loc, "")
	defer recoverRange()

	// 根据字面量类型创建相应的基础类型
	switch literalType.Literal.Kind {
	case ast.KindStringLiteral:
		return ssa.CreateStringType()
	case ast.KindNumericLiteral:
		return ssa.CreateNumberType()
	case ast.KindTrueKeyword, ast.KindFalseKeyword:
		return ssa.CreateBooleanType()
	case ast.KindNullKeyword:
		return ssa.CreateNullType()
	default:
		return ssa.CreateAnyType()
	}
}

// VisitTupleType 访问元组类型
func (b *builder) VisitTupleType(tupleType *ast.TupleTypeNode) ssa.Type {
	if tupleType == nil || b.IsStop() {
		return ssa.CreateAnyType()
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &tupleType.Loc, "")
	defer recoverRange()

	// 元组类型在运行时表现为数组
	// 这里简化为返回通用数组类型，实际实现可能需要更复杂的处理
	return ssa.NewSliceType(ssa.CreateAnyType())
}

// MergeTypeWithAnnotation 合并推断类型和显式类型注解
func (b *builder) MergeTypeWithAnnotation(inferredType, explicitType ssa.Type) ssa.Type {
	if explicitType == nil {
		return inferredType
	}
	if inferredType == nil {
		return explicitType
	}

	// 如果显式类型是any，则使用推断类型
	if explicitType.GetTypeKind() == ssa.AnyTypeKind {
		return inferredType
	}

	// 如果推断类型是any，则使用显式类型
	if inferredType.GetTypeKind() == ssa.AnyTypeKind {
		return explicitType
	}

	// 如果两个类型相同，直接返回
	if inferredType.GetTypeKind() == explicitType.GetTypeKind() {
		// 合并完整类型名
		mergedType := ssa.NewBasicType(explicitType.GetTypeKind(), explicitType.String())
		mergedType.SetFullTypeNames(explicitType.GetFullTypeNames())

		// 添加推断类型的类型名（如果不同）
		for _, name := range inferredType.GetFullTypeNames() {
			if !contains(mergedType.GetFullTypeNames(), name) {
				mergedType.AddFullTypeName(name)
			}
		}

		return mergedType
	}

	// 类型不匹配时，优先使用显式类型，但记录警告
	b.NewError(ssa.Warn, TAG, "Type mismatch: inferred type %s vs explicit type %s",
		inferredType.String(), explicitType.String())
	return explicitType
}

// contains 检查字符串切片是否包含指定字符串
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// VisitYieldExpression 访问yield表达式
func (b *builder) VisitYieldExpression(node *ast.YieldExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitMetaProperty 访问元属性
func (b *builder) VisitMetaProperty(node *ast.MetaProperty) ssa.Value { return b.EmitUndefined("") }

// VisitPropertyAssignment 访问属性赋值
func (b *builder) VisitPropertyAssignment(node *ast.PropertyAssignment) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitShorthandPropertyAssignment 访问简写属性赋值
func (b *builder) VisitShorthandPropertyAssignment(node *ast.ShorthandPropertyAssignment) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitSpreadAssignment 访问展开赋值
func (b *builder) VisitSpreadAssignment(node *ast.SpreadAssignment) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxElement 访问JSX元素
func (b *builder) VisitJsxElement(node *ast.JsxElement) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitTemplateSpan 访问模板跨度
func (b *builder) VisitTemplateSpan(node *ast.TemplateSpan) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitBigIntLiteral 访问BigInt字面量
func (b *builder) VisitBigIntLiteral(node *ast.BigIntLiteral) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return b.EmitConstInst(codec.Atoi64(strings.TrimRight(node.Text, "n")))
}

// VisitRegularExpressionLiteral 访问正则表达式字面量
func (b *builder) VisitRegularExpressionLiteral(node *ast.RegularExpressionLiteral) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()
	// TODO: 正则表达式处理 目前当作字符串 可能后续需要把他当成一个内置类处理
	return b.EmitConstInst(node.Text)
}

// VisitThisExpression 访问this表达式
func (b *builder) VisitThisExpression(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 尝试从当前作用域获取已存在的this
	if thisValue := b.PeekValue("this"); thisValue != nil {
		return thisValue
	}

	b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, ThisKeywordNotAvailableInCurrentContext())

	// 可能还需要设置this的类型
	// 如果在类方法中，设置为当前类的类型
	// 如果在全局上下文中，设置为全局对象的类型

	return b.EmitUndefined("")
}

// VisitSuperExpression 访问super表达式
func (b *builder) VisitSuperExpression(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 尝试从当前作用域获取已存在的super
	parent := b.PeekValue("super")
	if parent == nil {
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, SuperKeywordNotAvailableInCurrentContext())
		return b.EmitUndefined("")
	}
	cls := b.MarkedThisClassBlueprint.GetSuperBlueprint()
	parent.SetType(cls)
	return parent
}

// VisitClassExpression 访问类表达式
func (b *builder) VisitClassExpression(node *ast.ClassExpression) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitOmittedExpression 访问省略表达式
func (b *builder) VisitOmittedExpression(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitSyntheticExpression 访问合成表达式
func (b *builder) VisitSyntheticExpression(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitPartiallyEmittedExpression 访问部分发出的表达式
func (b *builder) VisitPartiallyEmittedExpression(node *ast.PartiallyEmittedExpression) ssa.Value {
	if node == nil || b.IsStop() {
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
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxSelfClosingElement 访问JSX自闭合元素
func (b *builder) VisitJsxSelfClosingElement(node *ast.JsxSelfClosingElement) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxFragment 访问JSX片段
func (b *builder) VisitJsxFragment(node *ast.JsxFragment) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxAttributes 访问JSX属性
func (b *builder) VisitJsxAttributes(node *ast.JsxAttributes) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxAttributeValue 访问JSX属性值
func (b *builder) VisitJsxAttributeValue(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitJsxChild 访问JSX子元素
func (b *builder) VisitJsxChild(node *ast.Node) ssa.Value {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// VisitComputedPropertyName 访问计算属性名
// 处理对象字面量或类中使用计算属性名的情况，如 { [expr]: value }
func (b *builder) VisitComputedPropertyName(node *ast.ComputedPropertyName) ssa.Value {
	if node == nil || b.IsStop() {
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
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	return nil
}

// ===== MISC =====

func (b *builder) VisitPropertyName(propertyName *ast.PropertyName) ssa.Value {
	if propertyName == nil || b.IsStop() {
		return nil
	}

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
		// panic("unknown property name kind")
		b.NewError(ssa.Error, TAG, UnhandledPropertyNameType())
	}
	return nil
}

// ProcessObjectBindingPattern 处理对象解构模式
func (b *builder) ProcessObjectBindingPattern(pattern *ast.BindingPattern, sourceObj ssa.Value, isLocal bool, isExport bool) {
	if pattern == nil || sourceObj == nil || b.IsStop() {
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
				var restVar *ssa.Variable
				if isLocal {
					restVar = b.CreateLocalVariable(restName)
				} else {
					restVar = b.CreateJSVariable(restName)
				}
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
			propertyKey = b.EmitConstInstPlaceholder(propName)
		} else {
			// 没有有效的属性名
			b.NewError(ssa.Error, TAG, InvalidPropertyBinding())
			continue
		}

		// 如果没有绑定名称，跳过
		if bindingElement.Name() == nil || propertyKey == nil {
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
			if isExport {
				b.namedValueExports[varName] = varName
			}
			var variable *ssa.Variable
			if isLocal {
				variable = b.CreateLocalVariable(varName)
			} else {
				variable = b.CreateJSVariable(varName)
			}
			b.AssignVariable(variable, propValue)

		case ast.IsBindingPattern(bindingElement.Name()):
			// 嵌套解构: let { a: { b, c } } = obj 或 let { a: [x, y] } = obj
			/*
				obj
				└── a
				    ├── b  → 绑定到变量 b
				    └── c  → 绑定到变量 c
			*/
			nestedPattern := bindingElement.Name()

			// 递归处理
			if ast.IsObjectBindingPattern(nestedPattern) {
				b.ProcessObjectBindingPattern(nestedPattern.AsBindingPattern(), propValue, isLocal, isExport)
			} else if ast.IsArrayBindingPattern(nestedPattern) {
				b.ProcessArrayBindingPattern(nestedPattern.AsBindingPattern(), propValue, isLocal, isExport)
			}
		}
	}
}

// ProcessArrayBindingPattern 处理数组解构模式
func (b *builder) ProcessArrayBindingPattern(pattern *ast.BindingPattern, sourceArr ssa.Value, isLocal bool, isExport bool) {
	if pattern == nil || sourceArr == nil || b.IsStop() {
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

		// 检查是否是剩余元素: `let [a, ...rest] = arr`
		isRest := bindingElement.DotDotDotToken != nil

		var elementValue ssa.Value

		if isRest {
			// 简化处理：直接将整个数组赋值给rest变量
			if ast.IsIdentifier(bindingElement.Name()) {
				restName := bindingElement.Name().AsIdentifier().Text
				var restVar *ssa.Variable
				if isLocal {
					restVar = b.CreateLocalVariable(restName)
				} else {
					restVar = b.CreateJSVariable(restName)
				}
				b.AssignVariable(restVar, sourceArr)
			}
			continue
		} else {
			// 普通元素: arr[i]
			indexValue := b.EmitConstInstPlaceholder(i)
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
			if isExport {
				b.namedValueExports[varName] = varName
			}
			var variable *ssa.Variable
			if isLocal {
				variable = b.CreateLocalVariable(varName)
			} else {
				variable = b.CreateJSVariable(varName)
			}
			b.AssignVariable(variable, elementValue)

		case ast.IsBindingPattern(bindingElement.Name()):
			// 嵌套解构: let [[a, b], {c}] = arr
			nestedPattern := bindingElement.Name()

			// 递归处理
			if ast.IsObjectBindingPattern(nestedPattern) {
				b.ProcessObjectBindingPattern(nestedPattern.AsBindingPattern(), elementValue, isLocal, isExport)
			} else if ast.IsArrayBindingPattern(nestedPattern) {
				b.ProcessArrayBindingPattern(nestedPattern.AsBindingPattern(), elementValue, isLocal, isExport)
			}
		}
	}
}

func (b *builder) ProcessFunctionParams(params *ast.NodeList) {
	if params == nil || len(params.Nodes) == 0 || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &params.Loc, "")
	defer recoverRange()

	for index, param := range params.Nodes {
		paramNode := param.AsParameterDeclaration()
		paramName := ""

		paramNodeName := paramNode.Name()
		if paramNodeName != nil {
			switch paramNodeName.Kind {
			case ast.KindIdentifier:
				paramName = paramNodeName.AsIdentifier().Text
			// TODO: 这里的函数参数中的绑定需要额外处理
			case ast.KindArrayBindingPattern:
			case ast.KindObjectBindingPattern:
			default:
			}
		}

		if paramName != "" {
			// 创建参数
			p := b.NewParam(paramName)

			// 设置参数类型（如果有显式类型注解）
			if paramNode.Type != nil {
				paramType := b.VisitTypeNode(paramNode.Type)
				p.SetType(paramType)
			}

			// 处理默认值
			if paramNode.Initializer != nil {
				defaultValue := b.VisitRightValueExpression(paramNode.Initializer)
				if defaultValue != nil {
					p.SetDefault(defaultValue)
				}
			}
		} else {
			b.NewError(ssa.Error, TAG, FunctionParamNameEmpty())
		}

		if index == len(params.Nodes)-1 && paramNode.DotDotDotToken != nil {
			b.HandlerEllipsis()
		}
	}
}

func (b *builder) ProcessClassMember(member *ast.ClassElement, class *ssa.Blueprint) {
	b.PushBlueprint(class)
	defer b.PopBlueprint()

	switch member.Kind {
	case ast.KindConstructor:
		b.ProcessClassCtor(member, class)
	case ast.KindGetAccessor, ast.KindSetAccessor, ast.KindMethodDeclaration:
		b.ProcessClassMethod(member, class)
		return
	case ast.KindPropertyDeclaration:
		propDecl := member.AsPropertyDeclaration()

		setMember := class.RegisterNormalMember
		if ast.HasStaticModifier(member) {
			setMember = class.RegisterStaticMember
		}

		nameVal := b.ProcessPropertyName(propDecl.Name())
		undefined := b.EmitUndefined(nameVal)
		setMember(nameVal, undefined, false)
		store := b.StoreFunctionBuilder()
		class.AddLazyBuilder(func() {
			switchHandler := b.SwitchFunctionBuilder(store)
			defer switchHandler()
			if propDecl.Initializer != nil {
				value := b.VisitRightValueExpression(propDecl.Initializer)
				if !utils.IsNil(value) {
					setMember(nameVal, value)
				}
			}
		})
		return
	case ast.KindSemicolonClassElement:
		return
	case ast.KindClassStaticBlockDeclaration:
		store := b.StoreFunctionBuilder()
		class.AddLazyBuilder(func() {
			switchHandler := b.SwitchFunctionBuilder(store)
			defer switchHandler()
			b.VisitBlock(member.AsClassStaticBlockDeclaration().Body.AsBlock())
		})
		return
	default:
		return
	}
}

func (b *builder) ProcessPropertyName(propertyName *ast.PropertyName) string {
	switch propertyName.Kind {
	case ast.KindIdentifier:
		return propertyName.AsIdentifier().Text
	case ast.KindPrivateIdentifier:
		return propertyName.AsPrivateIdentifier().Text
	case ast.KindStringLiteral:
		return propertyName.AsStringLiteral().Text
	case ast.KindNoSubstitutionTemplateLiteral:
		return propertyName.AsNoSubstitutionTemplateLiteral().Text
	// 在ECMAScript 规范 (ECMA-262) 里 PropertyKey = String | Symbol 所以数字会被当成string处理
	// const obj = { 42: "the answer", "42": "not the answer" }; "42"会覆盖前面的
	case ast.KindNumericLiteral:
		return propertyName.AsNumericLiteral().Text
	case ast.KindComputedPropertyName:
		return b.VisitComputedPropertyName(propertyName.AsComputedPropertyName()).String()
	// A 'bigint' literal cannot be used as a property name.
	case ast.KindBigIntLiteral:
		return propertyName.AsBigIntLiteral().Text[:len(propertyName.AsBigIntLiteral().Text)-1]
	default:
		b.NewError(ssa.Error, TAG, UnexpectedPropertyNameType())
		return uuid.NewString()
	}
}

func (b *builder) ProcessClassMethod(member *ast.ClassElement, class *ssa.Blueprint) {
	var methodName string
	var params *ast.NodeList
	var returnTypeNode *ast.TypeNode
	// 只有箭头函数的函数体可以是表达式 箭头函数一般不出现在类内部作为方法
	var bodyNode *ast.Block

	// 根据不同类型获取相应信息
	switch member.Kind {
	case ast.KindGetAccessor:
		getAccessor := member.AsGetAccessorDeclaration()
		methodName = b.ProcessPropertyName(getAccessor.Name())
		params = getAccessor.Parameters
		returnTypeNode = getAccessor.Type
		if !ast.IsBlock(getAccessor.Body) {
			return
		}
		bodyNode = getAccessor.Body.AsBlock()
	case ast.KindSetAccessor:
		setAccessor := member.AsSetAccessorDeclaration()
		methodName = b.ProcessPropertyName(setAccessor.Name())
		params = setAccessor.Parameters
		returnTypeNode = setAccessor.Type
		if !ast.IsBlock(setAccessor.Body) {
			return
		}
		bodyNode = setAccessor.Body.AsBlock()
	case ast.KindMethodDeclaration:
		methodDecl := member.AsMethodDeclaration()
		methodName = b.ProcessPropertyName(methodDecl.Name())
		params = methodDecl.Parameters
		funcLikeData := methodDecl.FunctionLikeData()
		if funcLikeData != nil {
			returnTypeNode = funcLikeData.Type
		}
		// TODO  这里还没处理箭头函数表达式的情况
		if methodDecl.Body != nil && ast.IsBlock(methodDecl.Body) {
			bodyNode = methodDecl.Body.AsBlock()
		}

	default:
		b.NewError(ssa.Error, TAG, UnexpectedClassMethodType())
		return
	}

	// 共同的处理逻辑
	funcName := fmt.Sprintf("%s_%s_%s", class.Name, methodName, uuid.NewString()[:4])
	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(methodName)

	// 使用 AST 提供的辅助方法检查修饰符
	isStatic := ast.HasStaticModifier(member)
	isAsync := ast.HasSyntacticModifier(member, ast.ModifierFlagsAsync)

	if isStatic {
		class.RegisterStaticMethod(methodName, newFunc)
	} else {
		class.RegisterNormalMethod(methodName, newFunc)
	}

	store := b.StoreFunctionBuilder()

	newFunc.AddLazyBuilder(func() {
		log.Infof("lazybuild: %s %s ", funcName, methodName)
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)

		if !isStatic {
			this := b.NewParam("this")
			this.SetType(class)
		}
		b.MarkedThisClassBlueprint = class

		// 处理函数参数
		if params != nil && len(params.Nodes) > 0 {
			b.ProcessFunctionParams(params)
		}

		// 处理函数体
		if bodyNode != nil && bodyNode.Kind == ast.KindBlock {
			blockNode := bodyNode.AsBlock()
			if blockNode.Statements != nil {
				b.VisitStatements(blockNode.Statements)
			}
		}

		// 设置函数返回类型（如果有显式类型注解）
		if returnTypeNode != nil {
			returnType := b.VisitTypeNode(returnTypeNode)
			b.SetCurrentReturnType(returnType)
		} else if isAsync {
			// async 方法如果没有显式类型注解，自动包装为 Promise<any>
			promiseType := ssa.NewObjectType()
			promiseType.AddFullTypeName("Promise<any>")
			b.SetCurrentReturnType(promiseType)
		}

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}

func (b *builder) ProcessClassCtor(member *ast.ClassElement, class *ssa.Blueprint) {
	ctor := member.AsConstructorDeclaration()
	ctorName := fmt.Sprintf("%s_%s_%s", class.Name, "Custom-Constructor", uuid.NewString()[:4])
	params := ctor.Parameters

	// 预处理参数属性（Parameter Properties）
	// 这是 TypeScript 的语法糖，带有访问修饰符的构造函数参数会自动成为类成员
	var parameterProperties []*ast.ParameterDeclaration
	if params != nil && len(params.Nodes) > 0 {
		for _, paramNode := range params.Nodes {
			paramDecl := paramNode.AsParameterDeclaration()
			// 检查参数是否有参数属性修饰符 (public, private, protected, readonly)
			modifierFlags := paramDecl.ModifierFlags()
			if modifierFlags&ast.ModifierFlagsParameterPropertyModifier != ast.ModifierFlagsNone {
				parameterProperties = append(parameterProperties, paramDecl)

				// 为参数属性在类上创建成员
				paramName := ""
				if paramDecl.Name() != nil && ast.IsIdentifier(paramDecl.Name()) {
					paramName = paramDecl.Name().AsIdentifier().Text
				}

				if paramName != "" {
					// 注册为类成员（参数属性总是实例成员，不会是静态成员）
					undefined := b.EmitUndefined(paramName)

					// 设置成员类型（如果参数有类型注解）
					if paramDecl.Type != nil {
						paramType := b.VisitTypeNode(paramDecl.Type)
						undefined.SetType(paramType)
					}

					class.RegisterNormalMember(paramName, undefined, false)
				}
			}
		}
	}

	newFunc := b.NewFunc(ctorName)
	newFunc.SetMethodName(ctorName)
	class.Constructor = newFunc
	class.RegisterMagicMethod(ssa.Constructor, newFunc)
	store := b.StoreFunctionBuilder()
	newFunc.AddLazyBuilder(func() {
		log.Infof("lazybuild: %s ", ctorName)
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		b.FunctionBuilder = b.PushFunction(newFunc)
		{
			b.NewParam("$this")
			container := b.EmitEmptyContainer()
			variable := b.CreateVariable("this")
			b.AssignVariable(variable, container)
			container.SetType(class)

			// 处理函数参数
			if params != nil && len(params.Nodes) > 0 {
				b.ProcessFunctionParams(params)
			}

			// 处理参数属性：在构造函数开头自动添加 this.prop = param 赋值
			for _, paramProp := range parameterProperties {
				paramName := ""
				if paramProp.Name() != nil && ast.IsIdentifier(paramProp.Name()) {
					paramName = paramProp.Name().AsIdentifier().Text
				}

				if paramName != "" {
					// 读取参数值
					paramValue := b.ReadValue(paramName)

					// 赋值给 this.paramName
					thisValue := b.ReadValue("this")
					if thisValue != nil && paramValue != nil {
						memberKey := b.EmitConstInst(paramName)
						memberVariable := b.CreateMemberCallVariable(thisValue, memberKey)
						b.AssignVariable(memberVariable, paramValue)
					}
				}
			}

			// 处理函数体
			if ctor.Body != nil && ctor.Body.Kind == ast.KindBlock {
				blockNode := ctor.Body.AsBlock()
				b.VisitBlock(blockNode)
			}

			b.EmitReturn([]ssa.Value{container})
			b.Finish()
		}

		b.FunctionBuilder = b.PopFunction()
	})
}

func (b *builder) ProcessMemberName(name *ast.MemberName) string {
	switch {
	case ast.IsIdentifier(name):
		return name.AsIdentifier().Text
	case ast.IsPrivateIdentifier(name):
		return name.AsPrivateIdentifier().Text
	default:
		// panic("unhandled member name type")
		b.NewError(ssa.Error, TAG, UnhandledMemberNameType())
	}
	return fmt.Sprintf("UnexoectedMemberNameKind_%s", uuid.NewString()[:8])
}

// VisitEnumDeclaration 访问枚举声明
func (b *builder) VisitEnumDeclaration(node *ast.EnumDeclaration) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 获取枚举名称
	enumName := ""
	if node.Name() != nil && ast.IsIdentifier(node.Name()) {
		enumName = node.Name().AsIdentifier().Text
	} else {
		b.NewError(ssa.Error, TAG, "Enum declaration must have a name")
		return nil
	}

	// 使用 AST 提供的辅助方法检查修饰符
	nodePtr := node.AsNode()
	isExport := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsExport)
	isDefault := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsDefault)

	if !ShouldVisit(b.PreHandler(), isExport) {
		return nil
	}
	// PreHandle阶段：创建Blueprint和基础对象，立即设置导出
	enumBlueprint := b.CreateBlueprint(enumName)
	enumBlueprint.SetKind(ssa.BlueprintClass)

	// 创建枚举对象实例
	enumObj := b.EmitMakeWithoutType(nil, nil)
	enumObj.SetType(enumBlueprint)

	// 将枚举注册为变量
	variable := b.CreateJSVariable(enumName)
	b.AssignVariable(variable, enumObj)

	// 处理导出 - 在PreHandle阶段就设置导出值
	if isExport {
		if !isDefault {
			b.namedTypeExports[enumName] = enumName
			b.namedValueExports[enumName] = enumName
		} else {
			b.namedTypeExports["default"] = enumName
			b.namedValueExports["default"] = enumName
		}
	}

	// 添加LazyBuilder来处理枚举成员
	store := b.StoreFunctionBuilder()
	enumBlueprint.AddLazyBuilder(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()
		if enumName == "Color" {
			b.GetBluePrint("Palette")
		}

		// 处理枚举成员
		if node.Members != nil && len(node.Members.Nodes) > 0 {
			b.VisitEnumMembers(node.Members, enumBlueprint)
		}
	})

	return enumObj
}

// VisitEnumMembers 访问枚举成员列表
func (b *builder) VisitEnumMembers(members *ast.NodeList, enumBlueprint *ssa.Blueprint) {
	if members == nil || len(members.Nodes) == 0 || b.IsStop() {
		return
	}

	currentValue := 0 // TypeScript数字枚举的默认起始值

	for _, memberNode := range members.Nodes {
		if memberNode == nil {
			continue
		}

		if ast.IsEnumMember(memberNode) {
			currentValue = b.VisitEnumMember(memberNode.AsEnumMember(), enumBlueprint, currentValue)
		}
	}
}

// VisitEnumMember 访问单个枚举成员
func (b *builder) VisitEnumMember(member *ast.EnumMember, enumBlueprint *ssa.Blueprint, defaultValue int) int {
	if member == nil || b.IsStop() {
		return defaultValue
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &member.Loc, "")
	defer recoverRange()

	// 获取枚举成员名称
	memberName := ""
	if member.Name() != nil {
		memberName = b.ProcessPropertyName(member.Name())
	}

	if memberName == "" {
		b.NewError(ssa.Error, TAG, "Enum member must have a name")
		return defaultValue
	}

	var memberValue ssa.Value
	var nextValue int

	// 处理枚举成员的值
	if member.Initializer != nil {
		// 有显式初始化值
		initValue := b.VisitRightValueExpression(member.Initializer)
		memberValue = initValue

		// 尝试获取数字值以确定下一个默认值
		if constValue, ok := ssa.ToConstInst(initValue); ok {
			rawValue := constValue.GetRawValue()
			if numVal, ok := rawValue.(int); ok {
				nextValue = numVal + 1
			} else if numVal, ok := rawValue.(int64); ok {
				nextValue = int(numVal) + 1
			} else if numVal, ok := rawValue.(float64); ok {
				nextValue = int(numVal) + 1
			} else {
				// 非数字值，下一个成员使用当前默认值
				nextValue = defaultValue + 1
			}
		} else {
			nextValue = defaultValue + 1
		}
	} else {
		// 使用默认值（自动递增的数字）
		memberValue = b.EmitConstInst(defaultValue)
		nextValue = defaultValue + 1
	}

	// 注册枚举成员作为静态成员
	// TypeScript枚举的成员既可以通过名称访问也可以通过值访问
	enumBlueprint.RegisterStaticMember(memberName, memberValue)

	// 对于数字枚举，还需要支持反向映射（通过值访问名称）
	if constValue, ok := ssa.ToConstInst(memberValue); ok {
		rawValue := constValue.GetRawValue()
		if numVal, ok := rawValue.(int); ok {
			reverseKey := fmt.Sprintf("%d", numVal)
			reverseName := b.EmitConstInst(memberName)
			enumBlueprint.RegisterStaticMember(reverseKey, reverseName)
		} else if numVal, ok := rawValue.(int64); ok {
			reverseKey := fmt.Sprintf("%d", numVal)
			reverseName := b.EmitConstInst(memberName)
			enumBlueprint.RegisterStaticMember(reverseKey, reverseName)
		} else if numVal, ok := rawValue.(float64); ok {
			reverseKey := fmt.Sprintf("%.0f", numVal)
			reverseName := b.EmitConstInst(memberName)
			enumBlueprint.RegisterStaticMember(reverseKey, reverseName)
		}
	}

	return nextValue
}

// VisitInterfaceDeclaration 访问接口声明
func (b *builder) VisitInterfaceDeclaration(node *ast.InterfaceDeclaration) interface{} {
	if node == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &node.Loc, "")
	defer recoverRange()

	// 获取接口名称
	interfaceName := ""
	if node.Name() != nil && ast.IsIdentifier(node.Name()) {
		interfaceName = node.Name().AsIdentifier().Text
	} else {
		b.NewError(ssa.Error, TAG, "Interface declaration must have a name")
		return nil
	}

	// 使用 AST 提供的辅助方法检查修饰符
	nodePtr := node.AsNode()
	isExport := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsExport)
	isDefault := ast.HasSyntacticModifier(nodePtr, ast.ModifierFlagsDefault)

	if !ShouldVisit(b.PreHandler(), isExport) {
		return nil
	}

	// PreHandle阶段：创建Blueprint和基础对象，立即设置导出
	interfaceBlueprint := b.CreateBlueprint(interfaceName)
	interfaceBlueprint.SetKind(ssa.BlueprintInterface)

	// 创建接口对象实例
	interfaceObj := b.EmitMakeWithoutType(nil, nil)
	interfaceObj.SetType(interfaceBlueprint)

	// 将接口注册为变量
	variable := b.CreateJSVariable(interfaceName)
	b.AssignVariable(variable, interfaceObj)

	// 处理导出 - 在PreHandle阶段就设置导出值
	if isExport {
		if !isDefault {
			b.namedTypeExports[interfaceName] = interfaceName
			b.namedValueExports[interfaceName] = interfaceName
		} else {
			b.namedTypeExports["default"] = interfaceName
			b.namedValueExports["default"] = interfaceName
		}
	}

	// 添加LazyBuilder来处理接口成员和继承
	store := b.StoreFunctionBuilder()
	interfaceBlueprint.AddLazyBuilder(func() {
		switchHandler := b.SwitchFunctionBuilder(store)
		defer switchHandler()

		// 处理继承的接口
		if node.HeritageClauses != nil && len(node.HeritageClauses.Nodes) > 0 {
			b.VisitInterfaceHeritageClauses(node.HeritageClauses, interfaceBlueprint)
		}

		// 处理接口成员
		if node.Members != nil && len(node.Members.Nodes) > 0 {
			b.VisitInterfaceMembers(node.Members, interfaceBlueprint)
		}
	})

	return interfaceObj
}

// VisitInterfaceHeritageClauses 访问接口继承子句
func (b *builder) VisitInterfaceHeritageClauses(heritageClauses *ast.NodeList, interfaceBlueprint *ssa.Blueprint) {
	if heritageClauses == nil || len(heritageClauses.Nodes) == 0 || b.IsStop() {
		return
	}

	for _, heritageNode := range heritageClauses.Nodes {
		if heritageNode == nil {
			continue
		}

		if ast.IsHeritageClause(heritageNode) {
			b.VisitInterfaceHeritageClause(heritageNode.AsHeritageClause(), interfaceBlueprint)
		}
	}
}

// VisitInterfaceHeritageClause 访问单个接口继承子句
func (b *builder) VisitInterfaceHeritageClause(heritageClause *ast.HeritageClause, interfaceBlueprint *ssa.Blueprint) {
	if heritageClause == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &heritageClause.Loc, "")
	defer recoverRange()

	// 处理继承的接口类型
	if heritageClause.Types != nil && len(heritageClause.Types.Nodes) > 0 {
		for _, typeNode := range heritageClause.Types.Nodes {
			if typeNode == nil {
				continue
			}

			if ast.IsExpressionWithTypeArguments(typeNode) {
				exprWithTypeArgs := typeNode.AsExpressionWithTypeArguments()
				if ast.IsIdentifier(exprWithTypeArgs.Expression) {
					parentInterfaceName := exprWithTypeArgs.Expression.AsIdentifier().Text

					// 获取或创建父接口Blueprint
					parentBlueprint := b.GetBluePrint(parentInterfaceName)
					if parentBlueprint == nil {
						parentBlueprint = b.CreateBlueprint(parentInterfaceName)
					}
					parentBlueprint.SetKind(ssa.BlueprintInterface)

					// 添加父接口到当前接口
					interfaceBlueprint.AddParentBlueprint(parentBlueprint)
				}
			}
		}
	}
}

// VisitInterfaceMembers 访问接口成员列表
func (b *builder) VisitInterfaceMembers(members *ast.NodeList, interfaceBlueprint *ssa.Blueprint) {
	if members == nil || len(members.Nodes) == 0 || b.IsStop() {
		return
	}

	for _, memberNode := range members.Nodes {
		if memberNode == nil {
			continue
		}

		if ast.IsTypeElement(memberNode) {
			b.VisitInterfaceMember(memberNode, interfaceBlueprint)
		}
	}
}

// VisitInterfaceMember 访问单个接口成员
func (b *builder) VisitInterfaceMember(member *ast.TypeElement, interfaceBlueprint *ssa.Blueprint) {
	if member == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &member.Loc, "")
	defer recoverRange()

	switch member.Kind {
	case ast.KindPropertySignature:
		b.VisitInterfacePropertySignature(member.AsPropertySignatureDeclaration(), interfaceBlueprint)
	case ast.KindMethodSignature:
		b.VisitInterfaceMethodSignature(member.AsMethodSignatureDeclaration(), interfaceBlueprint)
	case ast.KindCallSignature:
		b.VisitInterfaceCallSignature(member.AsCallSignatureDeclaration(), interfaceBlueprint)
	case ast.KindConstructSignature:
		b.VisitInterfaceConstructSignature(member.AsConstructSignatureDeclaration(), interfaceBlueprint)
	case ast.KindIndexSignature:
		b.VisitInterfaceIndexSignature(member.AsIndexSignatureDeclaration(), interfaceBlueprint)
	default:
		// 其他类型的接口成员暂时跳过
		return
	}
}

// VisitInterfacePropertySignature 访问接口属性签名
func (b *builder) VisitInterfacePropertySignature(propSig *ast.PropertySignatureDeclaration, interfaceBlueprint *ssa.Blueprint) {
	if propSig == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &propSig.Loc, "")
	defer recoverRange()

	// 获取属性名称
	propName := b.ProcessPropertyName(propSig.Name())
	if propName == "" {
		return
	}

	// 获取属性类型
	var propType ssa.Type
	if propSig.Type != nil {
		propType = b.VisitTypeNode(propSig.Type)
	} else {
		propType = ssa.CreateAnyType()
	}

	// 创建属性值（接口中的属性通常是undefined，因为接口不包含实现）
	propValue := b.EmitUndefined(propName)
	propValue.SetType(propType)

	// 注册为接口的静态成员
	interfaceBlueprint.RegisterStaticMember(propName, propValue)
}

// VisitInterfaceMethodSignature 访问接口方法签名
func (b *builder) VisitInterfaceMethodSignature(methodSig *ast.MethodSignatureDeclaration, interfaceBlueprint *ssa.Blueprint) {
	if methodSig == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &methodSig.Loc, "")
	defer recoverRange()

	// 获取方法名称
	methodName := b.ProcessPropertyName(methodSig.Name())
	if methodName == "" {
		return
	}

	// 处理参数类型
	var paramTypes []ssa.Type
	if methodSig.Parameters != nil {
		for _, param := range methodSig.Parameters.Nodes {
			if param == nil {
				continue
			}
			paramType := b.VisitTypeNode(param)
			paramTypes = append(paramTypes, paramType)
		}
	}

	// 处理返回类型
	var returnType ssa.Type
	if methodSig.Type != nil {
		returnType = b.VisitTypeNode(methodSig.Type)
	} else {
		returnType = ssa.CreateAnyType()
	}

	// 创建方法类型
	methodType := ssa.NewFunctionType(methodName, paramTypes, returnType, false)

	// 创建方法值
	methodValue := b.EmitUndefined(methodName)
	methodValue.SetType(methodType)

	// 注册为接口的静态成员
	interfaceBlueprint.RegisterStaticMember(methodName, methodValue)
}

// VisitInterfaceCallSignature 访问接口调用签名
func (b *builder) VisitInterfaceCallSignature(callSig *ast.CallSignatureDeclaration, interfaceBlueprint *ssa.Blueprint) {
	if callSig == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &callSig.Loc, "")
	defer recoverRange()

	// 处理参数类型
	var paramTypes []ssa.Type
	if callSig.Parameters != nil {
		for _, param := range callSig.Parameters.Nodes {
			if param == nil {
				continue
			}
			paramType := b.VisitTypeNode(param)
			paramTypes = append(paramTypes, paramType)
		}
	}

	// 处理返回类型
	var returnType ssa.Type
	if callSig.Type != nil {
		returnType = b.VisitTypeNode(callSig.Type)
	} else {
		returnType = ssa.CreateAnyType()
	}

	// 创建调用签名类型
	callSignatureType := ssa.NewFunctionType("", paramTypes, returnType, false)

	// 创建调用签名值
	callSignatureValue := b.EmitUndefined("")
	callSignatureValue.SetType(callSignatureType)

	// 注册为接口的静态成员（使用特殊名称表示调用签名）
	interfaceBlueprint.RegisterStaticMember("__call__", callSignatureValue)
}

// VisitInterfaceConstructSignature 访问接口构造签名
func (b *builder) VisitInterfaceConstructSignature(constructSig *ast.ConstructSignatureDeclaration, interfaceBlueprint *ssa.Blueprint) {
	if constructSig == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &constructSig.Loc, "")
	defer recoverRange()

	// 处理参数类型
	var paramTypes []ssa.Type
	if constructSig.Parameters != nil {
		for _, param := range constructSig.Parameters.Nodes {
			if param == nil {
				continue
			}
			paramType := b.VisitTypeNode(param)
			paramTypes = append(paramTypes, paramType)
		}
	}

	// 处理返回类型
	var returnType ssa.Type
	if constructSig.Type != nil {
		returnType = b.VisitTypeNode(constructSig.Type)
	} else {
		returnType = ssa.CreateAnyType()
	}

	// 创建构造签名类型
	constructSignatureType := ssa.NewFunctionType("", paramTypes, returnType, false)

	// 创建构造签名值
	constructSignatureValue := b.EmitUndefined("")
	constructSignatureValue.SetType(constructSignatureType)

	// 注册为接口的静态成员（使用特殊名称表示构造签名）
	interfaceBlueprint.RegisterStaticMember("__construct__", constructSignatureValue)
}

// VisitInterfaceIndexSignature 访问接口索引签名
func (b *builder) VisitInterfaceIndexSignature(indexSig *ast.IndexSignatureDeclaration, interfaceBlueprint *ssa.Blueprint) {
	if indexSig == nil || b.IsStop() {
		return
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &indexSig.Loc, "")
	defer recoverRange()

	// 处理索引参数类型
	var indexParamType ssa.Type
	if indexSig.Parameters != nil && len(indexSig.Parameters.Nodes) > 0 {
		indexParamType = b.VisitTypeNode(indexSig.Parameters.Nodes[0])
	} else {
		indexParamType = ssa.CreateAnyType()
	}

	// 处理返回值类型
	var returnType ssa.Type
	if indexSig.Type != nil {
		returnType = b.VisitTypeNode(indexSig.Type)
	} else {
		returnType = ssa.CreateAnyType()
	}

	// 创建索引签名类型
	indexSignatureType := ssa.NewFunctionType("", []ssa.Type{indexParamType}, returnType, false)

	// 创建索引签名值
	indexSignatureValue := b.EmitUndefined("")
	indexSignatureValue.SetType(indexSignatureType)

	// 注册为接口的静态成员（使用特殊名称表示索引签名）
	interfaceBlueprint.RegisterStaticMember("__index__", indexSignatureValue)
}

// VisitQualifiedName 访问QualifiedName
func (b *builder) VisitQualifiedName(name *ast.QualifiedName) string {
	if name == nil || b.IsStop() {
		return ""
	}

	recoverRange := b.GetRecoverRange(b.sourceFile, &name.Loc, "")
	defer recoverRange()

	var leftName, rightName string
	rightName = name.Right.Text()
	switch name.Left.Kind {
	case ast.KindQualifiedName:
		leftName = b.VisitQualifiedName(name.Left.AsQualifiedName())
	default:
		leftName = name.Left.Text()
	}
	return leftName + rightName
}

// HandlePromiseMethod 处理 Promise 的 then, catch, finally 方法
// 这些方法的特点是接收一个回调函数，并将 Promise 的值作为参数传递给回调
func (b *builder) HandlePromiseMethod(promiseValue ssa.Value, methodName string, args []ssa.Value) ssa.Value {
	if b.IsStop() {
		return nil
	}

	// 获取 Promise 对象（调用 getData() 的返回值）
	if promiseValue == nil {
		b.NewError(ssa.Error, TAG, "Promise object is nil")
		return b.EmitUndefined("")
	}

	// 检查是否有回调参数
	if len(args) == 0 {
		b.NewError(ssa.Error, TAG, "Promise method requires a callback function")
		return promiseValue
	}

	callback := args[0]
	if callback == nil {
		return promiseValue
	}

	// 根据不同的方法名处理
	switch methodName {
	case "then":
		// then 方法：将 Promise 的解析值传递给回调函数
		return b.HandlePromiseThen(promiseValue, callback)
	case "catch":
		// catch 方法：将 Promise 的拒绝原因传递给回调函数
		return b.HandlePromiseCatch(promiseValue, callback)
	case "finally":
		// finally 方法：无论成功或失败都调用回调函数
		return b.HandlePromiseFinally(promiseValue, callback)
	default:
		return promiseValue
	}
}

// HandlePromiseThen 处理 Promise.then() 方法
//
// 处理流程：
// 1. 从 promiseValue (Promise<T>) 中提取 T 类型
// 2. 创建一个类型为 T 的值（代表 Promise 解析后的结果）
// 3. 将这个值作为参数传递给回调函数
// 4. 将回调的返回值包装成新的 Promise 并返回（支持链式调用）
//
// 例如：getData().then(data => processData(data))
// - getData() 返回 Promise<string>
// - 提取出 string 类型
// - 创建一个 string 类型的值传给回调参数 data
// - 回调返回结果，包装成新的 Promise
func (b *builder) HandlePromiseThen(promiseValue ssa.Value, callback ssa.Value) ssa.Value {
	if b.IsStop() {
		return nil
	}

	// 从 Promise<T> 中提取 T 类型，并创建一个具有该类型的值
	// 这个值代表 Promise 完成后传递给回调的实际数据
	resolvedValue := b.ExtractPromiseResolvedValue(promiseValue)

	// 调用回调函数，将解析的值作为参数
	// 注意：这里我们在编译时模拟回调的调用，实际运行时这是异步的
	callResult := b.EmitCall(b.NewCall(callback, []ssa.Value{resolvedValue}))

	// then 方法返回一个新的 Promise
	// 如果回调返回 Promise，则返回该 Promise
	// 否则返回一个已解析的 Promise，值为回调的返回值
	if callResult != nil && b.IsPromiseType(callResult.GetType()) {
		// 回调本身返回 Promise，直接返回
		return callResult
	}

	// 回调返回普通值，包装成 Promise
	return b.WrapInPromise(callResult)
}

// HandlePromiseCatch 处理 Promise.catch() 方法
func (b *builder) HandlePromiseCatch(promiseValue ssa.Value, callback ssa.Value) ssa.Value {
	if b.IsStop() {
		return nil
	}

	// catch 接收错误对象
	errorValue := b.EmitUndefined("error")

	// 调用回调函数，将错误作为参数
	callResult := b.EmitCall(b.NewCall(callback, []ssa.Value{errorValue}))

	// catch 方法也返回一个 Promise
	return b.WrapInPromise(callResult)
}

// HandlePromiseFinally 处理 Promise.finally() 方法
func (b *builder) HandlePromiseFinally(promiseValue ssa.Value, callback ssa.Value) ssa.Value {
	if b.IsStop() {
		return nil
	}

	// finally 不接收任何参数
	b.EmitCall(b.NewCall(callback, []ssa.Value{}))

	// finally 返回原来的 Promise
	return promiseValue
}

// ExtractPromiseResolvedType 从 Promise<T> 类型中提取 T 类型
func (b *builder) ExtractPromiseResolvedType(promiseType ssa.Type) ssa.Type {
	if promiseType == nil {
		return ssa.CreateAnyType()
	}

	// 尝试从类型名称中提取泛型参数
	typeNames := promiseType.GetFullTypeNames()
	for _, typeName := range typeNames {
		// 如果类型名称是 "Promise<SomeType>"，提取 SomeType
		if len(typeName) > 8 && typeName[:8] == "Promise<" {
			innerTypeName := typeName[8 : len(typeName)-1]
			// 根据内部类型名创建相应的类型
			return b.CreateTypeByName(innerTypeName)
		}
	}

	// 如果无法提取类型，返回 any 类型
	return ssa.CreateAnyType()
}

// ExtractPromiseResolvedValue 从 Promise 值中提取解析后的值
// 这个值代表 Promise 完成后的结果
func (b *builder) ExtractPromiseResolvedValue(promiseValue ssa.Value) ssa.Value {
	if promiseValue == nil {
		return b.EmitUndefined("")
	}

	// 获取 Promise 的类型
	promiseType := promiseValue.GetType()
	if promiseType == nil {
		return b.EmitUndefined("")
	}

	// 从 Promise<T> 类型中提取 T 类型
	resolvedType := b.ExtractPromiseResolvedType(promiseType)

	// 创建一个具有解析类型的值（代表 Promise 的解析结果）
	resolvedValue := promiseValue
	resolvedValue.SetType(resolvedType)

	return resolvedValue
}

// WrapInPromise 将一个值包装成 Promise 类型
func (b *builder) WrapInPromise(value ssa.Value) ssa.Value {
	if value == nil {
		value = b.EmitUndefined("")
	}

	// 创建 Promise 类型
	promiseType := ssa.NewObjectType()
	valueType := value.GetType()
	if valueType != nil {
		promiseType.AddFullTypeName("Promise<" + valueType.String() + ">")
	} else {
		promiseType.AddFullTypeName("Promise<any>")
	}

	// 创建 Promise 值
	promiseValue := b.EmitUndefined("")
	promiseValue.SetType(promiseType)

	return promiseValue
}

// CreateTypeByName 根据类型名称字符串创建类型
// 支持基本类型、泛型类型（如 Promise<string>、Array<number>）等
func (b *builder) CreateTypeByName(typeName string) ssa.Type {
	// 处理基本类型
	switch typeName {
	case "string":
		return ssa.CreateStringType()
	case "number", "bigint":
		return ssa.CreateNumberType()
	case "boolean":
		return ssa.CreateBooleanType()
	case "void", "undefined":
		return ssa.CreateUndefinedType()
	case "null":
		return ssa.CreateNullType()
	case "any", "unknown":
		return ssa.CreateAnyType()
	case "never":
		// TypeScript 的 never 类型，表示永远不会发生的值
		return ssa.CreateAnyType()
	case "object":
		return ssa.NewObjectType()
	}

	// 处理泛型类型（如 Promise<string>、Array<number>）
	if len(typeName) > 0 {
		// 查找泛型参数的起始位置
		genericStart := -1
		for i, ch := range typeName {
			if ch == '<' {
				genericStart = i
				break
			}
		}

		if genericStart > 0 && typeName[len(typeName)-1] == '>' {
			// 提取基础类型名和泛型参数
			baseTypeName := typeName[:genericStart]
			genericParams := typeName[genericStart+1 : len(typeName)-1]

			// 递归处理泛型参数
			innerType := b.CreateTypeByName(genericParams)

			// 根据基础类型名创建相应的类型
			switch baseTypeName {
			case "Array":
				// Array<T> -> SliceType
				sliceType := ssa.NewSliceType(innerType)
				sliceType.AddFullTypeName(typeName)
				return sliceType
			case "Promise":
				// Promise<T> -> ObjectType with full type name
				promiseType := ssa.NewObjectType()
				promiseType.AddFullTypeName(typeName)
				return promiseType
			case "Map":
				// Map<K, V> -> MapType (简化处理，只使用值类型)
				mapType := ssa.NewMapType(ssa.CreateStringType(), innerType)
				mapType.AddFullTypeName(typeName)
				return mapType
			case "Set":
				// Set<T> -> SliceType (简化为数组类型)
				sliceType := ssa.NewSliceType(innerType)
				sliceType.AddFullTypeName(typeName)
				return sliceType
			default:
				// 其他泛型类型，作为对象类型处理
				objType := ssa.NewObjectType()
				objType.AddFullTypeName(typeName)
				return objType
			}
		}

		// 处理数组简写形式 (如 string[], number[])
		if len(typeName) > 2 && typeName[len(typeName)-2:] == "[]" {
			elementTypeName := typeName[:len(typeName)-2]
			elementType := b.CreateTypeByName(elementTypeName)
			sliceType := ssa.NewSliceType(elementType)
			sliceType.AddFullTypeName(typeName)
			return sliceType
		}
	}

	// 尝试查找 Blueprint（类、接口、枚举等）
	if blueprint := b.GetBluePrint(typeName); blueprint != nil {
		return blueprint
	}

	// 处理常见的内置对象类型
	switch typeName {
	case "Date", "RegExp", "Error", "Function", "Symbol":
		objType := ssa.NewObjectType()
		objType.AddFullTypeName(typeName)
		return objType
	}

	// 默认：创建对象类型并设置类型名
	objType := ssa.NewObjectType()
	objType.AddFullTypeName(typeName)
	return objType
}

// IsPromiseType 检查给定类型是否是 Promise 类型
func (b *builder) IsPromiseType(typ ssa.Type) bool {
	if typ == nil {
		return false
	}

	// 检查类型的完整类型名称
	typeNames := typ.GetFullTypeNames()
	for _, typeName := range typeNames {
		// 检查是否以 "Promise" 开头
		// 支持 "Promise<T>" 和 "Promise" 形式
		if len(typeName) >= 7 {
			if typeName == "Promise" || (len(typeName) > 8 && typeName[:8] == "Promise<") {
				return true
			}
		}
	}

	return false
}
