package ssa

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/samber/lo"
)

// LineDisasm disasm a instruction to string
func LineDisasm(v Instruction) string {
	return lineDisasm(v, NewFullDisasmLiner(100))
}

// LineShortDisasm disasm a instruction to string, but will use id or name
func LineShortDisasm(v Instruction) string {
	return lineDisasm(v, NewNameDisasmLiner())
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

func (n *NameDisasmLiner) DisasmValue(v Value) string {
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

func (f *FullDisasmLiner) DisasmValue(v Value) string {
	return lineDisasm(v, f)
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
	DisasmValue(v Value) string

	// level += 1; and check should stop?
	// if this method return true, should stop
	AddLevel() bool

	// cache // those method  should use `*cacheDisasmLiner`
	GetName(v Instruction) (string, bool)
	SetName(v Instruction, name string)
	DeleteName(v Instruction)
}

func lineDisasm(v Instruction, liner DisasmLiner) (ret string) {
	if liner.AddLevel() {
		_, isNameLiner := liner.(*NameDisasmLiner)
		if !isNameLiner {
			return "..."
		}
	}

	// check cache and set cache

	DisasmValues := func(vs Values) string {
		return strings.Join(
			lo.Map(vs, func(a Value, _ int) string {
				return liner.DisasmValue(a)
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
	case *Function, *BasicBlock, *Parameter, *ExternLib, *Undefined:
		return fmt.Sprintf("%s", v.GetName())
	case *Phi:
		liner.SetName(v, v.GetName())
		ret = fmt.Sprintf("phi(%s)[%s]", v.GetName(), DisasmValues(v.Edge))
		liner.DeleteName(v)
		return ret
	case *ConstInst:
		if v.Const.value != nil && !v.isIdentify && reflect.TypeOf(v.Const.value).Kind() == reflect.String {
			return fmt.Sprintf("\"%s\"", v.String())
		}
		return fmt.Sprintf("%s", v.String())
	case *BinOp:
		return fmt.Sprintf("%s(%s, %s)", BinaryOpcodeName[v.Op], liner.DisasmValue(v.X), liner.DisasmValue(v.Y))
	case *UnOp:
		return fmt.Sprintf("%s(%s)", UnaryOpcodeName[v.Op], liner.DisasmValue(v.X))
	case *Call:
		arg := ""
		if len(v.Args) != 0 {
			arg = DisasmValues(v.Args)
		}
		binding := ""
		if len(v.binding) != 0 {
			binding = ", binding(" + DisasmValues(v.binding) + ")"
		}
		return fmt.Sprintf("%s(%s%s)", liner.DisasmValue(v.Method), arg, binding)
	case *SideEffect:
		return fmt.Sprintf("side-effect(%s, %s)", liner.DisasmValue(v.target), v.GetName())
	case *Make:
		return fmt.Sprintf("make(%s)", v.GetType())
	case *Field:
		return fmt.Sprintf("%s.%s", liner.DisasmValue(v.Obj), liner.DisasmValue(v.Key))
	case *Update:
		return fmt.Sprintf("update(%s, %s)", liner.DisasmValue(v.Address), liner.DisasmValue(v.Value))
	case *Next:
		return fmt.Sprintf("next(%s)", liner.DisasmValue(v.Iter))
	case *TypeCast:
		return fmt.Sprintf("castType(%s, %s)", v.GetType().String(), liner.DisasmValue(v.Value))
	case *TypeValue:
		return fmt.Sprintf("typeValue(%s)", v.GetType())
	case *Recover:
		return "recover"
	case *Return:
		return fmt.Sprintf("return(%s)", DisasmValues(v.Results))
	case *Assert:
		return fmt.Sprintf("assert(%s, %s)", liner.DisasmValue(v.Cond), liner.DisasmValue(v.MsgValue))
	case *Panic:
		return fmt.Sprintf("panic(%s)", liner.DisasmValue(v.Info))
	case *Jump, *ErrorHandler:
		return ""
	case *If:
		return fmt.Sprintf("if (%s) {%s} else {%s}", liner.DisasmValue(v.Cond), liner.DisasmValue(v.True), liner.DisasmValue(v.False))
	case *Loop:
		return fmt.Sprintf("loop(%s)", liner.DisasmValue(v.Cond))
	case *Switch:
		return fmt.Sprintf(
			"switch(%s) {case:%s}",
			liner.DisasmValue(v.Cond),
			strings.Join(
				lo.Map(v.Label, func(label SwitchLabel, _ int) string {
					return fmt.Sprintf("%s: %s", liner.DisasmValue(label.Value), liner.DisasmValue(label.Dest))
				}),
				",",
			),
		)
	default:
		return ""
	}
}
