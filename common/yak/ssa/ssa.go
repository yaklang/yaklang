package ssa

import (
	"fmt"
	"go/constant"
	"sync"

	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

type Value interface {
	String() string
	GetUser() []User
	AddUser(User)
}

type User interface {
	Value
	String() string
	GetValue() []Value
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
	return v.String()
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
	parent    *Function // parent function if anonymous function; nil if global function.
	FreeValue []Value   // the value

	// User
	user []User

	// for build
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
	isSealed      bool
	inCompletePhi map[string]*Phi // variable -> phi

	// User
	user []User
}

type anInstruction struct {
	// function
	Parent *Function
	// basicblock
	Block *BasicBlock
}

// value

type Phi struct {
	anInstruction
	Edge []Value // edge[i] from phi.Block.Preds[i]
	user []User
	// for build
	variable string
}

// implement Value
type Const struct {
	user  []User
	value constant.Value
}

func NewConst(i any) *Const {
	return &Const{
		user:  []User{},
		value: constant.Make(i),
	}
}

// parameter
type Parameter struct {
	variable string
	parent   *Function
	user     []User
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

	Method *Function
	Args   []Value
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

// implement value
func (f *Function) String() string {
	ret := f.name
	for _, para := range f.Param {
		ret += para.String() + ", "
	}
	ret += "\n"

	if parent := f.parent; parent != nil {
		ret += "parent: " + parent.name + "\n"
	}

	instReg := make(map[Instruction]string)
	regindex := 0

	// init instReg
	newName := func() string {
		ret := fmt.Sprintf("%%%d", regindex)
		regindex += 1
		return ret
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
		for _, p := range b.Phis {
			setInst(p)
		}
	}

	// print instruction
	getStr := func(v Value) string {
		op := ""
		switch v := v.(type) {
		case Instruction:
			if name, ok := instReg[v]; ok {
				op = name
			} else {
				op = newName()
				instReg[v] = op
			}
		case *Const:
			op = v.String()
		}
		return op
	}

	handlerInst := func(i Instruction) string {
		ret := "\t" + getStr(i) + " = " + i.StringByFunc(getStr) + "\n"
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

func (b BasicBlock) String() string {
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
func (a *anInstruction) GetParent() *Function  { return a.Parent }

// ----------- Phi
func (p Phi) String() string {
	return p.StringByFunc(DefaultValueString)
}

func (p Phi) StringByFunc(getStr func(Value) string) string {
	ret := "phi "

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
	return c.value.String()
}

var _ Value = (*Const)(nil)


// ----------- Parameter
func (p *Parameter) String() string {
	return p.variable
}

var _ Value = (*Parameter)(nil)



// ----------- Jump
func (j Jump) String() string {
	return j.StringByFunc(DefaultValueString)
}

func (j Jump) StringByFunc(_ func(Value) string) string {
	return fmt.Sprintf("jump -> %s", j.To.Name)
}

var _ Value = (*Jump)(nil)
var _ User = (*Jump)(nil)
var _ Instruction = (*Jump)(nil)

// ----------- IF
func (i If) String() string {
	return i.StringByFunc(DefaultValueString)
}
func (i If) StringByFunc(getStr func(Value) string) string {
	return fmt.Sprintf("If [%s] true -> %s, false -> %s", getStr(i.Cond), i.True.Name, i.False.Name)
}

var _ Value = (*If)(nil)
var _ User = (*If)(nil)
var _ Instruction = (*If)(nil)
// ----------- BinOp
func (b BinOp) String() string {
	return b.StringByFunc(DefaultValueString)
}

func (b BinOp) StringByFunc(getStr func(Value) string) string {
	return fmt.Sprintf("%s %s %s", getStr(b.X), yakvm.OpcodeToName(b.Op), getStr(b.Y))
}

var _ Value = (*BinOp)(nil)
var _ User = (*BinOp)(nil)
var _ Instruction = (*BinOp)(nil)

