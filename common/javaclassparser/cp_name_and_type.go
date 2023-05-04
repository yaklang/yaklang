package javaclassparser

/*
*
给出字段或方法的名称和描述符

	CONSTANT_NAMEANDTYPE_INFO {
		u1 tag;
		u2 name_index;
		u2 descriptor_index
	}
*/
type ConstantNameAndTypeInfo struct {
	Type string
	//字段或方法名 指向一个CONSTANT_UTF8_INFO
	NameIndex        uint16
	NameIndexVerbose string
	//字段或方法的描述符 指向一个CONSTANT_UTF8_INFO
	DescriptorIndex        uint16
	DescriptorIndexVerbose string
}

func (self *ConstantNameAndTypeInfo) readInfo(cp *ClassParser) {
	self.NameIndex = cp.reader.readUint16()
	self.DescriptorIndex = cp.reader.readUint16()
}

/**
(1)类型描述符
	①基本类型
	byte -> B
	short -> S
	char -> C
	int -> I
	long -> J *注意long的描述符是J不是L
	float -> F
	double -> D
	②引用类型的描述符是 L+类的完全限定名+分号
	③数组类型的描述符是 [+数组元素类型描述符

(2)字段描述符就是字段类型的描述符
(3)方法描述符是 （分号分割的参数类型描述符） + 返回值类型描述符，void返回值由单个字母V表示

紫苑注：boolean的基本类型应该是Z
*/
