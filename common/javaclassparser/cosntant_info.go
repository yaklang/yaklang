package javaclassparser

/*
*
常量数据结构如下

	cp_info {
		u1 tag; -> 用来区分常量类型
		u2 Info[];
	}
*/
const (
	CONSTANT_Class              = 7
	CONSTANT_String             = 8
	CONSTANT_Fieldref           = 9
	CONSTANT_Methodref          = 10
	CONSTANT_InterfaceMethodref = 11
	CONSTANT_Integer            = 3
	CONSTANT_Float              = 4
	CONSTANT_Long               = 5
	CONSTANT_Double             = 6
	CONSTANT_NameAndType        = 12
	CONSTANT_Utf8               = 1
	CONSTANT_MethodHandle       = 15
	CONSTANT_MethodType         = 16
	CONSTANT_InvokeDynamic      = 18
)

/*
*
constant info类型的接口
*/
type ConstantInfo interface {
	//从class data中读取常量信息
	readInfo(reader *ClassParser)
}

/**
从class data中读取并创建对应tag的constant Info
*/
//func readConstantInfo(reader *ClassReader) ConstantInfo {
//	tag := reader.readUint8()
//	c := newConstantInfo(tag)
//	c.readInfo(reader)
//	return c
//}

/*
*
根据tag创建不同的constant Info
*/
func newConstantInfo(tag uint8) ConstantInfo {
	switch tag {
	case CONSTANT_Integer:
		return &ConstantIntegerInfo{}
	case CONSTANT_Float:
		return &ConstantFloatInfo{}
	case CONSTANT_Long:
		return &ConstantLongInfo{}
	case CONSTANT_Double:
		return &ConstantDoubleInfo{}
	case CONSTANT_Utf8:
		return &ConstantUtf8Info{}
	case CONSTANT_String:
		return &ConstantStringInfo{}
	case CONSTANT_Class:
		return &ConstantClassInfo{}
	case CONSTANT_Fieldref:
		return &ConstantFieldrefInfo{
			ConstantMemberrefInfo: ConstantMemberrefInfo{},
		}
	case CONSTANT_Methodref:
		return &ConstantMethodrefInfo{
			ConstantMemberrefInfo: ConstantMemberrefInfo{},
		}
	case CONSTANT_InterfaceMethodref:
		return &ConstantInterfaceMethodrefInfo{
			ConstantMemberrefInfo: ConstantMemberrefInfo{},
		}
	case CONSTANT_NameAndType:
		return &ConstantNameAndTypeInfo{}
	case CONSTANT_MethodType:
		return &ConstantMethodTypeInfo{}
	case CONSTANT_MethodHandle:
		return &ConstantMethodHandleInfo{}
	case CONSTANT_InvokeDynamic:
		return &ConstantInvokeDynamicInfo{}
	default:
		panic("java.lang.ClassFormatError: constant pool tag!")
	}
}
