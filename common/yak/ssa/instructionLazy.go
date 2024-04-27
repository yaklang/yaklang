package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func init() {
	ssautil.RegisterLazyInstructionBuilder(func(id int64) (ssautil.SSAValue, error) {
		return NewLazyInstruction(id)
	})
}

type LazyInstruction struct {
	Instruction
	Value
	User
	id     int64
	ir     *ssadb.IrCode
	cache  *Cache
	Modify bool
}

var (
	_ Instruction = (*LazyInstruction)(nil)
	_ Value       = (*LazyInstruction)(nil)
	_ User        = (*LazyInstruction)(nil)
)

// NewLazyInstruction : create a new lazy instruction, only create in cache
func NewLazyInstruction(id int64) (*LazyInstruction, error) {
	ir := ssadb.GetIrCodeById(consts.GetGormProjectDatabase(), id)
	if ir == nil {
		return nil, utils.Error("ircode [" + fmt.Sprint(id) + "]not found")
	}

	lz := &LazyInstruction{
		id: id,
		ir: ir,
	}
	return lz, nil
}

func (z *LazyInstruction) SetCache(i *Cache) {
	z.cache = i
	z.cache.InstructionCache.Set(z.id, instructionIrCode{
		inst:   z,
		irCode: z.ir,
	})
}

func (c *Cache) newLazyInstruction(id int64) *LazyInstruction {
	ins, err := NewLazyInstruction(id)
	if err != nil {
		log.Warnf("BUG or database error: failed to create lazy instruction: %v", err)
		return nil
	}
	ins.SetCache(c)
	return ins
}

// create real-instruction from lazy-instruction
func (lz *LazyInstruction) check() {
	if lz.Instruction != nil {
		return
	}
	lz.Instruction = lz.cache.IrCodeToInstruction(lz.ir)
	if value, ok := ToValue(lz.Instruction); ok {
		lz.Value = value
	}
	if user, ok := ToUser(lz.Instruction); ok {
		lz.User = user
	}
}

func (lz *LazyInstruction) ShouldSave() bool {
	if lz.Instruction == nil {
		return false
	}

	return lz.Modify
}

// just use lazy instruction
func (lz *LazyInstruction) GetId() int64 { return lz.id }

// just use IrCode
func (lz *LazyInstruction) GetName() string             { return lz.ir.Name }
func (lz *LazyInstruction) GetVerboseName() string      { return lz.ir.VerboseName }
func (lz *LazyInstruction) GetShortVerboseName() string { return lz.ir.ShortVerboseName }
func (lz *LazyInstruction) IsExtern() bool              { return lz.ir.IsExternal }
func (lz *LazyInstruction) GetOpcode() Opcode           { return Opcode(lz.ir.Opcode) }

func (lz *LazyInstruction) GetFunc() *Function     { lz.check(); return lz.Instruction.GetFunc() }
func (lz *LazyInstruction) SetFunc(f *Function)    { lz.check(); lz.Instruction.SetFunc(f) }
func (lz *LazyInstruction) GetBlock() *BasicBlock  { lz.check(); return lz.Instruction.GetBlock() }
func (lz *LazyInstruction) SetBlock(b *BasicBlock) { lz.check(); lz.Instruction.SetBlock(b) }
func (lz *LazyInstruction) GetProgram() *Program   { lz.check(); return lz.Instruction.GetProgram() }
func (lz *LazyInstruction) SetName(name string)    { lz.check(); lz.Instruction.SetName(name) }
func (lz *LazyInstruction) SetVerboseName(name string) {
	lz.check()
	lz.Instruction.SetVerboseName(name)
}

func (lz *LazyInstruction) SetId(id int64) { lz.check(); lz.Instruction.SetId(id) }

func (lz *LazyInstruction) GetRange() *Range { lz.check(); return lz.Instruction.GetRange() }

func (lz *LazyInstruction) SetRange(r *Range) { lz.check(); lz.Instruction.SetRange(r) }

func (lz *LazyInstruction) SetExtern(extern bool) { lz.check(); lz.Instruction.SetExtern(extern) }

func (lz *LazyInstruction) SelfDelete() { lz.check(); lz.Instruction.SelfDelete() }

func (lz *LazyInstruction) AddMask(v Value)               { lz.check(); lz.Value.AddMask(v) }
func (lz *LazyInstruction) AddMember(v1 Value, v2 Value)  { lz.check(); lz.Value.AddMember(v1, v2) }
func (lz *LazyInstruction) AddVariable(v *Variable)       { lz.check(); lz.Value.AddVariable(v) }
func (lz *LazyInstruction) DeleteMember(v Value)          { lz.check(); lz.Value.DeleteMember(v) }
func (lz *LazyInstruction) GetAllMember() map[Value]Value { lz.check(); return lz.Value.GetAllMember() }
func (lz *LazyInstruction) GetAllVariables() map[string]*Variable {
	lz.check()
	return lz.Value.GetAllVariables()
}
func (lz *LazyInstruction) GetIndexMember(i int) (Value, bool) {
	lz.check()
	return lz.Value.GetIndexMember(i)
}
func (lz *LazyInstruction) GetKey() Value                   { lz.check(); return lz.Value.GetKey() }
func (lz *LazyInstruction) GetLastVariable() *Variable      { lz.check(); return lz.Value.GetLastVariable() }
func (lz *LazyInstruction) GetMask() []Value                { lz.check(); return lz.Value.GetMask() }
func (lz *LazyInstruction) GetMember(v Value) (Value, bool) { lz.check(); return lz.Value.GetMember(v) }
func (lz *LazyInstruction) GetObject() Value                { lz.check(); return lz.Value.GetObject() }
func (lz *LazyInstruction) GetStringMember(n string) (Value, bool) {
	lz.check()
	return lz.Value.GetStringMember(n)
}
func (lz *LazyInstruction) GetType() Type     { lz.check(); return lz.Value.GetType() }
func (lz *LazyInstruction) GetUsers() Users   { lz.check(); return lz.Value.GetUsers() }
func (lz *LazyInstruction) GetValues() Values { lz.check(); return lz.Value.GetValues() }
func (lz *LazyInstruction) GetVariable(n string) *Variable {
	lz.check()
	return lz.Value.GetVariable(n)
}
func (lz *LazyInstruction) HasUsers() bool    { lz.check(); return lz.Value.HasUsers() }
func (lz *LazyInstruction) HasValues() bool   { lz.check(); return lz.Value.HasValues() }
func (lz *LazyInstruction) IsMember() bool    { lz.check(); return lz.Value.IsMember() }
func (lz *LazyInstruction) IsObject() bool    { lz.check(); return lz.Value.IsObject() }
func (lz *LazyInstruction) IsUndefined() bool { lz.check(); return lz.Value.IsUndefined() }
func (lz *LazyInstruction) Masked() bool      { lz.check(); return lz.Value.Masked() }
func (lz *LazyInstruction) NewError(e ErrorKind, t ErrorTag, msg string) {
	lz.check()
	lz.Value.NewError(e, t, msg)
}
func (lz *LazyInstruction) SetKey(v Value)    { lz.check(); lz.Value.SetKey(v) }
func (lz *LazyInstruction) SetObject(v Value) { lz.check(); lz.Value.SetObject(v) }
func (lz *LazyInstruction) SetType(t Type)    { lz.check(); lz.Value.SetType(t) }

// func (lz *LazyInstruction) String() string            { lz.check(); return "lazy:" + lz.Value.String() }
func (lz *LazyInstruction) String() string            { lz.check(); return lz.Value.String() }
func (lz *LazyInstruction) ReplaceValue(v1, v2 Value) { lz.check(); lz.User.ReplaceValue(v1, v2) }
