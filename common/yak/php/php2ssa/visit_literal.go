package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"math"
	"strconv"
	"strings"
)

func (y *builder) VisitConstant(raw phpparser.IConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ConstantContext)
	if i == nil {
		return nil
	}

	if i.Null() != nil {
		return y.ir.EmitConstInst(nil)
	} else if i.LiteralConstant() != nil {
		return y.VisitLiteralConstant(i.LiteralConstant())
	} else if i.MagicConstant() != nil {
		// magic __dir__ / __file__
		return y.ir.EmitUndefined(i.MagicConstant().GetText())
	} else if i.ClassConstant() != nil {
		// class constant
		log.Warnf("class constant not support yet: %s", i.ClassConstant().GetText())
	} else if i.QualifiedNamespaceName() != nil {
		y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	} else {
		log.Warnf("unknown constant: %s", i.GetText())
	}
	return nil
}

func (y *builder) VisitLiteralConstant(raw phpparser.ILiteralConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.LiteralConstantContext)
	if i == nil {
		return nil
	}

	if i.Real() != nil {
		/*
			// 匹配小数点后没有数字的情况
			$number2 = "123."; // LNum '.'

			// 匹配小数点前没有数字的情况
			$number3 = ".456"; // '.' LNum

			// 匹配有指数部分的数字
			$number4 = "123.456e7"; // LNum '.' LNum ExponentPart
			$number5 = "123.456E7"; // LNum '.' LNum ExponentPart
			$number6 = "123e7";     // LNum ExponentPart
			$number7 = ".456e7";    // '.' LNum ExponentPart

			// 匹配整数
			$number8 = "123"; // LNum
		*/
		pre, exponent, ok := strings.Cut(strings.ReplaceAll(strings.ToLower(i.Real().GetText()), "_", ""), "e")
		var exponentInt float64 = 1
		var preFloat = codec.Atof(pre)
		if ok {
			if len(exponent) > 0 {
				switch exponent[0] {
				case '-':
					rest := codec.Atoi(exponent[1:])
					exponentInt = math.Pow(10, -float64(rest))
				case '+':
					rest := codec.Atoi(exponent[1:])
					exponentInt = math.Pow(10, float64(rest))
				default:
					rest := codec.Atoi(exponent)
					exponentInt = math.Pow(10, float64(rest))
				}
			}
			preFloat = exponentInt * preFloat
		}
		return y.ir.EmitConstInst(preFloat)
	} else if i.BooleanConstant() != nil {
		switch strings.ToLower(i.BooleanConstant().GetText()) {
		case `true`:
			return y.ir.EmitConstInst(true)
		default: // case `false`:
			return y.ir.EmitConstInst(false)
		}
	} else if i.NumericConstant() != nil {
		return y.VisitNumericConstant(i.NumericConstant())
	} else if i.StringConstant() != nil {
		// log.Infof("string constant: %s", i.GetText())
		// magic! php string literal constant is not need any quote!
		return y.ir.EmitConstInst(i.GetText())
	}

	return nil
}

func (y *builder) VisitNumericConstant(raw phpparser.INumericConstantContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.NumericConstantContext)
	if i == nil {
		return nil
	}

	var err error
	var result int64
	numStr := strings.ToLower(i.GetText())
	switch true {
	case strings.HasPrefix(numStr, "0o"):
		result, err = strconv.ParseInt(numStr[2:], 8, 64)
	case strings.HasPrefix(numStr, "0x"):
		result, err = strconv.ParseInt(numStr[2:], 16, 64)
	case strings.HasPrefix(numStr, "0b"):
		result, err = strconv.ParseInt(numStr[2:], 2, 64)
	default:
		if len(numStr) > 1 && numStr[0] == '0' {
			result, err = strconv.ParseInt(numStr[1:], 8, 64)
		} else {
			result, err = strconv.ParseInt(numStr, 10, 64)
		}
	}
	if err != nil {
		log.Errorf("php parse int failed: %s", err)
		return nil
	}

	return y.ir.EmitConstInst(result)
}

func (y *builder) VisitString_(raw phpparser.IStringContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.StringContext)
	if i == nil {
		return nil
	}

	str, err := yakunquote.Unquote(raw.GetText())
	if err != nil {
		str = raw.GetText()
	}

	return y.ir.EmitConstInst(str)
}

func (y *builder) VisitIdentifier(raw phpparser.IIdentifierContext) string {
	if y == nil || raw == nil {
		return ""
	}

	return raw.GetText()
	//i, _ := raw.(*phpparser.IdentifierContext)
	//if i == nil {
	//	return
	//}
	//
	//return nil
}
