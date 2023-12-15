package sfvm

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

type Value[V any] struct {
	v    any
	data *omap.OrderedMap[string, V]
}

func NewValue[V any](v any) *Value[V] {
	switch v.(type) {
	case *omap.OrderedMap[string, V]:
		return &Value[V]{v: "", data: v.(*omap.OrderedMap[string, V])}
	}
	return &Value[V]{v: v, data: nil}
}

func (v *Value[V]) AsInt() int {
	return utils.InterfaceToInt(v.v)
}

func (v *Value[V]) AsMap() *omap.OrderedMap[string, V] {
	return v.data
}

func (v *Value[V]) AsString() string {
	return utils.InterfaceToString(v.v)
}

func (v *Value[V]) AsBool() bool {
	return utils.InterfaceToBoolean(v.v)
}

func (v *Value[V]) IsMap() bool {
	return v.data != nil
}
