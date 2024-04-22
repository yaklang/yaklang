package sfvm

import (
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"regexp"
)

type ValueMap struct {
	values *omap.OrderedMap[string, ValueOperator]
}

func (v *ValueMap) IsMap() bool {
	return true
}

func (v *ValueMap) IsList() bool {
	return false
}

func (v *ValueMap) ExactMatch(s string) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueMap does not support ExactMatch")
}

func (v *ValueMap) GlobMatch(glob glob.Glob) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueMap does not support GlobMatch")
}

func (v *ValueMap) RegexpMatch(regexp *regexp.Regexp) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueMap does not support RegexpMatch")
}

func (v *ValueMap) NumberEqual(i any) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueMap does not support NumberEqual")
}

func (v *ValueMap) GetFields() (ValueOperator, error) {
	var result []ValueOperator
	for k := range v.values.Keys() {
		result = append(result, AutoValue(k))
	}
	return NewValues(result), nil
}

func (v *ValueMap) GetMembers() (ValueOperator, error) {
	return NewValues(v.values.Values()), nil
}

func (v *ValueMap) GetFunctionCallArgs() (ValueOperator, error) {
	return nil, utils.Error("ValueMap does not support GetFunctionCallArgs")
}

func (v *ValueMap) GetSliceCallArgs() (ValueOperator, error) {
	return nil, utils.Error("ValueMap does not support GetSliceCallArgs")
}

func (v *ValueMap) Next() (ValueOperator, error) {
	return nil, utils.Error("ValueMap does not support Next")
}

func (v *ValueMap) DeepNext() (ValueOperator, error) {
	return nil, utils.Error("ValueMap does not support DeepNext")
}

func (v *ValueMap) GetName() string {
	return "map:" + v.values.String()
}

func NewValueMap() *ValueMap {
	return &ValueMap{
		values: omap.NewOrderedMap(make(map[string]ValueOperator)),
	}
}

var _ ValueOperator = (*ValueMap)(nil)
