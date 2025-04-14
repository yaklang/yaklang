package constant_pool

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/javaclassparser/types"
)

/*
* See: https://docs.oracle.com/javase/specs/jvms/se9/html/jvms-4.html#jvms-4.4.11
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
	CONSTANT_Module             = 19
	CONSTANT_Package            = 20
)

/*
*
constant info类型的接口
*/
type ConstantInfo interface {
	//从class data中读取常量信息
	readInfo(parser types.ClassReader)
	//写入class data
	writeInfo(writer types.ClassWriter)
	//获取常量类型
	GetTag() uint8
	SetType(name string)
	GetType() string
}

/*
*
从class data中读取并创建对应tag的constant Info
*/
func ReadConstantInfo(reader types.ClassReader) (ConstantInfo, error) {
	tag := reader.ReadUint8()
	c := newConstantInfo(tag)
	c.readInfo(reader)
	return c, nil
}

func WriteConstantInfo(writer types.ClassWriter, info ConstantInfo) error {
	writer.Write1Byte(info.GetTag())
	info.writeInfo(writer)
	return nil
}

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
	case CONSTANT_Module:
		return &ConstantModuleInfo{}
	case CONSTANT_Package:
		return &ConstantPackageInfo{}
	default:
		panic("java.lang.ClassFormatError: constant pool tag! met " + spew.Sdump(tag))
	}
}

/*
[WARN] 2024-11-07 20:14:33 [fs:135] walk file /Users/v1ll4n/Projects/java/compiling-failed-files/decompiler-err-module-info-2oO3wkCNSKbDbjtgKu1Nx6HOlsy.class failed: parse class error: java.lang.ClassFormatError: constant pool tag! met (uint8) 19

*/
