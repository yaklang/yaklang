package ssa

import (
	"fmt"
	"go/constant"
	"go/types"
	"strings"
	"sync"

	"github.com/samber/lo"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"golang.org/x/exp/slices"
)

var (
	ConstMap = make(map[any]*Const)
)

type Value interface {
	String() string

	GetUsers() []User
	AddUser(User)
	RemoveUser(User)
}

type User interface {
	Value

	String() string

	GetValues() []Value
	AddValue(Value)

	ReplaceValue(Value, Value)
}

type Instruction interface {
	User

	GetParent() *Function
	GetBlock() *BasicBlock

	String() string
	StringByFunc(func(Value) string) string
}

func DefaultValueString(v Value) string {
	op := ""
	switch v := v.(type) {
	case Instruction:
		op = "t0"
	case *Const:
		op = v.String()
	case *Parameter:
		op = v.String()
	default:
		panic("instruction unknow value type: " + v.String())
	}
	return op
}

type Program struct {
	// package list
	Packages []*Package

	// for build
	ast *yak.YaklangParser
}

type Package struct {
	name string
	// point to program
	Prog *Program
	// function list
	funcs []*Function

	// for build
	buildOnece sync.Once
	ast        *yak.YaklangParser
}

// implement Value
type Function struct {
	name string

	// package
	Package *Package

	Param []*Parameter

	// BasicBlock list
	Blocks     []*BasicBlock
	EnterBlock *BasicBlock
	ExitBlock  *BasicBlock

	// anonymous function in this function
	AnonFuncs []*Function

	// if this function is anonFunc
	parent *Function // parent function if anonymous function; nil if global function.
	FreeValues []Value    // the value, captured variable form parent-function,
	symbol     *Interface // for function symbol table

	// User
	user []User

	// for build
	target       *target
	currentBlock *BasicBlock
	currentDef   map[string]map[*BasicBlock]Value // currentDef[variable][block]value
}

// implement Value
type BasicBlock struct {
	Index int
	Name  string
	// function
	Parent *Function
	// basicblock graph
	Preds, Succs []*BasicBlock

	// instruction list
	Instrs []Instruction
	Phis   []*Phi

	// for build
	finish        bool // if emitJump finish!
	isSealed      bool
	inCompletePhi map[string]*Phi // variable -> phi

	// User
	user []User
}

type anInstruction struct {
	// function
	Func *Function
	// basicblock
	Block *BasicBlock
	// type
	typ types.Type
}

// value

// phi
// instruction
type Phi struct {
	anInstruction
	Edge []Value // edge[i] from phi.Block.Preds[i]
	user []User
	// for build
	variable string
}

// const
// only Value
type Const struct {
	user  []User
	value constant.Value
}

func NewConst(i any) *Const {
	c, ok := ConstMap[i]
	if !ok {
		c = &Const{
		user:  []User{},
		value: constant.Make(i),
	}
		// const should same
		// assert newConst(1) ==newConst(1)
		ConstMap[i] = c
	}
	return c
}

// parameter
// only value
type Parameter struct {
	variable    string
	Func        *Function
	isFreevalue bool

	user []User
}

// control-flow instructions  ----------------------------------------
// jump / if / return / call / switch

// The Jump instruction transfers control to the sole successor of its
// owning block.
//
// the block containing Jump instruction only have one successor block
type Jump struct {
	anInstruction
	To *BasicBlock
}

// The If instruction transfers control to one of the two successors
// of its owning block, depending on the boolean Cond: the first if
// true, the second if false.
type If struct {
	anInstruction
	Cond  Value
	True  *BasicBlock
	False *BasicBlock
	user  []User
}

// The Return instruction returns values and control back to the calling
// function.
type Return struct {
	anInstruction
	Results []Value
}

// call instruction call method function  with args as argument
type Call struct {
	anInstruction

	// for call function
	Method Value
	Args   []Value

	// call is a value
	user []User

	// ~ drop error
	isDropError bool
}

type Switch struct {
	anInstruction

	cond Value
}

// data-flow instructions  ----------------------------------------
// BinOp / UnOp

type BinOp struct {
	anInstruction

	Op   yakvm.OpcodeFlag
	X, Y Value
	user []User
}

type UnOp struct {
	anInstruction

	Op yakvm.OpcodeFlag
	X  Value
}

// special instruction ------------------------------------------

type InterfaceType int

const (
	InterfaceSlice = iota
	InterfaceStruct
	InterfaceMap
	InterfaceGlobal
)

// instruction + value + user
type Interface struct {
	anInstruction

	ITyp InterfaceType
	// when slice
	low, high, max Value

	parentI *Interface // parent interface

	// field
	field map[Value]*Field // field.key->field

	// when slice or map
	Len, Cap Value

	users []User
}

// instruction
type Field struct {
	anInstruction

	// field
	Key Value
	I   *Interface

	update []Value // value

	users []User

	//TODO:map[users]update
	// i can add the map[users]update,
	// to point what update value when user use this field

}

type Update struct {
	anInstruction
	value   Value
	address User
}

// implement value
func (f *Function) String() string {
	ret := f.name + " "
	ret += strings.Join(
		lo.Map(f.Param, func(item *Parameter, _ int) string { return item.variable }),
		", ")
	ret += "\n"

	if parent := f.parent; parent != nil {
		ret += "parent: " + parent.name + "\n"
	}

	instReg := make(map[Instruction]string)
	regindex := 0

	// init instReg
	newName := func() string {
		ret := fmt.Sprintf("t%d", regindex)
		regindex += 1
		return ret
	}

	// print instruction
	getStr := func(v Value) string {
		op := ""
		switch v := v.(type) {
		case Instruction:
			if i, ok := v.(*Interface); ok {
				if i.ITyp == InterfaceGlobal {
					return i.Func.name + "-symbol"
				}
			}
			if name, ok := instReg[v]; ok {
				op = name
			} else {
				op = newName()
				instReg[v] = op
			}
		case *Const:
			op = v.String()
		case *Parameter:
			op = v.String()
		default:
			panic("instruction unknow value type: " + v.String())
		}
		return op
	}

	if len(f.FreeValues) > 0 {
		ret += "freeValue: " + strings.Join(
			lo.Map(f.FreeValues, func(key Value, _ int) string {
				return getStr(key)
			}),
			// f.FreeValue,
			", ") + "\n"
	}

	setInst := func(i Instruction) {
		if _, ok := instReg[i]; !ok {
			instReg[i] = newName()
		}
	}

	for _, b := range f.Blocks {
		for _, i := range b.Instrs {
			setInst(i)
		}
		slices.SortFunc(b.Phis, func(p1 *Phi, p2 *Phi) bool {
			return p1.variable < p2.variable
		})
		for _, p := range b.Phis {
			setInst(p)
		}
	}

	handlerInst := func(i Instruction) string {
		ret := "\t" + i.StringByFunc(getStr) + "\n"
		return ret
	}

	for _, b := range f.Blocks {
		ret += b.String() + "\n"
		for _, p := range b.Phis {
			ret += handlerInst(p)
		}
		for _, i := range b.Instrs {
			ret += handlerInst(i)
		}
	}
	return ret
}

var _ Value = (*Function)(nil)

func (b *BasicBlock) String() string {
	ret := b.Name + ":"
	if len(b.Preds) != 0 {
		ret += " <- "
		for _, pred := range b.Preds {
			ret += pred.Name + " "
		}
	}
	return ret
}

var _ Value = (*BasicBlock)(nil)

// implement instruction
func (a *anInstruction) GetBlock() *BasicBlock { return a.Block }
func (a *anInstruction) GetParent() *Function  { return a.Func }

// ----------- Phi
func (p *Phi) String() string {
	return p.StringByFunc(DefaultValueString)
}

func (p *Phi) StringByFunc(getStr func(Value) string) string {
	ret := fmt.Sprintf("%s = phi ", getStr(p))
	for i := range p.Edge {
		v := p.Edge[i]
		b := p.Block.Preds[i]
		if v == nil {
			continue
		}
		ret += fmt.Sprintf("[%s, %s] ", getStr(v), b.Name)
	}
	return ret
}

var _ Value = (*Phi)(nil)
var _ User = (*Phi)(nil)
var _ Instruction = (*Phi)(nil)

// ----------- Const
func (c Const) String() string {
	return strings.Trim(c.value.String(), "\"")
}

var _ Value = (*Const)(nil)

// ----------- Parameter
func (p *Parameter) String() string {
	return p.variable
}

var _ Value = (*Parameter)(nil)

// ----------- Jump
func (j *Jump) String() string {
	return j.StringByFunc(DefaultValueString)
}

func (j *Jump) StringByFunc(_ func(Value) string) string {
	return fmt.Sprintf("jump -> %s", j.To.Name)
}

var _ Value = (*Jump)(nil)
var _ User = (*Jump)(nil)
var _ Instruction = (*Jump)(nil)

// ----------- IF
func (i *If) String() string {
	return i.StringByFunc(DefaultValueString)
}
func (i *If) StringByFunc(getStr func(Value) string) string {
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(i.Cond), i.True.Name, i.False.Name)
}

var _ Value = (*If)(nil)
var _ User = (*If)(nil)
var _ Instruction = (*If)(nil)

// ----------- Return
func (r *Return) String() string {
	return r.StringByFunc(DefaultValueString)
}

func (r *Return) StringByFunc(getStr func(Value) string) string {
	return fmt.Sprintf(
		"ret %s",
		strings.Join(
			lo.Map(r.Results, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
	)
}

var _ Value = (*Return)(nil)
var _ User = (*Return)(nil)
var _ Instruction = (*Return)(nil)

// ----------- Call
func (c *Call) String() string {
	return c.StringByFunc(DefaultValueString)
}

func (c *Call) StringByFunc(getStr func(Value) string) string {
	return fmt.Sprintf(
		"%s = call %s (%s)",
		getStr(c),
		getStr(c.Method),
		strings.Join(
			lo.Map(c.Args, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
	)
}

var _ Value = (*Call)(nil)
var _ User = (*Call)(nil)
var _ Instruction = (*Call)(nil)

// ----------- BinOp
func (b *BinOp) String() string {
	return b.StringByFunc(DefaultValueString)
}

func (b *BinOp) StringByFunc(getStr func(Value) string) string {
	return fmt.Sprintf("%s = %s %s %s", getStr(b), getStr(b.X), yakvm.OpcodeToName(b.Op), getStr(b.Y))
}

var _ Value = (*BinOp)(nil)
var _ User = (*BinOp)(nil)
var _ Instruction = (*BinOp)(nil)

// ----------- Interface
func (i *Interface) String() string {
	return i.StringByFunc(DefaultValueString)
}

func (i *Interface) StringByFunc(Str func(Value) string) string {
	if i.ITyp == InterfaceGlobal {
		return i.Func.name + "-symbol"
	} else {
		return fmt.Sprintf(
			"%s = Interface %s [%s, %s]",
			Str(i), i.typ, Str(i.Len), Str(i.Cap),
		)
	}
}

var _ Value = (*Interface)(nil)
var _ User = (*Interface)(nil)
var _ Instruction = (*Interface)(nil)

// ----------- Field
func (f *Field) String() string {
	return f.StringByFunc(DefaultValueString)
}

func (f *Field) StringByFunc(Str func(Value) string) string {
	return fmt.Sprintf(
		"%s = %s field[%s]",
		Str(f), Str(f.I), Str(f.Key),
	)
}

var _ Value = (*Field)(nil)
var _ User = (*Field)(nil)
var _ Instruction = (*Field)(nil)

// ----------- Update

func (s *Update) String() string {
	return s.StringByFunc(DefaultValueString)
}

func (s *Update) StringByFunc(Str func(Value) string) string {
	return fmt.Sprintf(
		"update [%s] = %s",
		Str(s.address), Str(s.value),
	)
}

var _ Value = (*Update)(nil)
var _ User = (*Update)(nil)
var _ Instruction = (*Update)(nil)
