package core

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

type Scope struct {
	VarTable map[int]*values.JavaRef
	VarId    *utils.VariableId
}

func NewScope() *Scope {
	return &Scope{
		VarTable: map[int]*values.JavaRef{},
		VarId:    utils.NewRootVariableId(),
	}
}

func (s *Scope) Next() *Scope {
	newScope := &Scope{
		VarTable: map[int]*values.JavaRef{},
		VarId:    s.VarId.Next(),
	}
	for k, v := range s.VarTable {
		newScope.VarTable[k] = v
	}
	return newScope
}
