package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
)

// LineDisASM disasm a instruction to string
func LineDisASM(v Instruction) string {
	return lineDisASM(v, NewFullDisasmLiner(100))
}

// LineShortDisASM disasm a instruction to string, but will use id or name
func LineShortDisASM(v Instruction) string {
	return lineDisASM(v, NewNameDisasmLiner())
}

type NameDisasmLiner struct {
	*cacheDisasmLiner
}

var _ DisasmLiner = (*NameDisasmLiner)(nil)

func NewNameDisasmLiner() *NameDisasmLiner {
	return &NameDisasmLiner{
		cacheDisasmLiner: newCacheDisasmLiner(),
	}
}

func (n *NameDisasmLiner) DisasmValue(v Instruction) string {
	if v == nil {
		return ""
	}
	if ret := v.GetName(); ret != "" {
		return ret
	}
	return fmt.Sprintf(`t%v`, v.GetId())
}

func (n *NameDisasmLiner) AddLevel() bool {
	return true
}

type FullDisasmLiner struct {
	*cacheDisasmLiner
	maxLevel int
	level    int
}

var _ DisasmLiner = (*FullDisasmLiner)(nil)

func NewFullDisasmLiner(max int) *FullDisasmLiner {
	return &FullDisasmLiner{
		cacheDisasmLiner: newCacheDisasmLiner(),
		maxLevel:         max,
		level:            0,
	}
}

func (f *FullDisasmLiner) DisasmValue(v Instruction) string {
	return lineDisASM(v, f)
}

func (f *FullDisasmLiner) AddLevel() bool {
	f.level++
	return f.level > 100
}

type cacheDisasmLiner struct {
	cache map[Instruction]string
}

func newCacheDisasmLiner() *cacheDisasmLiner {
	return &cacheDisasmLiner{
		cache: make(map[Instruction]string),
	}
}

func (b *cacheDisasmLiner) GetName(i Instruction) (string, bool) {
	name, ok := b.cache[i]
	return name, ok
}

func (b *cacheDisasmLiner) SetName(i Instruction, name string) {
	b.cache[i] = name
}

func (b *cacheDisasmLiner) DeleteName(i Instruction) {
	delete(b.cache, i)
}

type DisasmLiner interface {
	DisasmValue(v Instruction) string

	// level += 1; and check should stop?
	// if this method return true, should stop
	AddLevel() bool
	SkipLevelChecking() bool

	// cache // those method  should use `*cacheDisasmLiner`
	GetName(v Instruction) (string, bool)
	SetName(v Instruction, name string)
	DeleteName(v Instruction)
}

func (b *NameDisasmLiner) SkipLevelChecking() bool {
	return true
}

func (b *FullDisasmLiner) SkipLevelChecking() bool {
	return false
}

func lineDisASM(v Instruction, liner DisasmLiner) (ret string) {
	if liner.AddLevel() && !liner.SkipLevelChecking() {
		return "..."
	}

	// check cache and set cache

	DisasmValue := func(ids ...int64) string {
		return strings.Join(
			lo.Map(ids, func(id int64, _ int) string {
				value, ok := v.GetValueById(id)
				if !ok || value == nil {
					return fmt.Sprintf("<nil:%d>", id)
				}
				return liner.DisasmValue(value)
			}),
			",",
		)
	}

	if name, ok := liner.GetName(v); ok {
		return name
	}

	defer func() {
		liner.SetName(v, ret)
	}()

	switch v := v.(type) {
	case *ParameterMember:
		return fmt.Sprintf("%s-%s", SSAOpcode2Name[v.GetOpcode()], v.String())
	case *Parameter:
		return fmt.Sprintf("%s-%s", SSAOpcode2Name[v.GetOpcode()], v.GetName())
	case *Function, *BasicBlock, *ExternLib:
		return fmt.Sprintf("%s-%s", SSAOpcode2Name[v.GetOpcode()], v.GetVerboseName())
	case *Undefined:
		if v.Kind == UndefinedMemberValid {
			return fmt.Sprintf("%s-%s(valid)", SSAOpcode2Name[v.GetOpcode()], v.GetVerboseName())
		}
		return fmt.Sprintf("%s-%s", SSAOpcode2Name[v.GetOpcode()], v.GetVerboseName())
	case *Phi:
		liner.SetName(v, v.GetVerboseName())
		ret = fmt.Sprintf("phi(%s)[%s]", v.GetVerboseName(), DisasmValue(v.Edge...))
		liner.DeleteName(v)
		return ret
	case *ConstInst:
		if v.Const != nil && v.Const.value != nil && !v.isIdentify && reflect.TypeOf(v.Const.value).Kind() == reflect.String {
			return fmt.Sprintf("%#v", v.String())
		}
		return v.String()
	case *BinOp:
		return fmt.Sprintf("%s(%s, %s)", v.Op, DisasmValue(v.X), DisasmValue(v.Y))
	case *UnOp:
		return fmt.Sprintf("%s(%s)", v.Op, DisasmValue(v.X))
	case *Call:
		arg := ""
		if len(v.Args) != 0 {
			arg = DisasmValue(v.Args...)
		}
		binding := ""
		if len(v.Binding) != 0 {
			binding = " binding[" + DisasmValue(
				lo.MapToSlice(
					v.Binding,
					func(key string, item int64) int64 { return item },
				)...,
			) + "]"
		}
		member := ""
		if len(v.ArgMember) != 0 {
			member = " member[" + DisasmValue(v.ArgMember...) + "]"
		}
		return fmt.Sprintf("%s(%s)%s%s", DisasmValue(v.Method), arg, binding, member)
	case *SideEffect:
		return fmt.Sprintf("side-effect(%s, %s)", DisasmValue(v.Value), v.GetVerboseName())
	case *Make:
		if v.name != "" {
			return v.name
		}
		typ := v.GetType()
		return fmt.Sprintf("make(%v)", typ.String())
	case *Next:
		return fmt.Sprintf("next(%s)", DisasmValue(v.Iter))
	case *TypeCast:
		return fmt.Sprintf("castType(%s, %s)", v.GetType().String(), DisasmValue(v.Value))
	case *TypeValue:
		return fmt.Sprintf("typeValue(%s)", v.GetType())
	case *Recover:
		return "recover"
	case *Return:
		return fmt.Sprintf("return(%s)", DisasmValue(v.Results...))
	case *Assert:
		return fmt.Sprintf("assert(%s, %s)", DisasmValue(v.Cond), DisasmValue(v.MsgValue))
	case *Panic:
		return fmt.Sprintf("panic(%s)", DisasmValue(v.Info))
	case *Jump:
		return "jump"
	case *ErrorHandler:
		return "error-handler"
	case *ErrorCatch:
		if exception, ok := v.GetValueById(v.Exception); ok && exception != nil {
			return fmt.Sprintf("error-catch(%s)", liner.DisasmValue(exception))
		}
		return fmt.Sprintf("error-catch(<nil:%d>)", v.Exception)
	case *If:
		return fmt.Sprintf("if (%s)", DisasmValue(v.Cond))
	case *Loop:
		return fmt.Sprintf("loop(%s)", DisasmValue(v.Cond))
	case *Switch:
		return fmt.Sprintf("switch(%s)", DisasmValue(v.Cond))
	case *LazyInstruction:
		// switch liner.(type) {
		// case *NameDisasmLiner:
		// 	if s := v.ir.ReadableNameShort; s != "" {
		// 		return s
		// 	}
		// case *FullDisasmLiner:
		// 	// if s := v.ir.ReadableName; s != "" {
		// 	// 	return s
		// 	// }
		// default:
		// 	// return ""
		// }
		return lineDisASM(v.Self(), liner)

	default:
		return ""
	}
}
