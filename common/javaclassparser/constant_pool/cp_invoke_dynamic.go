package constant_pool

import "github.com/yaklang/yaklang/common/javaclassparser/types"

/*
	CONSTANT_MethodHandle_info {
	    u1 tag;
	    u1 reference_kind;
	    u2 reference_index;
	}
*/
type ConstantMethodHandleInfo struct {
	Type                  string
	ReferenceKind         uint8
	ReferenceKindVerbose  string
	ReferenceIndex        uint16
	ReferenceIndexVerbose string
}

/*
	CONSTANT_MethodType_info {
	    u1 tag;
	    u2 descriptor_index;
	}
*/
type ConstantMethodTypeInfo struct {
	Type                   string
	DescriptorIndex        uint16
	DescriptorIndexVerbose string
}

/*
	CONSTANT_InvokeDynamic_info {
	    u1 tag;
	    u2 bootstrap_method_attr_index;
	    u2 name_and_type_index;
	}
*/
type ConstantInvokeDynamicInfo struct {
	Type                            string
	BootstrapMethodAttrIndex        uint16
	BootstrapMethodAttrIndexVerbose string
	NameAndTypeIndex                uint16
	NameAndTypeIndexVerbose         string
}

func (self *ConstantInvokeDynamicInfo) GetTag() uint8 {
	return CONSTANT_InvokeDynamic
}

func (self *ConstantInvokeDynamicInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantInvokeDynamicInfo) GetType() string {
	return self.Type
}

func (self *ConstantMethodHandleInfo) readInfo(cp types.ClassReader) {
	self.ReferenceKind = cp.ReadUint8()
	self.ReferenceIndex = cp.ReadUint16()
}

func (self *ConstantMethodHandleInfo) writeInfo(writer types.ClassWriter) {
	writer.Write1Byte(uint8(self.ReferenceKind))
	writer.Write2Byte(uint16(self.ReferenceIndex))
}

func (self *ConstantMethodTypeInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(self.DescriptorIndex))
}

func (self *ConstantMethodTypeInfo) readInfo(cp types.ClassReader) {
	self.DescriptorIndex = cp.ReadUint16()
}

func (self *ConstantInvokeDynamicInfo) writeInfo(writer types.ClassWriter) {
	writer.Write2Byte(uint16(self.BootstrapMethodAttrIndex))
	writer.Write2Byte(uint16(self.NameAndTypeIndex))
}

func (self *ConstantInvokeDynamicInfo) readInfo(cp types.ClassReader) {
	self.BootstrapMethodAttrIndex = cp.ReadUint16()
	self.NameAndTypeIndex = cp.ReadUint16()
}

func (self *ConstantMethodHandleInfo) GetTag() uint8 {
	return CONSTANT_MethodHandle
}

func (self *ConstantMethodHandleInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantMethodHandleInfo) GetType() string {
	return self.Type
}

func (self *ConstantMethodTypeInfo) GetTag() uint8 {
	return CONSTANT_MethodType
}

func (self *ConstantMethodTypeInfo) SetType(name string) {
	self.Type = name
}

func (self *ConstantMethodTypeInfo) GetType() string {
	return self.Type
}
