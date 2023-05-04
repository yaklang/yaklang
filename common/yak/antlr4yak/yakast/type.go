package yakast

import (
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"strings"
)

func (y *YakCompiler) VisitTypeLiteral(raw yak.ITypeLiteralContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*yak.TypeLiteralContext)
	if i == nil {
		return nil
	}

	recoverRange := y.SetRange(i.BaseParserRuleContext)
	defer recoverRange()

	text := i.GetText()
	if text == "int" || text == "byte" || text == "string" || text == "bool" || text == "float" || text == "var" {
		y.writeString(text)
		y.pushType(text)
	} else if text == "uint8" {
		y.writeString("byte")
		y.pushType("byte")
	} else if text == "uint" || text == "uint16" || text == "uint32" || text == "uint64" ||
		text == "int8" || text == "int16" || text == "int32" || text == "int64" {
		y.writeString("int")
		y.pushType("int")
	} else if text == "double" || text == "float32" || text == "float64" {
		y.writeString("float")
		y.pushType("float")
	} else if slice := i.SliceTypeLiteral(); slice != nil {
		y.VisitSliceTypeLiteral(slice)
	} else if strings.HasPrefix(text, "map") {
		iMapType := i.MapTypeLiteral()
		if iMapType != nil {
			mapType := iMapType.(*yak.MapTypeLiteralContext)
			y.writeString("map[")
			y.VisitTypeLiteral(mapType.TypeLiteral(0))
			y.writeString("]")
			y.VisitTypeLiteral(mapType.TypeLiteral(1))
			y.pushType("map")
		}

	} else if strings.HasPrefix(text, "chan") {
		y.writeString("chan ")
		y.VisitTypeLiteral(i.TypeLiteral())
		y.pushType("chan")
	}

	return nil
}
