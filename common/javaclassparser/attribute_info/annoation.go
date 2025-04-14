package attribute_info

import (
	"fmt"

	"github.com/yaklang/yaklang/common/javaclassparser/constant_pool"
	"github.com/yaklang/yaklang/common/javaclassparser/types"
)

func ParseAnnotationElementValue(reader types.ClassReader, pool *constant_pool.ConstantPool) *ElementValuePairAttribute {
	getUtf8 := func(index uint16) string {
		s := pool.GetUtf8(int(index))
		if s == nil {
			panic(fmt.Errorf("get utf8 error"))
		}
		return s.Value
	}
	tag := reader.ReadUint8()
	ele := &ElementValuePairAttribute{
		Tag: tag,
	}
	switch tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		index := reader.ReadUint16()
		ele.Value = pool.IndexInfo(int(index))
		if ele.Value == nil {
			panic(fmt.Errorf("get constant info error"))
		}
	case 's':
		ele.Value = getUtf8(reader.ReadUint16())
	case 'e':
		val := &EnumConstValue{
			TypeName:  getUtf8(reader.ReadUint16()),
			ConstName: getUtf8(reader.ReadUint16()),
		}
		ele.Value = val
	case 'c':
		ele.Value = getUtf8(reader.ReadUint16())
	case '@':
		ele.Value = ParseAnnotation(reader, pool)
	case '[':
		length := reader.ReadUint16()
		l := []*ElementValuePairAttribute{}
		for k := 0; k < int(length); k++ {
			val := ParseAnnotationElementValue(reader, pool)
			l = append(l, val)
		}
		ele.Value = l
	default:
		panic(fmt.Errorf("parse annotation error, unknown tag: %c", tag))
	}
	return ele
}

func WriteAnnotation(writer types.ClassWriter, anno *AnnotationAttribute, pool *constant_pool.ConstantPool) {
	typeIndex := pool.SearchUtf8Index(anno.TypeName)
	writer.Write2Byte(uint16(typeIndex))
	writer.Write2Byte(uint16(len(anno.ElementValuePairs)))
	for _, pair := range anno.ElementValuePairs {
		WriteAnnotationElementValuePair(writer, pair, pool)
	}
}

func WriteAnnotationElementValue(writer types.ClassWriter, pair *ElementValuePairAttribute, pool *constant_pool.ConstantPool) {
	writer.Write1Byte(uint8(pair.Tag))

	switch pair.Tag {
	case 'B', 'C', 'D', 'F', 'I', 'J', 'S', 'Z':
		// For constant value types, write the constant pool index
		constantInfo, ok := pair.Value.(constant_pool.ConstantInfo)
		if !ok {
			panic(fmt.Errorf("write annotation error, value is not ConstantInfo"))
		}

		// Find the index of the constant info in the pool
		index := 0
		for i, info := range pool.GetData() {
			if info == constantInfo {
				index = i + 1
				break
			}
		}

		if index == 0 {
			// If not found, append it
			index = pool.AppendConstantInfo(constantInfo)
		}

		writer.Write2Byte(uint16(index))
	case 's':
		// For string value, write the utf8 index
		strValue, ok := pair.Value.(string)
		if !ok {
			panic(fmt.Errorf("write annotation error, value is not string"))
		}
		index := pool.SearchUtf8Index(strValue)
		writer.Write2Byte(uint16(index))
	case 'e':
		// For enum value, write type name and const name indexes
		enumValue, ok := pair.Value.(*EnumConstValue)
		if !ok {
			panic(fmt.Errorf("write annotation error, value is not EnumConstValue"))
		}
		typeIndex := pool.SearchUtf8Index(enumValue.TypeName)
		constIndex := pool.SearchUtf8Index(enumValue.ConstName)
		writer.Write2Byte(uint16(typeIndex))
		writer.Write2Byte(uint16(constIndex))
	case 'c':
		// For class info value, write the utf8 index
		classValue, ok := pair.Value.(string)
		if !ok {
			panic(fmt.Errorf("write annotation error, value is not string"))
		}
		index := pool.SearchUtf8Index(classValue)
		writer.Write2Byte(uint16(index))
	case '@':
		// For annotation value, recursively write it
		annoValue, ok := pair.Value.(*AnnotationAttribute)
		if !ok {
			panic(fmt.Errorf("write annotation error, value is not AnnotationAttribute"))
		}
		WriteAnnotation(writer, annoValue, pool)
	case '[':
		// For array value, write each element
		arrayValue, ok := pair.Value.([]*ElementValuePairAttribute)
		if !ok {
			panic(fmt.Errorf("write annotation error, value is not []*ElementValuePairAttribute"))
		}
		writer.Write2Byte(uint16(len(arrayValue)))
		for _, elem := range arrayValue {
			WriteAnnotationElementValue(writer, elem, pool)
		}
	default:
		panic(fmt.Errorf("write annotation error, unknown tag: %c", pair.Tag))
	}
}

func ParseAnnotationElementValuePair(reader types.ClassReader, pool *constant_pool.ConstantPool) *ElementValuePairAttribute {
	getUtf8 := func(index uint16) string {
		s := pool.GetUtf8(int(index))
		if s == nil {
			panic(fmt.Errorf("get utf8 error"))
		}
		return s.Value
	}
	nameIndex := reader.ReadUint16()
	name := getUtf8(nameIndex)
	value := ParseAnnotationElementValue(reader, pool)
	value.Name = name
	return value
}

func ParseAnnotation(reader types.ClassReader, pool *constant_pool.ConstantPool) *AnnotationAttribute {
	getUtf8 := func(index uint16) string {
		s := pool.GetUtf8(int(index))
		if s == nil {
			panic(fmt.Errorf("get utf8 error"))
		}
		return s.Value
	}

	typeIndex := reader.ReadUint16()
	elementLen := reader.ReadUint16()
	typeName := getUtf8(typeIndex)
	anno := &AnnotationAttribute{
		TypeName:          typeName,
		ElementValuePairs: make([]*ElementValuePairAttribute, elementLen),
	}
	for j := range anno.ElementValuePairs {
		anno.ElementValuePairs[j] = ParseAnnotationElementValuePair(reader, pool)
	}
	return anno
}

func WriteAnnotationElementValuePair(writer types.ClassWriter, pair *ElementValuePairAttribute, pool *constant_pool.ConstantPool) {
	nameIndex := pool.SearchUtf8Index(pair.Name)
	writer.Write2Byte(uint16(nameIndex))
	WriteAnnotationElementValue(writer, pair, pool)
}
