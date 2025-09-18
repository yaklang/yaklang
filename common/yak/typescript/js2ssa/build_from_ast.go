package js2ssa

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/ast"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/core"
	"github.com/yaklang/yaklang/common/yak/typescript/frontend/scanner"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

func (b *builder) GetRecoverRange(sourcefile *ast.SourceFile, node *core.TextRange, text string) func() {
	startLine, startCol := scanner.GetLineAndCharacterOfPosition(sourcefile, node.Pos())
	endLine, endCol := scanner.GetLineAndCharacterOfPosition(sourcefile, node.End())
	return b.SetRangeWithCommonTokenLoc(ssa.NewCommonTokenLoc(text, startLine, startCol, endLine, endCol))
}

func (b *builder) VisitSourceFile(sourcefile *ast.SourceFile) interface{} {
	if sourcefile == nil || b.IsStop() {
		return nil
	}

	recoverRange := b.GetRecoverRange(sourcefile, &sourcefile.Loc, sourcefile.Text())
	defer recoverRange()

	// js暂时不需要处理prehandle阶段
	if b.PreHandler() {
		return nil
	}

	if sourcefile.Statements != nil {
		for _, statement := range sourcefile.Statements.Nodes {
			if ast.IsPrologueDirective(statement) && statement.AsExpressionStatement().Expression.Text() == "use strict" {
				b.useStrict = true
				break
			}
		}
		b.VisitStatements(sourcefile.Statements)
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

	if decList := node.DeclarationList; decList != nil {
		b.VisitVariableDeclarationList(decList)
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
	return b.EmitConstInst(codec.Atoi(text))
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
			if utils.IsNil(condition) {
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
				b.VisitVariableDeclarationList(node.Initializer.AsVariableDeclarationList().AsNode())
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
			if utils.IsNil(condition) {
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
		b.EmitReturn([]ssa.Value{returnValue})
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
		lastCase := caseBlock.Clauses.Nodes[len(caseBlock.Clauses.Nodes)-1].AsCaseOrDefaultClause()
		if lastCase.Expression == nil {
			defaultCase = lastCase.AsNode()
			commonCase = caseBlock.Clauses.Nodes[:len(caseBlock.Clauses.Nodes)-1]
		} else {
			commonCase = caseBlock.Clauses.Nodes
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
func (b *builder) VisitVariableDeclarationList(node *ast.VariableDeclarationListNode) interface{} {
	if node == nil || b.IsStop() {
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

		var variable *ssa.Variable
		if isLocal {
			variable = b.CreateLocalVariable(identifier)
		} else {
			variable = b.CreateJSVariable(identifier)
		}

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
			b.ProcessObjectBindingPattern(name.AsBindingPattern(), initValue, isLocal)
		} else if ast.IsArrayBindingPattern(name) {
			// 数组解构: let [x, y] = arr
			b.ProcessArrayBindingPattern(name.AsBindingPattern(), initValue, isLocal)
		}

	default:
		b.NewError(ssa.Error, TAG, UnhandledVariableDeclarationType())
	}

	return nil
}

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

// VisitNamedExports 访问命名导出
func (b *builder) VisitNamedExports(node *ast.NamedExports) interface{} { return nil }

// VisitExportSpecifier 访问导出说明符
func (b *builder) VisitExportSpecifier(node *ast.ExportSpecifier) interface{} { return nil }

// VisitExportAssignment 访问导出赋值
func (b *builder) VisitExportAssignment(node *ast.ExportAssignment) interface{} { return nil }

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

		return nil, b.ReadValue(identifierName)
	case ast.KindPropertyAccessExpression:
		propertyAccessExp := node.AsPropertyAccessExpression()
		obj, propName := b.VisitPropertyAccessExpression(propertyAccessExp)
		var objName string
		if ast.IsIdentifier(propertyAccessExp.Expression) {
			objName = propertyAccessExp.Expression.AsIdentifier().Text
		}

		bp := b.GetBluePrint(objName) // 处理静态方法调用
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

	// 处理callee（被调用函数）
	funcValue := b.VisitRightValueExpression(node.Expression)
	if funcValue == nil {
		b.NewErrorWithPos(ssa.Error, TAG, b.CurrentRange, InvalidFunctionCallee())
		return b.EmitUndefined("")
	}

	// 创建调用
	// TODO: 函数调用导致实参发生改变如何处理?
	return b.EmitCall(b.NewCall(funcValue, args))
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
		object := b.ReadValue(className)
		if !utils.IsNil(object) {
			class.RegisterMagicMethod(ssa.Constructor, object)
		}
	}
	obj.SetType(class)
	args := []ssa.Value{obj}
	if node.Arguments != nil {
		for _, arg := range node.Arguments.Nodes {
			args = append(args, b.VisitRightValueExpression(arg))

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

	// 创建新的函数对象
	newFunc := b.NewFunc(funcName)

	// 切换到新函数的上下文
	b.FunctionBuilder = b.PushFunction(newFunc)

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

	// 完成函数构建
	b.Finish()

	// 恢复原来的函数上下文
	b.FunctionBuilder = b.PopFunction()

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

	// 创建新的函数对象
	newFunc := b.NewFunc(funcName)

	{
		// 切换到新函数的上下文
		b.FunctionBuilder = b.PushFunction(newFunc)

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
	}

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

	// 创建新的函数对象
	newFunc := b.NewFunc(funcName)

	{
		// 切换到新函数的上下文
		b.FunctionBuilder = b.PushFunction(newFunc)

		// 处理函数参数
		if node.Parameters != nil && len(node.Parameters.Nodes) > 0 {
			b.ProcessFunctionParams(node.Parameters)
		}

		// 处理函数体
		if node.Body != nil && node.Body.Kind == ast.KindBlock {
			b.VisitBlock(node.Body.AsBlock())
		} else if node.Body != nil {
			ret := b.VisitRightValueExpression(node.Body)
			b.EmitReturn([]ssa.Value{ret})
		}

		// 完成函数构建
		b.Finish()

		// 恢复原来的函数上下文
		b.FunctionBuilder = b.PopFunction()
	}

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

	return nil
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
func (b *builder) ProcessObjectBindingPattern(pattern *ast.BindingPattern, sourceObj ssa.Value, isLocal bool) {
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
			var variable *ssa.Variable
			if isLocal {
				variable = b.CreateLocalVariable(varName)
			} else {
				variable = b.CreateJSVariable(varName)
			}
			b.AssignVariable(variable, propValue)

		case ast.IsBindingPattern(bindingElement.Name()):
			// 嵌套解构: let { a: { b, c } } = obj 或 let { a: [x, y] } = obj
			nestedPattern := bindingElement.Name()

			// 递归处理
			if ast.IsObjectBindingPattern(nestedPattern) {
				b.ProcessObjectBindingPattern(nestedPattern.AsBindingPattern(), propValue, isLocal)
			} else if ast.IsArrayBindingPattern(nestedPattern) {
				b.ProcessArrayBindingPattern(nestedPattern.AsBindingPattern(), propValue, isLocal)
			}
		}
	}
}

// ProcessArrayBindingPattern 处理数组解构模式
func (b *builder) ProcessArrayBindingPattern(pattern *ast.BindingPattern, sourceArr ssa.Value, isLocal bool) {
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
				b.ProcessObjectBindingPattern(nestedPattern.AsBindingPattern(), elementValue, isLocal)
			} else if ast.IsArrayBindingPattern(nestedPattern) {
				b.ProcessArrayBindingPattern(nestedPattern.AsBindingPattern(), elementValue, isLocal)
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
	// 只有箭头函数的函数体可以是表达式 箭头函数一般不出现在类内部作为方法
	var bodyNode *ast.Block

	// 根据不同类型获取相应信息
	switch member.Kind {
	case ast.KindGetAccessor:
		getAccessor := member.AsGetAccessorDeclaration()
		methodName = b.ProcessPropertyName(getAccessor.Name())
		params = getAccessor.Parameters
		if !ast.IsBlock(getAccessor.Body) {
			return
		}
		bodyNode = getAccessor.Body.AsBlock()
	case ast.KindSetAccessor:
		setAccessor := member.AsSetAccessorDeclaration()
		methodName = b.ProcessPropertyName(setAccessor.Name())
		params = setAccessor.Parameters
		if !ast.IsBlock(setAccessor.Body) {
			return
		}
		bodyNode = setAccessor.Body.AsBlock()
	case ast.KindMethodDeclaration:
		methodDecl := member.AsMethodDeclaration()
		methodName = b.ProcessPropertyName(methodDecl.Name())
		params = methodDecl.Parameters
		if !ast.IsBlock(methodDecl.Body) {
			return
		}
		bodyNode = methodDecl.Body.AsBlock()
	default:
		b.NewError(ssa.Error, TAG, UnexpectedClassMethodType())
		return
	}

	// 共同的处理逻辑
	funcName := fmt.Sprintf("%s_%s_%s", class.Name, methodName, uuid.NewString()[:4])
	newFunc := b.NewFunc(funcName)
	newFunc.SetMethodName(methodName)

	isStatic := ast.HasStaticModifier(member)
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

		b.Finish()
		b.FunctionBuilder = b.PopFunction()
	})
}

func (b *builder) ProcessClassCtor(member *ast.ClassElement, class *ssa.Blueprint) {
	ctor := member.AsConstructorDeclaration()
	ctorName := fmt.Sprintf("%s_%s_%s", class.Name, "Custom-Constructor", uuid.NewString()[:4])
	params := ctor.Parameters

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
