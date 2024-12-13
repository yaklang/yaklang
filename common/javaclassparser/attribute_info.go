package javaclassparser

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
	readInfo(reader *ClassParser)
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
	Type            string
	AttrLen         uint32
	NumberOfClasses uint16
	Classes         []*InnerClassInfo
}

type InnerClassInfo struct {
	InnerClassInfoIndex   uint16
	OuterClassInfoIndex   uint16
	InnerNameIndex        uint16
	InnerClassAccessFlags uint16
}

func (i *InnerClassesAttribute) readInfo(cp *ClassParser) {
	i.NumberOfClasses = cp.reader.readUint16()
	i.Classes = make([]*InnerClassInfo, i.NumberOfClasses)
	for j := range i.Classes {
		i.Classes[j] = &InnerClassInfo{
			InnerClassInfoIndex:   cp.reader.readUint16(),
			OuterClassInfoIndex:   cp.reader.readUint16(),
			InnerNameIndex:        cp.reader.readUint16(),
			InnerClassAccessFlags: cp.reader.readUint16(),
		}
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
	AttributeNameIndex  uint16
	AttributeLength     uint32
	NumBootstrapMethods uint16
	BootstrapMethods    []*BootstrapMethod
}

func (r *BootstrapMethodsAttribute) readInfo(cp *ClassParser) {
	r.NumBootstrapMethods = cp.reader.readUint16()
	r.BootstrapMethods = make([]*BootstrapMethod, r.NumBootstrapMethods)
	for i := range r.BootstrapMethods {
		m := &BootstrapMethod{
			BootstrapMethodRef:    cp.reader.readUint16(),
			NumBootstrapArguments: cp.reader.readUint16(),
		}
		for j := 0; j < int(m.NumBootstrapArguments); j++ {
			m.BootstrapArguments = append(m.BootstrapArguments, cp.reader.readUint16())
		}
		r.BootstrapMethods[i] = m
	}
}

type BootstrapMethod struct {
	BootstrapMethodRef    uint16
	NumBootstrapArguments uint16
	BootstrapArguments    []uint16
}

// 没解析的属性
type UnparsedAttribute struct {
	Type   string
	Name   string
	Length uint32
	Info   []byte
}

func (self *UnparsedAttribute) readInfo(cp *ClassParser) {
	self.Info = cp.reader.readBytes(self.Length)
}

// 源文件属性
type SourceFileAttribute struct {
	Type                   string
	AttrLen                uint32
	SourceFileIndex        uint16
	SourceFileIndexVerbose string
}

/*
*
用于支持@Deprecated注解
*/
type DeprecatedAttribute struct {
	AttrLen uint32
	MarkerAttribute
}

/*
*
用来标记源文件中不存在的、由编译器生成的类成员，主要为了支持嵌套类（内部类）和嵌套接口
*/
type SyntheticAttribute struct {
	AttrLen uint32
	MarkerAttribute
}

/*
*
上面两个struct的父类，其中没有任何数据
*/
type MarkerAttribute struct {
	Type string
}

func (self *MarkerAttribute) readInfo(reader *ClassParser) {
	//read nothing
}

func (self *SourceFileAttribute) readInfo(cp *ClassParser) {
	self.SourceFileIndex = cp.reader.readUint16()
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
	Type            string
	AttrLen         uint32
	LineNumberTable []*LineNumberTableEntry
}

type LineNumberTableEntry struct {
	StartPc    uint16
	LineNumber uint16
}

func (self *LineNumberTableAttribute) readInfo(cp *ClassParser) {
	lineNumberTableLength := cp.reader.readUint16()
	self.LineNumberTable = make([]*LineNumberTableEntry, lineNumberTableLength)
	for i := range self.LineNumberTable {
		self.LineNumberTable[i] = &LineNumberTableEntry{
			StartPc:    cp.reader.readUint16(),
			LineNumber: cp.reader.readUint16(),
		}
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
	Type                      string
	AttrLen                   uint32
	ConstantValueIndex        uint16
	ConstantValueIndexVerbose string
}

func (self *ConstantValueAttribute) readInfo(cp *ClassParser) {
	self.ConstantValueIndex = cp.reader.readUint16()
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
	Type           string
	AttrLen        uint32
	MaxStack       uint16
	MaxLocals      uint16
	Code           []byte
	ExceptionTable []*ExceptionTableEntry
	Attributes     []AttributeInfo
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

type RuntimeVisibleAnnotationsAttribute struct {
	Type        string
	AttrLen     uint32
	Annotations []*AnnotationAttribute
}
type EnumConstValue struct {
	TypeName  string
	ConstName string
}

func (r *RuntimeVisibleAnnotationsAttribute) readInfo(cp *ClassParser) {
	annotationsCount := cp.reader.readUint16()
	r.Annotations = make([]*AnnotationAttribute, annotationsCount)
	for i := range r.Annotations {
		anno := ParseAnnotation(cp)
		r.Annotations[i] = anno
	}
}

func (self *CodeAttribute) readInfo(cp *ClassParser) {
	self.MaxStack = cp.reader.readUint16()
	self.MaxLocals = cp.reader.readUint16()
	codeLength := cp.reader.readUint32()
	self.Code = cp.reader.readBytes(codeLength)
	self.ExceptionTable = readExceptionTable(cp.reader)
	self.Attributes = cp.readAttributes()
}

func readExceptionTable(reader *ClassReader) []*ExceptionTableEntry {
	exceptionTableLength := reader.readUint16()
	exceptionTable := make([]*ExceptionTableEntry, exceptionTableLength)
	for i := range exceptionTable {
		exceptionTable[i] = &ExceptionTableEntry{
			StartPc:   reader.readUint16(),
			EndPc:     reader.readUint16(),
			HandlerPc: reader.readUint16(),
			CatchType: reader.readUint16(),
		}
	}
	return exceptionTable
}

func newAttributeInfo(attrName string, attrLen uint32) AttributeInfo {
	switch attrName {
	case "Code":
		return &CodeAttribute{AttrLen: attrLen}
	case "ConstantValue":
		return &ConstantValueAttribute{AttrLen: attrLen}
	case "Deprecated":
		return &DeprecatedAttribute{AttrLen: attrLen}
	case "Exceptions":
		return &ExceptionsAttribute{AttrLen: attrLen}
	case "LineNumberTable":
		return &LineNumberTableAttribute{AttrLen: attrLen}
	//case "LocalVariableTable":
	//	return &LocalVariableTableAttribute{}
	case "SourceFile":
		return &SourceFileAttribute{AttrLen: attrLen}
	case "Synthetic":
		return &SyntheticAttribute{AttrLen: attrLen}
	case "RuntimeVisibleAnnotations":
		return &RuntimeVisibleAnnotationsAttribute{AttrLen: attrLen}
	case "BootstrapMethods":
		return &BootstrapMethodsAttribute{AttributeLength: attrLen}
	case "InnerClasses":
		return &InnerClassesAttribute{AttrLen: attrLen}
	default:
		return &UnparsedAttribute{Name: attrName, Length: attrLen, Info: nil}

	}
}
