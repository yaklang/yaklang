//go:build !no_language
// +build !no_language

package go2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "GO"

func MultipleAssignFailed(left, right int) string {
	return fmt.Sprintf("multi-assign failed: left value length[%d] != right value length[%d]", left, right)
}

func AssignLeftSideEmpty() string {
	return "assign left side is empty"
}

func AssignRightSideEmpty() string {
	return "assign right side is empty"
}

func UnaryOperatorNotSupport(op string) string {
	return fmt.Sprintf("unary operator not support: %s", op)
}
func BinaryOperatorNotSupport(op string) string {
	return fmt.Sprintf("binary operator not support: %s", op)
}

func ArrowFunctionNeedExpressionOrBlock() string {
	return "BUG: arrow function need expression or block at least"
}

func ExpressionNotVariable(expr string) string {
	return fmt.Sprintf("Expression: %s is not a variable", expr)
}

func UndefineLabelstmt() string {
	return "can not find the label"
}

func UnexpectedBreakStmt() string {
	return "unexpected break stmt"
}

func UnexpectedContinueStmt() string {
	return "unexpected continue stmt"
}

func UnexpectedFallthroughStmt() string {
	return "unexpected fallthrough stmt"
}

func UnexpectedAssertStmt() string {
	return "unexpected assert stmt, this not expression"
}

func SliceCallExpressionTooMuch() string {
	return "slice call expression too much"
}

func SliceCallExpressionIsEmpty() string {
	return "slice call expression is empty"
}

func MakeSliceArgumentTooMuch() string {
	return "make slice expression argument too much!"
}

func NotSetTypeInMakeExpression(typ string) string {
	return fmt.Sprintf("not set type %s in make expression", typ)
}

func MakeUnknownType() string {
	return "make unknown type"
}

func InvalidChanType(typ string) string {
	return fmt.Sprintf("iteration (variable of type %s) permits only one right variable", typ)
}

func MakeArgumentTooMuch(typ string) string {
	return fmt.Sprintf("make %s expression argument too much!", typ)
}

func CannotAssign() string {
	return "cannot assign to const value"
}

func CannotParseString(test string, err string) string {
	return fmt.Sprintf("cannot parse string literal: %s failed: %s", test, err)
}

func NeedTwoExpression() string {
	return "in operator need two expression"
}

func UnhandledBool() string {
	return "unhandled bool literal"
}

func Unreachable() string {
	return "unreachable"
}

func ToDo() string {
	return "todo"
}

func OutofBounds(ml, vl int) string {
	return fmt.Sprintf("index %d is out of bounds (>= %d)", ml, vl)
}

func PackageNotFind(n string) string {
	return fmt.Sprintf("package %s is golang library", n)
}

func StructNotFind(n string) string {
	return fmt.Sprintf("struct %s not find, it may belong to the golang library", n)
}

func ImportNotFind(n string) string {
	return fmt.Sprintf("%s not import", n)
}

func MissInitExpr(name string) string {
	return fmt.Sprintf("miss init expression for %s", name)
}

func NotFunction(name string) string {
	return fmt.Sprintf("value %s is not a function", name)
}

func NotCreateBluePrint(name string) string {
	return fmt.Sprintf("[BUG]struct %v is not create blueprint", name)
}

func NotFindAnonymousFieldObject(a string) string {
	return fmt.Sprintf("[BUG]anonymous object %v not find (The anonymous will be created when its parent is created)", a)
}
