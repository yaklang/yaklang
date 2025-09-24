package java2ssa

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils/yakunquote"

	"github.com/yaklang/yaklang/common/log"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *singleFileBuilder) VisitLiteral(raw javaparser.ILiteralContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.LiteralContext)
	if i == nil {
		return nil
	}

	var res ssa.Value
	if ret := i.IntegerLiteral(); ret != nil {
		res = y.VisitIntegerLiteral(ret)
	} else if ret := i.FloatLiteral(); ret != nil {
		res = y.VisitFloatLiteral(ret)
	} else if ret := i.CHAR_LITERAL(); ret != nil {
		lit := ret.GetText()
		var s string
		var err error
		if lit == "'\\''" {
			s = "'"
		} else {
			lit = strings.ReplaceAll(lit, `"`, `\"`)
			s, err = strconv.Unquote(fmt.Sprintf("\"%s\"", lit[1:len(lit)-1]))
			if err != nil {
				log.Errorf("javaast %s: %s", y.CurrentRange.String(), fmt.Sprintf("unquote error %s", err))
				return y.EmitConstInst(s)
			}
		}
		runeChar := []rune(s)[0]
		if runeChar < 256 {
			res = y.EmitConstInst(byte(runeChar))
		} else {
			res = y.EmitConstInst(runeChar)
		}
	} else if ret := i.STRING_LITERAL(); ret != nil {
		text := ret.GetText()
		if text == "\"\"" {
			res = y.EmitConstInst(text)
		}
		val := yakunquote.TryUnquote(text)
		res = y.EmitConstInst(val)
	} else if ret := i.BOOL_LITERAL(); ret != nil {
		boolLit, err := strconv.ParseBool(ret.GetText())
		if err != nil {
			log.Errorf("javaast %s: %s", y.CurrentRange.String(), fmt.Sprintf("parse bool error %s", err))
			return y.EmitConstInst(boolLit)
		}
		res = y.EmitConstInst(boolLit)
	} else if ret = i.NULL_LITERAL(); ret != nil {
		res = y.EmitConstInst(nil)
	} else if ret = i.TEXT_BLOCK(); ret != nil {
		text := ret.GetText()
		val := text[3 : len(text)-3]
		res = y.EmitConstInst(val)
	}
	// set full type name for right literal value
	t := res.GetType()
	newTyp := y.AddFullTypeNameRaw(t.String(), t)
	res.SetType(newTyp)
	return res
}

// integer literal
func (y *singleFileBuilder) VisitIntegerLiteral(raw javaparser.IIntegerLiteralContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.IntegerLiteralContext)
	if i == nil {
		return nil
	}

	var err error
	originIntStr := i.GetText()
	intStr := strings.ToLower(originIntStr)
	var resultInt64 int64
	switch true {
	case strings.HasPrefix(intStr, "0b"): // 二进制
		resultInt64, err = strconv.ParseInt(intStr[2:], 2, 64)
	case strings.HasPrefix(intStr, "0x"): // 十六进制
		resultInt64, err = strconv.ParseInt(intStr[2:], 16, 64)
	case strings.HasPrefix(intStr, "0o"): // 八进制
		resultInt64, err = strconv.ParseInt(intStr[2:], 8, 64)
	case strings.HasSuffix(intStr, "l"): //长整型
		resultInt64, err = strconv.ParseInt(intStr[:len(intStr)-1], 8, 64)
	case len(intStr) > 1 && intStr[0] == '0':
		resultInt64, err = strconv.ParseInt(intStr[1:], 8, 64)
	default:
		resultInt64, err = strconv.ParseInt(intStr, 10, 64)
	}
	if err != nil {
		log.Warnf("javaast %s: %s", y.CurrentRange.String(), fmt.Sprintf("const parse %s as integer literal... is to large for int64: %v", originIntStr, err))
		// big.NewInt(0).SetString()
		// return nil
		v := y.EmitConstInst(intStr)
		v.SetType(ssa.CreateNumberType())
		return v
	}
	if resultInt64 > math.MaxInt {
		return y.EmitConstInst(int64(resultInt64))
	} else {
		return y.EmitConstInst(int(resultInt64))
	}
}

func (y *singleFileBuilder) VisitFloatLiteral(raw javaparser.IFloatLiteralContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()
	i, _ := raw.(*javaparser.FloatLiteralContext)
	if i == nil {
		return nil
	}

	lit := i.GetText()
	if strings.HasPrefix(lit, ".") {
		lit = "0" + lit
	}
	f, _ := strconv.ParseFloat(lit, 64)
	return y.EmitConstInst(f)

}
