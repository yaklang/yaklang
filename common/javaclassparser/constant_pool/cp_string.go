package constant_pool

import "github.com/yaklang/yaklang/common/javaclassparser/types"

/*
*
string info本身不存储字符串，只存了常量池索引，这个索引指向一个CONSTANT_UTF8_INFO。

	CONSTANT_STRING_INFO {
		u1 tag;
		u2 string_index;
	}
*/
type ConstantStringInfo struct {
	Type               string
	StringIndex        uint16
	StringIndexVerbose string
}

func (self *ConstantStringInfo) readInfo(cp types.ClassReader) {
	self.StringIndex = cp.ReadUint16()
}

func (self *ConstantStringInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(self.StringIndex))
}

func (self *ConstantStringInfo) GetTag() uint8 {
	return CONSTANT_String
}

func (self *ConstantStringInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantStringInfo) GetType() string {
	return self.Type
}
