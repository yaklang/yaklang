package yso

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yserx"
	"reflect"
	"runtime"
	"strings"
	"text/template"
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

func GetGadgetNameByFun(i interface{}) (string, error) {
	name := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
	if strings.Contains(name, ".") {
		name = strings.Split(name, ".")[1]
		if utils.MatchAllOfGlob(name, "Get*JavaObject") {
			l := len(name)
			return name[3 : l-10], nil
		}
	}
	return "", utils.Error("not found gadget name")
}

func SetJavaObjectClass(object yserx.JavaSerializable, classObject *javaclassparser.ClassObject) error {
	var tmpl *yserx.JavaObject
	WalkJavaSerializableObject(object, func(desc1 *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
		if desc1 == nil {
			return
		}
		if desc1.Detail.ClassName == "com.sun.org.apache.xalan.internal.xsltc.trax.TemplatesImpl" {
			tmpl = objSer.(*yserx.JavaObject)
		}
	})
	if tmpl == nil {
		return utils.Error("Not found template")
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

func CreateTemplateByClassObject(class *javaclassparser.ClassObject) *yserx.JavaObject {
	obj, err := yserx.ParseFromBytes(template_templateObjectSer)
	if err != nil {
		log.Error(err)
	}
	byts := class.Bytes()
	obj.ClassData[1].(*yserx.JavaClassData).Fields[2].(*yserx.JavaFieldValue).Object.(*yserx.JavaArray).Values[0].Object.(*yserx.JavaArray).Bytes = byts
	obj.ClassData[1].(*yserx.JavaClassData).Fields[2].(*yserx.JavaFieldValue).Object.(*yserx.JavaArray).Values[0].Object.(*yserx.JavaArray).Size = len(byts)
	return obj
}

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
				if len(arrObj.Values) != 2 {
					return utils.Error("template struct not match")
				}
				emptyClass, err := GenEmptyClassInTemplateClassObject()
				if err != nil {
					return err
				}
				arrObj.Values[0] = yserx.NewJavaFieldBytes(string(classBytes))
				arrObj.Values[1] = yserx.NewJavaFieldBytes(string(emptyClass.Bytes()))
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
//
//		}
//	} else {
//		return nil, utils.Error("not a java object")
//	}
//	return nil, utils.Error("not found template")
//}

type WalkJavaSerializableObjectHandle func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable)

func ReplaceStringInJavaSerilizable(objSer yserx.JavaSerializable, old string, new string, times int) error {
	err := utils.Error("not found string in java object")
	WalkJavaSerializableObject(objSer, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
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
	})
	return err
}

// ReplaceClassNameInJavaSerilizable 这个 ClassName 指的是要探测的目标 jar 包里是否存在该 ClassName
func ReplaceClassNameInJavaSerilizable(objSer yserx.JavaSerializable, old string, new string, times int) error {
	err := utils.Error("not found class name in java object")
	WalkJavaSerializableObject(objSer, func(desc *yserx.JavaClassDesc, objSer yserx.JavaSerializable) {
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

func WalkJavaSerializableObject(objSer yserx.JavaSerializable, handle WalkJavaSerializableObjectHandle) *JavaStruct {
	handleTable := map[uint64]yserx.JavaSerializable{}
	root := &JavaStruct{Name: "root"}
	structHandleTable := make(map[uint64]*JavaStruct)
	_WalkJavaSerializableObject(objSer, handleTable, handle, root, structHandleTable)
	return root
}
func _WalkJavaSerializableObject(objSer yserx.JavaSerializable, handleTable map[uint64]yserx.JavaSerializable, handle WalkJavaSerializableObjectHandle, node *JavaStruct, structHandleTable map[uint64]*JavaStruct) {
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
		for i, value := range ret.Values {
			//if value.Object
			v[i] = &JavaStruct{}
			_WalkJavaSerializableObject(value.Object, handleTable, handle, v[i], structHandleTable)
		}
		return
	case *yserx.JavaString:
		node.Type = ret.Type
		node.TypeVerbose = ret.TypeVerbose
		node.Value = ret.Value
		handle(nil, ret)
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
		handle(nil, ret)
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
	handle(desc, obj)
	if obj.Type == yserx.TC_OBJECT {
		for _, i := range obj.ClassData {
			classData, ok := i.(*yserx.JavaClassData)
			if !ok {
				continue
			}
			for _, b := range classData.BlockData {
				newNode := &JavaStruct{}
				node.BlockData = append(node.BlockData, newNode)
				_WalkJavaSerializableObject(b, handleTable, handle, newNode, structHandleTable)
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
						_WalkJavaSerializableObject(field.Object, handleTable, handle, node.Fields[fieldIndex-1], structHandleTable)
					} else {
						handle(nil, field)
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
func cmdToTemplate(cmd string, tempers ...Temper) (map[string]interface{}, error) {
	for _, r := range tempers {
		if r == nil {
			continue
		}
		cmd = r(cmd)
	}
	raw, err := json.Marshal(cmd)
	if err != nil {
		return nil, err
	}

	rawBase64, err := json.Marshal(codec.EncodeBase64(cmd))
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"Length":     len(cmd),
		"Command":    string(raw),
		"CommandRaw": string(rawBase64),
	}
	return result, nil
}

func cmdToTemplateImpl(cmd string, tempers []Temper, suffix ...string) (map[string]interface{}, error) {
	for _, r := range tempers {
		if r == nil {
			continue
		}
		cmd = r(cmd)
	}
	raw, err := yserx.ToJson(generateTemplates(cmd))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"TemplatesImpl": string(raw) + strings.Join(suffix, ""),
	}, nil
}

func buildTemplate(name string, raw string) (*template.Template, error) {
	templ, err := template.New(name).Parse(raw)
	if err != nil {
		return nil, err
	}

	return templ, nil
}

func createTemplatesImplGadgetFactory(name string, tmp string, temper []Temper, suffix ...string) GadgetFunc {
	return func(cmd string) (yserx.JavaSerializable, error) {
		params, err := cmdToTemplateImpl(cmd, temper, suffix...)
		if err != nil {
			return nil, err
		}

		tmp, err := buildTemplate(name, tmp)
		if err != nil {
			return nil, err
		}

		var buf bytes.Buffer
		err = tmp.Execute(&buf, params)
		if err != nil {
			return nil, err
		}

		r, err := yserx.FromJson(buf.Bytes())
		if err != nil {
			return nil, err
		}

		if len(r) > 0 {
			return r[0], nil
		}
		return nil, utils.Error("empty java serialization object")
	}
}

func createOrdinaryGadgetFactory(name string, tmp string, tempers ...Temper) GadgetFunc {
	return func(cmd string) (yserx.JavaSerializable, error) {
		var buf bytes.Buffer
		s, err := cmdToTemplate(cmd, tempers...)
		if err != nil {
			return nil, err
		}

		tmp, err := buildTemplate(name, tmp)
		if err != nil {
			return nil, err
		}

		err = tmp.Execute(&buf, s)
		if err != nil {
			return nil, err
		}

		rs, err := yserx.FromJson((&buf).Bytes())
		if err != nil {
			return nil, err
		}

		if len(rs) > 0 {
			return rs[0], nil
		}
		return nil, utils.Error("generate common collections failed: empty")
	}
}

func ToBcel(i interface{}) (string, error) {
	switch ret := i.(type) {
	case *javaclassparser.ClassObject:
		return ret.Bcel()
	default:
		return "", utils.Errorf("cannot support %v to bcel string", reflect.TypeOf(ret))
	}
}
func ToBytes(i interface{}) ([]byte, error) {
	switch ret := i.(type) {
	case *javaclassparser.ClassObject:
		return ret.Bytes(), nil
	case yserx.JavaSerializable:
		return yserx.MarshalJavaObjects(ret), nil
	default:
		return nil, utils.Errorf("cannot support %v to bytes", reflect.TypeOf(ret))
	}
}
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
	node := WalkJavaSerializableObject(serializableObj, func(desc *yserx.JavaClassDesc, obj yserx.JavaSerializable) {

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
