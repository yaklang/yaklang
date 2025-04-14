package constant_pool

import "github.com/yaklang/yaklang/common/javaclassparser/types"

type ConstantModuleInfo struct {
	Type             string
	NameIndex        uint16
	NameIndexVerbose string
}

func (c *ConstantModuleInfo) readInfo(parser types.ClassReader) {
	c.NameIndex = parser.ReadUint16()
}

func (c *ConstantModuleInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(c.NameIndex))
}

func (c *ConstantModuleInfo) GetTag() uint8 {
	return CONSTANT_Module
}

func (c *ConstantModuleInfo) SetType(name string) {
	c.Type = name
}

func (c *ConstantModuleInfo) GetType() string {
	return c.Type
}

type ConstantPackageInfo struct {
	Type             string
	NameIndex        uint16
	NameIndexVerbose string
}

func (c *ConstantPackageInfo) readInfo(parser types.ClassReader) {
	c.NameIndex = parser.ReadUint16()
}

func (c *ConstantPackageInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(c.NameIndex))
}

func (c *ConstantPackageInfo) GetTag() uint8 {
	return CONSTANT_Package
}

func (c *ConstantPackageInfo) SetType(name string) {
	c.Type = name
}

func (c *ConstantPackageInfo) GetType() string {
	return c.Type
}
