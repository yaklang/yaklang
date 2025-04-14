package constant_pool

import "github.com/yaklang/yaklang/common/javaclassparser/types"

/*
*

	CONSTANT_CLASS_INFO {
		u1 tag;
		u2 name_index;
	}
*/
type ConstantClassInfo struct {
	Type             string
	ConstantType     string
	NameIndex        uint16
	NameIndexVerbose string
}

func (self *ConstantClassInfo) readInfo(cp types.ClassReader) {
	self.NameIndex = cp.ReadUint16()
}

func (self *ConstantClassInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(self.NameIndex))
}

func (self *ConstantClassInfo) GetTag() uint8 {
	return CONSTANT_Class
}

func (self *ConstantClassInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantClassInfo) GetType() string {
	return self.Type
}
