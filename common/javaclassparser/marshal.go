package javaclassparser

import (
	"bytes"
	"encoding/json"

	"github.com/go-viper/mapstructure/v2"
	"github.com/yaklang/yaklang/common/javaclassparser/attribute_info"
	"github.com/yaklang/yaklang/common/javaclassparser/constant_pool"
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
	writer.Write2Byte(uint16(constantPoolLen + 1))

	//写常量池
	for i := 0; i < constantPoolLen; i++ {
		constantInfo := cp.ConstantPool[i]
		constant_pool.WriteConstantInfo(writer, constantInfo)
	}
	writer.Write2Byte(cp.AccessFlags)
	writer.Write2Byte(cp.ThisClass)
	writer.Write2Byte(cp.SuperClass)
	interfaceObjLen := len(cp.Interfaces)
	writer.Write2Byte(uint16(interfaceObjLen))
	for i := 0; i < interfaceObjLen; i++ {
		writer.Write2Byte(cp.Interfaces[i])
	}

	//写字段
	fieldsLen := len(cp.Fields)
	writer.Write2Byte(uint16(fieldsLen))
	for i := 0; i < fieldsLen; i++ {
		writer.Write2Byte(cp.Fields[i].AccessFlags)
		writer.Write2Byte(cp.Fields[i].NameIndex)
		writer.Write2Byte(cp.Fields[i].DescriptorIndex)
		attrs := cp.Fields[i].Attributes
		writeAttributes(writer, attrs, cp)
	}
	//写方法
	methodsLen := len(cp.Methods)
	writer.Write2Byte(uint16(methodsLen))
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

func writeAttributes(writer *JavaBufferWriter, info []attribute_info.AttributeInfo, classObj *ClassObject) {
	attributesLen := len(info)
	writer.Write2Byte(uint16(attributesLen))
	for j := 0; j < attributesLen; j++ {
		attribute_info.WriteAttributeInfo(writer, info[j], classObj.ConstantPoolManager)
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
	case []constant_pool.ConstantInfo:
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
	case []attribute_info.AttributeInfo:
		for _, r := range ret {
			AddVerboseAndType(classObj, r)
		}
	case *attribute_info.CodeAttribute:
		ret.SetType(CodeAttributeType)
		AddVerboseAndType(classObj, ret.Attributes)
	case *attribute_info.ConstantValueAttribute:
		ret.SetType(ConstantValueAttributeType)
		ret.ConstantValueIndexVerbose, _ = classObj.getUtf8(ret.ConstantValueIndex)
	case *attribute_info.DeprecatedAttribute:
		ret.SetType(DeprecatedAttributeType)
	case *attribute_info.ExceptionsAttribute:
		ret.SetType(ExceptionsAttributeType)
	case *attribute_info.LineNumberTableAttribute:
		ret.SetType(LineNumberTableAttributeType)
	case *attribute_info.SourceFileAttribute:
		ret.SetType(SourceFileAttributeType)
		ret.SourceFileIndexVerbose, _ = classObj.getUtf8(ret.SourceFileIndex)
	case *attribute_info.SyntheticAttribute:
		ret.SetType(SyntheticAttributeType)
	case *attribute_info.UnparsedAttribute:
		ret.SetType(UnparsedAttributeType)
	case *constant_pool.ConstantIntegerInfo:
		ret.SetType(ConstantInteger)
	case *constant_pool.ConstantFloatInfo:
		ret.SetType(ConstantFloat)
	case *constant_pool.ConstantLongInfo:
		ret.SetType(ConstantLong)
	case *constant_pool.ConstantDoubleInfo:
		ret.SetType(ConstantDouble)
	case *constant_pool.ConstantUtf8Info:
		ret.SetType(ConstantUtf8)
	case *constant_pool.ConstantStringInfo:
		ret.SetType(ConstantString)
		ret.StringIndexVerbose, _ = classObj.getUtf8(ret.StringIndex)
	case *constant_pool.ConstantClassInfo:
		ret.SetType(ConstantClass)
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantFieldrefInfo:
		ret.SetType(ConstantFieldref)
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
		ret.ClassIndexVerbose, _ = classObj.getUtf8(ret.ClassIndex)
	case *constant_pool.ConstantMethodrefInfo:
		ret.SetType(ConstantMethodref)
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
		ret.ClassIndexVerbose, _ = classObj.getUtf8(ret.ClassIndex)
	case *constant_pool.ConstantInterfaceMethodrefInfo:
		ret.SetType(ConstantInterfaceMethodref)
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
		ret.ClassIndexVerbose, _ = classObj.getUtf8(ret.ClassIndex)
	case *constant_pool.ConstantNameAndTypeInfo:
		ret.SetType(ConstantNameAndType)
		ret.DescriptorIndexVerbose, _ = classObj.getUtf8(ret.DescriptorIndex)
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantMethodTypeInfo:
		ret.SetType(ConstantMethodType)
		ret.DescriptorIndexVerbose, _ = classObj.getUtf8(ret.DescriptorIndex)
	case *constant_pool.ConstantMethodHandleInfo:
		ret.SetType(ConstantMethodHandle)
		ret.ReferenceIndexVerbose, _ = classObj.getUtf8(ret.ReferenceIndex)
	case *constant_pool.ConstantInvokeDynamicInfo:
		ret.SetType(ConstantInvokeDynamic)
		ret.BootstrapMethodAttrIndexVerbose, _ = classObj.getUtf8(ret.BootstrapMethodAttrIndex)
		ret.NameAndTypeIndexVerbose, _ = classObj.getUtf8(ret.NameAndTypeIndex)
	case *constant_pool.ConstantModuleInfo:
		ret.SetType(ConstantModule)
		ret.NameIndexVerbose, _ = classObj.getUtf8(ret.NameIndex)
	case *constant_pool.ConstantPackageInfo:
		ret.SetType(ConstantPackage)
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
	ConstantPool := []constant_pool.ConstantInfo{}
	memberInfo := []*MemberInfo{}
	Attributes := []attribute_info.AttributeInfo{}
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
			d := &attribute_info.CodeAttribute{}
			Attributes_bak := Attributes
			Attributes = []attribute_info.AttributeInfo{}
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
		// case ConstantValueAttributeType:
		// 	d := &attribute_info.ConstantValueAttribute{}
		// 	ConstantPool = append(ConstantPool, d)
		// 	err := mapstructure.Decode(objData, d)
		// 	if err != nil {
		// 		return err
		// 	}
		case DeprecatedAttributeType:
			d := &attribute_info.DeprecatedAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ExceptionsAttributeType:
			d := &attribute_info.ExceptionsAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case LineNumberTableAttributeType:
			d := &attribute_info.LineNumberTableAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case SourceFileAttributeType:
			d := &attribute_info.SourceFileAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case SyntheticAttributeType:
			d := &attribute_info.SyntheticAttribute{}
			Attributes = append(Attributes, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case UnparsedAttributeType:
			d := &attribute_info.UnparsedAttribute{}
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
			d := &constant_pool.ConstantIntegerInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantFloat:
			d := &constant_pool.ConstantFloatInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantLong:
			d := &constant_pool.ConstantLongInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantDouble:
			d := &constant_pool.ConstantDoubleInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantUtf8:
			d := &constant_pool.ConstantUtf8Info{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantString:
			d := &constant_pool.ConstantStringInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantClass:
			d := &constant_pool.ConstantClassInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantFieldref:
			d := &constant_pool.ConstantFieldrefInfo{ConstantMemberrefInfo: constant_pool.ConstantMemberrefInfo{}, Type: ConstantFieldref}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, &d.ConstantMemberrefInfo)
			if err != nil {
				return err
			}
		case ConstantMethodref:
			d := &constant_pool.ConstantMethodrefInfo{ConstantMemberrefInfo: constant_pool.ConstantMemberrefInfo{}, Type: ConstantMethodref}

			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, &d.ConstantMemberrefInfo)
			if err != nil {
				return err
			}
		case ConstantInterfaceMethodref:
			d := &constant_pool.ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: constant_pool.ConstantMemberrefInfo{}, Type: ConstantInterfaceMethodref}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, &d.ConstantMemberrefInfo)
			if err != nil {
				return err
			}
		case ConstantNameAndType:
			d := &constant_pool.ConstantNameAndTypeInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantMethodType:
			d := &constant_pool.ConstantMethodTypeInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantMethodHandle:
			d := &constant_pool.ConstantMethodHandleInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case ConstantInvokeDynamic:
			d := &constant_pool.ConstantInvokeDynamicInfo{}
			ConstantPool = append(ConstantPool, d)
			err := mapstructure.Decode(objData, d)
			if err != nil {
				return err
			}
		case MemberInfoType:
			d := &MemberInfo{}
			memberInfo = append(memberInfo, d)
			Attributes_bak := Attributes
			Attributes = []attribute_info.AttributeInfo{}
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
