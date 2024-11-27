package javaclassparser

import (
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

func (self *ConstantUtf8Info) readInfo(cp *ClassParser) {
	length := uint32(cp.reader.readUint16())
	bytes := cp.reader.readBytes(length)
	bytes, err := utils.SimplifyUtf8(bytes)
	if err != nil {
		log.Errorf("parse utf8 data error: %v", err)
	}
	self.Value = string(bytes)
}
