package loop_yaklangcode

import (
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

// compilerErrorHint describes an AI assistant hint for a Yak compiler / SSA error.
type compilerErrorHint struct {
	Name        string
	Contains    []string // all must match (after normalization)
	AnyOf       []string // at least one must match
	Globs       []string // glob match on normalized message
	LineGlobs   []string // glob match on source line (optional)
	LineRegexps []string
	Hint        string
	Examples    []string // [wrong, right]
	// Enrich, when set, runs before static Hint (e.g. YakDocument attachment).
	Enrich func(normalizedMessage string) string
}

func extractCoreCompilerMessage(full string) string {
	msg := strings.TrimSpace(full)
	msg = strings.TrimPrefix(msg, "[Error]: ")
	msg = strings.TrimPrefix(msg, "基础语法错误（Syntax Error）：")
	if idx := strings.Index(msg, " in ["); idx > 0 {
		msg = msg[:idx]
	}
	if idx := strings.Index(msg, " from SSA:"); idx > 0 {
		msg = msg[:idx]
	}
	if idx := strings.Index(msg, " from compiler"); idx > 0 {
		msg = msg[:idx]
	}
	return strings.TrimSpace(msg)
}

func lookupCompilerErrorHint(normalizedMessage, lineContent string) string {
	for _, pattern := range compilerErrorHints {
		if pattern.Enrich != nil {
			if enriched := pattern.Enrich(normalizedMessage); enriched != "" {
				return enriched
			}
		}
	}
	for _, pattern := range compilerErrorHints {
		if !matchesCompilerErrorHint(pattern, normalizedMessage, lineContent) {
			continue
		}
		if pattern.Hint != "" {
			return formatCompilerErrorHint(pattern)
		}
	}
	return lookupCompilerErrorFallback(normalizedMessage)
}

func matchesCompilerErrorHint(pattern compilerErrorHint, normalizedMessage, lineContent string) bool {
	for _, sub := range pattern.Contains {
		if !strings.Contains(normalizedMessage, sub) {
			return false
		}
	}
	if len(pattern.AnyOf) > 0 {
		matched := false
		for _, sub := range pattern.AnyOf {
			if strings.Contains(normalizedMessage, sub) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(pattern.Globs) > 0 {
		matched := false
		for _, glob := range pattern.Globs {
			if safeGlobMatch(normalizedMessage, glob) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(pattern.LineGlobs) > 0 {
		matched := false
		for _, glob := range pattern.LineGlobs {
			if safeGlobMatch(lineContent, glob) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if len(pattern.LineRegexps) > 0 {
		if !utils.MatchAnyOfRegexp(lineContent, pattern.LineRegexps...) {
			return false
		}
	}
	hasMessageMatcher := len(pattern.Contains) > 0 || len(pattern.AnyOf) > 0 || len(pattern.Globs) > 0
	hasLineMatcher := len(pattern.LineGlobs) > 0 || len(pattern.LineRegexps) > 0
	if !hasMessageMatcher && !hasLineMatcher {
		return false
	}
	if hasLineMatcher && lineContent == "" {
		return false
	}
	return true
}

func formatCompilerErrorHint(pattern compilerErrorHint) string {
	hint := pattern.Hint
	if len(pattern.Examples) >= 2 {
		hint += "\n错误: " + pattern.Examples[0]
		hint += "\n正确: " + pattern.Examples[1]
	}
	return hint
}

func lookupCompilerErrorFallback(normalizedMessage string) string {
	switch {
	case strings.HasPrefix(normalizedMessage, "基础语法错误"):
		return "Yaklang 基础语法解析失败。检查括号/花括号是否匹配、是否误用了 Go/JavaScript 语法，必要时用 grep_yaklang_samples 搜索相似写法。"
	case strings.Contains(normalizedMessage, "no viable alternative"):
		return "语法解析失败（no viable alternative）。常见原因：Go 风格类型声明、import/package、泛型或不被 Yaklang 支持的语法。请对照 Yaklang DSL 改写。"
	case strings.Contains(normalizedMessage, "mismatched input"):
		return "语法 token 不匹配（mismatched input）。检查是否缺少括号、逗号、运算符，或混入了 Go/Java 语法。"
	case strings.Contains(normalizedMessage, "extraneous input"):
		return "存在多余 token（extraneous input）。删除多余符号，或检查语句是否写完整。"
	case strings.Contains(normalizedMessage, "expecting"):
		return "语法不完整（expecting ...）。补全缺失的括号、分号或语句结束符。"
	default:
		return "编译器/静态分析报错。请根据上方错误信息定位行号，修正后再 modify_code；API 问题用 yakdoc_*，语法样例用 grep_yaklang_samples。"
	}
}

// compilerErrorHints is ordered: more specific patterns first.
var compilerErrorHints = []compilerErrorHint{
	{
		Name:     "ExternLibMissingMember",
		AnyOf:    []string{"ExternLib", "ExternType"},
		Contains: []string{"don't has"},
		Enrich:   EnrichExternFieldError,
		Hint:     "该库或类型不存在此成员。下方应已自动附加相近 API；若无附加信息，用 yakdoc_search 按功能搜索，禁止猜测 API 名。",
	},
	{
		Name:     "ValueUndefined",
		Contains: []string{"Value undefined:"},
		Hint:     "引用了未定义的变量/函数/库。确认名称拼写；标准库 API 用 yakdoc_search / yakdoc_function_details 查询，不要臆造名称。",
	},
	{
		Name:     "InvalidField",
		Contains: []string{"Invalid operation: unable to access the member or index"},
		Hint:     "对当前类型的值访问了不存在的成员或索引。检查变量实际类型（可用 desc()），确认字段名/下标是否正确。",
	},
	{
		Name:  "BindingNotFound",
		AnyOf: []string{"The closure function expects to capture variable", "but it was not found at the calling location", "but it was not found at the call"},
		Hint:  "闭包引用了调用处不可见的变量。将所需变量传入闭包作用域，或改用外层已定义的变量。",
	},
	{
		Name:     "ValueNotMember",
		Contains: []string{"unable to access the member with name or index"},
		Hint:     "成员访问失败。确认对象非 nil、类型正确，且成员名/索引存在。",
	},
	{
		Name:     "ContAssignExtern",
		Contains: []string{"cannot assign to", "this is extern-instance"},
		Hint:     "不能给 extern 实例整体赋值。只修改其字段，或使用库提供的 API。",
	},
	{
		Name:     "NoCheckMustInFirst",
		Contains: []string{"@ssa-nocheck must be the first line"},
		Hint:     "@ssa-nocheck 必须是文件第一行。将其移到文件开头，或删除该指令。",
	},
	{
		Name:     "ValueIsNull",
		Contains: []string{"This value is null"},
		Hint:     "对 nil 值进行了不允许的操作。在使用前检查返回值/对象是否为空。",
	},
	{
		Name:     "FunctionContReturnError",
		Contains: []string{"This function cannot return error"},
		Hint:     "该函数签名不包含 error 返回值。不要对其使用 `~` 丢弃 error，或改用会返回 error 的 API。",
	},
	{
		Name:     "GenericTypeError",
		Contains: []string{"should be", "but got"},
		Hint:     "泛型/类型参数约束不满足。检查传入类型是否与函数/generic 声明一致。",
	},
	{
		Name: "CallAssignmentMismatch",
		AnyOf: []string{
			"The function call returns (",
			"The function call with ~ returns (",
		},
		Contains: []string{"variables on the left side"},
		Hint:     "函数返回值个数与左侧接收变量个数不一致。查 yakdoc 确认返回值数量，或补全/减少左侧变量。",
	},
	{
		Name:     "PhiEdgeLengthMisMatch",
		Contains: []string{"Phi edges length < 2"},
		Hint:     "SSA 控制流异常（Phi 边不足）。通常是语法/控制流写法问题，检查 if/for/switch 分支是否完整。",
	},
	{
		Name:     "ArgumentTypeError",
		Contains: []string{"The No.", "argument (", "cannot use as (", "in call"},
		Hint:     "函数实参类型与形参不匹配。用 yakdoc_function_details 查正确参数类型，必要时做类型转换。",
	},
	{
		Name:     "NotEnoughArgument",
		Contains: []string{"Not enough arguments in call"},
		Hint:     "函数调用参数不足。用 yakdoc_function_details 查完整参数列表并补全。",
	},
	{
		Name:     "FreeValueUndefine",
		Contains: []string{"Can't find definition of this variable"},
		Hint:     "函数体内外都找不到该自由变量定义。在函数内声明变量，或通过参数/闭包传入。",
	},
	{
		Name: "ErrorUnhandled",
		AnyOf: []string{
			"Error Unhandled",
			"The value is (",
			"type, has unhandled error",
		},
		Hint: "函数返回了 error 但未处理。使用 `result, err = ...` 接收 error，或 `~` 显式丢弃（确认安全时）。",
	},
	{
		Name:     "BlockUnreachable",
		Contains: []string{"This block unreachable!"},
		Hint:     "存在不可达代码块。检查 return/break/continue 是否使后续代码永远无法执行。",
	},
	{
		Name:     "ConditionIsConst",
		Contains: []string{"condition is constant"},
		Hint:     "if/switch 条件为编译期常量，分支结果已确定。可简化逻辑或检查是否写错了条件表达式。",
	},
	{
		Name:     "MultipleAssignFailed",
		Contains: []string{"multi-assign failed:"},
		Hint:     "多赋值左右两侧数量不一致。确保 `a, b = f1(), f2()` 左右个数相同。",
	},
	{
		Name:     "AssignLeftSideEmpty",
		Contains: []string{"assign left side is empty"},
		Hint:     "赋值左侧为空。补全要接收值的变量名。",
	},
	{
		Name:     "AssignRightSideEmpty",
		Contains: []string{"assign right side is empty"},
		Hint:     "赋值右侧为空。补全表达式或函数调用。",
	},
	{
		Name:     "UnaryOperatorNotSupport",
		Contains: []string{"unary operator not support:"},
		Hint:     "不支持该一元运算符。查阅 Yaklang 支持的运算符，或改写表达式。",
	},
	{
		Name:     "BinaryOperatorNotSupport",
		Contains: []string{"binary operator not support:"},
		Hint:     "不支持该二元运算符。改用 Yaklang 支持的比较/算术/逻辑运算符。",
	},
	{
		Name:     "ExpressionNotVariable",
		Contains: []string{"Expression:", "is not a variable"},
		Hint:     "此处需要变量名，但给了表达式。改用可赋值的变量标识符。",
	},
	{
		Name:     "UnexpectedBreakStmt",
		Contains: []string{"break statement can only be used in for or switch"},
		Hint:     "break 只能用在 for 或 switch 内。将 break 移入循环/分支，或改用 return。",
	},
	{
		Name:     "UnexpectedContinueStmt",
		Contains: []string{"continue statement can only be used in for"},
		Hint:     "continue 只能用在 for 循环内。",
	},
	{
		Name:     "UnexpectedFallthroughStmt",
		Contains: []string{"fallthrough statement can only be used in switch"},
		Hint:     "fallthrough 只能用在 switch 内。",
	},
	{
		Name:     "UnexpectedAssertStmt",
		Contains: []string{"unexpected assert stmt"},
		Hint:     "assert 在此上下文非法。检查 assert 是否写在表达式位置。",
	},
	{
		Name:     "SliceCallExpressionTooMuch",
		Contains: []string{"slice call expression too much"},
		Hint:     "切片调用参数过多。检查 `obj[key](args)` 形式是否正确。",
	},
	{
		Name:     "SliceCallExpressionIsEmpty",
		Contains: []string{"slice call expression is empty"},
		Hint:     "切片调用缺少参数。补全调用参数。",
	},
	{
		Name:     "SliceCallArgumentTooMuch",
		Contains: []string{"slice call expression argument too much"},
		Hint:     "切片调用实参过多。减少参数或查阅 API 签名。",
	},
	{
		Name:     "MakeArgumentTooMuch",
		Contains: []string{"expression argument too much!"},
		Hint:     "make 参数过多。slice/map/chan 的 make 用法：`make([]T,n)`、`make(map[T]U)` 等。",
	},
	{
		Name:     "NotSetTypeInMakeExpression",
		Contains: []string{"not set type in make expression"},
		Hint:     "make 缺少类型。写成 `make([]int, 0)` 或 `make(map[string]var)` 等形式。",
	},
	{
		Name:     "MakeUnknownType",
		Contains: []string{"make unknown type"},
		Hint:     "make 的类型不受支持。仅支持 slice、map、bytes、chan。",
	},
	{
		Name:     "MakeStructUnsupported",
		Contains: []string{"cannot make struct{}"},
		Hint:     "不能用 make 创建 struct。改用 map 或字面量 {}。",
	},
	{
		Name:     "InvalidChanType",
		Contains: []string{"iteration (variable of type", "permits only one right variable"},
		Hint:     "channel 迭代/for-range 左侧只能有一个变量。改用 `for v = range ch` 形式。",
	},
	{
		Name:   "FieldCallTargetError",
		Globs:  []string{"* call target Error"},
		Hint:   "方法/字段调用目标错误。确认调用对象非 nil 且类型支持该方法。",
	},
	{
		Name:     "CallTargetNil",
		Contains: []string{"call target is nil"},
		Hint:     "对 nil 目标发起调用。先初始化对象或检查条件分支。",
	},
	{
		Name:     "AdditiveBinaryNeedTwo",
		Contains: []string{"additive binary operator need two expression"},
		Hint:     "加减运算符需要两个操作数。补全表达式。",
	},
	{
		Name:     "InOperatorNeedTwo",
		Contains: []string{"in operator need two expression"},
		Hint:     "in 运算符需要两个操作数。",
	},
	{
		Name:     "ArrowFunctionNeedBody",
		Contains: []string{"arrow function need expression or block"},
		Hint:     "箭头函数/闭包缺少函数体。补全 `{ ... }` 或表达式。",
	},
	{
		Name:     "UnhandledBoolLiteral",
		Contains: []string{"Unhandled bool literal"},
		Hint:     "布尔字面量解析失败。使用 true/false。",
	},
	{
		Name:     "UnquoteError",
		Contains: []string{"unquote error"},
		Hint:     "字符串转义/unquote 失败。检查引号与转义字符。",
	},
	{
		Name:     "IntegerLiteralTooLarge",
		Contains: []string{"is to large for int64"},
		Hint:     "整数字面量超出 int64 范围。改用字符串或拆分数值。",
	},
	{
		Name:     "CannotParseNumberLiteral",
		Contains: []string{"cannot parse num for literal:"},
		Hint:     "数字字面量无法解析。检查格式是否合法。",
	},
	{
		Name: "TemplateStringParseError",
		AnyOf: []string{
			"const parse",
			"template string literal",
			"parse template string literal error",
		},
		Hint: "模板字符串解析失败。检查反引号、f-string 插值语法。",
	},
	{
		Name:     "UnhandledMapLiteral",
		Contains: []string{"Unhandled Map(Object) Literal:"},
		Hint:     "map 字面量写法不被支持。改用 `{\"k\": v}` 或逐字段赋值。",
	},
	{
		Name:     "UnhandledSliceLiteral",
		Contains: []string{"Unhandled Slice Literal:"},
		Hint:     "slice 字面量解析失败。Yaklang 推荐 `[1, 2, 3]` 而非 Go 的 `[]T{...}`。",
	},
	{
		Name:     "UnhandledSliceTypedLiteral",
		Contains: []string{"unhandled Slice Typed Literal:"},
		Hint:     "带类型的 slice 字面量不被支持。去掉 Go 风格 `[]T{...}`，改用 `[...]`。",
	},
	{
		Name: "MapLiteralParseError",
		AnyOf: []string{
			"map literal map pairs parse error",
			"map typed literal parse error",
		},
		Hint: "map 字面量键值对解析失败。检查 `{\"key\": value}` 语法。",
	},
	{
		Name:        "FunctionParameterTypes",
		Globs:       []string{"*no viable alternative at input*", "*func(*"},
		LineRegexps: []string{`func\s*\([^)]*\s+(map\[|string|int|interface\{\}|\[\]|\*|chan)`},
		Hint:        "Yaklang DSL 中函数参数不允许有类型声明。请移除参数的类型声明。",
		Examples:    []string{"func(result map[string]interface{})", "func(result)"},
	},
	{
		Name: "VarTypeDeclarations",
		Globs: []string{
			"*no viable alternative*",
			"*extraneous input*",
			"*mismatched input*",
		},
		LineRegexps: []string{
			`var\s+\w+\s+(map\[|\[\]|string|int|interface\{\}|\*|chan)`,
			`\w+\s*:=\s*(map\[|\[\]string|\[\]int)`,
		},
		Hint:     "Yaklang DSL 中变量声明不需要显式类型。请使用简单的赋值语法。",
		Examples: []string{"var result map[string]interface{}", "result := {}"},
	},
	{
		Name:  "IncompleteStructure",
		Globs: []string{"*mismatched input*", "*expecting <EOF>*"},
		Hint:  "语法结构不完整，可能缺少匹配的括号、花括号或分号。请检查代码块的完整性。",
	},
	{
		Name:      "ArraySliceSyntax",
		Globs:     []string{"*no viable alternative*"},
		LineGlobs: []string{"*[]*{*", "*[]string*", "*[]int*"},
		Hint:      "Yaklang DSL 中数组/切片语法可能与 Go 不同。请使用 Yaklang 的数组语法。",
		Examples:  []string{`[]string{"a", "b"}`, `["a", "b"]`},
	},
	{
		Name:      "ImportStatements",
		Globs:     []string{"*no viable alternative*"},
		LineGlobs: []string{"*import*"},
		Hint:      "Yaklang DSL 不需要 import 语句。所有内置库都是自动可用的。请删除 import 语句。",
	},
	{
		Name:      "PackageDeclarations",
		Globs:     []string{"*no viable alternative*"},
		LineGlobs: []string{"*package*"},
		Hint:      "Yaklang DSL 不需要 package 声明。请删除 package 语句，直接编写代码逻辑。",
	},
	{
		Name:        "MethodReceivers",
		Globs:       []string{"*no viable alternative*"},
		LineRegexps: []string{`func\s*\([^)]+\)\s*\w+\s*\(`},
		Hint:        "Yaklang DSL 不支持方法接收者语法。请使用普通函数定义。",
		Examples:    []string{"func (t *Type) Method()", "func Method()"},
	},
	{
		Name:      "GenericSyntax",
		Globs:     []string{"*no viable alternative*"},
		LineGlobs: []string{"*<*>*"},
		Hint:      "Yaklang DSL 不支持泛型语法。请使用具体类型或 interface{}。",
	},
	{
		Name:        "PointerSyntax",
		Globs:       []string{"*no viable alternative*"},
		LineRegexps: []string{`[^"]*\*[^"]*`},
		Hint:        "Yaklang DSL 中指针语法可能不同。请检查是否需要指针，或使用 Yaklang 的引用方式。",
	},
	{
		Name:      "ChannelSyntax",
		Globs:     []string{"*no viable alternative*"},
		LineGlobs: []string{"*chan*"},
		Hint:      "Yaklang DSL 的并发模型可能与 Go 不同。请查阅 Yaklang 的并发语法文档。",
	},
}

// allBuiltinCompilerErrorMessages lists canonical messages from ssa / yak2ssa / ssa4analyze
// for regression testing — every entry must produce a non-empty hint.
var allBuiltinCompilerErrorMessages = []struct {
	name    string
	message string
}{
	{name: "BindingNotFound", message: "The closure function expects to capture variable [x], but it was not found at the calling location [1:1--1:2]."},
	{name: "BindingNotFoundInCall", message: "The closure function expects to capture variable [x], but it was not found at the call"},
	{name: "ValueNotMember", message: "The undefined foo unable to access the member with name or index {bar} at the calling location [1:1--1:2]."},
	{name: "ValueNotMemberInCall", message: "The value foo unable to access the member with name or index {bar} at the call."},
	{name: "ExternLib", message: "ExternLib [poc] don't has [Get], maybe you meant Post ?"},
	{name: "ExternType", message: "ExternType [[]number] don't has [CCCCC], maybe you meant Cap ?"},
	{name: "ContAssignExtern", message: "cannot assign to  cli, this is extern-instance"},
	{name: "NoCheckMustInFirst", message: "@ssa-nocheck must be the first line in the file"},
	{name: "ValueUndefined", message: "Value undefined:foo"},
	{name: "ValueIsNull", message: "This value is null"},
	{name: "FunctionContReturnError", message: "This function cannot return error"},
	{name: "GenericTypeError", message: "T should be string, but got number"},
	{name: "CallAssignmentMismatch", message: "The function call returns (number) type, but 2 variables on the left side. "},
	{name: "CallAssignmentMismatchDropError", message: "The function call with ~ returns (number) type, but 2 variables on the left side. "},
	{name: "PhiEdgeLengthMisMatch", message: "Phi edges length < 2"},
	{name: "InvalidField", message: "Invalid operation: unable to access the member or index of variable of type {number} with name or index {foo}."},
	{name: "MultipleAssignFailed", message: "multi-assign failed: left value length[2] != right value length[3]"},
	{name: "AssignLeftSideEmpty", message: "assign left side is empty"},
	{name: "AssignRightSideEmpty", message: "assign right side is empty"},
	{name: "UnaryOperatorNotSupport", message: "unary operator not support: ++"},
	{name: "BinaryOperatorNotSupport", message: "binary operator not support: >>>"},
	{name: "ArrowFunctionNeedExpressionOrBlock", message: "BUG: arrow function need expression or block at least"},
	{name: "ExpressionNotVariable", message: "Expression: 1+1 is not a variable"},
	{name: "UnexpectedBreakStmt", message: "break statement can only be used in for or switch"},
	{name: "UnexpectedContinueStmt", message: "continue statement can only be used in for"},
	{name: "UnexpectedFallthroughStmt", message: "fallthrough statement can only be used in switch"},
	{name: "UnexpectedAssertStmt", message: "unexpected assert stmt, this not expression"},
	{name: "SliceCallExpressionTooMuch", message: "slice call expression too much"},
	{name: "SliceCallExpressionIsEmpty", message: "slice call expression is empty"},
	{name: "MakeArgumentTooMuch", message: "make slice expression argument too much!"},
	{name: "NotSetTypeInMakeExpression", message: "not set type in make expression"},
	{name: "MakeUnknownType", message: "make unknown type"},
	{name: "FieldCallTargetError", message: "foo call target Error"},
	{name: "InvalidChanType", message: "iteration (variable of type chan number) permits only one right variable"},
	{name: "FreeValueUndefine", message: "Can't find definition of this variable x both inside and outside the function."},
	{name: "ErrorUnhandled", message: "Error Unhandled "},
	{name: "ErrorUnhandledWithType", message: "The value is (string) type, has unhandled error"},
	{name: "ArgumentTypeError", message: "The No.1 argument (string), cannot use as (bytes) in call f1"},
	{name: "NotEnoughArgument", message: "Not enough arguments in call foo have (1) want (2, string)"},
	{name: "BlockUnreachable", message: "This block unreachable!"},
	{name: "ConditionIsConst", message: "The if condition is constant"},
	{name: "SliceCallArgumentTooMuch", message: "slice call expression argument too much"},
	{name: "MakeStructUnsupported", message: "cannot make struct{}; type must be slice, map, bytes, or channel"},
	{name: "CallTargetNil", message: "call target is nil"},
	{name: "AdditiveBinaryNeedTwo", message: "additive binary operator need two expression"},
	{name: "InOperatorNeedTwo", message: "in operator need two expression"},
}
