package luaast

type constError string
type CompilerLanguage string

const (
	zh CompilerLanguage = "zh"
	en CompilerLanguage = "en"
)

const (
	breakError                     constError = "break statement can only be used in for or switch"
	continueError                             = "continue statement can only be used in for"
	fallthroughError                          = "fallthrough statement can only be used in switch"
	sliceCallNoParamError                     = "at least one param for slice call"
	sliceCallTooManyParamError                = "too many params for slice call"
	sliceCallStepMustBeNumberError            = "step must be a number"
	CreateSymbolError                         = "SymbolTable cannot create build-in symbol[%s]"
	assertExpressionError                     = "assert statement second argument expect expression"
	notImplemented                            = "[%s] not implemented"
	forceCreateSymbolFailed                   = "BUG: cannot force create symbol for `%s`"
	autoCreateSymbolFailed                    = "BUG: cannot auto create symbol for `%s`"
	autoCreateLabelFailed                     = "BUG: cannot auto create label for `%s`"
	labelAlreadyDefined                       = "label '%s' already defined"
	labelNotDefined                           = "no visible label '%s' for <goto>"
	integerIsTooLarge                         = "cannot parse `%s` as integer literal... is too large for int64"
	contParseNumber                           = "cannot parse num for literal: %s"
)

var i18n = map[CompilerLanguage]map[constError]string{
	zh: {
		breakError:                 "break 语句只能在 for 或 switch 中使用",
		continueError:              "continue 语句只能在 for 中使用",
		fallthroughError:           "fallthrough 语句只能在 switch 中使用",
		sliceCallNoParamError:      "切片操作至少需要一个参数",
		sliceCallTooManyParamError: "切片操作参数过多",
		assertExpressionError:      "assert 语句第二个参数必须是表达式",
		notImplemented:             "[%s] 未实现",
		forceCreateSymbolFailed:    "BUG: 无法强制创建符号 `%s`",
		autoCreateSymbolFailed:     "BUG: 无法自动创建符号 `%s`",
		integerIsTooLarge:          "无法解析 `%s` 为整数, 因为对于int64来说太大了",
		contParseNumber:            "无法解析数字字面量: %s",
	},
}

func (l *LuaTranslator) GetConstError(e constError) string {
	if l.language == en {
		return string(e)
	}
	if constsInfo, ok := i18n[l.language]; ok {
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
