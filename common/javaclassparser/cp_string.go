package javaclassparser

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

func (self *ConstantStringInfo) readInfo(cp *ClassParser) {
	self.StringIndex = cp.reader.readUint16()
}
