package mq

import (
	"fmt"
	"reflect"
	"sync"
)

type RPCDescription struct {
	FunctionType reflect.Type
}

type RPCSpec struct {
	table *sync.Map
}

func NewRPCSpec() *RPCSpec {
	return &RPCSpec{
		table: new(sync.Map),
	}
}

func (r *RPCSpec) Register(f string, desc *RPCDescription) {
	r.table.Store(f, desc)
}

func (r *RPCSpec) ValidateFunc(name string, f interface{}) (bool, string) {
	raw, ok := r.table.Load(name)
	if !ok {
		return false, "no such function in spec"
	}

	desc := raw.(*RPCDescription)
	if desc.FunctionType != reflect.TypeOf(f) {
		return false, fmt.Sprintf("function is invalid type(%v): expect: %v", reflect.TypeOf(f), desc.FunctionType)
	}

	return true, ""
}
