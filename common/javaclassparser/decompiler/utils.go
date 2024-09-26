package decompiler

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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
	NEQ  = "!="
	EQ   = "=="
	LT   = "<"
	GTE  = ">="
	GT   = ">"
	NE   = "!="
	LTE  = "<="
	SUB  = "-"
	REM  = "%"
	DIV  = "/"
	MUL  = "*"
	AND  = "&"
	OR   = "|"
	XOR  = "^"
	SHL  = "<<"
	SHR  = ">>"
	USHR = ">>>"
)
const (
	T_BOOLEAN = "boolean"
	T_CHAR    = "char"
	T_FLOAT   = "float"
	T_DOUBLE  = "double"
	T_BYTE    = "byte"
	T_SHORT   = "short"
	T_INT     = "int"
	T_LONG    = "long"
)

func GetPrimerArrayType(id int) JavaType {
	switch id {
	case 4:
		return JavaBoolean
	case 5:
		return JavaChar
	case 6:
		return JavaFloat
	case 7:
		return JavaDouble
	case 8:
		return JavaByte
	case 9:
		return JavaShort
	case 10:
		return JavaInteger
	case 11:
		return JavaLong
	default:
		panic(fmt.Sprintf("unknow primer array type: %d", id))
	}
}
func GetNotOp(code *OpCode) string {
	op := GetOp(code)
	switch op {
	case EQ:
		return NE
	case NE:
		return EQ
	case LT:
		return GTE
	case GTE:
		return LT
	case GT:
		return LTE
	case LTE:
		return GT
	default:
		panic(fmt.Sprintf("unknow opcode: 0x%x", code.Instr.OpCode))
	}
}
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
func Convert4bytesToInt(data []byte) uint32 {
	return binary.BigEndian.Uint32(data)
}
func ParseDescriptor(descriptor string) (JavaType, error) {
	returnType, _, err := parseType(descriptor)
	return returnType, err
}

// parseMethodDescriptor 解析 Java 方法描述符
func ParseMethodDescriptor(descriptor string) ([]JavaType, JavaType, error) {
	if descriptor == "" {
		return nil, nil, fmt.Errorf("descriptor is empty")
	}

	if descriptor[0] != '(' {
		return nil, nil, fmt.Errorf("invalid descriptor format")
	}

	// 查找参数部分和返回类型部分
	endIndex := strings.Index(descriptor, ")")
	if endIndex == -1 {
		return nil, nil, fmt.Errorf("invalid descriptor format")
	}

	paramDescriptor := descriptor[1:endIndex]
	returnTypeDescriptor := descriptor[endIndex+1:]

	// 解析参数类型
	paramTypes, err := parseTypes(paramDescriptor)
	if err != nil {
		return nil, nil, err
	}

	// 解析返回类型
	returnType, _, err := parseType(returnTypeDescriptor)
	if err != nil {
		return nil, nil, err
	}

	return paramTypes, returnType, nil
}

// parseTypes 解析多个类型描述符
func parseTypes(descriptor string) ([]JavaType, error) {
	var types []JavaType
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
func parseFuncType(desc string) (*JavaFuncType, string, error) {
	if desc == "" {
		return nil, "", fmt.Errorf("descriptor is empty")
	}
	if desc[0] != '(' {
		return nil, "", fmt.Errorf("invalid descriptor format")
	}
	endIndex := strings.Index(desc, ")")
	if endIndex == -1 {
		return nil, "", fmt.Errorf("invalid descriptor format")
	}
	paramDesc := desc[1:endIndex]
	returnDesc := desc[endIndex+1:]
	params, err := parseTypes(paramDesc)
	if err != nil {
		return nil, "", err
	}
	returnType, _, err := parseType(returnDesc)
	if err != nil {
		return nil, "", err
	}
	return NewJavaFuncType(desc, params, returnType), "", nil
}

// parseType 解析单个类型描述符
func parseType(descriptor string) (JavaType, string, error) {
	if len(descriptor) == 0 {
		return nil, "", fmt.Errorf("empty descriptor")
	}

	switch descriptor[0] {
	case 'B':
		return JavaByte, descriptor[1:], nil
	case 'C':
		return JavaChar, descriptor[1:], nil
	case 'D':
		return JavaDouble, descriptor[1:], nil
	case 'F':
		return JavaFloat, descriptor[1:], nil
	case 'I':
		return JavaInteger, descriptor[1:], nil
	case 'J':
		return JavaLong, descriptor[1:], nil
	case 'S':
		return JavaShort, descriptor[1:], nil
	case 'Z':
		return JavaBoolean, descriptor[1:], nil
	case 'V':
		return JavaVoid, descriptor[1:], nil
	case 'L':
		// 类类型，以 L 开头，以 ; 结尾
		endIndex := strings.Index(descriptor, ";")
		if endIndex == -1 {
			return nil, "", fmt.Errorf("invalid class descriptor format")
		}
		name := strings.Replace(descriptor[1:endIndex], "/", ".", -1)
		return NewJavaClass(name), descriptor[endIndex+1:], nil
	case '[':
		// 数组类型，以 [ 开头，后跟元素类型
		elemType, rest, err := parseType(descriptor[1:])
		if err != nil {
			return nil, "", err
		}
		switch ret := elemType.(type) {
		case *JavaArrayType:
			ret.Length = append(ret.Length)
			return ret, rest, nil
		default:
			return NewJavaArrayType(elemType), rest, nil
		}
	default:
		return nil, "", fmt.Errorf("unknown type descriptor: %c", descriptor[0])
	}
}

func SplitPackageClassName(s string) (string, string) {
	splits := strings.Split(s, ".")
	if len(splits) > 0 {
		return strings.Join(splits[:len(splits)-1], "."), splits[len(splits)-1]
	}
	log.Errorf("split package name and class name failed: %v", s)
	return "", ""
}

func GetShortName(ctx *FunctionContext, name string) string {
	libs := append(ctx.BuildInLibs, ctx.ClassName)
	for _, lib := range libs {
		pkg, className := SplitPackageClassName(lib)
		fpkg, fclassName := SplitPackageClassName(name)
		if fpkg == pkg && (className == "*" || fclassName == className) {
			return fclassName
		}
	}
	return name
}

func CutNode(src, target *Node) {
	for i, item := range src.Next {
		if item == target {
			src.Next = append(src.Next[:i], src.Next[i+1:]...)
			break
		}
	}
	for i, item := range target.Source {
		if item == src {
			target.Source = append(target.Source[:i], target.Source[i+1:]...)
			break
		}
	}
}
func LinkNode(src, target *Node) {
	target.Source = append(target.Source, src)
	src.Next = append(src.Next, target)
}
func SetOpcode(src, target *OpCode) {
	target.Source = append(target.Source, src)
	src.Target = append(src.Target, target)
}
func ShowStatementNodes(nodes []*Node) {
	funcCtx := &FunctionContext{}
	for _, item := range nodes {
		fmt.Printf("%d %s\n", item.Id, item.Statement.String(funcCtx))
	}
}
func ShowOpcodes(opcodes []*OpCode) {
	for i, opcode := range opcodes {
		fmt.Printf("%d %s\n", i, opcode.Instr.Name)
	}
}

//func WalkGraph[T any](node T, next func(T) []T, visit func(T)) {
//	visit(node)
//	for _, n := range next(node) {
//		WalkGraph(n, next, visit)
//	}
//}

func WalkGraph[T any](node T, next func(T) ([]T, error)) error {
	stack := utils.NewStack[T]()
	visited := utils.NewSet[any]()
	stack.Push(node)
	for stack.Len() > 0 {
		current := stack.Pop()
		if visited.Has(current) {
			continue
		}
		visited.Add(current)
		n, err := next(current)
		if err != nil {
			return err
		}
		for _, n := range n {
			stack.Push(n)
		}
	}
	return nil
}

func StatementsString(statements []Statement, funcCtx *FunctionContext) string {
	var res string
	for _, statement := range statements {
		res += statement.String(funcCtx)
	}
	return res
}

func NodeToStatement(nodes []*Node) []Statement {
	res := make([]Statement, len(nodes))
	for i, node := range nodes {
		res[i] = node.Statement
	}
	return res
}

func VisitBody(raw Statement, f func(statement Statement) Statement) Statement {
	switch ret := raw.(type) {
	case *SwitchStatement:
		for _, item := range ret.Cases {
			for i, bodyItem := range item.Body {
				item.Body[i] = VisitBody(bodyItem, f)
			}
		}
		return ret
	case *IfStatement:
		for i, bodyItem := range ret.IfBody {
			ret.IfBody[i] = VisitBody(bodyItem, f)
		}
		for i, bodyItem := range ret.ElseBody {
			ret.ElseBody[i] = VisitBody(bodyItem, f)
		}
		return ret
	case *ForStatement:
		for i, bodyItem := range ret.SubStatements {
			ret.SubStatements[i] = VisitBody(bodyItem, f)
		}
		return ret
	default:
		return f(ret)
	}
}
