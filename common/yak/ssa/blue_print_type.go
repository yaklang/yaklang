package ssa

import (
	"fmt"
	"github.com/samber/lo"
)

var _ Type = (*Blueprint)(nil)

/// ============= implement type interface

func (b *Blueprint) String() string {
	str := fmt.Sprintf("ClassBluePrint: %s", b.Name)
	return str
}

func (b *Blueprint) PkgPathString() string {
	return ""
}

func (b *Blueprint) RawString() string {
	return ""
}

func (b *Blueprint) GetTypeKind() TypeKind {
	return ClassBluePrintTypeKind
}

func (b *Blueprint) SetMethod(m map[string]*Function) {
	b.NormalMethod = m
}

func (b *Blueprint) AddMethod(key string, fun *Function) {
	b.RegisterNormalMethod(key, fun)
}

func (b *Blueprint) GetMethod() map[string]*Function {
	return b.NormalMethod
}

func (b *Blueprint) SetMethodGetter(f func() map[string]*Function) {
}

func (b *Blueprint) AddFullTypeName(name string) {
	if b == nil {
		return
	}
	if !lo.Contains(b.fullTypeName, name) {
		b.fullTypeName = append(b.fullTypeName, name)
	}
}

func (b *Blueprint) GetFullTypeNames() []string {
	if b == nil {
		return nil
	}
	return b.fullTypeName
}

func (b *Blueprint) SetFullTypeNames(names []string) {
	if b == nil {
		return
	}
	b.fullTypeName = names
}
