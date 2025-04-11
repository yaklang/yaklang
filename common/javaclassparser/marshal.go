package javaclassparser

import (
	"bytes"
	"encoding/json"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

const (
	ClassObjectType              = "ClassObject"
	MemberInfoType               = "MemberInfo"
	ConstantInteger              = "ConstantInteger"
	ConstantFloat                = "ConstantFloat"
	ConstantLong                 = "ConstantLong"
	ConstantDouble               = "ConstantDouble"
	ConstantUtf8                 = "ConstantUtf8"
	ConstantString               = "ConstantString"
	ConstantClass                = "ConstantClass"
	ConstantFieldref             = "ConstantFieldref"
	ConstantMethodref            = "ConstantMethodref"
	ConstantInterfaceMethodref   = "ConstantInterfaceMethodref"
	ConstantNameAndType          = "ConstantNameAndType"
	ConstantMethodType           = "ConstantMethodType"
	ConstantMethodHandle         = "ConstantMethodHandle"
	ConstantInvokeDynamic        = "ConstantInvokeDynamic"
	ConstantModule               = "ConstantModule"
	ConstantPackage              = "ConstantPackage"
	CodeAttributeType            = "CodeAttribute"
	ConstantValueAttributeType   = "ConstantValueAttribute"
	DeprecatedAttributeType      = "DeprecatedAttribute"
	ExceptionsAttributeType      = "ExceptionsAttribute"
	LineNumberTableAttributeType = "LineNumberTableAttribute"
	SourceFileAttributeType      = "SourceFileAttribute"
	SyntheticAttributeType       = "SyntheticAttribute"
	UnparsedAttributeType        = "UnparsedAttribute"
)

func _MarshalJavaClass(cp *ClassObject, charLength int) []byte {
	defer func() {
		if err1 := recover(); err1 != nil {
			log.Error(err1)
			return
		}
	}()
	writer := NewJavaBufferWrite()
	writer.charLength = charLength
	//var buf bytes.Buffer
	writer.WriteHex("CAFEBABE")
	writer.Write2Byte(cp.MinorVersion)
	writer.Write2Byte(cp.MajorVersion)
	constantPoolLen := len(cp.ConstantPool)
	writer.Write2Byte(constantPoolLen + 1)

	//写常量池
	for i := 0; i < constantPoolLen; i++ {
		switch cp.ConstantPool[i].(type) {
		case *ConstantIntegerInfo:
			writer.Write1Byte(CONSTANT_Integer)
			writer.Write4Byte(cp.ConstantPool[i].(*ConstantIntegerInfo).Value)
		case *ConstantFloatInfo:
			writer.Write1Byte(CONSTANT_Float)
			writer.Write4Byte(cp.ConstantPool[i].(*ConstantFloatInfo).Value)
		case *ConstantLongInfo:
			writer.Write1Byte(CONSTANT_Long)
			writer.Write8Byte(cp.ConstantPool[i].(*ConstantLongInfo).Value)
		case *ConstantDoubleInfo:
			writer.Write1Byte(CONSTANT_Double)
			writer.Write8Byte(cp.ConstantPool[i].(*ConstantDoubleInfo).Value)
		case *ConstantUtf8Info:
			writer.Write1Byte(CONSTANT_Utf8)
			str := cp.ConstantPool[i].(*ConstantUtf8Info).Value
			writer.WriteString(str)

		case *ConstantStringInfo:
			writer.Write1Byte(CONSTANT_String)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantStringInfo).StringIndex)
		case *ConstantClassInfo:
			writer.Write1Byte(CONSTANT_Class)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantClassInfo).NameIndex)
		case *ConstantFieldrefInfo:
			writer.Write1Byte(CONSTANT_Fieldref)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantFieldrefInfo).ClassIndex)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantFieldrefInfo).NameAndTypeIndex)
		case *ConstantMethodrefInfo:
			writer.Write1Byte(CONSTANT_Methodref)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantMethodrefInfo).ClassIndex)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantMethodrefInfo).NameAndTypeIndex)
		case *ConstantInterfaceMethodrefInfo:
			writer.Write1Byte(CONSTANT_InterfaceMethodref)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantInterfaceMethodrefInfo).ClassIndex)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantInterfaceMethodrefInfo).NameAndTypeIndex)
		case *ConstantNameAndTypeInfo:
			writer.Write1Byte(CONSTANT_NameAndType)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantNameAndTypeInfo).NameIndex)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantNameAndTypeInfo).DescriptorIndex)
		case *ConstantMethodTypeInfo:
			writer.Write1Byte(CONSTANT_MethodType)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantMethodTypeInfo).DescriptorIndex)
		case *ConstantMethodHandleInfo:
			writer.Write1Byte(CONSTANT_MethodHandle)
			writer.Write1Byte(cp.ConstantPool[i].(*ConstantMethodHandleInfo).ReferenceKind)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantMethodHandleInfo).ReferenceIndex)
		case *ConstantInvokeDynamicInfo:
			writer.Write1Byte(CONSTANT_InvokeDynamic)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantInvokeDynamicInfo).BootstrapMethodAttrIndex)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantInvokeDynamicInfo).NameAndTypeIndex)
		case *ConstantModuleInfo:
			writer.Write1Byte(CONSTANT_Module)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantModuleInfo).NameIndex)
		case *ConstantPackageInfo:
			writer.Write1Byte(CONSTANT_Package)
			writer.Write2Byte(cp.ConstantPool[i].(*ConstantPackageInfo).NameIndex)
		case nil:
			continue
		default:
			panic("java.lang.ClassFormatError: constant pool tag!")
		}
	}
	writer.Write2Byte(cp.AccessFlags)
	writer.Write2Byte(cp.ThisClass)
	writer.Write2Byte(cp.SuperClass)
	interfaceObjLen := len(cp.Interfaces)
	writer.Write2Byte(interfaceObjLen)
	for i := 0; i < interfaceObjLen; i++ {
		writer.Write2Byte(cp.Interfaces[i])
	}

	//写字段
	fieldsLen := len(cp.Fields)
	writer.Write2Byte(fieldsLen)
	for i := 0; i < fieldsLen; i++ {
		writer.Write2Byte(cp.Fields[i].AccessFlags)
		writer.Write2Byte(cp.Fields[i].NameIndex)
		writer.Write2Byte(cp.Fields[i].DescriptorIndex)
		attrs := cp.Fields[i].Attributes
		writeAttributes(writer, attrs, cp)
	}
	//写方法
	methodsLen := len(cp.Methods)
	writer.Write2Byte(methodsLen)
	for i := 0; i < methodsLen; i++ {
		writer.Write2Byte(cp.Methods[i].AccessFlags)
		writer.Write2Byte(cp.Methods[i].NameIndex)
		writer.Write2Byte(cp.Methods[i].DescriptorIndex)
		attrs := cp.Methods[i].Attributes
		writeAttributes(writer, attrs, cp)
	}
	//写属性
	writeAttributes(writer, cp.Attributes, cp)
	return writer.Bytes()
}

func writeAttributes(writer *JavaBufferWriter, info []AttributeInfo, classObj *ClassObject) {
	attributesLen := len(info)
	writer.Write2Byte(attributesLen)
	for j := 0; j < attributesLen; j++ {
		switch info[j].(type) {
		case *CodeAttribute:
			codeAttr := info[j].(*CodeAttribute)
			n := classObj.findUtf8IndexFromPool("Code") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(codeAttr.AttrLen)
			writer.Write2Byte(codeAttr.MaxStack)
			writer.Write2Byte(codeAttr.MaxLocals)
			codel := len(codeAttr.Code)
			writer.Write4Byte(codel)
			writer.Write(codeAttr.Code)

			exceptionTable := codeAttr.ExceptionTable
			writer.Write2Byte(len(exceptionTable))
			for exceptionTableIndex := 0; exceptionTableIndex < len(exceptionTable); exceptionTableIndex++ {
				writer.Write2Byte(exceptionTable[exceptionTableIndex].StartPc)
				writer.Write2Byte(exceptionTable[exceptionTableIndex].EndPc)
				writer.Write2Byte(exceptionTable[exceptionTableIndex].HandlerPc)
				writer.Write2Byte(exceptionTable[exceptionTableIndex].CatchType)
			}
			writeAttributes(writer, codeAttr.Attributes, classObj)
		case *ConstantValueAttribute:
			n := classObj.findUtf8IndexFromPool("ConstantValue") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*ConstantValueAttribute).AttrLen)
			writer.Write2Byte(info[j].(*ConstantValueAttribute).ConstantValueIndex)
		case *DeprecatedAttribute:
			n := classObj.findUtf8IndexFromPool("Deprecated") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*DeprecatedAttribute).AttrLen)
		case *ExceptionsAttribute:
			n := classObj.findUtf8IndexFromPool("Exceptions") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*ExceptionsAttribute).AttrLen)
			exceptionIndexTable := info[j].(*ExceptionsAttribute).ExceptionIndexTable
			l := len(exceptionIndexTable)
			writer.Write2Byte(l)
			for t := 0; t < l; t++ {
				writer.Write2Byte(exceptionIndexTable[t])
			}
		case *LineNumberTableAttribute:
			n := classObj.findUtf8IndexFromPool("LineNumberTable") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*LineNumberTableAttribute).AttrLen)
			l := len(info[j].(*LineNumberTableAttribute).LineNumberTable)
			writer.Write2Byte(l)
			for t := 0; t < l; t++ {
				writer.Write2Byte(info[j].(*LineNumberTableAttribute).LineNumberTable[t].StartPc)
				writer.Write2Byte(info[j].(*LineNumberTableAttribute).LineNumberTable[t].LineNumber)
			}
		case *SourceFileAttribute:
			n := classObj.findUtf8IndexFromPool("SourceFile") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*SourceFileAttribute).AttrLen)
			writer.Write2Byte(info[j].(*SourceFileAttribute).SourceFileIndex)
		case *SyntheticAttribute:
			n := classObj.findUtf8IndexFromPool("Synthetic") + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*SyntheticAttribute).AttrLen)
		case *UnparsedAttribute:
			n := classObj.findUtf8IndexFromPool(info[j].(*UnparsedAttribute).Name) + 1
			writer.Write2Byte(n)
			writer.Write4Byte(info[j].(*UnparsedAttribute).Length)
			writer.Write(info[j].(*UnparsedAttribute).Info)
		}
	}

}

func _MarshalToJson(classObj *ClassObject) (string, error) {
	AddVerboseAndType(classObj, classObj)
	byteBuf := bytes.NewBuffer([]byte{})
	encoder := json.NewEncoder(byteBuf)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(classObj)

	var buf bytes.Buffer
	if err != nil {
		return "", err
	}
	err = json.Indent(&buf, byteBuf.Bytes(), "", " ")

	//rjs, err := json.MarshalIndent(classObj, "", " ")
	return buf.String(), err
}
func AddVerboseAndType(classObj *ClassObject, obj interface{}) {
	switch ret := obj.(type) {
	case *ClassObject:
		ret.Type = ClassObjectType
		ret.SuperClassVerbose, _ = classObj.getUtf8(ret.SuperClass)
		ret.ThisClassVerbose, _ = classObj.getUtf8(ret.ThisClass)
		ret.InterfacesVerbose = []string{}
		for _, interfaceIndex := range ret.Interfaces {
			name, err := classObj.getUtf8(interfaceIndex)
			if err != nil {
				name = "NULL"
			}
			classObj.InterfacesVerbose = append(ret.InterfacesVerbose, name)
		}
		AddVerboseAndType(classObj, classObj.ConstantPool)
		AddVerboseAndType(classObj, classObj.Fields)
		AddVerboseAndType(classObj, classObj.Methods)
		AddVerboseAndType(classObj, classObj.Attributes)
	case []ConstantInfo:
		for _, r := range ret {
			AddVerboseAndType(classObj, r)
		}
	case []*MemberInfo:
		for _, r := range ret {
			AddVerboseAndType(classObj, r)
		}
	case *MemberInfo:
		ret.Type = MemberInfoType
		ret.AccessFlagsVerbose, _ = getFieldAccessFlagsVerbose(ret.AccessFlags)
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
		ret.DescriptorIndexVerbose, _ = classObj.getUtf8(ret.DescriptorIndex)
		//ret.AccessFlagsVerbose, _ = classObj.getUtf8(ret.AccessFlags)
		AddVerboseAndType(classObj, ret.Attributes)
	case []AttributeInfo:
		for _, r := range ret {
			AddVerboseAndType(classObj, r)
		}
	case *CodeAttribute:
		ret.Type = CodeAttributeType
		AddVerboseAndType(classObj, ret.Attributes)
	case *ConstantValueAttribute:
		ret.Type = ConstantValueAttributeType
		ret.ConstantValueIndexVerbose, _ = classObj.getUtf8(ret.ConstantValueIndex)
	case *DeprecatedAttribute:
		ret.Type = DeprecatedAttributeType
	case *ExceptionsAttribute:
		ret.Type = ExceptionsAttributeType
	case *LineNumberTableAttribute:
		ret.Type = LineNumberTableAttributeType
	case *SourceFileAttribute:
		ret.Type = SourceFileAttributeType
		ret.SourceFileIndexVerbose, _ = classObj.getUtf8(ret.SourceFileIndex)
	case *SyntheticAttribute:
		ret.Type = SyntheticAttributeType
	case *UnparsedAttribute:
		ret.Type = UnparsedAttributeType
	case *ConstantIntegerInfo:
		ret.Type = ConstantInteger
	case *ConstantFloatInfo:
		ret.Type = ConstantFloat
	case *ConstantLongInfo:
		ret.Type = ConstantLong
	case *ConstantDoubleInfo:
		ret.Type = ConstantDouble
	case *ConstantUtf8Info:
		ret.Type = ConstantUtf8
	case *ConstantStringInfo:
		ret.Type = ConstantString
		ret.StringIndexVerbose, _ = classObj.getUtf8(ret.StringIndex)
	case *ConstantClassInfo:
		ret.Type = ConstantClass
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	case *ConstantFieldrefInfo:
		ret.Type = ConstantFieldref
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
		ret.ClassIndexVerbose, _ = classObj.getUtf8(ret.ClassIndex)
	case *ConstantMethodrefInfo:
		ret.Type = ConstantMethodref
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
		ret.ClassIndexVerbose, _ = classObj.getUtf8(ret.ClassIndex)
	case *ConstantInterfaceMethodrefInfo:
		ret.Type = ConstantInterfaceMethodref
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
		ret.ClassIndexVerbose, _ = classObj.getUtf8(ret.ClassIndex)
	case *ConstantNameAndTypeInfo:
		ret.Type = ConstantNameAndType
		ret.DescriptorIndexVerbose, _ = classObj.getUtf8(ret.DescriptorIndex)
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	case *ConstantMethodTypeInfo:
		ret.Type = ConstantMethodref
		ret.DescriptorIndexVerbose, _ = classObj.getUtf8(ret.DescriptorIndex)
	case *ConstantMethodHandleInfo:
		ret.Type = ConstantMethodHandle
		ret.ReferenceIndexVerbose, _ = classObj.getUtf8(ret.ReferenceIndex)
	case *ConstantInvokeDynamicInfo:
		ret.Type = ConstantInvokeDynamic
		ret.BootstrapMethodAttrIndexVerbose, _ = classObj.getUtf8(ret.BootstrapMethodAttrIndex)
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
	case *ConstantModuleInfo:
		ret.Type = ConstantModule
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	case *ConstantPackageInfo:
		ret.Type = ConstantPackage
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	}
}
func _UnmarshalToClassObject(jsonData string) (*ClassObject, error) {
	data := []byte(jsonData)
	var obj map[string]interface{}
	err := json.Unmarshal(data, &obj)
	if err != nil {
		return nil, err
	}
	classObj, err := mapToClassObject(obj)
	if err != nil {
		return nil, err
	}
	return classObj, err
}

func mapToClassObject(objData map[string]interface{}) (_ *ClassObject, err error) {
	defer func() {
		if err1 := recover(); err1 != nil {
			err = utils.Error(err1)
			return
		}
	}()
	classObj := NewClassObject()
	ConstantPool := []ConstantInfo{}
	memberInfo := []*MemberInfo{}
	Attributes := []AttributeInfo{}
	var Fields, Methods []*MemberInfo
	var parseType func(objData map[string]interface{}) error
	parseType = func(objData map[string]interface{}) error {
		switch objData["Type"] {
		case ClassObjectType:
			for _, AttributesData := range objData["Attributes"].([]interface{}) {
				err = parseType(AttributesData.(map[string]interface{}))
				if err != nil {
					return err
				}
			}
			for _, AttributesData := range objData["ConstantPool"].([]interface{}) {
				err = parseType(AttributesData.(map[string]interface{}))
				if err != nil {
					return err
				}
			}
			for _, AttributesData := range objData["Methods"].([]interface{}) {
				err = parseType(AttributesData.(map[string]interface{}))
				if err != nil {
					return err
				}
			}
			Methods = memberInfo
			memberInfo = []*MemberInfo{}
			for _, AttributesData := range objData["Fields"].([]interface{}) {
				err = parseType(AttributesData.(map[string]interface{}))
				if err != nil {
					return err
				}
			}
			Fields = memberInfo
			deleteStringKeysFromMap(objData, "Attributes", "ConstantPool", "Methods", "Fields")
			err = mapstructure.Decode(objData, classObj)
			if err != nil {
				return err
			}
			classObj.Attributes = Attributes
			classObj.ConstantPool = ConstantPool
			classObj.Methods = Methods
			classObj.Fields = Fields
		case CodeAttributeType:
			d := &CodeAttribute{}
			Attributes_bak := Attributes
			Attributes = []AttributeInfo{}
			for _, AttributesData := range objData["Attributes"].([]interface{}) {
				err = parseType(AttributesData.(map[string]interface{}))
				if err != nil {
					return err
				}
			}
			delete(objData, "Attributes")

			var Code []byte
			Code, err = codec.DecodeBase64(objData["Code"].(string))
			if err != nil {
				return err
			}
			objData["Code"] = Code
			err = mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
			d.Attributes = Attributes
			Attributes = Attributes_bak
			Attributes = append(Attributes, d)
		case ConstantValueAttributeType:
			d := &ConstantValueAttribute{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case DeprecatedAttributeType:
			d := &DeprecatedAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ExceptionsAttributeType:
			d := &ExceptionsAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case LineNumberTableAttributeType:
			d := &LineNumberTableAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case SourceFileAttributeType:
			d := &SourceFileAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case SyntheticAttributeType:
			d := &SyntheticAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case UnparsedAttributeType:
			d := &UnparsedAttribute{}
			Attributes = append(Attributes, d)
			var Code []byte
			Code, err = codec.DecodeBase64(objData["Info"].(string))
			if err != nil {
				return err
			}
			objData["Info"] = Code
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantInteger:
			d := &ConstantIntegerInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantFloat:
			d := &ConstantFloatInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantLong:
			d := &ConstantLongInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantDouble:
			d := &ConstantDoubleInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantUtf8:
			d := &ConstantUtf8Info{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantString:
			d := &ConstantStringInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantClass:
			d := &ConstantClassInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantFieldref:
			d := &ConstantFieldrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{}, Type: ConstantFieldref}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, &d.ConstantMemberrefInfo)
			if err != nil {
				return err
			}
		case ConstantMethodref:
			d := &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{}, Type: ConstantMethodref}

			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, &d.ConstantMemberrefInfo)
			if err != nil {
				return err
			}
		case ConstantInterfaceMethodref:
			d := &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{}, Type: ConstantInterfaceMethodref}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, &d.ConstantMemberrefInfo)
			if err != nil {
				return err
			}
		case ConstantNameAndType:
			d := &ConstantNameAndTypeInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantMethodType:
			d := &ConstantMethodTypeInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantMethodHandle:
			d := &ConstantMethodHandleInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantInvokeDynamic:
			d := &ConstantInvokeDynamicInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case MemberInfoType:
			d := &MemberInfo{}
			memberInfo = append(memberInfo, d)
			Attributes_bak := Attributes
			Attributes = []AttributeInfo{}
			for _, AttributesData := range objData["Attributes"].([]interface{}) {
				err := parseType(AttributesData.(map[string]interface{}))
				if err != nil {
					return err
				}
			}
			delete(objData, "Attributes")
			err = mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
			d.Attributes = Attributes
			Attributes = Attributes_bak
		default:
			return utils.Error("error Type")
		}
		return nil
	}
	err = parseType(objData)
	if err != nil {
		return nil, err
	}
	return classObj, nil
}
