package javaclassparser

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

var ValueTypeError = utils.Error("error value type")

func deleteStringKeysFromMap(data map[string]interface{}, keys ...string) {
	for _, key := range keys {
		delete(data, key)
	}
}
func Interface2Uint64(v interface{}) (uint64, error) {
	switch v.(type) {
	case uint:
		return uint64(v.(uint)), nil
	case int:
		return uint64(v.(int)), nil
	case uint64:
		return v.(uint64), nil
	case int64:
		return uint64(v.(int64)), nil
	case uint32:
		return uint64(v.(uint32)), nil
	case int32:
		return uint64(v.(int32)), nil
	case uint16:
		return uint64(v.(uint16)), nil
	case int16:
		return uint64(v.(int16)), nil
	case uint8:
		return uint64(v.(uint8)), nil
	case int8:
		return uint64(v.(int)), nil
	default:
		return 0, ValueTypeError
	}
}
func GetMap() ([]int, []int) {
	CHAR_MAP := make([]int, 48)
	MAP_CHAR := make([]int, 256)

	j := 0
	var i int
	for i = 65; i <= 90; {
		CHAR_MAP[j] = i
		MAP_CHAR[i] = j
		i += 1
		j += 1

	}

	for i = 103; i <= 122; {
		CHAR_MAP[j] = i
		MAP_CHAR[i] = j
		i += 1
		j += 1
	}

	CHAR_MAP[j] = 36
	MAP_CHAR[36] = j
	j += 1
	CHAR_MAP[j] = 95

	MAP_CHAR[95] = j
	return CHAR_MAP, MAP_CHAR
}
func Bcel2bytes(becl string) ([]byte, error) {
	pre := "$$BCEL$$"
	if !strings.HasPrefix(becl, pre) {
		return nil, utils.Error("Invalid becl header(\"$$BCEL$$\")!")
	}
	becl = becl[len(pre):]
	//生成CHAR_MAP和MAP_CHAR
	_, MAP_CHAR := GetMap()
	//reader
	rd := strings.NewReader(becl)
	var buf bytes.Buffer
	read := func() int {
		for {
			c, err := rd.ReadByte()
			if err != nil {
				return -1
			}
			if c != '$' {
				return int(c)
			} else {
				c, err = rd.ReadByte()
				if err != nil {
					return -1
				}
				if (c < 48 || c > 57) && (c < 97 || c > 102) {
					return MAP_CHAR[c]
				} else {
					c1, err := rd.ReadByte()
					if err != nil {
						return -1
					}
					byts, err := codec.DecodeHex(string([]byte{c, c1}))
					if err != nil {
						return -1
					}
					n := byts[0]
					return int(n)
				}

			}
		}
	}
	for {
		n := read()
		if n != -1 {
			buf.WriteByte(byte(n))
		} else {
			break
		}
	}
	reader, err := gzip.NewReader(&buf)
	if err != nil {
		var out []byte
		return out, err
	}
	defer reader.Close()
	return ioutil.ReadAll(reader)
}
func bytes2bcel(data []byte) (string, error) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	if _, err := gz.Write(data); err != nil {
		return "", err
	}
	if err := gz.Flush(); err != nil {
		return "", err
	}
	if err := gz.Close(); err != nil {
		return "", err
	}
	data = b.Bytes()

	CHAR_MAP, _ := GetMap()
	var buf strings.Builder
	isJavaIdentifierPart := func(ch int) bool {
		return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '_'
	}
	write := func(b int) {
		if isJavaIdentifierPart(b) && b != 36 {
			buf.WriteByte(byte(b))
		} else {
			buf.WriteByte(36)
			if b >= 0 && b < 48 {
				buf.WriteByte(byte(CHAR_MAP[b]))
			} else {
				strHex := codec.EncodeToHex([]byte{byte(b)})
				if len(strHex) == 1 {
					buf.WriteByte(48)
					buf.WriteByte(strHex[0])
				} else {
					buf.WriteString(strHex)
				}
			}
		}

	}
	l := len(data)
	for i := 0; i < l; i += 1 {
		in := int(data[i]) & 255
		write(in)
	}
	return "$$BCEL$$" + buf.String(), nil
}

func ParseAnnotationElementValue(cp *ClassParser) *ElementValuePairAttribute {
	getUtf8 := func(index uint16) string {
		s, err := cp.classObj.getUtf8(index)
		if err != nil {
			panic(fmt.Errorf("get utf8 error: %s", err))
		}
		return s
	}
	reader := cp.reader
	tag := reader.readUint8()
	ele := &ElementValuePairAttribute{
		Tag: tag,
	}
	switch tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		index := reader.readUint16()
		var err error
		ele.Value, err = cp.classObj.getConstantInfo(index)
		if err != nil {
			panic(fmt.Errorf("get constant info error: %s", err))
		}
	case 's':
		ele.Value = getUtf8(reader.readUint16())
	case 'e':
		val := &EnumConstValue{
			TypeName:  getUtf8(reader.readUint16()),
			ConstName: getUtf8(reader.readUint16()),
		}
		ele.Value = val
	case 'c':
		ele.Value = getUtf8(reader.readUint16())
	case '@':
		ele.Value = ParseAnnotation(cp)
	case '[':
		length := reader.readUint16()
		l := []*ElementValuePairAttribute{}
		for k := 0; k < int(length); k++ {
			val := ParseAnnotationElementValue(cp)
			l = append(l, val)
		}
		ele.Value = l
	default:
		panic(fmt.Errorf("parse annotation error, unknown tag: %c", tag))
	}
	return ele
}
func ParseAnnotationElementValuePair(cp *ClassParser) *ElementValuePairAttribute {
	getUtf8 := func(index uint16) string {
		s, err := cp.classObj.getUtf8(index)
		if err != nil {
			panic(fmt.Errorf("get utf8 error: %s", err))
		}
		return s
	}
	reader := cp.reader
	nameIndex := reader.readUint16()
	name := getUtf8(nameIndex)
	value := ParseAnnotationElementValue(cp)
	value.Name = name
	return value
}
func ParseAnnotation(cp *ClassParser) *AnnotationAttribute {
	getUtf8 := func(index uint16) string {
		s, err := cp.classObj.getUtf8(index)
		if err != nil {
			panic(fmt.Errorf("get utf8 error: %s", err))
		}
		return s
	}
	reader := cp.reader

	typeIndex := reader.readUint16()
	elementLen := reader.readUint16()
	typeName := getUtf8(typeIndex)
	anno := &AnnotationAttribute{
		TypeName:          typeName,
		ElementValuePairs: make([]*ElementValuePairAttribute, elementLen),
	}
	for j := range anno.ElementValuePairs {
		anno.ElementValuePairs[j] = ParseAnnotationElementValuePair(cp)
	}
	return anno
}
