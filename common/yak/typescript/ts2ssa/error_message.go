//go:build !no_language
// +build !no_language

package ts2ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

const TAG ssa.ErrorTag = "JS"

// current incomplete implement

func UnexpectedArithmeticOP() string {
	return "unexpected binary arithmetic operator"
}

func UnexpectedBinaryBitWiseOP() string {
	return "unexpected binary bitwise operator"
}

func UnexpectedComparisonOP() string {
	return "unexpected binary comparison operator"
}

func UnexpectedUnaryOP() string {
	return "unexpected unary operator"
}

// semantic error

func UnexpectedRightValueForObjectPropertyAccess() string {
	return "unexpected right value for object property access"
}

func NoOperandFoundForPrefixUnaryExp() string {
	return "missing operand for prefix unary expression"
}

func NoViableOperandForPrefixUnaryExp() string {
	return "missing viable operand for prefix unary expression"
}

func NoViableOperandForPostfixUnaryExp() string {
	return "missing viable operand for postfix unary expression"
}

func UnexpectedRightValueForElementAccess() string {
	return "unexpected right value for element access"
}

func UnexpectedVariableDeclarationModifierError(name string) string {
	return fmt.Sprintf("unexpected modifier when declare variable: %s", name)
}

func ConstDeclarationWithoutInitializer() string {
	return "const declaration without an initializer"
}
func BindPatternDeclarationWithoutInitializer() string {
	return "binding pattern declaration without an initializer"
}

func NoDeclarationName() string {
	return "no declaration name found"
}

func RestElementRequiresIdentifier() string {
	return "rest element requires identifier"
}

func InvalidPropertyBinding() string {
	return "invalid property binding"
}

func VariableIsNotDefined() string {
	return "variable is not defined"
}

func InvalidFunctionCallee() string {
	return "invalid function callee"
}

func SuperKeywordNotAvailableInCurrentContext() string {
	return "super not available in current context"
}

func ThisKeywordNotAvailableInCurrentContext() string {
	return "this keyword not available in current context"
}

func LabelNameEmptyNotAllowed() string {
	return "label name empty not allowed"
}

func LabelNameDupNotAllowed() string {
	return "label name dup not allowed"
}

func UnexpectedBreakStmt() string {
	return "unexpected break stmt"
}

func UnexpectedContinueStmt() string {
	return "unexpected continue stmt"
}

func UnexpectedPropertyNameType() string {
	return "unexpected property name type"
}

func UnexpectedClassMethodType() string {
	return "unexpected class method type"
}

func FunctionParamNameEmpty() string {
	return "function parameter name empty"
}

// SSA error

func TypeofValueIsNil() string { return "type value is nil" }

func InstanceOfGotNilValue() string { return "instance of got nil value" }

func BinOPWithNilSSAValue() string { return "binary op with nil SSA" }

func NewExpressionOnlySupportIdentifierClassName() string {
	return "new expression only supports identifier class name"
}

func UnhandledStatement() string {
	return "unhandled statement"
}

func UnhandledVariableDeclarationType() string {
	return "unhandled variable declaration type"
}

func UnhandledBinOP() string {
	return "unhandled bin-op"
}

func UnhandledPropertyNameType() string {
	return "unhandled property name type"
}

func UnhandledMemberNameType() string {
	return "unhandled member name type"
}
