package decompiler

import (
	"fmt"
	"strings"
)

func GetRetrieveIdx(code *OpCode) int {
	switch code.Instr.OpCode {
	case OP_ALOAD, OP_ILOAD, OP_LLOAD, OP_DLOAD, OP_FLOAD, OP_IINC:
		res := int(code.Data[0])
		if res < 0 {
			res += 256
		}
		return res
	case OP_ALOAD_0, OP_ILOAD_0, OP_LLOAD_0, OP_DLOAD_0, OP_FLOAD_0:
		return 0
	case OP_ALOAD_1, OP_ILOAD_1, OP_LLOAD_1, OP_DLOAD_1, OP_FLOAD_1:
		return 1
	case OP_ALOAD_2, OP_ILOAD_2, OP_LLOAD_2, OP_DLOAD_2, OP_FLOAD_2:
		return 2
	case OP_ALOAD_3, OP_ILOAD_3, OP_LLOAD_3, OP_DLOAD_3:
		return 3
	case OP_RET:
		return int(code.Data[0])
	default:
		return -1
	}
}
func GetStoreIdx(code *OpCode) int {
	switch code.Instr.OpCode {
	case OP_ASTORE, OP_ISTORE, OP_LSTORE, OP_DSTORE, OP_FSTORE, OP_IINC:
		res := int(code.Data[0])
		if res < 0 {
			res += 256
		}
		return res
	case OP_ASTORE_0, OP_ISTORE_0, OP_LSTORE_0, OP_DSTORE_0, OP_FSTORE_0:
		return 0
	case OP_ASTORE_1, OP_ISTORE_1, OP_LSTORE_1, OP_DSTORE_1, OP_FSTORE_1:
		return 1
	case OP_ASTORE_2, OP_ISTORE_2, OP_LSTORE_2, OP_DSTORE_2, OP_FSTORE_2:
		return 2
	case OP_ASTORE_3, OP_ISTORE_3, OP_LSTORE_3, OP_DSTORE_3, OP_FSTORE_3:
		return 3
	default:
		return -1
	}
}

const (
	EQ  = "eq"
	LT  = "lt"
	GTE = "gte"
	GT  = "gt"
	NE  = "ne"
	LTE = "lte"
)

func GetOp(code *OpCode) string {
	switch code.Instr.OpCode {
	case OP_IF_ICMPEQ, OP_IF_ACMPEQ:
		return EQ
	case OP_IF_ICMPLT:
		return LT
	case OP_IF_ICMPGE:
		return GTE
	case OP_IF_ICMPGT:
		return GT
	case OP_IF_ICMPNE, OP_IF_ACMPNE:
		return NE
	case OP_IF_ICMPLE:
		return LTE
	case OP_IFEQ:
		return EQ
	case OP_IFNE:
		return NE
	case OP_IFLE:
		return LTE
	case OP_IFLT:
		return LT
	case OP_IFGE:
		return GTE
	case OP_IFGT:
		return GT
	default:
		panic(fmt.Sprintf("unknow opcode: 0x%x", code.Instr.OpCode))
	}
}
func Convert2bytesToInt(data []byte) uint16 {
	b1 := uint16(data[0])
	b2 := uint16(data[1])
	return ((b1 & 0xFF) << 8) | (b2 & 0xFF)
}

// parseMethodDescriptor 解析 Java 方法描述符
func ParseMethodDescriptor(descriptor string) ([]string, string, error) {
	if descriptor == "" {
		return nil, "", fmt.Errorf("descriptor is empty")
	}

	if descriptor[0] != '(' {
		return nil, "", fmt.Errorf("invalid descriptor format")
	}

	// 查找参数部分和返回类型部分
	endIndex := strings.Index(descriptor, ")")
	if endIndex == -1 {
		return nil, "", fmt.Errorf("invalid descriptor format")
	}

	paramDescriptor := descriptor[1:endIndex]
	returnTypeDescriptor := descriptor[endIndex+1:]

	// 解析参数类型
	paramTypes, err := parseTypes(paramDescriptor)
	if err != nil {
		return nil, "", err
	}

	// 解析返回类型
	returnType, _, err := parseType(returnTypeDescriptor)
	if err != nil {
		return nil, "", err
	}

	return paramTypes, returnType, nil
}

// parseTypes 解析多个类型描述符
func parseTypes(descriptor string) ([]string, error) {
	var types []string
	for len(descriptor) > 0 {
		t, rest, err := parseType(descriptor)
		if err != nil {
			return nil, err
		}
		types = append(types, t)
		descriptor = rest
	}
	return types, nil
}

// parseType 解析单个类型描述符
func parseType(descriptor string) (string, string, error) {
	if len(descriptor) == 0 {
		return "", "", fmt.Errorf("empty descriptor")
	}

	switch descriptor[0] {
	case 'B':
		return "byte", descriptor[1:], nil
	case 'C':
		return "char", descriptor[1:], nil
	case 'D':
		return "double", descriptor[1:], nil
	case 'F':
		return "float", descriptor[1:], nil
	case 'I':
		return "int", descriptor[1:], nil
	case 'J':
		return "long", descriptor[1:], nil
	case 'S':
		return "short", descriptor[1:], nil
	case 'Z':
		return "boolean", descriptor[1:], nil
	case 'V':
		return "void", descriptor[1:], nil
	case 'L':
		// 类类型，以 L 开头，以 ; 结尾
		endIndex := strings.Index(descriptor, ";")
		if endIndex == -1 {
			return "", "", fmt.Errorf("invalid class descriptor format")
		}
		return descriptor[1:endIndex], descriptor[endIndex+1:], nil
	case '[':
		// 数组类型，以 [ 开头，后跟元素类型
		elemType, rest, err := parseType(descriptor[1:])
		if err != nil {
			return "", "", err
		}
		return "[]" + elemType, rest, nil
	default:
		return "", "", fmt.Errorf("unknown type descriptor: %c", descriptor[0])
	}
}
