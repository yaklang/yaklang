package yakast

import (
	"strings"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
)

func (y *YakCompiler) VisitTypeLiteral(raw yak.ITypeLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.TypeLiteralContext)
	if i == nil {
		return nil
	}

	recoverRange := y.SetRange(&i.BaseParserRuleContext)
	defer recoverRange()

	text := i.GetText()
	switch text {
	case "string", "bool":
		y.pushType(text)
	case "var", "any":
		y.pushType("any")
	case "byte", "uint8":
		y.pushType("byte")
	case "int", "uint", "uint16", "uint32", "uint64", "int8", "int16", "int32", "int64":
		y.pushType("int")
	case "double", "float", "float32", "float64":
		y.pushType("float")
	case "omap":
		y.pushType("omap")
	default:
		if slice := i.SliceTypeLiteral(); slice != nil {
			y.VisitSliceTypeLiteral(slice)
		} else if strings.HasPrefix(text, "map") {
			iMapType := i.MapTypeLiteral()
			if iMapType != nil {
				mapType := iMapType.(*yak.MapTypeLiteralContext)
				y.VisitTypeLiteral(mapType.TypeLiteral(0))
				y.VisitTypeLiteral(mapType.TypeLiteral(1))
				y.pushType("map")
			}
		} else if strings.HasPrefix(text, "chan") {
			y.VisitTypeLiteral(i.TypeLiteral())
			y.pushType("chan")
		}
	}

	return nil
}
