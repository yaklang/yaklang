package sfvm

import (
	"github.com/yaklang/yaklang/common/log"
	"reflect"
)

func AutoValue(i any) ValueOperator {
	log.Warnf("TBD: AutoValue: %v", i)
	return i.(ValueOperator)
}

func valuesLen(i ValueOperator) int {
	switch ret := i.(type) {
	case *ValueList:
		return len(ret.values)
	case interface{ Length() int }:
		return ret.Length()
	case interface{ Len() int }:
		return ret.Len()
	default:
		kd := reflect.TypeOf(i).Kind()
		if kd == reflect.Array || kd == reflect.Slice {
			return reflect.ValueOf(i).Len()
		}
	}
	return 0
}
