package javaclassparser

import "yaklang.io/yaklang/common/utils"

type ClassParser struct {
	reader   *ClassReader
	classObj *ClassObject
}

func NewClassParser(data []byte) *ClassParser {

	return &ClassParser{
		reader:   NewClassReader(data),
		classObj: &ClassObject{},
	}
}

/*
*
记录方法抛出的异常表

	EXCEPTIONS_ATTRIBUTE {
		u2 attribute_name_index;
		u4 attribute_length;
		u2 number_of_exceptions;
		u2 exception_index_table[number_of_exceptions];
	}
*/
type ExceptionsAttribute struct {
	Type                string
	AttrLen             uint32
	ExceptionIndexTable []uint16
}

func (self *ExceptionsAttribute) readInfo(cp *ClassParser) {
	self.ExceptionIndexTable = cp.reader.readUint16s()
}

func (this *ClassParser) Parse() (*ClassObject, error) {
	var err error
	err = this.parseAndCheckMagic()
	if err != nil {
		return nil, err
	}
	err = this.readAndCheckVersion()
	err = this.readConstantPool()
	this.classObj.AccessFlags = this.reader.readUint16()
	this.classObj.AccessFlagsVerbose = getAccessFlagsVerbose(this.classObj.AccessFlags)
	this.classObj.ThisClass = this.reader.readUint16()
	this.classObj.SuperClass = this.reader.readUint16()
	this.classObj.Interfaces = this.reader.readUint16s()
	this.classObj.Fields, err = this.readMembers()
	this.classObj.Methods, err = this.readMembers()
	this.classObj.Attributes = this.readAttributes()
	return this.classObj, nil
}
func (this *ClassParser) readMembers() ([]*MemberInfo, error) {
	memberCount := this.reader.readUint16()
	members := make([]*MemberInfo, memberCount)
	for i := range members {
		members[i] = this.readMember()
	}
	return members, nil
}
func (this *ClassParser) readMember() *MemberInfo {
	return &MemberInfo{
		AccessFlags:     this.reader.readUint16(),
		NameIndex:       this.reader.readUint16(),
		DescriptorIndex: this.reader.readUint16(),
		Attributes:      this.readAttributes(),
	}
}
func (this *ClassParser) readAttributes() []AttributeInfo {
	attributesCount := this.reader.readUint16()
	attributes := make([]AttributeInfo, attributesCount)
	for i := range attributes {
		attributes[i] = this.readAttribute()
	}
	return attributes
}
func (this *ClassParser) readAttribute() AttributeInfo {
	attributeNameIndex := this.reader.readUint16()
	attrName, err := this.classObj.getUtf8(attributeNameIndex)
	if err != nil {
		panic(utils.Errorf("Parse Attribute error: %v", err))
	}
	attrLen := this.reader.readUint32()
	attrInfo := newAttributeInfo(attrName, attrLen)
	attrInfo.readInfo(this)
	return attrInfo
}

func (this *ClassParser) parseAndCheckMagic() (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = utils.Errorf("read magic error: %v", e)
		}
	}()
	magic := this.reader.readUint32()
	if magic != 0xCAFEBABE {
		return utils.Error("java.lang.ClassFormatError: Magic error")
	}
	this.classObj.Magic = magic
	return nil
}
func (this *ClassParser) readAndCheckVersion() error {
	this.classObj.MinorVersion = this.reader.readUint16()
	this.classObj.MajorVersion = this.reader.readUint16()
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
	cpCount := int(this.reader.readUint16())
	cp := make([]ConstantInfo, cpCount-1)

	//索引从1开始，这里用了 <cpCount 说明index是从1到cpCount-1 及上文的1 ~ n-1
	for i := 0; i < cpCount-1; i++ {
		constantInfo, err := this.readConstantInfo()
		if err != nil {
			return err
		}
		cp[i] = constantInfo
		switch cp[i].(type) {
		case *ConstantLongInfo, *ConstantDoubleInfo:
			//占两个位置
			i++
		}
	}
	this.classObj.ConstantPool = cp
	return nil
}
func (this *ClassParser) readConstantInfo() (ConstantInfo, error) {
	tag := this.reader.readUint8()
	c := newConstantInfo(tag)
	c.readInfo(this)
	return c, nil
}
