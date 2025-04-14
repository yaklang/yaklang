package javaclassparser

import (
	"fmt"

	"github.com/yaklang/yaklang/common/utils"
)

type ClassObjectBuilder struct {
	Errors   []error
	classObj *ClassObject
}

func NewClassObjectBuilder(c *ClassObject) *ClassObjectBuilder {
	return &ClassObjectBuilder{classObj: c}
}
func (c *ClassObjectBuilder) GetObject() *ClassObject {
	return c.classObj
}
func (c *ClassObjectBuilder) NewError(msg string) {
	c.Errors = append(c.Errors, utils.Error(msg))
}
func (c *ClassObjectBuilder) GetErrors() []error {
	return c.Errors
}
func (c *ClassObjectBuilder) SetValue(old, new string) *ClassObjectBuilder {
	constant := c.classObj.FindConstStringFromPool(old)
	if constant == nil {
		c.NewError("Can't find constant string " + old)
		return c
	}
	constant.Value = new
	return c
}
func (c *ClassObjectBuilder) SetParam(k, v string) *ClassObjectBuilder {
	old := fmt.Sprintf("{{%s}}", k)
	constant := c.classObj.FindConstStringFromPool(old)
	if constant == nil {
		c.NewError("Can't find constant string " + old)
		return c
	}
	constant.Value = v
	return c
}
