package go2ssa

import (
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"math"
	"strconv"
	"strings"
)

func (y *builder) VisitLiteral(raw goparser.ILiteralContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if raw == nil || y == nil {
		return nil
	}

	i := raw.(*goparser.LiteralContext)
	if i == nil {
		return nil
	}

	if ret := i.BasicLit(); ret != nil {
		return y.VisitBasicLit(ret)
	} else if ret := i.CompositeLit(); ret != nil {
		return y.VisitCompositeLit(ret)
	} else if ret := i.FunctionLit(); ret != nil {
		return y.VisitFunctionLit(ret)
	}
	return nil
}

func (y *builder) VisitBasicLit(raw goparser.IBasicLitContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.BasicLitContext)
	if i == nil {
		return nil
	}

	if ret := i.NIL_LIT(); ret != nil {
		return y.ir.EmitConstInstNil()
	} else if ret := i.Integer(); ret != nil {
		return y.VisitInteger(ret)
	} else if ret := i.String_(); ret != nil {
		return y.VisitStringEx(ret)
	} else if ret := i.FLOAT_LIT(); ret != nil {
		lit := ret.GetText()
		f, _ := strconv.ParseFloat(lit, 64)
		return y.ir.EmitConstInst(f)
	} else if ret := i.IMAGINARY_LIT(); ret != nil {
		lit := ret.GetText()
		return y.ir.EmitConstInst(lit)
	} else if ret := i.RUNE_LIT(); ret != nil {
		lit := ret.GetText()
		if len(lit) != 3 {
			y.ir.NewError(ssa.Error, "go", "unsupport rune literal")
		}
		lit = lit[1:]
		runeLit := []rune(lit)
		return y.ir.EmitConstInst(runeLit[0])
	}

	y.ir.NewError(ssa.Error, "go", "cannot parse basic literal %v", i.GetText())
	return nil
}

func (y *builder) VisitCompositeLit(raw goparser.ICompositeLitContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.CompositeLitContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitFunctionLit(raw goparser.IFunctionLitContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.FunctionLitContext)
	if i == nil {
		return nil
	}

	return nil

}

func (y *builder) VisitInteger(raw goparser.IIntegerContext) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.IntegerContext)
	if i == nil {
		return nil
	}
	originIntStr := i.GetText()
	var resultInt64 int64
	var err error
	if ret := i.DECIMAL_LIT(); ret != nil {
		resultInt64, err = strconv.ParseInt(originIntStr, 10, 64)
	} else if ret := i.BINARY_LIT(); ret != nil {
		resultInt64, err = strconv.ParseInt(originIntStr[2:], 2, 64)
	} else if ret := i.OCTAL_LIT(); ret != nil {
		if strings.HasPrefix(ret.GetText(), "0o") || strings.HasPrefix(ret.GetText(), "0O") {
			resultInt64, err = strconv.ParseInt(originIntStr[2:], 8, 64)
		}
		resultInt64, err = strconv.ParseInt(originIntStr[1:], 8, 64)
	} else if ret := i.HEX_LIT(); ret != nil {
		resultInt64, err = strconv.ParseInt(originIntStr[2:], 16, 64)
	} else if ret := i.IMAGINARY_LIT(); ret != nil {
		return y.ir.EmitConstInst(ret.GetText())
	} else if ret := i.RUNE_LIT(); ret != nil {
		if len(originIntStr) != 3 {
			y.ir.NewError(ssa.Error, "go", "unsupport rune literal")
		}
		originIntStr = originIntStr[1:]
		runeLit := []rune(originIntStr)
		resultInt64 = int64(runeLit[0])
	}

	if err != nil {
		y.ir.NewError(ssa.Error, "go", "const parse %s as integer literal... is to large for int64: %v", originIntStr, resultInt64, err)
		return nil
	}
	if resultInt64 > math.MaxInt {
		return y.ir.EmitConstInst(int64(resultInt64))
	} else {
		return y.ir.EmitConstInst(int(resultInt64))
	}

}

func (y *builder) VisitStringEx(raw goparser.IString_Context) ssa.Value {
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	if y == nil || raw == nil {
		return nil
	}

	i := raw.(*goparser.String_Context)
	if i == nil {
		return nil
	}

	text := i.GetText()
	if text == "" {
		return y.ir.EmitConstInst(text)
	}

	prefix := 0

	switch text[0] {
	case '"':
		var val string
		if lit := text; len(lit) >= 2 {
			val = lit[1 : len(lit)-1]
		} else {
			val = lit
		}
		return y.ir.EmitConstInstWithUnary(val, prefix)
	case '`':
		val := text[1 : len(text)-1]
		return y.ir.EmitConstInstWithUnary(val, prefix)
	default:
		y.ir.NewError(ssa.Error, "go", "unsupported string literal: %s", text)
		return nil
	}
}
