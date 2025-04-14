package javaclassparser

import (
	"github.com/yaklang/yaklang/common/javaclassparser/attribute_info"
	"github.com/yaklang/yaklang/common/javaclassparser/constant_pool"
	"github.com/yaklang/yaklang/common/utils"
)

type ClassParser struct {
	reader   *ClassReader
	classObj *ClassObject
}

func NewClassParser(data []byte) *ClassParser {

	return &ClassParser{
		reader:   NewClassReader(data),
		classObj: NewClassObject(),
	}
}

func (this *ClassParser) Parse() (*ClassObject, error) {
	var err error
	err = this.parseAndCheckMagic()
	if err != nil {
		return nil, err
	}
	err = this.readAndCheckVersion()
	err = this.readConstantPool()
	this.classObj.AccessFlags = this.reader.ReadUint16()
	this.classObj.AccessFlagsVerbose, this.classObj.AccessFlagsToCode = getClassAccessFlagsVerbose(this.classObj.AccessFlags)
	this.classObj.ThisClass = this.reader.ReadUint16()
	this.classObj.SuperClass = this.reader.ReadUint16()
	this.classObj.Interfaces = this.reader.ReadUint16s()
	this.classObj.Fields, err = this.readMembers()
	this.classObj.Methods, err = this.readMembers()
	attributes, err := this.readAttributes()
	if err != nil {
		return nil, err
	}
	this.classObj.Attributes = attributes
	return this.classObj, nil
}
func (this *ClassParser) readMembers() ([]*MemberInfo, error) {
	memberCount := this.reader.ReadUint16()
	members := make([]*MemberInfo, memberCount)
	for i := range members {
		member, err := this.readMember()
		if err != nil {
			return nil, err
		}
		members[i] = member
	}
	return members, nil
}
func (this *ClassParser) readMember() (*MemberInfo, error) {
	accessFlags := this.reader.ReadUint16()
	nameIndex := this.reader.ReadUint16()
	descriptorIndex := this.reader.ReadUint16()
	attributes, err := this.readAttributes()
	if err != nil {
		return nil, err
	}
	return &MemberInfo{
		AccessFlags:     accessFlags,
		NameIndex:       nameIndex,
		DescriptorIndex: descriptorIndex,
		Attributes:      attributes,
	}, nil
}
func (this *ClassParser) readAttributes() ([]attribute_info.AttributeInfo, error) {
	attributesCount := this.reader.ReadUint16()
	attributes := make([]attribute_info.AttributeInfo, attributesCount)
	for i := range attributes {
		attrInfo, err := this.readAttribute()
		if err != nil {
			return nil, err
		}
		attributes[i] = attrInfo
	}
	return attributes, nil
}
func (this *ClassParser) readAttribute() (attribute_info.AttributeInfo, error) {
	attrInfo, err := attribute_info.ReadAttributeInfo(this.reader, this.classObj.ConstantPoolManager)
	if err != nil {
		panic(utils.Errorf("Parse Attribute error: %v", err))
	}
	return attrInfo, nil
}

func (this *ClassParser) parseAndCheckMagic() (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Errorf("read magic error: %v", e)
		}
	}()
	magic := this.reader.ReadUint32()
	if magic != 0xCAFEBABE {
		return utils.Error("java.lang.ClassFormatError: Magic error")
	}
	this.classObj.Magic = magic
	return nil
}
func (this *ClassParser) readAndCheckVersion() error {
	this.classObj.MinorVersion = this.reader.ReadUint16()
	this.classObj.MajorVersion = this.reader.ReadUint16()
	switch this.classObj.MajorVersion {
	case 45:
		return nil
	case 46, 47, 48, 49, 50, 51, 52:
		if this.classObj.MinorVersion == 0 {
			return nil
		}
	}
	return utils.Error("java.lang.UnsupportedClassVersionError!")
}
func (this *ClassParser) readConstantPool() error {
	cpCount := int(this.reader.ReadUint16())
	cp := make([]constant_pool.ConstantInfo, cpCount-1)

	//索引从1开始，这里用了 <cpCount 说明index是从1到cpCount-1 及上文的1 ~ n-1
	for i := 0; i < cpCount-1; i++ {
		constantInfo, err := this.readConstantInfo()
		if err != nil {
			return err
		}
		cp[i] = constantInfo
		switch cp[i].(type) {
		case *constant_pool.ConstantLongInfo, *constant_pool.ConstantDoubleInfo:
			//占两个位置
			i++
		}
	}
	this.classObj.ConstantPool = cp
	return nil
}
func (this *ClassParser) readConstantInfo() (constant_pool.ConstantInfo, error) {
	c, err := constant_pool.ReadConstantInfo(this.reader)
	if err != nil {
		return nil, err
	}
	return c, nil
}
