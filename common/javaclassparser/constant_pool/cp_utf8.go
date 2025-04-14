package constant_pool

import (
	"github.com/yaklang/yaklang/common/javaclassparser/types"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

/*
*

	CONSTANT_UTF8_INFO {
		u1 tag;
		u2 Length;
		u1 bytes[Length];
	}
*/
type ConstantUtf8Info struct {
	Type  string
	Value string
}

func (self *ConstantUtf8Info) GetTag() uint8 {
	return CONSTANT_Utf8
}

func (self *ConstantUtf8Info) readInfo(cp types.ClassReader) {
	length := uint32(cp.ReadUint16())
	bytes := cp.ReadBytes(length)
	bytes, err := utils.SimplifyUtf8(bytes)
	if err != nil {
		log.Errorf("parse utf8 data error: %v", err)
	}
	self.Value = string(bytes)
}

func (self *ConstantUtf8Info) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(len(self.Value)))
	writer.WriteBytes([]byte(self.Value))
}

func (self *ConstantUtf8Info) SetType(name string) {
	self.Type = name
}

func (self *ConstantUtf8Info) GetType() string {
	return self.Type
}
