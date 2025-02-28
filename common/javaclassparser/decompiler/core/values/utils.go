package values

import (
	"fmt"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

func GetNotOpWithError(op string) (string, error) {
	switch op {
	case "==":
		return "!=", nil
	case "!=":
		return "==", nil
	case "<":
		return ">=", nil
	case ">=":
		return "<", nil
	case ">":
		return "<=", nil
	case "<=":
		return ">", nil
	default:
		return "", fmt.Errorf("[not support op %s]", op)
	}
}

func GetNotOp(op string) string {
	res, err := GetNotOpWithError(op)
	if err != nil {
		return err.Error()
	}
	return res
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
				if v1.Op == Not {
					return v1.Values[0]
				} else {
					reverseOp, err := GetNotOpWithError(v1.Op)
					if err == nil {
						resVal = NewBinaryExpression(v1.Values[0], v1.Values[1], reverseOp, types.NewJavaPrimer(types.JavaBoolean))
					}
				}
			}
		}
	}
	return resVal
}
func UnpackSoltValue(value JavaValue) JavaValue {
	if ref, ok := value.(*SlotValue); ok {
		return UnpackSoltValue(ref.GetValue())
	}
	return value
}
