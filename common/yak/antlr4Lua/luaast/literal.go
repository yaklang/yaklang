package luaast

import (
	"math"
	"strconv"
	"strings"
	lua "github.com/yaklang/yaklang/common/yak/antlr4Lua/parser"
)

func (l *LuaTranslator) VisitNumber(raw lua.INumberContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.NumberContext)
	if i == nil {
		return nil
	}

	if s := i.INT(); s != nil {
		var originIntStr = s.GetText()
		var intStr = strings.ToLower(originIntStr)
		var resultInt64 int64
		resultInt64, _ = strconv.ParseInt(intStr, 10, 64)
		// todo 添加Compiler panic
		//if err != nil {
		//	l.panicCompilerError(integerIsTooLarge, originIntStr)
		//}
		if resultInt64 > math.MaxInt {
			l.pushInt64(resultInt64, originIntStr)
		} else {
			l.pushInteger(int(resultInt64), originIntStr)
		}
		return nil
	}

	if s := i.HEX(); s != nil {
		var originIntStr = s.GetText()
		var intStr = strings.ToLower(originIntStr)
		var resultInt64 int64
		resultInt64, _ = strconv.ParseInt(intStr[2:], 16, 64)
		// todo 添加Compiler panic
		//if err != nil {
		//	l.panicCompilerError(integerIsTooLarge, originIntStr)
		//}
		if resultInt64 > math.MaxInt {
			l.pushInt64(resultInt64, originIntStr)
		} else {
			l.pushInteger(int(resultInt64), originIntStr)
		}
		return nil
	}

	if s := i.FLOAT(); s != nil {
		var originFloatStr = s.GetText()
		var intStr = strings.ToLower(originFloatStr)
		var resultFloat64 float64
		lit := s.GetText()
		if strings.HasPrefix(lit, ".") {
			lit = "0" + lit
		}
		resultFloat64, _ = strconv.ParseFloat(intStr, 64)
		// todo 添加Compiler panic
		//if err != nil {
		//	l.panicCompilerError(integerIsTooLarge, originFloatStr)
		//}

		l.pushFloat(resultFloat64, originFloatStr)
		return nil

	}

	if s := i.HEX_FLOAT(); s != nil {
		var originFloatStr = s.GetText()
		var intStr = strings.ToLower(originFloatStr)
		var resultFloat64 float64
		lit := s.GetText()
		if strings.HasPrefix(lit, ".") {
			lit = "0" + lit
		}
		resultFloat64, _ = strconv.ParseFloat(intStr, 64)
		// todo 添加Compiler panic
		//if err != nil {
		//	l.panicCompilerError(integerIsTooLarge, originFloatStr)
		//}

		l.pushFloat(resultFloat64, originFloatStr)
		return nil
	}

	return nil
}

func (l *LuaTranslator) VisitString(raw lua.IStringContext) interface{} {
	if l == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*lua.StringContext)
	if i == nil {
		return nil
	}
	if normalStr := i.NORMALSTRING(); normalStr != nil {
		l.pushString(strings.Trim(normalStr.GetText(), `"`), strings.Trim(normalStr.GetText(), `"`))
		return nil
	}

	if charStr := i.CHARSTRING(); charStr != nil {
		l.pushString(strings.Trim(charStr.GetText(), `'`), strings.Trim(charStr.GetText(), `'`))
		return nil
	}

	if longStr := i.LONGSTRING(); longStr != nil {
		nestedStr := strings.Trim(longStr.GetText(), "[")
		nestedStr = strings.Trim(nestedStr, "]")
		l.pushString(nestedStr, nestedStr)
		return nil
	}

	return nil
}
