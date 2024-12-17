package utils

import (
	"fmt"
)

type VariableId struct {
	base *int
	id   int
}

func NewVariableId(base *int) *VariableId {
	return &VariableId{
		base: base,
	}
}

func (v *VariableId) String() string {
	return fmt.Sprintf("var%d", v.id+*v.base)
}
func (v *VariableId) Int() int {
	return v.id + *v.base
}
func (v *VariableId) Next() *VariableId {
	newV := *v
	newV.id++
	return &newV
}
