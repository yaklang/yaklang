package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"golang.org/x/exp/slices"
)

type anInstruction struct {
	fun   *Function
	block *BasicBlock
	R     *Range
	// scope *Scope

	name        string
	verboseName string // verbose name for output or debug or tag
	id          int

	isExtern  bool
	variables *omap.OrderedMap[string, *Variable] // map[string]*Variable

	// mask is a map, key is variable name, value is variable value
	// it record the variable is masked by closure function or some scope changed
	mask *omap.OrderedMap[string, Value]
}

func (i *anInstruction) IsUndefined() bool {
	return false
}

func (i *anInstruction) SelfDelete() {
	DeleteInst(i)
}

func (i *anInstruction) ReplaceValue(Value, Value) {
}

func (i *anInstruction) AddMask(v Value) {
	i.mask.Add(v)
}

func (i *anInstruction) GetVerboseName() string {
	if i.verboseName != "" {
		return i.verboseName
	}
	if i.name != "" {
		return i.name
	}
	return ""
}

func (i *anInstruction) SetVerboseName(verbose string) {
	i.verboseName = verbose
}

func (i *anInstruction) GetMask() []Value {
	return i.mask.Values()
}

func (i *anInstruction) Masked() bool {
	return i.mask.Len() != 0
}

func NewInstruction() anInstruction {
	return anInstruction{
		variables: omap.NewOrderedMap(make(map[string]*Variable)),
		id:        -1,
		mask:      omap.NewEmptyOrderedMap[string, Value](),
	}
}

// ssa function and block
func (a *anInstruction) SetFunc(f *Function)        { a.fun = f }
func (a *anInstruction) GetFunc() *Function         { return a.fun }
func (a *anInstruction) GetProgram() *Program       { return a.fun.Package.Prog }
func (a *anInstruction) SetBlock(block *BasicBlock) { a.block = block }
func (a *anInstruction) GetBlock() *BasicBlock      { return a.block }

// source code position
func (c *anInstruction) GetRange() *Range { return c.R }

func (c *anInstruction) SetRange(pos *Range) {
	// if c.Pos == nil {
	c.R = pos
	// }
}

func (c *anInstruction) IsExtern() bool   { return c.isExtern }
func (c *anInstruction) SetExtern(b bool) { c.isExtern = b }

// error logger
func (c *anInstruction) NewError(kind ErrorKind, tag ErrorTag, msg string) {
	c.GetFunc().NewErrorWithPos(kind, tag, c.GetRange(), msg)
}

// symbol-table
// func (a *anInstruction) GetScope() *Scope  { return a.scope }
// func (a *anInstruction) SetScope(s *Scope) { a.scope = s }

// variable
func (a *anInstruction) SetName(v string) { a.name = v }
func (a *anInstruction) GetName() string  { return a.name }

// id
func (a *anInstruction) SetId(id int) { a.id = id }
func (a *anInstruction) GetId() int   { return a.id }

func (a *anInstruction) LineDisasm() string { return "" }

// opcode
func (a *anInstruction) GetOpcode() Opcode      { return OpUnknown } // cover by instruction
func (a *anInstruction) GetOperands() Values    { return nil }       // cover by instruction
func (a *anInstruction) GetOperand(i int) Value { return a.GetOperands()[i] }
func (a *anInstruction) GetOperandNum() int     { return len(a.GetOperands()) }

func (a *anInstruction) GetVariable(name string) *Variable {
	if ret, ok := a.variables.Get(name); ok {
		return ret
	} else {
		return nil
	}
}

func (a *anInstruction) GetLastVariable() *Variable {
	_, v, _ := a.variables.Last()
	return v
}

func (a *anInstruction) GetAllVariables() map[string]*Variable {
	return a.variables.GetMap()
}
func (a *anInstruction) AddVariable(v *Variable) {
	name := v.GetName()
	a.variables.Set(name, v)
	a.variables.BringKeyToLastOne(name)
}

var _ Instruction = (*anInstruction)(nil)

type anValue struct {
	typ      Type
	userList Users

	object Value
	key    Value
	member *omap.OrderedMap[Value, Value] // map[Value]Value
}

func (n *anValue) IsMember() bool {
	return n.object != nil
}
func (n *anValue) SetObject(v Value) {
	n.object = v
}

func (n *anValue) GetObject() Value {
	return n.object
}

func (n *anValue) SetKey(k Value) {
	n.key = k
}

func (n *anValue) GetKey() Value {
	return n.key
}

func (n *anValue) IsObject() bool {
	return n.member.Len() != 0
}

func (n *anValue) IsMemberCallVariable() bool {
	return n.object != nil && n.key != nil
}

func (n *anValue) AddMember(k, v Value) {
	// n.member = append(n.member, v)
	// n.member[k] = v
	n.member.Set(k, v)
}

func (n *anValue) DeleteMember(k Value) {
	n.member.Delete(k)
}

func (n *anValue) GetMember(key Value) (Value, bool) {
	ret, ok := n.member.Get(key)
	if !ok {
		return nil, false
	}
	return ret, true
}

func (n *anValue) GetIndexMember(i int) (Value, bool) {
	return n.member.GetByIndex(i)
}

func (n *anValue) GetStringMember(key string) (Value, bool) {
	for _, i := range n.member.Keys() {
		lit, ok := i.(*ConstInst)
		if !ok {
			continue
		}
		if lit.value == key {
			return n.member.Get(i)
		}
	}
	return nil, false
}

func (n *anValue) GetAllMember() map[Value]Value {
	return n.member.GetMap()
}

func NewValue() anValue {
	return anValue{
		typ:      BasicTypes[AnyTypeKind],
		userList: make(Users, 0),
		member:   omap.NewOrderedMap(map[Value]Value{}),
	}
}

func (n *anValue) String() string { return "" }

// has/get user and value
func (n *anValue) HasUsers() bool  { return len(n.userList) != 0 }
func (n *anValue) GetUsers() Users { return n.userList }

// for Value
func (n *anValue) AddUser(u User) {
	if index := slices.Index(n.userList, u); index == -1 {
		n.userList = append(n.userList, u)
	}
}

func (n *anValue) RemoveUser(u User) {
	n.userList = utils.RemoveSliceItem(n.userList, u)
}

// for Value : type
func (n *anValue) GetType() Type    { return n.typ }
func (n *anValue) SetType(typ Type) { n.typ = typ }
