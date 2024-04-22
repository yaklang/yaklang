package sfvm

import (
	"fmt"
	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"regexp"
)

func AutoValue(i any) ValueOperator {
	log.Warnf("TBD: AutoValue: %v", i)
	return NewValue(nil)
}

func MergeValues(values ...ValueOperator) ValueOperator {
	return &ValueList{values: values}
}

func NewValues(values []ValueOperator) ValueOperator {
	return &ValueList{values: values}
}

type ValueList struct {
	values []ValueOperator
}

func (v *ValueList) GetName() string {
	return fmt.Sprintf("%v", v.values)
}

func (v *ValueList) IsMap() bool {
	return false
}

func (v *ValueList) IsList() bool {
	return true
}

func (v *ValueList) ExactMatch(s string) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueList does not support ExactMatch")
}

func (v *ValueList) GlobMatch(s glob.Glob) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueList does not support GlobMatch")
}

func (v *ValueList) RegexpMatch(regexp *regexp.Regexp) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueList does not support RegexpMatch")
}

func (v *ValueList) NumberEqual(i any) (bool, ValueOperator, error) {
	return false, nil, utils.Error("ValueList does not support NumberEqual")
}

func (v *ValueList) GetFields() (ValueOperator, error) {
	var result []ValueOperator
	for k := range v.values {
		result = append(result, AutoValue(k))
	}
	return NewValues(result), nil
}

func (v *ValueList) GetMembers() (ValueOperator, error) {
	var result []ValueOperator
	for _, k := range v.values {
		result = append(result, k)
	}
	return MergeValues(result...), nil
}

func (v *ValueList) GetFunctionCallArgs() (ValueOperator, error) {
	return nil, utils.Error("ValueList does not support GetFunctionCallArgs")
}

func (v *ValueList) GetSliceCallArgs() (ValueOperator, error) {
	return nil, utils.Error("ValueList does not support GetSliceCallArgs")
}

func (v *ValueList) Next() (ValueOperator, error) {
	return nil, utils.Error("ValueList does not support Next")
}

func (v ValueList) DeepNext() (ValueOperator, error) {
	return nil, utils.Error("ValueList does not support DeepNext")
}

var _ ValueOperator = &ValueList{}
