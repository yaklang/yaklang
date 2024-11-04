package types

import (
	"fmt"
	"strings"
)

func GetPrimerArrayType(id int) JavaType {
	switch id {
	case 4:
		return NewJavaPrimer(JavaBoolean)
	case 5:
		return NewJavaPrimer(JavaChar)
	case 6:
		return NewJavaPrimer(JavaFloat)
	case 7:
		return NewJavaPrimer(JavaDouble)
	case 8:
		return NewJavaPrimer(JavaByte)
	case 9:
		return NewJavaPrimer(JavaShort)
	case 10:
		return NewJavaPrimer(JavaInteger)
	case 11:
		return NewJavaPrimer(JavaLong)
	default:
		return nil
	}
}
func ParseDescriptor(descriptor string) (JavaType, error) {
	returnType, _, err := ParseJavaDescription(descriptor)
	return returnType, err
}

// ParseMethodDescriptor 解析 Java 方法描述符
func ParseMethodDescriptor(descriptor string) (JavaType, error) {
	if descriptor == "" {
		return nil, fmt.Errorf("descriptor is empty")
	}

	if descriptor[0] != '(' {
		return nil, fmt.Errorf("invalid descriptor format")
	}

	// 查找参数部分和返回类型部分
	endIndex := strings.Index(descriptor, ")")
	if endIndex == -1 {
		return nil, fmt.Errorf("invalid descriptor format")
	}

	paramDescriptor := descriptor[1:endIndex]
	returnTypeDescriptor := descriptor[endIndex+1:]

	// 解析参数类型
	paramTypes, err := parseTypes(paramDescriptor)
	if err != nil {
		return nil, err
	}

	// 解析返回类型
	returnType, _, err := ParseJavaDescription(returnTypeDescriptor)
	if err != nil {
		return nil, err
	}

	return newJavaTypeWrap(NewJavaFuncType(descriptor, paramTypes, returnType)), nil
}

// parseTypes 解析多个类型描述符
func parseTypes(descriptor string) ([]JavaType, error) {
	var types []JavaType
	for len(descriptor) > 0 {
		t, rest, err := ParseJavaDescription(descriptor)
		if err != nil {
			return nil, err
		}
		types = append(types, t)
		descriptor = rest
	}
	return types, nil
}

// ParseJavaDescription 解析单个类型描述符
func ParseJavaDescription(descriptor string) (JavaType, string, error) {
	if len(descriptor) == 0 {
		return nil, "", fmt.Errorf("empty descriptor")
	}

	switch descriptor[0] {
	case 'B':
		return NewJavaPrimer(JavaByte), descriptor[1:], nil
	case 'C':
		return NewJavaPrimer(JavaChar), descriptor[1:], nil
	case 'D':
		return NewJavaPrimer(JavaDouble), descriptor[1:], nil
	case 'F':
		return NewJavaPrimer(JavaFloat), descriptor[1:], nil
	case 'I':
		return NewJavaPrimer(JavaInteger), descriptor[1:], nil
	case 'J':
		return NewJavaPrimer(JavaLong), descriptor[1:], nil
	case 'S':
		return NewJavaPrimer(JavaShort), descriptor[1:], nil
	case 'Z':
		return NewJavaPrimer(JavaBoolean), descriptor[1:], nil
	case 'V':
		return NewJavaPrimer(JavaVoid), descriptor[1:], nil
	case 'L':
		endIndex := strings.Index(descriptor, ";")
		if endIndex == -1 {
			return nil, "", fmt.Errorf("invalid class descriptor format")
		}
		name := strings.Replace(descriptor[1:endIndex], "/", ".", -1)
		return NewJavaClass(name), descriptor[endIndex+1:], nil
	case '[':
		elemType, rest, err := ParseJavaDescription(descriptor[1:])
		if err != nil {
			return nil, "", err
		}
		return NewJavaArrayType(elemType), rest, nil
	default:
		return nil, "", fmt.Errorf("unknown type descriptor: %c", descriptor[0])
	}
}
