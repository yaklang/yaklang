package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssa/ssautil"
)

func init() {
	ssautil.RegisterLazyInstructionBuilder(func(id int64) (ssautil.SSAValue, error) {
		return NewLazyInstruction(id)
	})
}

type LazyInstruction struct {
	// self
	Instruction
	Value
	User
	// cache
	id          int64
	variableDB  map[string]*ssadb.IrIndex
	variable    map[string]*Variable
	ir          *ssadb.IrCode
	programName string
	cache       *Cache
	Modify      bool
}

var (
	_ Instruction = (*LazyInstruction)(nil)
	_ Value       = (*LazyInstruction)(nil)
	_ User        = (*LazyInstruction)(nil)
)

func NewInstructionFromLazy[T Instruction](id int64, Cover func(Instruction) (T, bool)) (T, error) {
	var zero T
	lz, err := NewLazyInstruction(id)
	if err != nil {
		return zero, err
	}

	inst, ok := Cover(lz)
	if !ok {
		return zero, utils.Errorf("BUG: lazyInstruction cover failed")
	}
	return inst, nil
}

// NewLazyInstruction : create a new lazy instruction, only create in cache
func NewLazyInstruction(id int64) (Value, error) {
	ir := ssadb.GetIrCodeById(ssadb.GetDB(), id)
	return NewLazyInstructionFromIrCode(ir)
}

func NewLazyInstructionFromIrCode(ir *ssadb.IrCode) (Value, error) {
	if ir == nil {
		return nil, utils.Error("ircode is nil")
	}
	cache := GetCacheFromPool(ir.ProgramName)
	return newLazyInstruction(int64(ir.ID), ir, cache)
}

func newLazyInstruction(id int64, ir *ssadb.IrCode, cache *Cache) (Value, error) {
	if ret, ok := cache.InstructionCache.Get(id); ok {
		value, ok := ToValue(ret.inst)
		if !ok {
			log.Warnf("BUG: cache return not a value")
			return nil, utils.Errorf("BUG: LazyInstruction cache return not a value\n")
		}
		return value, nil
	}
	if ir == nil {
		ir = ssadb.GetIrCodeById(ssadb.GetDB(), id)
		if ir == nil {
			return nil, utils.Errorf("ircode [" + fmt.Sprint(id) + "]not found")
		}
	}
	lz := &LazyInstruction{
		id:          id,
		ir:          ir,
		variable:    make(map[string]*Variable),
		programName: ir.ProgramName,
	}
	lz.cache = cache
	lz.cache.InstructionCache.Set(lz.id, instructionIrCode{
		inst:   lz,
		irCode: lz.ir,
	})
	return lz, nil
}

func (lz *LazyInstruction) IsLazy() bool { return true }

func (lz *LazyInstruction) IsFromDB() bool {
	return false
}

func (lz *LazyInstruction) SetIsFromDB(isFromDB bool) {
}

func (lz *LazyInstruction) Self() Instruction {
	if lz.Value == nil {
		lz.check()
	}
	if lz.Value != nil {
		return lz.Value
	}
	return lz.Instruction
}

func (lz *LazyInstruction) IsBlock(name string) bool {
	if lz.Value == nil {
		lz.check()
	}
	if lz.Value == nil {
		return false
	}
	return lz.Value.IsBlock(name)
}

// create real-instruction from lazy-instruction
func (lz *LazyInstruction) check() {
	if lz.Instruction == nil {
		inst := CreateInstruction(Opcode(lz.GetOpcode()))
		if inst == nil {
			log.Infof("unknown opcode: %d: %s", lz.GetOpcode(), lz.ir.OpcodeName)
			return
		}
		lz.Instruction = inst
		// set range for instruction
		lz.GetRange()
		lz.cache.IrCodeToInstruction(lz.Instruction, lz.ir)
	}
	if lz.Value == nil {
		if value, ok := ToValue(lz.Instruction); ok {
			lz.Value = value
		}
	}
	if lz.User == nil {
		if user, ok := ToUser(lz.Instruction); ok {
			lz.User = user
		}
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
func (lz *LazyInstruction) GetName() string {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.Name
}

func (lz *LazyInstruction) GetVerboseName() string {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.VerboseName
}

func (lz *LazyInstruction) GetShortVerboseName() string {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.ShortVerboseName
}

func (lz *LazyInstruction) IsExtern() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.IsExternal
}

func (lz *LazyInstruction) GetOpcode() Opcode {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return SSAOpcodeUnKnow
	}
	return Opcode(lz.ir.Opcode)
}

func (lz *LazyInstruction) String() string {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.String
}

func (lz *LazyInstruction) HasUsers() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return len(lz.ir.Users) == 0
}

func (lz *LazyInstruction) HasValues() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return len(lz.ir.Defs) == 0
}

func (lz *LazyInstruction) IsMember() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.IsObjectMember
}

func (lz *LazyInstruction) IsObject() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.IsObject
}

func (lz *LazyInstruction) IsUndefined() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodeUndefined)
}

func (lz *LazyInstruction) IsParameter() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodeParameter)
}

func (lz *LazyInstruction) IsSideEffect() bool {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodeSideEffect)
}

func (lz *LazyInstruction) GetProgramName() string {
	if lz.ir == nil {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.ProgramName
}

func (lz *LazyInstruction) GetFunc() *Function {
	lz.check()
	if lz.Instruction == nil {
		return nil
	}
	return lz.Instruction.GetFunc()
}

func (lz *LazyInstruction) SetFunc(f *Function) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetFunc(f)
}

func (lz *LazyInstruction) GetBlock() *BasicBlock {
	lz.check()
	if lz.Instruction == nil {
		return nil
	}
	return lz.Instruction.GetBlock()
}

func (lz *LazyInstruction) SetBlock(b *BasicBlock) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetBlock(b)
}

func (lz *LazyInstruction) GetProgram() *Program {
	lz.check()
	if lz.Instruction == nil {
		return nil
	}
	return lz.Instruction.GetProgram()
}

func (lz *LazyInstruction) SetProgram(p *Program) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetProgram(p)
}

func (lz *LazyInstruction) SetName(name string) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetName(name)
}

func (lz *LazyInstruction) SetVerboseName(name string) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetVerboseName(name)
}

func (lz *LazyInstruction) SetIsAnnotation(b bool) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetIsAnnotation(b)
}

func (lz *LazyInstruction) IsAnnotation() bool {
	lz.check()
	if lz.Instruction == nil {
		return false
	}
	return lz.Instruction.IsAnnotation()
}

func (lz *LazyInstruction) SetId(id int64) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetId(id)
}

func (lz *LazyInstruction) GetRange() memedit.RangeIf {
	lz.check()
	if lz.Instruction == nil {
		return nil
	}
	if lz.Instruction.GetRange() == nil {
		editor, start, end, err := lz.ir.GetStartAndEndPositions()
		if err != nil {
			switch ret := lz.Self().(type) {
			case *BasicBlock:
				if len(ret.Insts) > 0 {
					startRng := ret.Insts[0].GetRange()
					endRng := ret.Insts[len(ret.Insts)-1].GetRange()
					fallbackRange := memedit.NewRange(startRng.GetStart(), endRng.GetEnd())
					fallbackRange.SetEditor(startRng.GetEditor())
					lz.Instruction.SetRange(fallbackRange)
					return fallbackRange
				}

				// check if block has no instruction
				var startRng memedit.RangeIf
				for _, start := range ret.Preds {
					if rng := start.GetRange(); rng != nil {
						startRng = rng
						break
					}
				}
				var endRng memedit.RangeIf
				for _, end := range ret.Succs {
					if rng := end.GetRange(); rng != nil {
						endRng = rng
						break
					}
				}
				if startRng != nil && endRng != nil {
					log.Infof("use pred start range and succ end range for %v(%T)", lz.GetId(), lz.Self())
					fallbackRange := memedit.NewRange(startRng.GetStart(), endRng.GetEnd())
					fallbackRange.SetEditor(startRng.GetEditor())
					lz.Instruction.SetRange(fallbackRange)
					return fallbackRange
				}

				if startRng != nil {
					log.Infof("just use pred start range for %v(%T)", lz.GetId(), lz.Self())
					lz.Instruction.SetRange(startRng)
					return startRng
				}

				if endRng != nil {
					log.Infof("just use succ end range for %v(%T)", lz.GetId(), lz.Self())
					lz.Instruction.SetRange(endRng)
					return endRng
				}
			}
			log.Warnf("LazyInstruction(%T).GetRange failed: %v", lz.Self(), err)
			return nil
		}
		lz.Instruction.SetRange(editor.GetRangeByPosition(start, end))
	}
	return lz.Instruction.GetRange()
}

func (lz *LazyInstruction) SetRange(r memedit.RangeIf) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetRange(r)
}

func (lz *LazyInstruction) GetSourceCode() string {
	lz.check()
	if lz.Instruction == nil {
		return ""
	}
	r := lz.Instruction.GetRange()
	if r == nil {
		lz.Instruction.SetRange(lz.GetRange())
	}
	return lz.Instruction.GetSourceCode()
}

func (lz *LazyInstruction) GetSourceCodeContext(n int) string {
	lz.check()
	if lz.Instruction == nil {
		return ""
	}
	r := lz.Instruction.GetRange()
	if r == nil {
		lz.Instruction.SetRange(lz.GetRange())
	}
	return lz.Instruction.GetSourceCodeContext(n)
}

func (lz *LazyInstruction) SetExtern(extern bool) {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SetExtern(extern)
}

func (lz *LazyInstruction) SelfDelete() {
	lz.check()
	if lz.Instruction == nil {
		return
	}
	lz.Instruction.SelfDelete()
}

func (lz *LazyInstruction) IsCFGEnterBlock() ([]Instruction, bool) {
	lz.check()
	if lz.Instruction == nil {
		return nil, false
	}
	return lz.Instruction.IsCFGEnterBlock()
}

func (lz *LazyInstruction) AddMask(v Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.AddMask(v)
}

func (lz *LazyInstruction) AddMember(v1 Value, v2 Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.AddMember(v1, v2)
}

func (lz *LazyInstruction) AddVariable(v *Variable) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.AddVariable(v)
}

func (lz *LazyInstruction) DeleteMember(v Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.DeleteMember(v)
}

func (lz *LazyInstruction) ForEachMember(fn func(Value, Value) bool) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.ForEachMember(fn)
}

func (lz *LazyInstruction) GetAllMember() map[Value]Value {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetAllMember()
}

func (lz *LazyInstruction) GetAllVariables() map[string]*Variable {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetAllVariables()
}

func (lz *LazyInstruction) GetIndexMember(i int) (Value, bool) {
	lz.check()
	if lz.Value == nil {
		return nil, false
	}
	return lz.Value.GetIndexMember(i)
}

func (lz *LazyInstruction) GetKey() Value {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetKey()
}

func (lz *LazyInstruction) GetLastVariable() *Variable {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetLastVariable()
}

func (lz *LazyInstruction) GetMask() []Value {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetMask()
}

func (lz *LazyInstruction) GetMember(v Value) (Value, bool) {
	lz.check()
	if lz.Value == nil {
		return nil, false
	}
	return lz.Value.GetMember(v)
}

func (lz *LazyInstruction) GetObject() Value {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetObject()
}

func (lz *LazyInstruction) GetStringMember(n string) (Value, bool) {
	lz.check()
	if lz.Value == nil {
		return nil, false
	}
	return lz.Value.GetStringMember(n)
}

func (lz *LazyInstruction) GetType() Type {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetType()
}

func (lz *LazyInstruction) GetUsers() Users {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetUsers()
}

func (lz *LazyInstruction) GetValues() Values {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetValues()
}

func (lz *LazyInstruction) GetVariable(n string) *Variable {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	if v, ok := lz.variable[n]; ok {
		return v
	}
	{
		v := NewVariable(0, n, false, nil).(*Variable)
		v.Assign(lz)
		offset := ssadb.GetOffsetByVariable(n, lz.id)
		for _, o := range offset {
			editor, start, end, err := o.GetStartAndEndPositions()
			if err != nil {
				log.Errorf("GetStartAndEndPositions failed: %v", err)
				continue
			}
			rng := editor.GetRangeByPosition(start, end)
			v.AddRange(rng, true)
		}
		lz.variable[n] = v
		return v
	}
	// return lz.Value.GetVariable(n)
}

func (lz *LazyInstruction) Masked() bool {
	lz.check()
	if lz.Value == nil {
		return false
	}
	return lz.Value.Masked()
}

func (lz *LazyInstruction) NewError(e ErrorKind, t ErrorTag, msg string) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.NewError(e, t, msg)
}

func (lz *LazyInstruction) SetKey(v Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.SetKey(v)
}

func (lz *LazyInstruction) SetObject(v Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.SetObject(v)
}

func (lz *LazyInstruction) SetType(t Type) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.SetType(t)
}

func (lz *LazyInstruction) ReplaceValue(v1, v2 Value) {
	lz.check()
	if lz.User == nil {
		return
	}
	lz.User.ReplaceValue(v1, v2)
}

func (lz *LazyInstruction) GetPointer() Values {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetPointer()
}

func (lz *LazyInstruction) AddPointer(v Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.AddPointer(v)
}
func (lz *LazyInstruction) GetReference() Value {
	lz.check()
	if lz.Value == nil {
		return nil
	}
	return lz.Value.GetReference()
}

func (lz *LazyInstruction) SetReference(v Value) {
	lz.check()
	if lz.Value == nil {
		return
	}
	lz.Value.SetReference(v)
}

func (lz *LazyInstruction) AddOccultation(p Value) {

}

func (lz *LazyInstruction) FlatOccultation() []Value {
	var ret []Value
	var handler func(i *anValue)

	handler = func(i *anValue) {
		for _, v := range i.occultation {
			ret = append(ret, v)
			if p, ok := ToPhi(v); ok {
				handler(&p.anValue)
			}
		}
	}
	if u, ok := ToUndefined(lz.Value); ok {
		handler(&u.anValue)
	}

	return ret
}
