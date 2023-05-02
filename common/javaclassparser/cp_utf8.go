package javaclassparser

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
	self.Value = string(bytes)
}
