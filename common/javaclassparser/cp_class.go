package javaclassparser

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

func (self *ConstantClassInfo) readInfo(cp *ClassParser) {
	self.NameIndex = cp.reader.readUint16()
}
