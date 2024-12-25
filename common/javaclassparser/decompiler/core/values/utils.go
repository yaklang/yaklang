package values

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"

func GetNotOp(op string) string {
	switch op {
	case "==":
		return "!="
	case "!=":
		return "=="
	case "<":
		return ">="
	case ">=":
		return "<"
	case ">":
		return "<="
	case "<=":
		return ">"
	default:
		return "[not support op " + op + "]"
	}
}

func SimplifyConditionValue(condition JavaValue) JavaValue {
	resVal := condition
	if val, ok := resVal.(*JavaExpression); ok {
		vals := []JavaValue{}
		for _, value := range val.Values {
			vals = append(vals, SimplifyConditionValue(value))
		}
		resVal = &JavaExpression{
			Op:     val.Op,
			Values: vals,
			Typ:    val.Typ,
		}
		if val.Op == Not {
			if v1, ok := vals[0].(*JavaExpression); ok {
				resVal = NewBinaryExpression(v1.Values[0], v1.Values[1], GetNotOp(v1.Op), types.NewJavaPrimer(types.JavaBoolean))
			}
		}
	}
	return resVal
}
