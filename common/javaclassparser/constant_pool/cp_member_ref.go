package constant_pool

import "github.com/yaklang/yaklang/common/javaclassparser/types"

/*
*
ConstantFieldrefInfo、ConstantMethodrefInfo、ConstantInterfaceMethodrefInfo
这三个结构体继承自ConstantMemberrefInfo
Go语言没有"继承"的概念，而是通过结构体嵌套的方式实现的
*/
type ConstantMemberrefInfo struct {
	ClassIndex              uint16
	ClassIndexVerbose       string
	NameAndTypeIndex        uint16
	NameAndTypeIndexVerbose string
}

/*
*

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

func (self *ConstantFieldrefInfo) GetTag() uint8 {
	return CONSTANT_Fieldref
}

func (self *ConstantFieldrefInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantFieldrefInfo) GetType() string {
	return self.Type
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

func (self *ConstantMethodrefInfo) GetTag() uint8 {
	return CONSTANT_Methodref
}

func (self *ConstantMethodrefInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantMethodrefInfo) GetType() string {
	return self.Type
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

func (self *ConstantInterfaceMethodrefInfo) GetTag() uint8 {
	return CONSTANT_InterfaceMethodref
}

func (self *ConstantInterfaceMethodrefInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantInterfaceMethodrefInfo) GetType() string {
	return self.Type
}

func (self *ConstantMemberrefInfo) readInfo(cp types.ClassReader) {
	self.ClassIndex = cp.ReadUint16()
	self.NameAndTypeIndex = cp.ReadUint16()
}

func (self *ConstantMemberrefInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(self.ClassIndex))
	writer.Write2Byte(uint16(self.NameAndTypeIndex))
}
