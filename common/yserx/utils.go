package yserx

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/yak/yaklib/codec"
)

func addPrefixToLine(r string, indent int) string {
	var lines []string
	for _, line := range utils.ParseStringToLines(r) {
		lines = append(lines, strings.Repeat(INDENT, indent)+line)
	}
	return strings.Join(lines, "\n") + "\n"
}

func ToJson(i interface{}) ([]byte, error) {
	return json.MarshalIndent(i, "", "  ")
}

func marshalString(str string) []byte {
	return append(IntTo2Bytes(len(str)), []byte(str)...)
}

// utils method
func ReadBytesLength(r *bufio.Reader, length uint64) ([]byte, error) {
	var buf = make([]byte, length)
	n, err := io.ReadAtLeast(r, buf, int(length))
	//n, err := r.Read(buf)
	if err != nil {
		return buf[:], err
	}
	_ = n

	if uint64(n) < length {
		return buf[:], utils.Errorf("yserx readBytes len current length[%v] is not [%v]: 0x%v", n, length, codec.EncodeToHex(buf))
	}
	return buf[:], nil
}

func ReadBytesLengthInt(r *bufio.Reader, length int) ([]byte, error) {
	return ReadBytesLength(r, uint64(length))
}

func Read2ByteToInt(r *bufio.Reader) (int, error) {
	raw, err := ReadBytesLength(r, 2)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}

	raw = append(bytes.Repeat([]byte{0x00}, 6), raw...)
	i := binary.BigEndian.Uint64(raw)
	return int(i), nil
}

func ReadByteToInt(r *bufio.Reader) (int, error) {
	raw, err := ReadBytesLength(r, 1)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}

	raw = append(bytes.Repeat([]byte{0x00}, 7), raw...)
	i := binary.BigEndian.Uint64(raw)
	return int(i), nil
}

func Read4ByteToInt(r *bufio.Reader) (int, error) {
	i, err := Read4ByteToUint64(r)
	if err != nil {
		return 0, err
	}
	return int(i), nil
}

func Read4ByteToUint64(r *bufio.Reader) (uint64, error) {
	raw, err := ReadBytesLength(r, 4)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}

	raw = append(bytes.Repeat([]byte{0x00}, 4), raw...)
	return binary.BigEndian.Uint64(raw), nil
}

func Read8BytesToUint64(r *bufio.Reader) (uint64, error) {
	raw, err := ReadBytesLength(r, 8)
	if err != nil {
		return 0, utils.Errorf("read bytes length failed: %s", err)
	}
	return binary.BigEndian.Uint64(raw), nil
}

func IntTo2Bytes(i int) []byte {
	var buf = make([]byte, 2)
	buf[0] = byte(i >> 8)
	buf[1] = byte(i)
	return buf[:]
}

func IntToByte(i int) []byte {
	var buf = make([]byte, 1)
	buf[0] = byte(i)
	return buf[:]
}

func IntTo4Bytes(i int) []byte {
	var buf = make([]byte, 4)
	buf[0] = byte(i >> 24)
	buf[1] = byte(i >> 16)
	buf[2] = byte(i >> 8)
	buf[3] = byte(i)
	return buf[:]
}

func Uint64To4Bytes(i uint64) []byte {
	var buf = make([]byte, 4)
	buf[0] = byte(i >> 24)
	buf[1] = byte(i >> 16)
	buf[2] = byte(i >> 8)
	buf[3] = byte(i)
	return buf[:]
}

func Uint64To8Bytes(i uint64) []byte {
	var buf = make([]byte, 8)
	buf[0] = byte(i >> 56)
	buf[1] = byte(i >> 48)
	buf[2] = byte(i >> 40)
	buf[3] = byte(i >> 32)
	buf[4] = byte(i >> 24)
	buf[5] = byte(i >> 16)
	buf[6] = byte(i >> 8)
	buf[7] = byte(i)
	return buf[:]
}

func Float32To4Byte(float float32) []byte {
	bits := math.Float32bits(float)
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, bits)

	return bytes
}

func Float64To8Byte(float float64) []byte {
	bits := math.Float64bits(float)
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, bits)

	return bytes
}

func (p *JavaSerializationParser) ShowByte(raw []byte) {
	b := raw[0]
	if b > 0x20 && b <= 0x7e {
		p.debug("(byte) 0x%02x (ASCII: %v)", b, string(raw))
	} else {
		p.debug("(byte) 0x%02x (ASCII: %v)", b, strconv.Quote(string(raw)))
	}
}

func (p *JavaSerializationParser) ShowChar(raw []byte) {
	p.debug("(char) 0x%v (ASCII: %v)", codec.EncodeToHex(raw), strconv.Quote(string(raw)))
}

func (p *JavaSerializationParser) ShowDouble(raw []byte) {
	if len(raw) == 8 {
		value, _ := Read8BytesToUint64(bufio.NewReader(bytes.NewBuffer(raw)))
		p.debug("(double) 0x%v (Value: %v)", codec.EncodeToHex(raw), float32(value))
	} else {
		p.debug("(double) 0x%v (ASCII: %v)", codec.EncodeToHex(raw), strconv.Quote(string(raw)))
	}
}

func (p *JavaSerializationParser) ShowFloat(raw []byte) {
	if len(raw) == 4 {
		value, _ := Read4ByteToInt(bufio.NewReader(bytes.NewBuffer(raw)))
		p.debug("(float) 0x%v (Value: %v)", codec.EncodeToHex(raw), float32(value))
	} else {
		p.debug("(float) 0x%v (ASCII: %v)", codec.EncodeToHex(raw), strconv.Quote(string(raw)))
	}
}

func (p *JavaSerializationParser) ShowInt(raw []byte) {
	i, _ := Read4ByteToInt(bufio.NewReader(bytes.NewBuffer(raw)))
	p.debug("(int) 0x%v (INT: %v)", codec.EncodeToHex(raw), i)
}

func (p *JavaSerializationParser) ShowLong(raw []byte) {
	i, _ := Read8BytesToUint64(bufio.NewReader(bytes.NewBuffer(raw)))
	p.debug("(long) 0x%v (INT: %v)", codec.EncodeToHex(raw), i)
}

func (p *JavaSerializationParser) ShowShort(raw []byte) {
	i, _ := Read2ByteToInt(bufio.NewReader(bytes.NewBuffer(raw)))
	p.debug("(long) 0x%v (INT: %v)", codec.EncodeToHex(raw), i)
}

func (p *JavaSerializationParser) ShowBool(raw []byte) {
	p.debug("(long) 0x%v (BOOL: %v)", codec.EncodeToHex(raw), strconv.Quote(string(raw)))
}

type _middleSerialization struct {
	TypeVerbose string `json:"type_verbose"`
}

func _getBytes(m map[string]json.RawMessage, key string) []byte {
	o, ok := m[key]
	if !ok {
		return nil
	}
	return o
}
func _rawIdentToJavaSerializable(raw []byte) (JavaSerializable, error) {
	var mj _middleSerialization
	err := json.Unmarshal(raw, &mj)
	if err != nil {
		return nil, err
	}
	var m = make(map[string]json.RawMessage)
	err = json.Unmarshal(raw, &m)
	if err != nil {
		return nil, err
	}

	haveIsEmptyField := func() bool {
		if m == nil {
			return false
		}
		if isEmpty, ok := m["is_empty"]; ok {
			var b bool
			_ = json.Unmarshal(isEmpty, &b)
			return b
		}
		return false
	}

	switch mj.TypeVerbose {
	case "TC_NULL":
		nullObj := NewJavaNull()
		nullObj.IsEmpty = haveIsEmptyField()
		return nullObj, nil
	case "TC_STRING":
		var n JavaString
		err := json.Unmarshal(raw, &n)
		if err != nil {
			return nil, err
		}
		if n.Value != "" {
			n.Raw = []byte(n.Value)
		}
		return &n, nil
	case "TC_PROXYCLASSDESC":
		fallthrough
	case "TC_CLASSDESC":
		j := &JavaClassDetails{}
		if mj.TypeVerbose == "TC_PROXYCLASSDESC" {
			j.DynamicProxyClass = true
		}

		err = json.Unmarshal(_getBytes(m, "class_name"), &j.ClassName)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(_getBytes(m, "serial_version"), &j.SerialVersion)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(_getBytes(m, "handle"), &j.Handle)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(_getBytes(m, "desc_flag"), &j.DescFlag)
		if err != nil {
			return nil, err
		}

		obj, err := _rawIdentToJavaSerializable(_getBytes(m, "fields"))
		if err != nil {
			return nil, err
		}
		var ok bool
		j.Fields, ok = obj.(*JavaClassFields)
		if !ok {
			return nil, utils.Errorf("parse TC_CLASSDESC/PROXYCLASSDESC's fields failed: obj[%v]", reflect.TypeOf(obj))
		}

		// general super class
		j.SuperClass, err = _rawIdentToJavaSerializable(_getBytes(m, "super_class"))
		if err != nil {
			return nil, err
		}

		if !j.DynamicProxyClass {
			var annotations []json.RawMessage
			err = json.Unmarshal(_getBytes(m, "annotations"), &annotations)
			if err != nil {
				return nil, err
			}
			for _, a := range annotations {
				obj, err := _rawIdentToJavaSerializable(a)
				if err != nil {
					return nil, err
				}
				j.Annotations = append(j.Annotations, obj)
			}

		} else {
			var proxyAnnotations []json.RawMessage
			err = json.Unmarshal(_getBytes(m, "dynamic_proxy_annotation"), &proxyAnnotations)
			if err != nil {
				return nil, err
			}
			for _, a := range proxyAnnotations {
				obj, err := _rawIdentToJavaSerializable(a)
				if err != nil {
					return nil, err
				}
				j.DynamicProxyAnnotation = append(j.DynamicProxyAnnotation, obj)
			}
			err = json.Unmarshal(_getBytes(m, "dynamic_proxy_class_interface_names"), &j.DynamicProxyClassInterfaceNames)
			if err != nil {
				return nil, err
			}
			j.DynamicProxyClassInterfaceCount = len(j.DynamicProxyClassInterfaceNames)
		}
		return j, nil
	case "TC_BLOCKDATA":
		var j JavaBlockData
		err = json.Unmarshal(raw, &j)
		if err != nil {
			return nil, err
		}
		return &j, nil
	case "X_CLASSDESC":
		obj, err := _rawIdentToJavaSerializable(_getBytes(m, "detail"))
		if err != nil {
			return nil, err
		}
		details, ok := obj.(*JavaClassDetails)
		if !ok {
			return nil, utils.Errorf("javaClassDesc need javaClassDetails but got: %v", reflect.TypeOf(obj))
		}
		j := &JavaClassDesc{}
		j.SetDetails(details)
		desc, ok := details.SuperClass.(*JavaClassDetails)
		_ = desc
		//if ok {
		//	j.AddSuperClassDetails(desc)
		//}
		return j, nil
	case "X_CLASSFIELD":
		j := &JavaClassField{}
		json.Unmarshal(_getBytes(m, "name"), &j.Name)
		json.Unmarshal(_getBytes(m, "field_type"), &j.FieldType)
		json.Unmarshal(_getBytes(m, "field_type_verbose"), &j.FieldTypeVerbose)
		j.ClassName1, err = _rawIdentToJavaSerializable(_getBytes(m, "class_name_1"))
		if err != nil {
			return nil, err
		}
		return j, nil
	case "X_CLASSFIELDS":
		var objs []json.RawMessage
		j := &JavaClassFields{}
		err = json.Unmarshal(_getBytes(m, "fields"), &objs)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			newObj, err := _rawIdentToJavaSerializable(o)
			if err != nil {
				return nil, err
			}
			field, ok := newObj.(*JavaClassField)
			if !ok {
				return nil, utils.Errorf("need javaClassField but got: %v", reflect.TypeOf(newObj))
			}
			j.Fields = append(j.Fields, field)
		}
		j.FieldCount = len(j.Fields)
		return j, nil
	case "TC_ENUM":
		j := &JavaEnumDesc{}
		err = json.Unmarshal(_getBytes(m, "handle"), &j.Handle)
		if err != nil {
			return nil, err
		}
		j.TypeClassDesc, err = _rawIdentToJavaSerializable(_getBytes(m, "type_class_desc"))
		if err != nil {
			return nil, err
		}
		j.ConstantName, err = _rawIdentToJavaSerializable(_getBytes(m, "constant_name"))
		if err != nil {
			return nil, err
		}
		return j, nil
	case "TC_ARRAY":
		j := &JavaArray{}
		j.ClassDesc, err = _rawIdentToJavaSerializable(_getBytes(m, "class_desc"))
		err = json.Unmarshal(_getBytes(m, "size"), &j.Size)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(_getBytes(m, "handle"), &j.Handle)
		if err != nil {
			return nil, err
		}

		//	这儿有点特殊，如果是 TC_ARRAY 需要针对 [B 的 bytescode 做优化，不然大的离谱了
		_ = json.Unmarshal(_getBytes(m, "bytescode"), &j.Bytescode)
		if j.Bytescode {
			err = json.Unmarshal(_getBytes(m, "bytes"), &j.Bytes)
			if err != nil {
				return nil, err
			}
			//j.Values = funk.Map(j.Bytes, func(i byte) *JavaFieldValue {
			//	f := NewJavaFieldByteValue(i)
			//	initTCType(f)
			//	return f
			//}).([]*JavaFieldValue)
			return j, nil
		}

		var objs []json.RawMessage
		err = json.Unmarshal(_getBytes(m, "values"), &objs)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			obj, err := _rawIdentToJavaSerializable(o)
			if err != nil {
				return nil, err
			}
			f, ok := obj.(*JavaFieldValue)
			if !ok {
				return nil, utils.Errorf("need *JavaFieldValue but got: %v", reflect.TypeOf(obj))
			}
			j.Values = append(j.Values, f)
		}
		return j, nil
	case "TC_REFERENCE":
		var j JavaReference
		err := json.Unmarshal(raw, &j)
		if err != nil {
			return nil, err
		}
		return &j, nil
	case "TC_OBJECT":
		var j = &JavaObject{}
		j.Class, err = _rawIdentToJavaSerializable(_getBytes(m, "class_desc"))
		if err != nil {
			return nil, err
		}
		var objs []json.RawMessage
		err = json.Unmarshal(_getBytes(m, "class_data"), &objs)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			obj, err := _rawIdentToJavaSerializable(o)
			if err != nil {
				return nil, err
			}
			j.ClassData = append(j.ClassData, obj)
		}
		err = json.Unmarshal(_getBytes(m, "handle"), &j.Handle)
		if err != nil {
			return nil, err
		}
		return j, nil
	case "X_CLASSDATA":
		j := &JavaClassData{}
		var objs []json.RawMessage
		err = json.Unmarshal(_getBytes(m, "fields"), &objs)
		if err != nil {
			return nil, err
		}
		for _, o := range objs {
			obj, err := _rawIdentToJavaSerializable(o)
			if err != nil {
				return nil, err
			}
			j.Fields = append(j.Fields, obj)
		}
		err = json.Unmarshal(_getBytes(m, "fields"), &objs)
		if err != nil {
			return nil, err
		}

		objs = nil
		err = json.Unmarshal(_getBytes(m, "block_data"), &objs)
		for _, o := range objs {
			obj, err := _rawIdentToJavaSerializable(o)
			if err != nil {
				return nil, err
			}
			j.BlockData = append(j.BlockData, obj)
		}
		return j, nil
	case "X_FIELDVALUE":
		j := &JavaFieldValue{}
		err = json.Unmarshal(_getBytes(m, "field_type"), &j.FieldType)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(_getBytes(m, "field_type_verbose"), &j.FieldTypeVerbose)
		if err != nil {
			return nil, err
		}
		if j.FieldType == JT_OBJECT || j.FieldType == JT_ARRAY {
			j.Object, err = _rawIdentToJavaSerializable(_getBytes(m, "object"))
			if err != nil {
				return nil, err
			}
		} else {
			err = json.Unmarshal(_getBytes(m, "bytes"), &j.Bytes)
			if err != nil {
				return nil, err
			}
		}
		return j, nil
	case "TC_CLASS":
		j := &JavaClass{}
		err = json.Unmarshal(_getBytes(m, "handle"), &j.Handle)
		if err != nil {
			return nil, err
		}
		j.Desc, err = _rawIdentToJavaSerializable(_getBytes(m, "class_desc"))
		if err != nil {
			return nil, err
		}
		return j, nil
	case "TC_ENDBLOCKDATA":
		endblock := NewJavaEndBlockData()
		endblock.IsEmpty = haveIsEmptyField()
		return endblock, nil
	default:
		if bytes.Equal([]byte("null"), raw) {
			return nil, nil
		}
		log.Infof("parse bytes to JavaSerializable failed: %v", string(raw))
		return nil, utils.Errorf("unsupported type: %v", mj.TypeVerbose)
	}
}

func FromJson(raw []byte) ([]JavaSerializable, error) {
	var objs []json.RawMessage
	_ = json.Unmarshal(raw, &objs)
	if len(objs) > 0 {
		var serls []JavaSerializable
		for _, raw := range objs {
			o, err := _rawIdentToJavaSerializable(raw)
			if err != nil {
				return nil, err
			}
			initTCType(o)
			serls = append(serls, o)
		}
		return serls, nil
	}

	o, err := _rawIdentToJavaSerializable(raw)
	if err != nil {
		return nil, err
	}
	initTCType(o)
	return []JavaSerializable{o}, nil
}

func initTCType(j JavaSerializable) {
	switch ret := j.(type) {
	case *JavaNull:
		ret.Type = TC_NULL
		ret.TypeVerbose = tcToVerbose(TC_NULL)
	case *JavaString:
		if ret.IsLong {
			ret.Type = TC_LONGSTRING
			ret.TypeVerbose = tcToVerbose(TC_LONGSTRING)
		} else {
			ret.Type = TC_STRING
			ret.TypeVerbose = tcToVerbose(TC_STRING)
		}
	case *JavaClassDetails:
		if ret.IsJavaNull() {
			ret = &JavaClassDetails{}
			ret.TypeVerbose = tcToVerbose(TC_NULL)
			ret.Type = TC_NULL
		} else {
			if ret.DynamicProxyClass {
				ret.TypeVerbose = tcToVerbose(TC_PROXYCLASSDESC)
				ret.Type = TC_PROXYCLASSDESC
			} else {
				ret.TypeVerbose = tcToVerbose(TC_CLASSDESC)
				ret.Type = TC_CLASSDESC
			}
		}

		if ret.Fields == nil {
			ret.Fields = NewJavaClassFields()
		}
		initTCType(ret.Fields)
		initTCType(ret.SuperClass)

		if ret.DynamicProxyClass {
			for _, i := range ret.DynamicProxyAnnotation {
				initTCType(i)
			}
		} else {
			for _, i := range ret.Annotations {
				initTCType(i)
			}
		}
	case *JavaClass:
		ret.TypeVerbose = tcToVerbose(TC_CLASS)
		ret.Type = TC_CLASS
		initTCType(ret.Desc)
	case *JavaBlockData:
		if ret.IsLong {
			ret.TypeVerbose = tcToVerbose(TC_BLOCKDATALONG)
			ret.Type = TC_BLOCKDATALONG
		} else {
			ret.TypeVerbose = tcToVerbose(TC_BLOCKDATA)
			ret.Type = TC_BLOCKDATA
		}
	case *JavaClassDesc:
		ret.TypeVerbose = "X_CLASSDESC"
		initTCType(ret.Detail)
	case *JavaClassField:
		ret.TypeVerbose = "X_CLASSFIELD"
		initTCType(ret.ClassName1)
	case *JavaClassFields:
		ret.TypeVerbose = "X_CLASSFIELDS"
		for _, i := range ret.Fields {
			initTCType(i)
		}
	case *JavaEnumDesc:
		ret.TypeVerbose = tcToVerbose(TC_ENUM)
		ret.Type = TC_ENUM
		initTCType(ret.ConstantName)
		initTCType(ret.TypeClassDesc)
	case *JavaArray:
		ret.TypeVerbose = tcToVerbose(TC_ARRAY)
		ret.Type = TC_ARRAY
		initTCType(ret.ClassDesc)

		for _, v := range ret.Values {
			initTCType(v)
		}
	case *JavaReference:
		ret.TypeVerbose = tcToVerbose(TC_REFERENCE)
		ret.Type = TC_REFERENCE
	case *JavaObject:
		ret.TypeVerbose = tcToVerbose(TC_OBJECT)
		ret.Type = TC_OBJECT
		for _, d := range ret.ClassData {
			initTCType(d)
		}
		initTCType(ret.Class)
	case *JavaClassData:
		ret.TypeVerbose = "X_CLASSDATA"
		for _, f := range ret.Fields {
			initTCType(f)
		}
		for _, b := range ret.BlockData {
			initTCType(b)
		}
	case *JavaFieldValue:
		ret.TypeVerbose = "X_FIELDVALUE"
		initTCType(ret.Object)
	case *JavaEndBlockData:
		ret.TypeVerbose = tcToVerbose(TC_ENDBLOCKDATA)
		ret.Type = TC_ENDBLOCKDATA
	}
}
