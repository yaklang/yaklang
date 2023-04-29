package yakast

type constError string
type CompilerLanguage string

const (
	zh CompilerLanguage = "zh"
	en CompilerLanguage = "en"
)

const (
	compileError                              = "compile error: %v"
	breakError                     constError = "break statement can only be used in for or switch"
	continueError                             = "continue statement can only be used in for"
	fallthroughError                          = "fallthrough statement can only be used in switch"
	sliceCallNoParamError                     = "at least one param for slice call"
	sliceCallTooManyParamError                = "too many params for slice call"
	sliceCallStepMustBeNumberError            = "step must be a number"
	CreateSymbolError                         = "SymbolTable cannot create build-in symbol[%s]"
	assertExpressionError                     = "assert statement second argument expect expression"
	bitBinaryError                            = "BUG: unimplemented bit binary operator: %s"
	multiplicativeBinaryError                 = "BUG: unimplemented multiplicative binary operator: %s"
	expressionError                           = "BUG: cannot parse `%s` as expression"
	includeUnquoteError                       = "include path[%s] unquote error: %v"
	includePathNotFoundError                  = "include path[%s] not found"
	readFileError                             = "read file[%s] read error: %v"
	stringLiteralError                        = "invalid string literal: %s"
	notImplemented                            = "[%s] not implemented"
	forceCreateSymbolFailed                   = "BUG: cannot force create symbol for `%s`"
	autoCreateSymbolFailed                    = "BUG: cannot auto create symbol for `%s`"
	integerIsTooLarge                         = "cannot parse `%s` as integer literal... is too large for int64"
	contParseNumber                           = "cannot parse num for literal: %s"
	notFoundDollarVariable                    = "undefined dollor variable: $%v"
	bugMembercall                             = "BUG: no identifier or $identifier to call via member"
	notFoundVariable                          = "(strict mode) undefined variable: %v"
	syntaxUnrecoverableError                  = "grammar parser error: cannot continue to parse syntax (unrecoverable), maybe there are unbalanced brace"
)

var i18n = map[CompilerLanguage]map[constError]string{
	zh: {
		compileError:               "编译错误: %v",
		breakError:                 "break 语句只能在 for 或 switch 中使用",
		continueError:              "continue 语句只能在 for 中使用",
		fallthroughError:           "fallthrough 语句只能在 switch 中使用",
		sliceCallNoParamError:      "切片操作至少需要一个参数",
		sliceCallTooManyParamError: "切片操作参数过多",
		assertExpressionError:      "assert 语句第二个参数必须是表达式",
		bitBinaryError:             "BUG: 未实现的二元位运算符: %s",
		multiplicativeBinaryError:  "BUG: 未实现的二元运算符: %s",
		expressionError:            "BUG: 无法将 `%s` 解析为表达式",
		includeUnquoteError:        "包含路径[%s] 解析错误: %v",
		includePathNotFoundError:   "包含路径[%s] 不存在",
		readFileError:              "读取文件[%s] 错误: %v",
		stringLiteralError:         "非法的字符串字面量: %s",
		notImplemented:             "[%s] 未实现",
		forceCreateSymbolFailed:    "BUG: 无法强制创建符号 `%s`",
		autoCreateSymbolFailed:     "BUG: 无法自动创建符号 `%s`",
		integerIsTooLarge:          "无法解析 `%s` 为整数, 因为对于int64来说太大了",
		contParseNumber:            "无法解析数字字面量: %s",
		notFoundDollarVariable:     "未定义的 $ 变量: $%v",
		bugMembercall:              "BUG: 没有通过成员调用的标识符或$标识符",
		notFoundVariable:           "(严格模式) 未定义变量: %v",
		syntaxUnrecoverableError:   "语法解析器错误：此处语法错误导致解析器中断(无法继续)，可能是括号不平衡所致",
	},
}

func (y *YakCompiler) GetConstError(e constError) string {
	if y.language == en {
		return string(e)
	}
	if constsInfo, ok := i18n[y.language]; ok {
		if msg, ok := constsInfo[e]; ok {
			return msg
		} else {
			return string(e)
		}
	} else {
		panic("not support language")
	}
	return ""
}
