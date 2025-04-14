package attribute_info

import (
	"fmt"

	"github.com/yaklang/yaklang/common/javaclassparser/constant_pool"
	"github.com/yaklang/yaklang/common/javaclassparser/types"
	"github.com/yaklang/yaklang/common/utils"
)

/*
*
属性表，储存了方法的字节码等信息

	attribute_info {
		u2 attribute_name_index;
		u4 attribute_length;
		u1 Info[attribute_length];
	}
*/
type AttributeInfo interface {
	readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool)
	writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool)
	SetType(name string)
	GetType() string
	GetBaseAttributeInfo() *BaseAttributeInfo
}

type BaseAttributeInfo struct {
	Name   string
	Raw    []byte
	Type   string
	Length uint32
}

func (self *BaseAttributeInfo) SetType(name string) {
	self.Type = name
}

func (self *BaseAttributeInfo) GetType() string {
	return self.Type
}

func GetAttributeInfoByName(name string) AttributeInfo {
	switch name {
	case "Code":
		return &CodeAttribute{}
	case "ConstantValue":
		return &ConstantValueAttribute{}
	case "Deprecated":
		return &DeprecatedAttribute{}
	case "Exceptions":
		return &ExceptionsAttribute{}
	case "LineNumberTable":
		return &LineNumberTableAttribute{}
	case "SourceFile":
		return &SourceFileAttribute{}
	case "Synthetic":
		return &SyntheticAttribute{}
	case "RuntimeVisibleAnnotations":
		return &RuntimeVisibleAnnotationsAttribute{}
	case "RuntimeVisibleTypeAnnotations":
		return &RuntimeVisibleTypeAnnotationsAttribute{}
	case "BootstrapMethods":
		return &BootstrapMethodsAttribute{}
	case "InnerClasses":
		return &InnerClassesAttribute{}
	case "Signature":
		return &SignatureAttribute{}
	}
	return nil
}

var _ AttributeInfo = &CodeAttribute{}
var _ AttributeInfo = &ConstantValueAttribute{}
var _ AttributeInfo = &DeprecatedAttribute{}
var _ AttributeInfo = &ExceptionsAttribute{}
var _ AttributeInfo = &LineNumberTableAttribute{}
var _ AttributeInfo = &SourceFileAttribute{}
var _ AttributeInfo = &SyntheticAttribute{}
var _ AttributeInfo = &RuntimeVisibleAnnotationsAttribute{}
var _ AttributeInfo = &RuntimeVisibleTypeAnnotationsAttribute{}
var _ AttributeInfo = &BootstrapMethodsAttribute{}
var _ AttributeInfo = &InnerClassesAttribute{}
var _ AttributeInfo = &SignatureAttribute{}
var _ AttributeInfo = &UnparsedAttribute{}

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
	BaseAttributeInfo
	ExceptionIndexTable []uint16
}

func (self *ExceptionsAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

func (self *ExceptionsAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	self.ExceptionIndexTable = reader.ReadUint16s()
}

func (self *ExceptionsAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	exceptionIndexTable := self.ExceptionIndexTable
	l := len(exceptionIndexTable)
	writer.Write2Byte(uint16(l))
	for t := 0; t < l; t++ {
		writer.Write2Byte(exceptionIndexTable[t])
	}
}

/*
	Signature_attribute {
		u2 attribute_name_index;
		u4 attribute_length;
		u2 signature_index;
	}
*/
type SignatureAttribute struct {
	BaseAttributeInfo
	SignatureIndex uint16
}

func (i *SignatureAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &i.BaseAttributeInfo
}

func (i *SignatureAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	i.SignatureIndex = reader.ReadUint16()
}

func (i *SignatureAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(i.SignatureIndex)
}

/*
	InnerClasses_attribute {
	    u2 attribute_name_index;
	    u4 attribute_length;
	    u2 number_of_classes;
	    {   u2 inner_class_info_index;
	        u2 outer_class_info_index;
	        u2 inner_name_index;
	        u2 inner_class_access_flags;
	    } classes[number_of_classes];
	}
*/
type InnerClassesAttribute struct {
	BaseAttributeInfo
	NumberOfClasses uint16
	Classes         []*InnerClassInfo
}

func (self *InnerClassesAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

type InnerClassInfo struct {
	InnerClassInfoIndex   uint16
	OuterClassInfoIndex   uint16
	InnerNameIndex        uint16
	InnerClassAccessFlags uint16
}

func (i *InnerClassesAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	i.NumberOfClasses = reader.ReadUint16()
	i.Classes = make([]*InnerClassInfo, i.NumberOfClasses)
	for j := range i.Classes {
		i.Classes[j] = &InnerClassInfo{
			InnerClassInfoIndex:   reader.ReadUint16(),
			OuterClassInfoIndex:   reader.ReadUint16(),
			InnerNameIndex:        reader.ReadUint16(),
			InnerClassAccessFlags: reader.ReadUint16(),
		}
	}
}

func (i *InnerClassesAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(i.NumberOfClasses)
	for _, class := range i.Classes {
		writer.Write2Byte(class.InnerClassInfoIndex)
		writer.Write2Byte(class.OuterClassInfoIndex)
		writer.Write2Byte(class.InnerNameIndex)
		writer.Write2Byte(class.InnerClassAccessFlags)
	}
}

/*
	BootstrapMethods_attribute {
	    u2 attribute_name_index;
	    u4 attribute_length;
	    u2 num_bootstrap_methods;
	    {   u2 bootstrap_method_ref;
	        u2 num_bootstrap_arguments;
	        u2 bootstrap_arguments[num_bootstrap_arguments];
	    } bootstrap_methods[num_bootstrap_methods];
	}
*/
type BootstrapMethodsAttribute struct {
	BaseAttributeInfo
	NumBootstrapMethods uint16
	BootstrapMethods    []*BootstrapMethod
}

func (r *BootstrapMethodsAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &r.BaseAttributeInfo
}

func (r *BootstrapMethodsAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	r.NumBootstrapMethods = reader.ReadUint16()
	r.BootstrapMethods = make([]*BootstrapMethod, r.NumBootstrapMethods)
	for i := range r.BootstrapMethods {
		m := &BootstrapMethod{
			BootstrapMethodRef:    reader.ReadUint16(),
			NumBootstrapArguments: reader.ReadUint16(),
		}
		for j := 0; j < int(m.NumBootstrapArguments); j++ {
			m.BootstrapArguments = append(m.BootstrapArguments, reader.ReadUint16())
		}
		r.BootstrapMethods[i] = m
	}
}

func (r *BootstrapMethodsAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(r.NumBootstrapMethods)
	for _, method := range r.BootstrapMethods {
		writer.Write2Byte(method.BootstrapMethodRef)
		writer.Write2Byte(method.NumBootstrapArguments)
		for _, arg := range method.BootstrapArguments {
			writer.Write2Byte(arg)
		}
	}
}

type BootstrapMethod struct {
	BootstrapMethodRef    uint16
	NumBootstrapArguments uint16
	BootstrapArguments    []uint16
}

// 没解析的属性
type UnparsedAttribute struct {
	BaseAttributeInfo
	Info []byte
}

func (self *UnparsedAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

func (self *UnparsedAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	self.Info = reader.ReadBytes(self.Length)
}

func (self *UnparsedAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.WriteBytes(self.Info)
}

// 源文件属性
type SourceFileAttribute struct {
	BaseAttributeInfo
	SourceFileIndex        uint16
	SourceFileIndexVerbose string
}

func (self *SourceFileAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

func (s *SourceFileAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(s.SourceFileIndex)
}

/*
*
用于支持@Deprecated注解
*/
type DeprecatedAttribute struct {
	BaseAttributeInfo
	MarkerAttribute
}

func (self *DeprecatedAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

/*
*
用来标记源文件中不存在的、由编译器生成的类成员，主要为了支持嵌套类（内部类）和嵌套接口
*/
type SyntheticAttribute struct {
	BaseAttributeInfo
	MarkerAttribute
}

func (self *SyntheticAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

/*
*
上面两个struct的父类，其中没有任何数据
*/
type MarkerAttribute struct {
	BaseAttributeInfo
}

func (self *MarkerAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	//read nothing
}

func (self *MarkerAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	//read nothing
}

func (self *SourceFileAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	self.SourceFileIndex = reader.ReadUint16()
}

/*
*
存放方法的行号信息，是调试信息

	LINE_NUMBER_TABLE_ATTRIBUTE {
		u2 attribute_name_index;
		u4 attribute_length;
		u2 line_number_table_length;
		{
			u2 start_pc;
			u2 lint_number;
		} line_number_table[line_number_table_length];
	}
*/
type LineNumberTableAttribute struct {
	BaseAttributeInfo
	LineNumberTable []*LineNumberTableEntry
}

func (self *LineNumberTableAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

type LineNumberTableEntry struct {
	StartPc    uint16
	LineNumber uint16
}

func (self *LineNumberTableAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	lineNumberTableLength := reader.ReadUint16()
	self.LineNumberTable = make([]*LineNumberTableEntry, lineNumberTableLength)
	for i := range self.LineNumberTable {
		self.LineNumberTable[i] = &LineNumberTableEntry{
			StartPc:    reader.ReadUint16(),
			LineNumber: reader.ReadUint16(),
		}
	}
}

func (self *LineNumberTableAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	l := len(self.LineNumberTable)
	writer.Write2Byte(uint16(l))
	for t := 0; t < l; t++ {
		writer.Write2Byte(self.LineNumberTable[t].StartPc)
		writer.Write2Byte(self.LineNumberTable[t].LineNumber)
	}
}

/*
*

	CONSTANTVALUE_ATTRIBUTE {
		u2 attribute_name_index;
		u4 attribute_length;
		u2 constantvalue_index;
	}
*/
type ConstantValueAttribute struct {
	BaseAttributeInfo
	ConstantValueIndex        uint16
	ConstantValueIndexVerbose string
}

func (self *ConstantValueAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

func (self *ConstantValueAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	self.ConstantValueIndex = reader.ReadUint16()
}

func (self *ConstantValueAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(self.ConstantValueIndex)
}

/*
*

	CODE_ATTRIBUTE {
		u2 attribute_name_index;
		u4 attribute_length;
		u2 max_stack; -> 操作数栈的最大深度
		u2 max_locals; -> 局部变量表大小
		u4 code_length;
		u1 Code[code_length];
		u2 exception_table_length;
		{
			u2 start_pc;
			u2 end_pc;
			u2 handle_pc;
			u2 catch_type;
		} exception_table[exception_table_length];
		u2 attributes_count;
		attribute_info Attributes[attributes_count]
	}
*/
type CodeAttribute struct {
	BaseAttributeInfo
	MaxStack       uint16
	MaxLocals      uint16
	Code           []byte
	ExceptionTable []*ExceptionTableEntry
	Attributes     []AttributeInfo
}

func (self *CodeAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

/*
*
异常表
*/
type ExceptionTableEntry struct {
	StartPc   uint16
	EndPc     uint16
	HandlerPc uint16
	CatchType uint16
}
type ElementValuePairAttribute struct {
	Tag   uint8
	Name  string
	Value any
}
type AnnotationAttribute struct {
	TypeName          string
	ElementValuePairs []*ElementValuePairAttribute
}

/*
		RuntimeVisibleParameterAnnotations_attribute {
	    u2 attribute_name_index;
	    u4 attribute_length;
	    u1 num_parameters;
	    {   u2         num_annotations;
	        annotation annotations[num_annotations];
	    } parameter_annotations[num_parameters];
	}
*/
type RuntimeVisibleParameterAnnotationsAttribute struct {
}

/*
	RuntimeInvisibleParameterAnnotations_attribute {
	    u2 attribute_name_index;
	    u4 attribute_length;
	    u1 num_parameters;
	    {   u2         num_annotations;
	        annotation annotations[num_annotations];
	    } parameter_annotations[num_parameters];
	}
*/
type RuntimeInvisibleParameterAnnotationsAttribute struct {
}

/*
	AnnotationDefault_attribute {
	    u2            attribute_name_index;
	    u4            attribute_length;
	    element_value default_value;
	}
*/
type AnnotationDefaultAttribute struct {
}

/*
	RuntimeVisibleTypeAnnotations_attribute {
	    u2 attribute_name_index;
	    u4 attribute_length;
	    u2 num_annotations;
	    type_annotation annotations[num_annotations];
	}
*/
type RuntimeVisibleTypeAnnotationsAttribute struct {
	RuntimeVisibleAnnotationsAttribute
}
type RuntimeVisibleAnnotationsAttribute struct {
	BaseAttributeInfo
	Annotations []*AnnotationAttribute
}

func (self *RuntimeVisibleAnnotationsAttribute) GetBaseAttributeInfo() *BaseAttributeInfo {
	return &self.BaseAttributeInfo
}

type EnumConstValue struct {
	TypeName  string
	ConstName string
}

func (r *RuntimeVisibleAnnotationsAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	annotationsCount := reader.ReadUint16()
	r.Annotations = make([]*AnnotationAttribute, annotationsCount)
	for i := range r.Annotations {
		anno := ParseAnnotation(reader, pool)
		r.Annotations[i] = anno
	}
}

func (r *RuntimeVisibleAnnotationsAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(uint16(len(r.Annotations)))
	for _, anno := range r.Annotations {
		WriteAnnotation(writer, anno, pool)
	}
}

func (r *RuntimeVisibleTypeAnnotationsAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(uint16(len(r.Annotations)))
	// 注意：完整实现需要写入注解内容，此处简化处理
}

func (self *CodeAttribute) readInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) {
	self.MaxStack = reader.ReadUint16()
	self.MaxLocals = reader.ReadUint16()
	codeLength := reader.ReadUint32()
	self.Code = reader.ReadBytes(codeLength)
	self.ExceptionTable = readExceptionTable(reader)
	attributes, err := readAttributes(reader, pool)
	if err != nil {
		panic(err)
	}
	self.Attributes = attributes
}

func (codeAttr *CodeAttribute) writeInfo(writer types.ClassWriter, pool *constant_pool.ConstantPool) {
	writer.Write2Byte(codeAttr.MaxStack)
	writer.Write2Byte(codeAttr.MaxLocals)
	codel := len(codeAttr.Code)
	writer.Write4Byte(uint32(codel))
	writer.WriteBytes(codeAttr.Code)

	exceptionTable := codeAttr.ExceptionTable
	writer.Write2Byte(uint16(len(exceptionTable)))
	for exceptionTableIndex := 0; exceptionTableIndex < len(exceptionTable); exceptionTableIndex++ {
		writer.Write2Byte(exceptionTable[exceptionTableIndex].StartPc)
		writer.Write2Byte(exceptionTable[exceptionTableIndex].EndPc)
		writer.Write2Byte(exceptionTable[exceptionTableIndex].HandlerPc)
		writer.Write2Byte(exceptionTable[exceptionTableIndex].CatchType)
	}
	writeAttributes(writer, codeAttr.Attributes, pool)
}

func readExceptionTable(reader types.ClassReader) []*ExceptionTableEntry {
	exceptionTableLength := reader.ReadUint16()
	exceptionTable := make([]*ExceptionTableEntry, exceptionTableLength)
	for i := range exceptionTable {
		exceptionTable[i] = &ExceptionTableEntry{
			StartPc:   reader.ReadUint16(),
			EndPc:     reader.ReadUint16(),
			HandlerPc: reader.ReadUint16(),
			CatchType: reader.ReadUint16(),
		}
	}
	return exceptionTable
}
func writeAttributes(writer types.ClassWriter, info []AttributeInfo, pool *constant_pool.ConstantPool) {
	attributesLen := len(info)
	writer.Write2Byte(uint16(attributesLen))
	for j := 0; j < attributesLen; j++ {
		WriteAttributeInfo(writer, info[j], pool)
	}
}

func readAttributes(reader types.ClassReader, pool *constant_pool.ConstantPool) ([]AttributeInfo, error) {
	var err error
	attributesCount := reader.ReadUint16()
	attributes := make([]AttributeInfo, attributesCount)
	for i := range attributes {
		attributes[i], err = ReadAttributeInfo(reader, pool)
		if err != nil {
			return nil, err
		}
	}
	return attributes, nil
}

func WriteAttributeInfo(writer types.ClassWriter, info AttributeInfo, pool *constant_pool.ConstantPool) (err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("WriteAttributeInfo error: %v", e)
		}
	}()
	n := pool.SearchUtf8Index(info.GetBaseAttributeInfo().Name) + 1
	writer.Write2Byte(uint16(n))
	writer.Write4Byte(info.GetBaseAttributeInfo().Length)
	info.writeInfo(writer, pool)
	return err
}

func ReadAttributeInfo(reader types.ClassReader, pool *constant_pool.ConstantPool) (res AttributeInfo, err error) {
	defer func() {
		if e := recover(); e != nil {
			err = fmt.Errorf("ReadAttributeInfo error: %v", e)
		}
	}()
	attributeNameIndex := reader.ReadUint16()
	attrName := pool.GetUtf8(int(attributeNameIndex))
	if attrName == nil {
		return nil, utils.Errorf("parse attribute name failed")
	}

	attributeInfo := GetAttributeInfoByName(attrName.Value)
	baseAttr := attributeInfo.GetBaseAttributeInfo()
	baseAttr.Name = attrName.Value
	baseAttr.Length = reader.ReadUint32()

	attributeInfo.readInfo(reader, pool)

	return attributeInfo, nil
}
