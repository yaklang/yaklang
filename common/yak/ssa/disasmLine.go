package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
)

// func setSymbolVariable(v Value, name string) {
// 	symbolLock.Lock()
// 	defer symbolLock.Unlock()
// 	symbol[v] = name
// }

// func unsetSymbolVariable(v Value) {
// 	symbolLock.Lock()
// 	defer symbolLock.Unlock()
// 	delete(symbol, v)
// }

//	func readSymbolVariable(v Value) string {
//		symbolLock.RLock()
//		defer symbolLock.RUnlock()
//		if name, ok := symbol[v]; ok {
//			return name
//		} else {
//			return ""
//		}
//	}
type DisasmLiner struct {
	symbol map[Instruction]string
	level  int
}

func NewDisasmLiner() *DisasmLiner {
	return &DisasmLiner{
		symbol: make(map[Instruction]string),
		level:  0,
	}
}

func lineDisasms(vs Values, liner *DisasmLiner) string {
	return strings.Join(
		lo.Map(vs, func(a Value, _ int) string {
			return lineDisasm(a, liner)
		}),
		",",
	)
}

func LineDisasm(v Instruction) string {
	return lineDisasm(v, NewDisasmLiner())
}

func lineDisasm(v Instruction, liner *DisasmLiner) (ret string) {
	if liner == nil {
		liner = NewDisasmLiner()
	}

	liner.level++
	if liner.level > 100 {
		return "..."
	}

	if name, ok := liner.symbol[v]; ok {
		return name
	}

	defer func() {
		liner.symbol[v] = ret
	}()

	switch v := v.(type) {
	case *Function, *BasicBlock, *Parameter, *ExternLib, *Undefined:
		return fmt.Sprintf("%s", v.GetName())
	case *Phi:
		liner.symbol[v] = v.GetName()
		ret = fmt.Sprintf("phi(%s)[%s]", v.GetName(), lineDisasms(v.Edge, liner))
		delete(liner.symbol, v)
		return ret
	case *ConstInst:
		if v.Const.value != nil && !v.isIdentify && reflect.TypeOf(v.Const.value).Kind() == reflect.String {
			return fmt.Sprintf("\"%s\"", v.String())
		} else {
			return fmt.Sprintf("%s", v.String())
		}
	case *BinOp:
		return fmt.Sprintf("%s(%s, %s)", BinaryOpcodeName[v.Op], lineDisasm(v.X, liner), lineDisasm(v.Y, liner))
	case *UnOp:
		return fmt.Sprintf("%s(%s)", UnaryOpcodeName[v.Op], lineDisasm(v.X, liner))
	case *Call:
		arg := ""
		if len(v.Args) != 0 {
			arg = lineDisasms(v.Args, liner)
		}
		binding := ""
		if len(v.binding) != 0 {
			binding = ", binding(" + lineDisasms(v.binding, liner) + ")"
		}
		return fmt.Sprintf("%s(%s%s)", lineDisasm(v.Method, liner), arg, binding)
	case *SideEffect:
		return fmt.Sprintf("side-effect(%s, %s)", lineDisasm(v.target, liner), v.GetName())
	case *Make:
		return fmt.Sprintf("make(%s)", v.GetType())
	case *Field:
		return fmt.Sprintf("%s.%s", lineDisasm(v.Obj, liner), lineDisasm(v.Key, liner))
	case *Update:
		return fmt.Sprintf("update(%s, %s)", lineDisasm(v.Address, liner), lineDisasm(v.Value, liner))
	case *Next:
		return fmt.Sprintf("next(%s)", lineDisasm(v.Iter, liner))
	case *TypeCast:
		return fmt.Sprintf("castType(%s, %s)", v.GetType().String(), lineDisasm(v.Value, liner))
	case *TypeValue:
		return fmt.Sprintf("typeValue(%s)", v.GetType())
	case *Recover:
		return "recover"
	case *Return:
		return fmt.Sprintf("return(%s)", lineDisasms(v.Results, liner))
	case *Assert:
		return fmt.Sprintf("assert(%s, %s)", lineDisasm(v.Cond, liner), lineDisasm(v.MsgValue, liner))
	case *Panic:
		return fmt.Sprintf("panic(%s)", lineDisasm(v.Info, liner))
	case *Jump, *ErrorHandler:
		return ""
	case *If:
		return fmt.Sprintf("if (%s) {%s} else {%s}", lineDisasm(v.Cond, liner), lineDisasm(v.True, liner), lineDisasm(v.False, liner))
	case *Loop:
		return fmt.Sprintf("loop(%s)", lineDisasm(v.Cond, liner))
	case *Switch:
		return fmt.Sprintf(
			"switch(%s) {case:%s}",
			lineDisasm(v.Cond, liner),
			strings.Join(
				lo.Map(v.Label, func(label SwitchLabel, _ int) string {
					return fmt.Sprintf("%s: %s", lineDisasm(label.Value, liner), lineDisasm(label.Dest, liner))
				}),
				",",
			),
		)
	default:
		return ""
	}
}
