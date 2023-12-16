package sfvm

import (
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"strconv"
)

type Value struct {
	v    any
	data *omap.OrderedMap[string, any]
}

func NewValue(v any) *Value {
	switch v.(type) {
	case *omap.OrderedMap[string, any]:
		return &Value{v: "", data: v.(*omap.OrderedMap[string, any])}
	}
	return &Value{v: v, data: nil}
}

func (v *Value) AsInt() int {
	return utils.InterfaceToInt(v.v)
}

func (v *Value) AsMap() *omap.OrderedMap[string, any] {
	return v.data
}

func (v *Value) AsString() string {
	return utils.InterfaceToString(v.v)
}

func (v *Value) AsBool() bool {
	return utils.InterfaceToBoolean(v.v)
}

func (v *Value) IsMap() bool {
	return v.data != nil
}

func (v *Value) Value() any {
	if v.IsMap() {
		return v.data
	}
	return v.v
}

func (v *Value) VerboseString() string {
	if v.IsMap() {
		return fmt.Sprintf("(len: %v) omap: {...}", v.data.Len())
	}

	switch ret := v.v.(type) {
	case string:
		return strconv.Quote(ret)
	case int:
		return strconv.Itoa(ret)
	case bool:
		return strconv.FormatBool(ret)
	}

	//fallback
	return fmt.Sprintf("verbose: %v", spew.Sdump(v.v))
}
