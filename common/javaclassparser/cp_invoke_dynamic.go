package javaclassparser

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

func (self *ConstantMethodHandleInfo) readInfo(cp *ClassParser) {
	self.ReferenceKind = cp.reader.readUint8()
	self.ReferenceIndex = cp.reader.readUint16()
}

func (self *ConstantMethodTypeInfo) readInfo(cp *ClassParser) {
	self.DescriptorIndex = cp.reader.readUint16()
}

func (self *ConstantInvokeDynamicInfo) readInfo(cp *ClassParser) {
	self.BootstrapMethodAttrIndex = cp.reader.readUint16()
	self.NameAndTypeIndex = cp.reader.readUint16()
}
