package javaclassparser

/*
*
ConstantFieldrefInfo、ConstantMethodrefInfo、ConstantInterfaceMethodrefInfo
这三个结构体继承自ConstantMemberrefInfo
Go语言没有“继承”的概念，而是通过结构体嵌套的方式实现的
*/
type ConstantMemberrefInfo struct {
	ClassIndex              uint16
	ClassIndexVerbose       string
	NameAndTypeIndex        uint16
	NameAndTypeIndexVerbose string
}

/*
*
字段符号引用

	CONSTANT_FIELDREF_INFO {
		u1 tag;
		u2 class_index;
		u2 name_and_type_index;
	}
*/
type ConstantFieldrefInfo struct {
	Type string
	ConstantMemberrefInfo
}

/*
*
普通（非接口）方法符号引用

	CONSTANT_METHODREF_INFO {
		u1 tag;
		u2 class_index;
		u2 name_and_type_index;
	}
*/
type ConstantMethodrefInfo struct {
	Type string
	ConstantMemberrefInfo
}

/*
*
接口方法符号引用

	CONSTANT_INTERFACEMETHODREF_INFO {
		u1 tag;
		u2 class_index;
		u2 name_and_type_index;
	}
*/
type ConstantInterfaceMethodrefInfo struct {
	Type string
	ConstantMemberrefInfo
}

func (self *ConstantMemberrefInfo) readInfo(cp *ClassParser) {
	self.ClassIndex = cp.reader.readUint16()
	self.NameAndTypeIndex = cp.reader.readUint16()
}
