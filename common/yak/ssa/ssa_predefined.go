package ssa

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/omap"
	"golang.org/x/exp/slices"
)

type anInstruction struct {
	fun   Value
	prog  *Program
	block Instruction
	R     *memedit.Range
	// scope *Scope

	name        string
	verboseName string // verbose name for output or debug or tag
	id          int64

	isAnnotation bool
	isExtern     bool
	isFromDB     bool

	str               string
	readableName      string
	readableNameShort string
}

var _ Instruction = (*anInstruction)(nil)

func (v *anInstruction) RefreshString() {
	inst := v.Self()
	if utils.IsNil(inst) {
		return
	}
	if op := inst.GetOpcode(); op == SSAOpcodeFunction || op == SSAOpcodeBasicBlock {
		v.str = fmt.Sprintf("[%s]%s", inst.GetOpcode().String(), inst.GetName())
	} else {
		v.str = inst.String()
		v.readableName = LineDisASM(inst)
	}

	v.readableNameShort = LineShortDisASM(inst)
}

func (v *anInstruction) GetSourceCode() string {
	r := v.GetRange()
	if r == nil {
		return ""
	}
	return r.GetText()
}

func (v *anInstruction) GetSourceCodeContext(n int) string {
	r := v.GetRange()
	if r == nil {
		return ""
	}
	return r.GetTextContext(n)
}

func (i *anInstruction) IsUndefined() bool {
	return false
}

func (i *anInstruction) IsParameter() bool {
	return false
}

func (i *anInstruction) IsSideEffect() bool {
	return false
}

func (i *anInstruction) IsPhi() bool {
	return false
}

func (i *anInstruction) IsBlock(name string) bool {
	if i.GetOpcode() == SSAOpcodeBasicBlock {
		return strings.HasPrefix(i.GetName(), name)
	}
	return false
}

func (i *anInstruction) SelfDelete() {
	DeleteInst(i)
}

func (i *anInstruction) IsCFGEnterBlock() ([]Instruction, bool) {
	return nil, false
}

func (i *anInstruction) IsLazy() bool { return false }

func (i *anInstruction) IsFromDB() bool { return i.isFromDB }

func (i *anInstruction) SetIsFromDB(b bool) { i.isFromDB = b }

func (i *anInstruction) Self() Instruction {
	inst, _ := i.GetProgram().GetInstructionById(i.GetId())
	return inst
}

func (i *anInstruction) ReplaceValue(Value, Value) {
}

func (i *anInstruction) GetVerboseName() string {
	if utils.IsNil(i) {
		return ""
	}
	if i.verboseName != "" {
		return i.verboseName
	}
	if i.name != "" {
		return i.name
	}
	return ""
}

func (i *anInstruction) GetShortVerboseName() string {
	if utils.IsNil(i) {
		return ""
	}
	if i.name != "" {
		return i.name
	}
	return "t" + fmt.Sprint(i.GetId())
}

func (i *anInstruction) SetVerboseName(verbose string) {
	i.verboseName = verbose
}

func NewInstruction() anInstruction {
	return anInstruction{
		id: -1,
	}
}

// ssa function and block
func (a *anInstruction) SetFunc(f *Function) {
	a.fun = f
	a.prog = f.GetProgram()
}

func (a *anInstruction) GetFunc() *Function {
	f, ok := ToFunction(a.fun)
	if ok {
		return f
	}
	return nil
}

func (a *anInstruction) GetProgram() *Program {
	return a.prog
}

func (a *anInstruction) GetProgramName() string {
	if a.prog == nil {
		return ""
	}
	return a.prog.Name
}

func (a *anInstruction) SetProgram(prog *Program) {
	a.prog = prog
}

func (a *anInstruction) SetIsAnnotation(b bool) {
	a.isAnnotation = b
}
func (v *anInstruction) IsSupportConstMethod() bool {
	config := v.GetProgram().Application.config
	if config == nil {
		log.Errorf("[BUG]SupportConstMethod config is not init")
		return false
	}
	return v.prog.config.isSupportConstMethod
}
func (a *anInstruction) IsAnnotation() bool {
	return a.isAnnotation
}

func (a *anInstruction) SetBlock(block *BasicBlock) { a.block = block }
func (a *anInstruction) GetBlock() *BasicBlock {
	if a.block == nil {
		return nil
	}
	if block, ok := ToBasicBlock(a.block); ok {
		return block
	}
	log.Warnf("GetBlock: block is not a BasicBlock but: %v", a.block)
	return nil
}

// source code position
func (c *anInstruction) GetRange() *memedit.Range {
	if c.R != nil {
		return c.R
	}
	return nil
}

func (c *anInstruction) SetRange(pos *memedit.Range) {
	// if c.Pos == nil {
	c.R = pos
	// }
}

// func (c *anInstruction) SetRangeInit(editor *memedit.MemEditor) {
// 	if c.R == nil {
// 		fullRange := editor.GetFullRange()
// 		c.R = NewRange(editor, fullRange.GetStart(), fullRange.GetEnd())
// 	}
// }

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
func (a *anInstruction) SetName(v string) {
	if utils.IsNil(a) {
		return
	}
	a.name = v
}
func (a *anInstruction) GetName() string {
	if utils.IsNil(a) {
		return ""
	}
	return a.name
}

// id
func (a *anInstruction) SetId(id int64) { a.id = id }

func (a *anInstruction) GetId() int64 {
	if a == nil {
		return -1
	}
	return a.id
}

func (a *anInstruction) LineDisasm() string { return "" }

// opcode
func (a *anInstruction) GetOpcode() Opcode { return SSAOpcodeUnKnow } // cover by instruction

func (a *anInstruction) String() string {
	this, ok := a.GetValueById(a.GetId())
	if !ok {
		return ""
	}
	return fmt.Sprintf("Instruction: %s %s", SSAOpcode2Name[this.GetOpcode()], this.GetName())
}

var _ Instruction = (*anInstruction)(nil)

type anValue struct {
	anInstruction

	typ      Type
	userList []int64

	object int64
	key    int64
	member *omap.OrderedMap[int64, int64] // map[Value]Value

	variables *omap.OrderedMap[string, *Variable] // map[string]*Variable

	// mask is a map, key is variable name, value is variable value
	// it record the variable is masked by closure function or some scope changed
	mask *omap.OrderedMap[string, int64]

	pointer   []int64 // the pointer is point to this value
	reference int64   // the value is pointed by this value

	occultation []int64
}

var defaultAnyType = CreateAnyType()

func NewValue() anValue {
	return anValue{
		anInstruction: NewInstruction(),
		typ:           defaultAnyType,
		userList:      make([]int64, 0),
		member:        omap.NewOrderedMap(map[int64]int64{}),

		variables: omap.NewOrderedMap(map[string]*Variable{}),
		mask:      omap.NewOrderedMap(map[string]int64{}),
	}
}

func (n *anValue) IsMember() bool {
	return n.object > 0 && n.key > 0
}

func (n *anValue) SetObject(v Value) {
	n.object = v.GetId()
}

func (n *anValue) GetObject() Value {
	obj, _ := n.GetValueById(n.object)
	return obj
}

func (n *anValue) SetKey(k Value) {
	n.key = k.GetId()
}

func (n *anValue) GetKey() Value {
	key, _ := n.GetValueById(n.key)
	return key
}

func (n *anValue) IsObject() bool {
	if n.member == nil {
		return false
	}
	return n.member.Len() != 0
}

func (n *anValue) AddMember(k, v Value) {
	// n.member = append(n.member, v)
	// n.member[k] = v
	n.member.Set(k.GetId(), v.GetId())
}

func (n *anValue) DeleteMember(k Value) {
	n.member.Delete(k.GetId())
}

func (n *anValue) GetMember(key Value) (Value, bool) {
	ret, ok := n.member.Get(key.GetId())
	if !ok {
		return nil, false
	}
	val, ok := n.GetValueById(ret)
	return val, ok
}

func (n *anValue) GetIndexMember(i int) (Value, bool) {
	id, ok := n.member.GetByIndex(i)
	if !ok {
		return nil, false
	}
	val, ok := n.GetValueById(id)
	return val, ok
}

func (n *anValue) GetStringMember(key string) (Value, bool) {
	keys := n.member.Keys()
	for index := len(keys) - 1; index >= 0; index-- {
		i, ok := n.GetValueById(keys[index])
		if !ok {
			continue
		}
		lit, ok := ToConstInst(i)
		if !ok {
			continue
		}
		if lit.value == key {
			return n.GetMember(i)
		}
	}
	return nil, false
}

func (n *anValue) SetStringMember(key string, v Value) {
	for _, id := range n.member.Keys() {
		i, ok := n.GetValueById(id)
		if !ok {
			continue
		}
		lit, ok := i.(*ConstInst)
		if !ok {
			continue
		}
		if lit.value == key {
			n.AddMember(i, v)
		}
	}
}

func (n *anValue) GetAllMember() map[Value]Value {
	ret := make(map[Value]Value, n.member.Len())
	for key, value := range n.member.GetMap() {
		k, ok1 := n.GetValueById(key)
		v, ok2 := n.GetValueById(value)
		if !ok1 || !ok2 {
			log.Errorf("BUG in anValue.GetAllMember(), is nil key[%d](%v) member[%d](%v)", key, k, value, v)
			continue
		}
		ret[k] = v
	}
	return ret
}

func (n *anValue) ForEachMember(fn func(Value, Value) bool) {
	n.member.ForEach(func(i, v int64) bool {
		val1, ok1 := n.GetValueById(i)
		val2, ok2 := n.GetValueById(v)
		if !ok1 || !ok2 {
			return true
		}
		return fn(val1, val2)
	})
}

func (n *anValue) String() string { return "" }

// has/get user and value
func (n *anValue) HasUsers() bool  { return len(n.userList) != 0 }
func (n *anValue) GetUsers() Users { return n.GetUsersByIDs(n.userList) }

// for Value
func (n *anValue) AddUser(u User) {
	id := u.GetId()
	if index := slices.Index(n.userList, id); index == -1 {
		n.userList = append(n.userList, id)
	}
}

func (n *anValue) RemoveUser(u User) {
	n.userList = utils.RemoveSliceItem(n.userList, u.GetId())
}

// for Value : type
func (n *anValue) GetType() Type {
	if n == nil {
		log.Errorf("BUG in *anValue.GetType(), the *anValue is nil!")
		return CreateAnyType()
	}
	return n.typ
}

func (n *anValue) SetType(typ Type) {
	if typ == nil {
		return
	}

	if n.IsFromDB() {
		n.typ = typ
		return
	}

	value, ok := n.GetValueById(n.GetId())
	if !ok {
		n.typ = typ
		return
	}
	saveTypeWithValue(value, typ)

	switch t := typ.(type) {
	case *Blueprint:
		n.typ = t.Apply(value)
	case *FunctionType:
		n.typ = typ
		this := value
		if this == nil {
			return
		}
		if fun := t.This; fun != nil {
			Point(this, fun)
		}
		for _, f := range t.AnnotationFunc {
			f(this)
		}

	default:
		n.typ = typ
	}
}

func (a *anValue) GetVariable(name string) *Variable {
	if ret, ok := a.variables.Get(name); ok {
		return ret
	} else {
		if a.IsFromDB() {
			v := GetVariableFromDB(a.GetId(), name)
			a.AddVariable(v)
			return v
		}
	}
	return nil
}

func (a *anValue) GetLastVariable() *Variable {
	_, v, _ := a.variables.Last()
	return v
}

func (a *anValue) GetAllVariables() map[string]*Variable {
	return a.variables.GetMap()
}

func (a *anValue) AddVariable(v *Variable) {
	name := v.GetName()
	a.variables.Set(name, v)
	a.variables.BringKeyToLastOne(name)
}

func (i *anValue) AddMask(v Value) {
	i.mask.Add(v.GetId())
}

func (i *anValue) GetMask() []Value {
	return i.GetValuesByIDs(i.mask.Values())
}

func (i *anValue) Masked() bool {
	return i.mask.Len() != 0
}

func (i *anValue) SetReference(v Value) {
	i.reference = v.GetId()
}

func (i *anValue) GetReference() Value {
	ref, _ := i.GetValueById(i.reference)
	return ref
}

func (i *anValue) AddPointer(v Value) {
	i.pointer = append(i.pointer, v.GetId())
}

func (i *anValue) GetPointer() Values {
	return i.GetValuesByIDs(i.pointer)
}

func (i *anValue) AddOccultation(p Value) {
	i.occultation = append(i.occultation, p.GetId())
}

func (i *anValue) GetOccultation() []Value {
	return i.GetValuesByIDs(i.occultation)
}

func (i *anValue) FlatOccultation() []Value {
	var ret []Value
	var handler func(i *anValue)

	handler = func(i *anValue) {
		for _, id := range i.occultation {
			v, ok := i.GetValueById(id)
			if !ok {
				continue
			}
			ret = append(ret, v)
			if p, ok := ToPhi(v); ok {
				handler(&p.anValue)
			}
		}
	}
	handler(i)

	return ret
}

func (i *anValue) getAnValue() *anValue {
	return i
}

func (i *anInstruction) getAnInstruction() *anInstruction {
	return i
}
