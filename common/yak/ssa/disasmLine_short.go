package ssa

import (
	"fmt"
	"github.com/samber/lo"
	"reflect"
	"strings"
)

func lineShortDisasms(i []Value, liner *DisasmLiner) string {
	return strings.Join(
		lo.Map(i, func(a Value, _ int) string {
			if a.GetName() == "" {
				return fmt.Sprintf("t%v", a.GetId())
			}
			return a.GetName()
		}),
		",",
	)
}

func lineShortDisasmValue(v Value, liner *DisasmLiner) string {
	if v == nil {
		return ""
	}
	if ret := v.GetName(); ret != "" {
		return ret
	}
	return fmt.Sprintf(`t%v`, v.GetId())
}

func lineShortDisasm(v Instruction, liner *DisasmLiner) (ret string) {
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
		ret = fmt.Sprintf("phi(%s)[%s]", v.GetName(), lineShortDisasms(v.Edge, liner))
		delete(liner.symbol, v)
		return ret
	case *ConstInst:
		if v.Const.value != nil && !v.isIdentify && reflect.TypeOf(v.Const.value).Kind() == reflect.String {
			return fmt.Sprintf("\"%s\"", v.String())
		} else {
			return fmt.Sprintf("%s", v.String())
		}
	case *BinOp:
		return fmt.Sprintf("%s(%s, %s)", BinaryOpcodeName[v.Op], lineShortDisasmValue(v.X, liner), lineShortDisasmValue(v.Y, liner))
	case *UnOp:
		return fmt.Sprintf("%s(%s)", UnaryOpcodeName[v.Op], lineShortDisasmValue(v.X, liner))
	case *Call:
		arg := ""
		if len(v.Args) != 0 {
			arg = lineShortDisasms(v.Args, liner)
		}
		binding := ""
		if len(v.binding) != 0 {
			binding = ", binding(" + lineShortDisasms(v.binding, liner) + ")"
		}
		return fmt.Sprintf("%s(%s%s)", lineShortDisasmValue(v.Method, liner), arg, binding)
	case *SideEffect:
		return fmt.Sprintf("side-effect(%s, %s)", lineShortDisasmValue(v.target, liner), v.GetName())
	case *Make:
		return fmt.Sprintf("make(%s)", v.GetType())
	case *Field:
		return fmt.Sprintf("%s.%s", lineShortDisasmValue(v.Obj, liner), lineShortDisasmValue(v.Key, liner))
	case *Update:
		return fmt.Sprintf("update(%s, %s)", lineShortDisasmValue(v.Address, liner), lineShortDisasmValue(v.Value, liner))
	case *Next:
		return fmt.Sprintf("next(%s)", lineShortDisasmValue(v.Iter, liner))
	case *TypeCast:
		return fmt.Sprintf("castType(%s, %s)", v.GetType().String(), lineShortDisasmValue(v.Value, liner))
	case *TypeValue:
		return fmt.Sprintf("typeValue(%s)", v.GetType())
	case *Recover:
		return "recover"
	case *Return:
		return fmt.Sprintf("return(%s)", lineShortDisasms(v.Results, liner))
	case *Assert:
		return fmt.Sprintf("assert(%s, %s)", lineShortDisasmValue(v.Cond, liner), lineShortDisasmValue(v.MsgValue, liner))
	case *Panic:
		return fmt.Sprintf("panic(%s)", lineShortDisasmValue(v.Info, liner))
	case *Jump, *ErrorHandler:
		return ""
	case *If:
		return fmt.Sprintf("if (%s) {%s} else {%s}", lineShortDisasmValue(v.Cond, liner), lineShortDisasmValue(v.True, liner), lineDisasm(v.False, liner))
	case *Loop:
		return fmt.Sprintf("loop(%s)", lineShortDisasmValue(v.Cond, liner))
	case *Switch:
		return fmt.Sprintf(
			"switch(%s) {case:%s}",
			lineShortDisasmValue(v.Cond, liner),
			strings.Join(
				lo.Map(v.Label, func(label SwitchLabel, _ int) string {
					return fmt.Sprintf("%s: %s", lineShortDisasmValue(label.Value, liner), lineShortDisasmValue(label.Dest, liner))
				}),
				",",
			),
		)
	default:
		return ""
	}
}
