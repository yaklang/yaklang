package ssa

import (
	"fmt"
	"go/types"
	"reflect"
	"strings"
	"sync"

	"github.com/samber/lo"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
	"golang.org/x/exp/slices"
)

type Type types.Type
type Types []Type

// return true  if org != typs
// return false if org == typs
func (org Types) Compare(typs Types) bool {
	if len(org) == 0 && len(typs) != 0 {
		return true
	}
	return slices.CompareFunc(org, typs, func(org, typ Type) int {
		if types.Identical(org, typ) {
			return 0
		}
		return 1
	}) != 0
}

func (t Types) String() string {
	return strings.Join(
		lo.Map(t, func(typ Type, _ int) string {
			if typ == nil {
				return "nil"
			} else {
				return typ.String()
			}
		}),
		", ",
	)
}

var (
	ConstMap = make(map[any]*Const)
)

// TODO
// data flow graph node
type Node interface {
	String() string

	GetType() Types

	GetUsers() []User
	GetValues() []Value
}

type Value interface {
	Node

	String() string

	GetUsers() []User
	AddUser(User)
	RemoveUser(User)

	SetType(Types)
}

type User interface {
	Node

	String() string

	GetValues() []Value
	AddValue(Value)

	ReplaceValue(Value, Value)

	// inference type
	InferenceType()
}

type Instruction interface {
	GetParent() *Function
	GetBlock() *BasicBlock

	String() string
	// dis-asm
	StringByFunc(func(Node) string) string
	// asm
	// ParseByString(string) *Function

	// pos
	Pos() string
}

func DefaultValueString(v Node) string {
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

	Param  []*Parameter
	Return []*Return

	// BasicBlock list
	Blocks     []*BasicBlock
	EnterBlock *BasicBlock
	ExitBlock  *BasicBlock

	// anonymous function in this function
	AnonFuncs []*Function

	// if this function is anonFunc
	parent     *Function  // parent function if anonymous function; nil if global function.
	FreeValues []Value    // the value, captured variable form parent-function,
	symbol     *Interface // for function symbol table

	// User
	user []User
	Pos  *Position // current position

	// for build
	currtenPos   *Position
	currentBlock *BasicBlock                      // current block to build
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

type Position struct {
	// SourceCodeFilePath *string
	SourceCode  string
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

type anInstruction struct {
	// function
	Func *Function
	// basicblock
	Block *BasicBlock
	// type
	typs Types

	// source code position
	pos *Position
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
	value any
	typ   Types
	str   string
}

// parameter
// only value
type Parameter struct {
	variable    string
	Func        *Function
	isFreevalue bool
	typs        Types

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

	binding []Value

	// caller
	caller Value
	// ~ drop error
	isDropError bool
}

type switchlabel struct {
	value Value
	dest  *BasicBlock
}
type Switch struct {
	anInstruction

	cond         Value
	defaultBlock *BasicBlock

	label []switchlabel
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
	address *Field
}

type FunctionAsmFlag int

const (
	DisAsmDefault FunctionAsmFlag = 1 << iota
	DisAsmWithoutSource
)

// implement value
func (f *Function) String() string {
	return f.DisAsm(DisAsmDefault)
}
func (f *Function) DisAsm(flag FunctionAsmFlag) string {
	ret := f.name + " "
	ret += strings.Join(
		lo.Map(f.Param, func(item *Parameter, _ int) string { return item.variable }),
		", ")
	ret += "\n"

	if parent := f.parent; parent != nil {
		ret += fmt.Sprintf("parent: %s\n", parent.name)
	}

	if f.Pos != nil {
		ret += fmt.Sprintf("pos: %s\n", f.Pos)
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
	getStr := func(v Node) string {
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
		case *Function:
			op = v.name
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

	if len(f.Return) > 0 {
		ret += "return: " + strings.Join(
			lo.Map(f.Return, func(key *Return, _ int) string {
				return getStr(key)
			}),
			", ") + "\n"
	}

	handlerInst := func(i Instruction) string {
		ret := fmt.Sprintf(
			"\t%s",
			i.StringByFunc(getStr),
		)
		return ret
	}

	for _, b := range f.Blocks {
		ret += b.String() + "\n"
		if flag&DisAsmWithoutSource == 0 {
			for _, p := range b.Phis {
				ret += handlerInst(p) + "\n"
			}
			for _, i := range b.Instrs {
				ret += handlerInst(i) + "\n"
			}
		} else {
			insts := make([]string, 0, len(b.Instrs)+len(b.Phis))
			pos := make([]string, 0, len(b.Instrs)+len(b.Phis))
			for _, p := range b.Phis {
				insts = append(insts, handlerInst(p))
				pos = append(pos, p.Pos())
			}
			for _, i := range b.Instrs {
				insts = append(insts, handlerInst(i))
				pos = append(pos, i.Pos())
			}
			// get maxlen
			max := 0
			for _, s := range insts {
				if len(s) > max {
					max = len(s)
				}
			}
			format := fmt.Sprintf("\t%%-%ds\t\t%%s\n", max)
			for i := range insts {
				ret += fmt.Sprintf(format, insts[i], pos[i])
			}
		}
	}
	return ret
}

func (f *Function) GetType() Types {
	return nil
}

func (f *Function) SetType(ts Types) {
}

var _ Node = (*Function)(nil)
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

func (b *BasicBlock) GetType() Types {
	return nil
}

func (b *BasicBlock) SetType(ts Types) {
}

var _ Node = (*BasicBlock)(nil)
var _ Value = (*BasicBlock)(nil)

func (p *Position) String() string {
	return fmt.Sprintf(
		"%3d:%-3d - %3d:%-3d: %s",
		p.StartLine, p.StartColumn,
		p.EndLine, p.EndColumn,
		p.SourceCode,
	)
}

// implement instruction
func (a *anInstruction) GetBlock() *BasicBlock { return a.Block }
func (a *anInstruction) GetParent() *Function  { return a.Func }
func (a *anInstruction) Pos() string {
	if a.pos != nil {
		return a.pos.String()
	} else {
		return ""
	}
}
func (a *anInstruction) GetType() Types {
	return a.typs
}

func (a *anInstruction) SetType(ts Types) {
	a.typs = ts
}

// ----------- Phi
func (p *Phi) String() string {
	return p.StringByFunc(DefaultValueString)
}

func (p *Phi) StringByFunc(getStr func(Node) string) string {
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

var _ Node = (*Phi)(nil)
var _ Value = (*Phi)(nil)
var _ User = (*Phi)(nil)
var _ Instruction = (*Phi)(nil)

// ----------- Const
// create const
func NewConst(i any) *Const {
	c, ok := ConstMap[i]
	if !ok {
		// build new const
		typestr := reflect.TypeOf(i).String()
		c = &Const{
			user:  make([]User, 0),
			value: i,
			typ:   Types{basicTypes[typestr]},
			str:   fmt.Sprintf("%v", i),
		}
		// const should same
		// assert newConst(1) ==newConst(1)
		ConstMap[i] = c
	}
	return c
}

// string
func (c Const) String() string {
	return c.str
}

// get type
func (c Const) GetType() Types {
	return c.typ
}

func (c *Const) SetType(ts Types) {
	c.typ = ts
}

var _ Node = (*Const)(nil)
var _ Value = (*Const)(nil)

// ----------- Parameter
func (p *Parameter) String() string {
	return p.variable
}
func (p *Parameter) GetType() Types {
	return p.typs
}

func (p *Parameter) SetType(ts Types) {
	p.typs = ts
}

var _ Node = (*Parameter)(nil)
var _ Value = (*Parameter)(nil)

// ----------- Jump
func (j *Jump) String() string {
	return j.StringByFunc(DefaultValueString)
}

func (j *Jump) StringByFunc(_ func(Node) string) string {
	return fmt.Sprintf("jump -> %s", j.To.Name)
}

var _ Instruction = (*Jump)(nil)

// ----------- IF
func (i *If) String() string {
	return i.StringByFunc(DefaultValueString)
}
func (i *If) StringByFunc(getStr func(Node) string) string {
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(i.Cond), i.True.Name, i.False.Name)
}

var _ Node = (*If)(nil)
var _ User = (*If)(nil)
var _ Instruction = (*If)(nil)

// ----------- Return
func (r *Return) String() string {
	return r.StringByFunc(DefaultValueString)
}

func (r *Return) StringByFunc(getStr func(Node) string) string {
	return fmt.Sprintf(
		"ret %s",
		strings.Join(
			lo.Map(r.Results, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
	)
}

var _ Node = (*Return)(nil)
var _ User = (*Return)(nil)
var _ Instruction = (*Return)(nil)

// ----------- Call
func (c *Call) String() string {
	return c.StringByFunc(DefaultValueString)
}

func (c *Call) StringByFunc(getStr func(Node) string) string {
	return fmt.Sprintf(
		"%s = call %s (%s) [%s]",
		getStr(c),
		getStr(c.Method),
		strings.Join(
			lo.Map(c.Args, func(v Value, _ int) string { return getStr(v) }),
			", ",
		),
		strings.Join(
			lo.Map(c.binding, func(v Value, _ int) string {
				return getStr(v)
			}),
			", ",
		),
	)
}

var _ Node = (*Call)(nil)
var _ Value = (*Call)(nil)
var _ User = (*Call)(nil)
var _ Instruction = (*Call)(nil)

// ----------- Switch
func (sw *Switch) String() string {
	return sw.StringByFunc(DefaultValueString)
}

func (sw *Switch) StringByFunc(Str func(Node) string) string {
	return fmt.Sprintf(
		"switch %s default:[%s] {%s}",
		Str(sw.cond),
		sw.defaultBlock.Name,
		strings.Join(
			lo.Map(sw.label, func(label switchlabel, _ int) string {
				return fmt.Sprintf("%s:%s", Str(label.value), label.dest.Name)
			}),
			", ",
		),
	)
}

var _ Node = (*Switch)(nil)
var _ User = (*Switch)(nil)
var _ Instruction = (*Switch)(nil)

// ----------- BinOp
func (b *BinOp) String() string {
	return b.StringByFunc(DefaultValueString)
}

func (b *BinOp) StringByFunc(getStr func(Node) string) string {
	return fmt.Sprintf("%s = %s %s %s", getStr(b), getStr(b.X), yakvm.OpcodeToName(b.Op), getStr(b.Y))
}

var _ Value = (*BinOp)(nil)
var _ User = (*BinOp)(nil)
var _ Node = (*BinOp)(nil)
var _ Instruction = (*BinOp)(nil)

// ----------- Interface
func (i *Interface) String() string {
	return i.StringByFunc(DefaultValueString)
}

func (i *Interface) StringByFunc(Str func(Node) string) string {
	if i.ITyp == InterfaceGlobal {
		return i.Func.name + "-symbol"
	} else {
		return fmt.Sprintf(
			"%s = Interface %s [%s, %s]",
			Str(i), i.typs, Str(i.Len), Str(i.Cap),
		)
	}
}

var _ Node = (*Interface)(nil)
var _ Value = (*Interface)(nil)
var _ User = (*Interface)(nil)
var _ Instruction = (*Interface)(nil)

// ----------- Field
func (f *Field) String() string {
	return f.StringByFunc(DefaultValueString)
}

func (f *Field) StringByFunc(Str func(Node) string) string {
	return fmt.Sprintf(
		"%s = %s field[%s]",
		Str(f), Str(f.I), Str(f.Key),
	)
}

var _ Node = (*Field)(nil)
var _ Value = (*Field)(nil)
var _ User = (*Field)(nil)
var _ Instruction = (*Field)(nil)

// ----------- Update

func (s *Update) String() string {
	return s.StringByFunc(DefaultValueString)
}

func (s *Update) StringByFunc(Str func(Node) string) string {
	return fmt.Sprintf(
		"update [%s] = %s",
		Str(s.address), Str(s.value),
	)
}

var _ Node = (*Update)(nil)
var _ Value = (*Update)(nil)
var _ User = (*Update)(nil)
var _ Instruction = (*Update)(nil)
