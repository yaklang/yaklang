package yserx

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"io"
	"reflect"
	"strconv"
	"strings"
)

// https://docs.oracle.com/javase/7/docs/platform/serialization/spec/protocol.html
// https://docs.oracle.com/en/java/javase/17/docs/specs/serialization/protocol.html
// https://github.com/NickstaDB/SerializationDumper/blob/49dbece69f0b8230aaed4a0c50ca56fecc9376c0/src/nb/deser/SerializationDumper.java

// first class
const (
	TC_OBJECT         byte = 0x73 // s
	TC_CLASS          byte = 0x76 // v
	TC_ARRAY          byte = 0x75 // u
	TC_STRING         byte = 0x74 // t
	TC_LONGSTRING     byte = 0x7c // |
	TC_ENUM           byte = 0x7e // ~
	TC_CLASSDESC      byte = 0x72 // r
	TC_PROXYCLASSDESC byte = 0x7d // }
	TC_EXCEPTION      byte = 0x7b // { - ignore
	TC_RESET          byte = 0x79 // y - ignore
	TC_REFERENCE      byte = 0x71 // q
	TC_NULL           byte = 0x70 // p
	TC_BLOCKDATA      byte = 0x77 // w
	TC_BLOCKDATALONG  byte = 0x7a // z
	TC_ENDBLOCKDATA   byte = 0x78 // x
	TC_UNKNOWN        byte = 0    // 0x00
	X_CLASSDESC       byte = 0
	X_CLASSFIELD      byte = 0
	X_CLASSFIELDS     byte = 0
	X_CLASSDATA       byte = 0
	X_FIELDVALUE      byte = 0
)

func tcVerboseToByte(i string) byte {
	switch i {
	case "TC_OBJECT":
		return TC_OBJECT
	case "TC_CLASS":
		return TC_CLASS
	case "TC_ARRAY":
		return TC_ARRAY
	case "TC_STRING":
		return TC_STRING
	case "TC_LONGSTRING":
		return TC_LONGSTRING
	case "TC_ENUM":
		return TC_ENUM
	case "TC_CLASSDESC":
		return TC_CLASSDESC
	case "TC_PROXYCLASSDESC":
		return TC_PROXYCLASSDESC
	case "TC_EXCEPTION":
		return TC_EXCEPTION
	case "TC_RESET":
		return TC_RESET
	case "TC_REFERENCE":
		return TC_REFERENCE
	case "TC_NULL":
		return TC_NULL
	case "TC_BLOCKDATA":
		return TC_BLOCKDATA
	case "TC_BLOCKDATALONG":
		return TC_BLOCKDATALONG
	case "TC_ENDBLOCKDATA":
		return TC_ENDBLOCKDATA
	default:
		return TC_UNKNOWN
	}
}

func tcToVerbose(n byte) string {
	switch n {
	case TC_OBJECT:
		return `TC_OBJECT`
	case TC_CLASS:
		return `TC_CLASS`
	case TC_ARRAY:
		return `TC_ARRAY`
	case TC_STRING:
		return `TC_STRING`
	case TC_LONGSTRING:
		return `TC_LONGSTRING`
	case TC_ENUM:
		return `TC_ENUM`
	case TC_CLASSDESC:
		return `TC_CLASSDESC`
	case TC_PROXYCLASSDESC:
		return `TC_PROXYCLASSDESC`
	case TC_EXCEPTION:
		return `TC_EXCEPTION`
	case TC_RESET:
		return `TC_RESET`
	case TC_REFERENCE:
		return `TC_REFERENCE`
	case TC_NULL:
		return `TC_NULL`
	case TC_BLOCKDATA:
		return `TC_BLOCKDATA`
	case TC_BLOCKDATALONG:
		return `TC_BLOCKDATALONG`
	case TC_ENDBLOCKDATA:
		return `TC_ENDBLOCKDATA`
	}
	return fmt.Sprintf("TC_UNKNOWN(0x%02x)", n)
}

const (
	JT_BYTE   byte = 'B'
	JT_CHAR   byte = 'C'
	JT_DOUBLE byte = 'D'
	JT_FLOAT  byte = 'F'
	JT_INT    byte = 'I'
	JT_LONG   byte = 'J'
	JT_SHORT  byte = 'S'
	JT_BOOL   byte = 'Z'
	JT_ARRAY  byte = '['
	JT_OBJECT byte = 'L'
)

func jtToVerbose(r byte) string {
	switch r {
	case JT_BYTE:
		return "byte"
	case JT_OBJECT:
		return "object"
	case JT_DOUBLE:
		return "double"
	case JT_FLOAT:
		return "float"
	case JT_INT:
		return "int"
	case JT_LONG:
		return "long"
	case JT_SHORT:
		return "short"
	case JT_BOOL:
		return "boolean"
	case JT_ARRAY:
		return "array"
	case JT_CHAR:
		return "char"
	default:
		return "[N/A]"
	}
}

/*******************
 * Read a classDesc from the data stream.
 *
 * Could be:
 *	TC_CLASSDESC		(0x72)
 *	TC_PROXYCLASSDESC	(0x7d)
 *	TC_NULL				(0x70)
 *	TC_REFERENCE		(0x71)
 ******************/
func (p *JavaSerializationParser) readClassDesc(r io.Reader) (JavaSerializable, error) {
	raw, err := ReadByte(r)
	if err != nil {
		if err == io.EOF {
			n := NewJavaNull()
			n.IsEmpty = true
			return n, nil
		}
		return nil, err
	}
	switch raw[0] {
	case TC_CLASSDESC:
		return p.readTC_CLASSDESC(r)
	case TC_PROXYCLASSDESC:
		return p.readTC_PROXYCLASSDESC(r)
	case TC_NULL:
		return p.readTC_NULL(r)
	case TC_REFERENCE:
		refHandler, err := p.readPrevObject(r)
		return refHandler, err
		//if err != nil {
		//	return nil, err
		//}
		//refHandleValue := refHandler.GetHandle()
		//for _, desc := range p.ClassDescriptions {
		//	for index, details := range desc.Items {
		//		if details.Handle == refHandleValue {
		//			descGroup, err := desc.GetDescByIndex(index)
		//			if err != nil {
		//				return nil, err
		//			}
		//			descGroup.FromJavaReference = true
		//			return descGroup, nil
		//		}
		//	}
		//}
		//return nil, utils.Errorf("cannot found handler[0x%v] from existed class details", codec.EncodeToHex(refHandler.Value))
	default:
		return nil, utils.Errorf("unsupported classDesc: %v", codec.EncodeToHex(raw))
	}
}

/*******************
 * Read a newClassDesc from the stream.
 *
 * Could be:
 *	TC_CLASSDESC		(0x72)
 *	TC_PROXYCLASSDESC	(0x7d)
 ******************/
func (p *JavaSerializationParser) readNewClassDesc(r io.Reader) (*JavaClassDesc, error) {
	raw, err := ReadByte(r)
	if err != nil {
		return nil, err
	}
	switch raw[0] {
	case TC_CLASSDESC:
		return p.readTC_CLASSDESC(r)
	case TC_PROXYCLASSDESC:
		return p.readTC_PROXYCLASSDESC(r)
	default:
		return nil, utils.Errorf("read new class desc failed")
	}
}

/*******************
 * Read a TC_PROXYCLASSDESC from the stream.
 *
 * TC_PROXYCLASSDESC	newHandle	proxyClassDescInfo
 ******************/
func (p *JavaSerializationParser) readTC_PROXYCLASSDESC(r io.Reader) (*JavaClassDesc, error) {
	g := &JavaClassDesc{}
	p.ClassDescriptions = append(p.ClassDescriptions, g)

	c := newJavaClassDetails()
	g.SetDetails(c)
	c.DynamicProxyClass = true

	p.increaseIndent()
	defer p.decreaseIndent()

	c.Handle = p.newHandler("->", "TC_PROXYCLASSDESC")
	p.ClassDetails[c.Handle] = c

	err := p.readProxyClassDescInfo(r, g)
	if err != nil {
		return nil, err
	}
	return g, nil
}

/*******************
 * Read a proxyClassDescInfo from the stream.
 *
 * (int)count	(utf)proxyInterfaceName[count]	classAnnotation		superClassDesc
 ******************/
func (p *JavaSerializationParser) readProxyClassDescInfo(r io.Reader, c *JavaClassDesc) error {
	ifCount, err := Read4ByteToInt(r)
	if err != nil {
		return err
	}

	p.debug("Interface count - %v - 0x%v", ifCount, IntTo4Bytes(ifCount))
	p.increaseIndent()
	var interfaceNames []string
	for keyIndex := range make([]byte, ifCount) {
		p.debug("%v", keyIndex)
		p.increaseIndent()
		str, err := p.readStringUTF(r)
		p.decreaseIndent()
		if err != nil {
			return err
		}

		interfaceNames = append(interfaceNames, str)
	}
	p.decreaseIndent()

	c.Detail.DynamicProxyClass = true
	c.Detail.DynamicProxyClassInterfaceCount = ifCount
	c.Detail.DynamicProxyClassInterfaceNames = interfaceNames

	err = p.readClassAnnotation(r, c)
	if err != nil {
		return err
	}

	newGroup, err := p.readSuperClassDesc(r)
	if err != nil {
		return err
	}
	if newGroup != nil {
		c.Detail.SuperClass = newGroup
		//RETRY:
		//	switch ret := newGroup.(type) {
		//	case *JavaReference:
		//		newGroup, _ = p.ClassDetails[ret.Handle]
		//		goto RETRY
		//	case *JavaClassDesc:
		//		c.AddSuperClassDetails(ret.Detail)
		//		c.AddSuperClassDetails(ret.superClasses...)
		//	case *JavaClassDetails:
		//		c.AddSuperClassDetails(ret)
		//	case *JavaNull:
		//		desc := &JavaClassDesc{}
		//		desc.SetDetails(&JavaClassDetails{IsNull: true})
		//		newGroup = desc
		//		goto RETRY
		//	default:
		//		panic(fmt.Sprintf("cannot use superClass type: %v ", reflect.TypeOf(newGroup)))
		//	}
	}
	return nil
}

/*******************
 * Read a superClassDesc from the stream.
 *
 * classDesc
 ******************/
func (p *JavaSerializationParser) readSuperClassDesc(r io.Reader) (JavaSerializable, error) {
	p.debug("superClassDesc")
	p.increaseIndent()
	defer p.decreaseIndent()
	return p.readClassDesc(r)
	//var err error
	//c.LastDetails.SuperClass, err = p.readClassDesc(r)
	//if err != nil {
	//	return err
	//}
	//return nil
}

func (p *JavaSerializationParser) readClassAnnotation(r io.Reader, c *JavaClassDesc) error {

	p.debug("classAnnotations")
	p.increaseIndent()
	defer p.decreaseIndent()

	var contents []JavaSerializable
	for {
		data, err := p.readContentElement(r)
		if err != nil {
			return err
		}
		if _, ok := data.(*JavaEndBlockData); ok {
			p.debug("TC_ENDBLOCKDATA - 0x78")
			break
		}
		contents = append(contents, data)
	}
	if c.Detail.DynamicProxyClass {
		c.Detail.DynamicProxyAnnotation = contents
	} else {
		c.Detail.Annotations = contents
	}
	return nil
}

/*******************
 * Read a TC_CLASSDESC from the stream.
 *
 * TC_CLASSDESC		className	serialVersionUID	newHandle	classDescInfo
 ******************/
func (p *JavaSerializationParser) readTC_CLASSDESC(r io.Reader) (*JavaClassDesc, error) {
	groups := &JavaClassDesc{}
	p.ClassDescriptions = append(p.ClassDescriptions, groups)
	p.debug("TC_CLASSDESC - 0x%02x", TC_CLASSDESC)
	p.increaseIndent()
	defer p.decreaseIndent()

	c := newJavaClassDetails()

	// className
	p.debug("className")
	p.increaseIndent()
	className, err := p.readStringUTF(r)
	if err != nil {
		return nil, err
	}
	c.ClassName = className
	p.decreaseIndent()

	// read serial version
	verRaw, err := ReadBytesLength(r, 8)
	if err != nil {
		return nil, err
	}
	c.SerialVersion = verRaw
	p.debug("serialVersionUID - 0x%v", codec.EncodeToHex(c.SerialVersion))

	c.Handle = p.newHandler()
	p.ClassDetails[c.Handle] = c

	groups.SetDetails(c)
	err = p.readClassDescInfo(r, groups)
	if err != nil {
		return nil, err
	}

	return groups, nil
}

/*******************
 * Read a classDescInfo from the stream.
 *
 * classDescFlags	fields	classAnnotation	superClassDesc
 ******************/
func (p *JavaSerializationParser) readClassDescInfo(r io.Reader, c *JavaClassDesc) error {
	raw, err := ReadBytesLength(r, 1)
	if err != nil {
		return err
	}

	flag := raw[0]
	var features []string
	if c.Detail != nil {
		c.Detail.DescFlag = flag
		if c.Detail.Is_SC_WRITE_METHOD() {
			features = append(features, "SC_WRITE_METHOD")
		}
		if c.Detail.Is_SC_SERIALIZABLE() {
			features = append(features, "SC_SERIALIZABLE")
		}
		if c.Detail.Is_SC_EXTERNALIZABLE() {
			features = append(features, "SC_EXTERNALIZABLE")
		}
		if c.Detail.Is_SC_BLOCKDATA() {
			features = append(features, "SC_BLOCKDATA")
		}
	}

	p.debug("classDescFlags - 0x%02x - %v", flag, strings.Join(features, " | "))

	err = p.readFields(r, c)
	if err != nil {
		return err
	}

	err = p.readClassAnnotation(r, c)
	if err != nil {
		return nil
	}
	result, err := p.readSuperClassDesc(r)
	if err != nil {
		return err
	}
	if result != nil {
		c.Detail.SuperClass = result
		//RETRY:
		//	switch ret := result.(type) {
		//	case *JavaReference:
		//		result, _ = p.ClassDetails[ret.Handle]
		//		goto RETRY
		//	case *JavaClassDesc:
		//		c.AddSuperClassDetails(ret.Detail)
		//		c.AddSuperClassDetails(ret.superClasses...)
		//	case *JavaClassDetails:
		//		c.AddSuperClassDetails(ret)
		//	case *JavaNull:
		//		desc := &JavaClassDesc{}
		//		desc.SetDetails(&JavaClassDetails{IsNull: true})
		//		result = desc
		//		goto RETRY
		//	default:
		//		panic(fmt.Sprintf("cannot use superClass type: %v ", reflect.TypeOf(result)))
		//	}
	}
	return nil
}

/*******************
 * Read a fields element from the stream.
 *
 * (short)count		fieldDesc[count]
 ******************/
func (p *JavaSerializationParser) readFields(r io.Reader, c *JavaClassDesc) error {
	if c.Detail == nil {
		return nil
	}
	i, err := Read2ByteToInt(r)
	if err != nil {
		return err
	}
	p.debug("fieldCount: %v", i)

	c.Detail.Fields.FieldCount = i
	if i > 0 {
		p.debug("Fields")
		p.increaseIndent()
		defer p.decreaseIndent()

		for index := range make([]int, i) {
			p.debug("%v: ", index)
			p.increaseIndent()
			err := p.readFieldDesc(r, c)
			if err != nil {
				return err
			}
			p.decreaseIndent()
		}
	}
	return nil
}

/*******************
 * Read a fieldDesc from the stream.
 *
 * Could be either:
 *	prim_typecode	fieldName
 *	obj_typecode	fieldName	className1
 ******************/
func (p *JavaSerializationParser) readFieldDesc(r io.Reader, c *JavaClassDesc) error {
	if c.Detail == nil {
		return utils.Errorf("buffer class details failed")
	}

	raw, err := ReadBytesLength(r, 1)
	if err != nil {
		return err
	}
	field := &JavaClassField{}
	field.FieldType = raw[0]
	field.FieldTypeVerbose = jtToVerbose(raw[0])
	switch raw[0] {
	case JT_BYTE:
		p.debug("Byte - B - 0x%02x", raw[0])
	case JT_CHAR:
		p.debug("Char - C - 0x%02x", raw[0])
	case JT_DOUBLE:
		p.debug("Double - D - 0x%02x", raw[0])
	case JT_FLOAT:
		p.debug("Float - F - 0x%02x", raw[0])
	case JT_INT:
		p.debug("Int - I - 0x%02x", raw[0])
	case JT_LONG:
		p.debug("Long - L - 0x%02x", raw[0])
	case JT_SHORT:
		p.debug("Short - S - 0x%02x", raw[0])
	case JT_BOOL:
		p.debug("Boolean - Z - 0x%02x", raw[0])
	case JT_ARRAY:
		p.debug("Array - [ - 0x%02x", raw[0])
		//peeked, err := r.Peek(1)
		//if err != nil {
		//	return err
		//}
		//switch peeked[0] {
		//case JT_BYTE, JT_CHAR, JT_DOUBLE, JT_OBJECT,
		//	JT_FLOAT, JT_INT, JT_LONG, JT_SHORT, JT_BOOL:
		//
		//case JT_ARRAY:
		//	return utils.Errorf("[[ is not supported")
		//}
	case JT_OBJECT:
		p.debug("Object - L - 0x%02x", raw[0])
	default:
		return utils.Errorf("invalid type[%v]", utils.EscapeInvalidUTF8Byte(raw))
	}

	p.debug("fieldName")
	p.increaseIndent()
	field.Name, err = p.readStringUTF(r)
	if err != nil {
		return err
	}
	p.decreaseIndent()

	//log.Infof("read field: %v type: %v", field.Name, field.TypeVerbose)
	switch raw[0] {
	// https://github.com/NickstaDB/SerializationDumper/blob/49dbece69f0b8230aaed4a0c50ca56fecc9376c0/src/nb/deser/SerializationDumper.java#L834
	case JT_ARRAY:
		fallthrough
	case JT_OBJECT:
		p.debug("className1")
		p.increaseIndent()
		str, err := p.readStringObject(r)
		if err != nil {
			return utils.Errorf("read string/type failed: %s", err)
		}
		field.ClassName1 = str
		p.decreaseIndent()
	}
	c.Detail.Fields.Fields = append(c.Detail.Fields.Fields, field)

	return nil
}

/*******************
 * Read a TC_ARRAY from the stream.
 *
 * TC_ARRAY		classDesc	newHandle	(int)size	values[size]
 ******************/
func (p *JavaSerializationParser) readTC_ARRAY(r io.Reader) (*JavaArray, error) {
	p.debug("TC_ARRAY - 0x%02x", TC_ARRAY)
	p.increaseIndent()
	defer p.decreaseIndent()

	c, err := p.readClassDesc(r)
	if err != nil {
		return nil, err
	}

	handler := p.newHandler()
	p.ClassDetails[handler] = c

	arr := &JavaArray{
		Handle: handler,
	}
	arr.ClassDesc = c

	// newHandle
	// this.newHandle();

	size, err := Read4ByteToInt(r)
	if err != nil {
		return nil, err
	}

	p.debug("Array size - %v - 0x%v", size, codec.EncodeToHex(IntTo4Bytes(size)))
	arr.Size = size
	defer arr.fixBytescode()

RETRY:
	switch ret := c.(type) {
	case *JavaClassDetails:
		desc := &JavaClassDesc{}
		desc.SetDetails(ret)
		c = desc
		goto RETRY
	case *JavaClassDesc:
		p.debug("Values")
		p.increaseIndent()
		defer p.decreaseIndent()

		for i := 0; i < size; i++ {
			p.debug("Index %v:", i)
			p.increaseIndent()
			if len(ret.Detail.ClassName) <= 0 {
				return nil, utils.Error("className empty...")
			}
			value, err := p.readFieldValue(r, ret.Detail.ClassName[1])
			p.decreaseIndent()

			if err != nil {
				return nil, err
			}
			arr.Values = append(arr.Values, value)
		}
		return arr, nil
	case *JavaReference:
		c, _ = p.ClassDetails[ret.Handle]
		goto RETRY
	default:
		panic(fmt.Sprintf("TC_ARRAY cannot support: %v", reflect.TypeOf(c)))
		//for i := 0; i < size; i++ {
		//	value, err := p.readFieldValue(r, c.LastDetails.ClassName[1])
		//	if err != nil {
		//		return nil, err
		//	}
		//	arr.Values = append(arr.Values, value)
		//}
		//return arr, nil
	}

}

/*******************
 * Read a field value based on the type code.
 *
 * @param typeCode The field type code.
 ******************/
func (p *JavaSerializationParser) readFieldValue(r io.Reader, t byte) (*JavaFieldValue, error) {
	v := &JavaFieldValue{
		FieldType: t, FieldTypeVerbose: jtToVerbose(t),
	}
	var err error
	switch t {
	case JT_BYTE:
		v.Bytes, err = ReadBytesLengthInt(r, 1)
		if err != nil {
			return nil, err
		}
		p.ShowByte(v.Bytes)
		return v, nil
	case JT_BOOL: // 1
		v.Bytes, err = ReadBytesLengthInt(r, 1)
		if err != nil {
			return nil, err
		}
		p.ShowBool(v.Bytes)
		return v, nil
	case JT_SHORT:
		v.Bytes, err = ReadBytesLengthInt(r, 2)
		if err != nil {
			return nil, err
		}
		p.ShowShort(v.Bytes)
		return v, nil
	case JT_CHAR: // 2
		v.Bytes, err = ReadBytesLengthInt(r, 2)
		if err != nil {
			return nil, err
		}
		p.ShowChar(v.Bytes)
		return v, nil
	case JT_FLOAT:
		v.Bytes, err = ReadBytesLengthInt(r, 4)
		if err != nil {
			return nil, err
		}
		p.ShowFloat(v.Bytes)
		return v, nil
	case JT_INT:
		v.Bytes, err = ReadBytesLengthInt(r, 4)
		if err != nil {
			return nil, err
		}
		p.ShowInt(v.Bytes)
		return v, nil
	case JT_DOUBLE:
		v.Bytes, err = ReadBytesLengthInt(r, 8)
		if err != nil {
			return nil, err
		}
		p.ShowDouble(v.Bytes)
		return v, nil
	case JT_LONG:
		v.Bytes, err = ReadBytesLengthInt(r, 8)
		if err != nil {
			return nil, err
		}
		p.ShowLong(v.Bytes)
		return v, nil
	case JT_ARRAY:
		v.Object, err = p.readArrayField(r)
		if err != nil {
			return nil, err
		}
		return v, nil
	case JT_OBJECT:
		v.Object, err = p.readObjectField(r)
		if err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, utils.Errorf("failed for fieldtype %v", codec.EncodeToHex(t))
}

func (p *JavaSerializationParser) readTC_NULL(r io.Reader) (*JavaNull, error) {
	p.debug("TC_NULL - 0x%02x", TC_NULL)
	return &JavaNull{}, nil
}

func (p *JavaSerializationParser) readStringObject(r io.Reader) (JavaSerializable, error) {
	raw, err := ReadByte(r)
	if err != nil {
		return nil, err
	}
	switch raw[0] {
	case TC_STRING:
		return p.readTC_STRING(r)
	case TC_LONGSTRING:
		return p.readTC_LONGSTRING(r)
	case TC_REFERENCE:
		return p.readPrevObject(r)
	default:
		return nil, utils.Errorf("String object should be TC_STRING/LONGSTRING/REFERENCE, but got: %v", tcToVerbose(raw[0]))
	}
}

/*******************
 * Read a newString element from the stream.
 *
 * Could be:
 *	TC_STRING		(0x74)
 *	TC_LONGSTRING	(0x7c)
 ******************/
func (p *JavaSerializationParser) readNewString(r io.Reader) (*JavaString, error) {
	raw, err := ReadByte(r)
	if err != nil {
		return nil, err
	}
	switch raw[0] {
	case TC_STRING:
		return p.readTC_STRING(r)
	case TC_LONGSTRING:
		return p.readTC_LONGSTRING(r)
	//case TC_REFERENCE:
	//	data, err := p.readPrevObject(r)
	//	if err != nil {
	//		return nil, utils.Errorf("prev object failed: %s", err)
	//	}
	//	return &JavaString{
	//		IsReference:    true,
	//		ReferenceValue: data,
	//	}, nil
	default:
		return nil, utils.Errorf("invalid new string... %v", codec.EncodeToHex(raw[0]))
	}
}

// TC_REF
func (p *JavaSerializationParser) readPrevObject(r io.Reader) (*JavaReference, error) {
	p.debug("TC_REFERENCE - 0x%02x", TC_REFERENCE)
	p.increaseIndent()
	defer p.decreaseIndent()

	handle, err := Read4ByteToUint64(r)
	if err != nil {
		return nil, err
	}
	p.debug("Handle - 0x%v", codec.EncodeToHex(Uint64To4Bytes(handle)))
	return &JavaReference{Value: Uint64To4Bytes(handle), Handle: handle}, nil
}

func (p *JavaSerializationParser) readTC_STRING(r io.Reader) (*JavaString, error) {
	p.debug("TC_STRING - 0x%02x", TC_STRING)
	p.increaseIndent()
	defer p.decreaseIndent()

	handler := p.newHandler()
	raw, err := p.readStringUTF(r)
	if err != nil {
		return nil, err
	}
	obj := &JavaString{
		IsLong: false,
		Size:   uint64(len(raw)),
		Raw:    []byte(raw),
		Value:  raw,
	}
	p.ClassDetails[handler] = obj
	return obj, nil
}

func (p *JavaSerializationParser) readTC_LONGSTRING(r io.Reader) (*JavaString, error) {
	p.debug("TC_LONGSTRING - 0x%v", codec.EncodeToHex(TC_LONGSTRING))
	p.increaseIndent()
	p.decreaseIndent()

	handler := p.newHandler()
	raw, err := p.readLongStringUTF(r)
	if err != nil {
		return nil, err
	}
	obj := &JavaString{
		IsLong: false,
		Size:   uint64(len(raw)),
		Raw:    []byte(raw),
		Value:  raw,
	}
	p.ClassDetails[handler] = obj
	return obj, nil
}

/*******************
 * Read a UTF string from the stream.
 *
 * (short: 2 byte)length	contents
 ******************/
func (p *JavaSerializationParser) readStringUTF(r io.Reader) (string, error) {
	stringLen, err := Read2ByteToInt(r)
	if err != nil {
		return "", err
	}

	raw, err := ReadBytesLength(r, uint64(stringLen))
	if err != nil {
		return "", err
	}
	raw, err = utils.SimplifyUtf8(raw)
	if err != nil {
		return "", err
	}
	p.debug("Length - %v", stringLen)
	p.debug("Value  - %v", strconv.Quote(string(raw)))
	// https://github.com/NickstaDB/SerializationDumper/blob/49dbece69f0b8230aaed4a0c50ca56fecc9376c0/src/nb/deser/SerializationDumper.java#L1295
	return string(raw), nil
}
func (p *JavaSerializationParser) readLongStringUTF(r io.Reader) (string, error) {
	stringLen, err := Read8BytesToUint64(r)
	if err != nil {
		return "", err
	}

	raw, err := ReadBytesLength(r, stringLen)
	if err != nil {
		return "", err
	}

	// https://github.com/NickstaDB/SerializationDumper/blob/49dbece69f0b8230aaed4a0c50ca56fecc9376c0/src/nb/deser/SerializationDumper.java#L1295
	return string(raw), nil
}

/*******************
 * Read a content element from the data stream.
 *
 * Could be any of:
 *	TC_OBJECT			(0x73)
 *	TC_CLASS			(0x76)
 *	TC_ARRAY			(0x75)
 *	TC_STRING			(0x74)
 *	TC_LONGSTRING		(0x7c)
 *	TC_ENUM				(0x7e)
 *	TC_CLASSDESC		(0x72)
 *	TC_PROXYCLASSDESC	(0x7d)
 *	TC_REFERENCE		(0x71)
 *	TC_NULL				(0x70)
 *	TC_EXCEPTION		(0x7b)
 *	TC_RESET			(0x79)
 *	TC_BLOCKDATA		(0x77)
 *	TC_BLOCKDATALONG	(0x7a)
 ******************/
func (p *JavaSerializationParser) readContentElement(r io.Reader) (JavaSerializable, error) {
	raw, err := ReadByte(r)
	if err != nil {
		return nil, err
	}

	//log.Infof("start to handle: %v", tcToVerbose(raw[0]))
	switch raw[0] {
	case TC_STRING:
		return p.readTC_STRING(r)
	case TC_LONGSTRING:
		return p.readTC_LONGSTRING(r)
	case TC_NULL: // done
		return p.readTC_NULL(r)
	case TC_OBJECT:
		return p.readNewObject(r)
	case TC_CLASS:
		return p.readNewClass(r)
	case TC_ARRAY:
		return p.readTC_ARRAY(r)
	case TC_ENUM:
		return p.readTC_ENUM(r)
	case TC_CLASSDESC:
		return p.readTC_CLASSDESC(r)
	case TC_PROXYCLASSDESC:
		return p.readTC_PROXYCLASSDESC(r)
	case TC_REFERENCE: // done
		return p.readPrevObject(r)
	case TC_BLOCKDATA:
		return p.readBlockData(r)
	case TC_BLOCKDATALONG:
		return p.readLongBlockData(r)
	case TC_ENDBLOCKDATA:
		return &JavaEndBlockData{}, nil
	}
	return nil, utils.Errorf("error for read 0x%v", codec.EncodeToHex(raw))
}

/*******************
 * Read an enum element from the data stream.
 *
 * TC_ENUM		classDesc	newHandle	enumConstantName
 ******************/
func (p *JavaSerializationParser) readTC_ENUM(r io.Reader) (*JavaEnumDesc, error) {
	d := &JavaEnumDesc{}
	classDesc, err := p.readClassDesc(r)
	if err != nil {
		return nil, err
	}
	d.Handle = p.newHandler()
	p.ClassDetails[d.Handle] = d

	d.TypeClassDesc = classDesc
	d.ConstantName, err = p.readStringObject(r)
	if err != nil {
		return nil, err
	}
	return d, nil
}

/*******************
 * Read an object element from the data stream.
 *
 * TC_OBJECT	classDesc	newHandle	classdata[]
 ******************/
func (p *JavaSerializationParser) readNewObject(r io.Reader) (*JavaObject, error) {
	p.debug("TC_OBJECT - 0x%02x", TC_OBJECT)
	p.increaseIndent()
	defer p.decreaseIndent()

	obj := &JavaObject{}
	c, err := p.readClassDesc(r)
	if err != nil {
		return nil, err
	}

	obj.Handle = p.newHandler()
	p.ClassDetails[obj.Handle] = obj
	obj.Class = c

RETRY:
	switch ret := c.(type) {
	case *JavaClassDesc:
		values, err := p.readClassData(r, ret)
		if err != nil {
			return nil, err
		}
		obj.ClassData = values
		return obj, nil
	case *JavaReference:
		c, _ = p.ClassDetails[ret.Handle]
		goto RETRY
	case *JavaClassDetails:
		desc := &JavaClassDesc{}
		desc.SetDetails(ret)
		c = desc
		goto RETRY
		//values, haveBlockData, err := p.readClassData(r, desc)
		//if err != nil {
		//	return nil, err
		//}
		//obj.ClassData = values
		//obj.HaveBlockData = haveBlockData
		//return obj, nil
	default:
		return nil, utils.Errorf("object classDesc type: %v is not supported", reflect.TypeOf(ret))
	}
}

/*******************
 * Read classdata from the stream.
 *
 * Consists of data for each class making up the object starting with the
 * most super class first. The length and type of data depends on the
 * classDescFlags and field descriptions.
 ******************/
func (p *JavaSerializationParser) readClassData(r io.Reader, c *JavaClassDesc) ([]JavaSerializable, error) {
	p.debug("classdata")
	p.increaseIndent()
	defer p.decreaseIndent()

	//if c.Detail.Is_SC_EXTERNALIZABLE() {
	//	p.debug("externalContents")
	//	p.increaseIndent()
	//	p.debug("Unable to parse externalContents as the format is specific to the implementation class.")
	//	p.decreaseIndent()
	//	panic("ERROR: unable to parse external content element")
	//}

	// 从 super class 开始
	var classDataArray []JavaSerializable
	targets := []*JavaClassDetails{c.Detail}
	var temp = c.Detail
	for {
		if temp == nil {
			break
		}
		if temp.SuperClass != nil {
			var tempSuperClass = temp.SuperClass
		RETRY:
			switch ret := tempSuperClass.(type) {
			case *JavaReference:
				newGroup, ok := p.ClassDetails[ret.Handle]
				if !ok {
					return nil, utils.Errorf("cannot found classDetails by TC_REFERENCE")
				}
				var superClass *JavaClassDetails
				superClass, ok = newGroup.(*JavaClassDetails)
				if !ok {
					tempSuperClass = newGroup
					goto RETRY
				}
				temp = superClass
				targets = append(targets, superClass)
			case *JavaClassDesc:
				temp = ret.Detail
				targets = append(targets, ret.Detail)
			case *JavaClassDetails:
				temp = ret
				targets = append(targets, ret)
			case *JavaNull:
				temp = &JavaClassDetails{IsNull: true}
				targets = append(targets, &JavaClassDetails{IsNull: true})
			default:
				return nil, utils.Errorf("cannot handle superClass type: %v", reflect.TypeOf(ret))
			}
		} else {
			break
		}
	}
	//targets = append(targets, c.superClasses...)
	for i := len(targets) - 1; i >= 0; i-- {
		details := targets[i]
		arr := &JavaClassData{}
		classDataArray = append(classDataArray, arr)

		if details.DynamicProxyClass {
			p.debug("<dynamic proxy class>")
		} else {
			if details.ClassName != "" {
				p.debug("%v - [%v/%v]", details.ClassName, i, len(targets))
			}
		}
		p.increaseIndent()

		//if details.Is_SC_SERIALIZABLE() && !details.Is_SC_WRITE_METHOD() {
		if details.Is_SC_SERIALIZABLE() {
			p.debug("values")
			p.increaseIndent()
			for _, f := range details.Fields.Fields {
				value, err := p.readClassDataField(r, f)
				if err != nil {
					return nil, err
				}
				_ = value
				arr.Fields = append(arr.Fields, value)
			}
			p.decreaseIndent()
		}

		if (details.Is_SC_WRITE_METHOD() && details.Is_SC_SERIALIZABLE()) || (details.Is_SC_EXTERNALIZABLE() && details.Is_SC_BLOCKDATA()) {
			p.debug("objectAnnotation")
			p.increaseIndent()

			var isEmptyBlockData = false
			for {
				value, err := p.readContentElement(r)
				if err != nil {
					if err == io.EOF {
						p.debug("TC_ENDBLOCKDATA - 0x%02x (EOF)", TC_ENDBLOCKDATA)
						isEmptyBlockData = true
						break
					}
					log.Errorf("read byte failed: %s", err)
					return nil, err
				}

				if _, ok := value.(*JavaEndBlockData); ok {
					p.debug("TC_ENDBLOCKDATA - 0x%02x", TC_ENDBLOCKDATA)
					break
				}
				arr.BlockData = append(arr.BlockData, value)
				//data = append(data, value)
				//haveBlockData = true
			}
			arr.BlockData = append(arr.BlockData, &JavaEndBlockData{IsEmpty: isEmptyBlockData})
			p.decreaseIndent()
		}

		p.decreaseIndent()
	}

	return classDataArray, nil
}

func (p *JavaSerializationParser) readClassDataField(r io.Reader, f *JavaClassField) (*JavaFieldValue, error) {

	p.debug(f.Name)
	p.increaseIndent()
	defer p.decreaseIndent()
	return p.readFieldValue(r, f.FieldType)
}

func (p *JavaSerializationParser) readNewClass(r io.Reader) (*JavaClass, error) {
	p.debug("TC_CLASS - 0x%02x", TC_CLASS)
	p.increaseIndent()

	cl := &JavaClass{}
	d, err := p.readClassDesc(r)
	if err != nil {
		return nil, err
	}
	cl.Desc = d

	p.decreaseIndent()
	// newHandle
	cl.Handle = p.newHandler(" -> TC_CLASS")
	p.ClassDetails[cl.Handle] = cl
	return cl, nil
}

/*
******************
  - Read a blockdatashort element from the stream.
    *
  - TC_BLOCKDATA		(unsigned byte)size		contents
*/
func (p *JavaSerializationParser) readBlockData(r io.Reader) (*JavaBlockData, error) {
	l, err := ReadByteToInt(r)
	if err != nil {
		return nil, err
	}

	raw, err := ReadBytesLengthInt(r, l)
	if err != nil {
		return nil, err
	}
	return &JavaBlockData{IsLong: false, Size: uint64(l), Contents: raw}, nil
}

/*******************
 * Read a blockdatalong element from the stream.
 *
 * TC_BLOCKDATALONG		(int)size	contents
 ******************/
func (p *JavaSerializationParser) readLongBlockData(r io.Reader) (*JavaBlockData, error) {
	l, err := Read4ByteToInt(r)
	if err != nil {
		return nil, err
	}

	raw, err := ReadBytesLengthInt(r, l)
	if err != nil {
		return nil, err
	}
	return &JavaBlockData{
		IsLong:   true,
		Size:     uint64(l),
		Contents: raw,
	}, nil
}

/*******************
 * Read an array field.
 ******************/
func (p *JavaSerializationParser) readArrayField(r io.Reader) (JavaSerializable, error) {
	raw, err := ReadByte(r)
	if err != nil {
		return nil, err
	}

	p.debug("(array)")
	p.increaseIndent()
	defer p.decreaseIndent()

	switch raw[0] {
	case TC_NULL:
		return p.readTC_NULL(r)
	case TC_ARRAY:
		return p.readTC_ARRAY(r)
	case TC_REFERENCE:
		return p.readPrevObject(r)
	}
	return nil, utils.Errorf("unexpected array fieldtype: %v", codec.EncodeToHex(raw))
}

/*******************
 * Read an object field.
 ******************/
func (p *JavaSerializationParser) readObjectField(r io.Reader) (JavaSerializable, error) {
	p.debug("(object)")
	p.increaseIndent()
	defer p.decreaseIndent()

	raw, err := ReadByte(r)
	if err != nil {
		return nil, err
	}

	switch raw[0] {
	case TC_OBJECT:
		return p.readNewObject(r)
	case TC_REFERENCE:
		return p.readPrevObject(r)
	case TC_NULL:
		return p.readTC_NULL(r)
	case TC_STRING:
		return p.readTC_STRING(r)
	case TC_CLASS:
		return p.readNewClass(r)
	case TC_ARRAY:
		return p.readTC_ARRAY(r)
	case TC_ENUM:
		return p.readTC_ENUM(r)
	//case TC_BLOCKDATALONG:
	//	return p.readLongBlockData(r)
	//case TC_BLOCKDATA:
	//	return p.readBlockData(r)
	default:
		return nil, utils.Errorf("unexpected object field value: 0x%v", codec.EncodeToHex(raw))
	}
}

func (p *JavaSerializationParser) newHandler(extra ...string) uint64 {
	originValue := p._Handler
	p.debug("newHandle 0x%v %v", codec.EncodeToHex(Uint64To4Bytes(originValue)), strings.Join(extra, " "))
	p._Handler++
	return originValue
}
