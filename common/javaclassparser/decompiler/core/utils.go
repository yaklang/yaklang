package core

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/statements"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func GetTypeSize(typ types.JavaType) int {
	funCtx := &class_context.ClassContext{}
	typStr := typ.String(funCtx)
	if typStr == types.NewJavaPrimer(types.JavaLong).String(funCtx) || typStr == types.NewJavaPrimer(types.JavaDouble).String(funCtx) {
		return 2
	} else {
		return 1
	}
}
func GetRetrieveIdx(code *OpCode) int {
	if code.IsWide {
		return int(Convert2bytesToInt(code.Data))
	}
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
	if code.IsWide {
		return int(Convert2bytesToInt(code.Data))
	}
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
func GetReverseOp(op string) string {
	switch op {
	case values.EQ:
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
		panic(fmt.Sprintf("unknow opcode: %s", op))
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

func StatementsString(statements []statements.Statement, funcCtx *class_context.ClassContext) string {
	var res string
	for _, statement := range statements {
		res += statement.String(funcCtx)
	}
	return res
}

func VisitBody(raw statements.Statement, f func(statement statements.Statement) statements.Statement) statements.Statement {
	switch ret := raw.(type) {
	case *statements.SwitchStatement:
		for _, item := range ret.Cases {
			for i, bodyItem := range item.Body {
				item.Body[i] = VisitBody(bodyItem, f)
			}
		}
		return ret
	case *statements.IfStatement:
		for i, bodyItem := range ret.IfBody {
			ret.IfBody[i] = VisitBody(bodyItem, f)
		}
		for i, bodyItem := range ret.ElseBody {
			ret.ElseBody[i] = VisitBody(bodyItem, f)
		}
		return ret
	case *statements.ForStatement:
		for i, bodyItem := range ret.SubStatements {
			ret.SubStatements[i] = VisitBody(bodyItem, f)
		}
		return ret
	default:
		return f(ret)
	}
}

func SetOpcode(src, target *OpCode) {
	target.Source = append(target.Source, src)
	src.Target = append(src.Target, target)
}

func ShowOpcodes(opcodes []*OpCode) {
	for i, opcode := range opcodes {
		fmt.Printf("%d %s\n", i, opcode.Instr.Name)
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

//
//func GetShortName(ctx *class_context.ClassContext, name string) string {
//	libs := append(ctx.BuildInLibs, ctx.ClassName)
//	for _, lib := range libs {
//		pkg, className := SplitPackageClassName(lib)
//		fpkg, fclassName := SplitPackageClassName(name)
//		if fpkg == pkg && (className == "*" || fclassName == className) {
//			return fclassName
//		}
//	}
//	return name
//}

func NodesToStatements(nodes []*Node) []statements.Statement {
	var result []statements.Statement
	for _, item := range nodes {
		result = append(result, item.Statement)
	}
	return result
}

func CutNode(src, target *Node) func() {
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
	return func() {
		src.Next = append(src.Next, target)
		target.Source = append(target.Source, src)
	}
}

func GraphToList(code *OpCode) []*OpCode {
	res := []*OpCode{}
	WalkGraph[*OpCode](code, func(code *OpCode) ([]*OpCode, error) {
		res = append(res, code)
		return code.Target, nil
	})
	return res
}

func NodeFilter(nodes []*Node, f func(*Node) bool) []*Node {
	var res []*Node
	for _, node := range nodes {
		if f(node) {
			res = append(res, node)
		}
	}
	return res
}