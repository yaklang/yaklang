package ssa

import (
	"sync"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// func init() {
// 	ssautil.RegisterLazyInstructionBuilder(func(id int64) (ssautil.SSAValue, error) {
// 		return NewLazyValue(id)
// 	})
// }

type LazyInstruction struct {
	// self
	Instruction Instruction
	Value
	User
	// cache
	id          int64
	variableDB  map[string]*ssadb.IrIndex
	variable    map[string]*Variable
	ir          *ssadb.IrCode
	programName string
	cache       *ProgramCache
	prog        *Program
	Modify      bool

	once sync.Once
}

var (
	_ Instruction = (*LazyInstruction)(nil)
	_ Value       = (*LazyInstruction)(nil)
	_ User        = (*LazyInstruction)(nil)
)

func NewLazyValue(prog *Program, id int64) (Value, error) {
	inst, err := NewLazyEx(prog, id, ToValue)
	if err != nil {
		return nil, err
	}
	return inst, nil
}

func NewLazyEx[T Instruction](prog *Program, id int64, Cover func(Instruction) (T, bool)) (T, error) {
	var zero T
	lz, err := NewLazyInstruction(prog, id)
	if err != nil {
		return zero, err
	}

	inst, ok := Cover(lz)
	if !ok {
		return zero, utils.Errorf("BUG: lazyInstruction cover failed")
	}
	return inst, nil
}

func NewInstructionWithCover[T Instruction](prog *Program, id int64, Cover func(Instruction) (T, bool)) (T, error) {
	var zero T
	lz, err := NewLazyInstruction(prog, id)
	if err != nil {
		return zero, err
	}

	inst, ok := Cover(lz)
	if !ok {
		return zero, utils.Errorf("BUG: lazyInstruction cover failed")
	}
	return inst, nil
}

// // NewLazyInstruction : create a new lazy instruction, only create in cache
func NewLazyInstruction(prog *Program, id int64) (Instruction, error) {
	ir := ssadb.GetIrCodeById(ssadb.GetDB(), prog.GetProgramName(), id)
	if ir == nil {
		return nil, utils.Error("IrCode is nil")
	}
	// prog, ok := GetProgramFromPool(ir.ProgramName)
	// if !ok {
	// log.Errorf("program not found: %s", ir.ProgramName)
	// return nil, utils.Errorf("program not found: %s", ir.ProgramName)
	// }
	return NewLazyInstructionFromIrCode(ir, prog)
}

func NewLazyInstructionFromIrCode(ir *ssadb.IrCode, prog *Program, ignoreCache ...bool) (Instruction, error) {
	if ir == nil {
		return nil, utils.Error("IrCode is nil")
	}
	if prog == nil {
		return nil, utils.Errorf("BUG: program is nil: %s", ir.ProgramName)
	}
	if ir == nil || ir.CodeID == 0 {
		log.Infof("ircode is nil or id is 0")
	}
	lz := &LazyInstruction{
		id:          ir.GetIdInt64(),
		ir:          ir,
		variable:    make(map[string]*Variable),
		programName: ir.ProgramName,
		cache:       prog.Cache,
		prog:        prog,
	}
	return lz, nil
}

func (lz *LazyInstruction) IsLazy() bool { return true }

func (lz *LazyInstruction) IsFromDB() bool {
	return false
}

func (lz *LazyInstruction) SetIsFromDB(isFromDB bool) {
}

func (lz *LazyInstruction) Self() Instruction {
	if utils.IsNil(lz.Value) {
		lz.check()
	}
	if lz.Value != nil {
		return lz.Value
	}
	return lz.Instruction
}

func (lz *LazyInstruction) IsBlock(name string) bool {
	if utils.IsNil(lz.Value) {
		lz.check()
	}
	if utils.IsNil(lz.Value) {
		return false
	}
	return lz.Value.IsBlock(name)
}

// create real-instruction from lazy-instruction
func (lz *LazyInstruction) check() {
	lz.once.Do(func() {
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("panic: %v", err)
				utils.PrintCurrentGoroutineRuntimeStack()
			}
		}()
		var inst Instruction
		if utils.IsNil(lz.Instruction) {
			inst = CreateInstruction(Opcode(lz.GetOpcode()))
			if inst == nil {
				log.Infof("unknown opcode: %d: %s", lz.GetOpcode(), lz.ir.OpcodeName)
				return
			}
			inst.SetProgram(lz.prog)
			lz.cache.IrCodeToInstruction(inst, lz.ir, lz.cache)
			lz.Instruction = inst
		}
		if utils.IsNil(lz.Value) {
			if value, ok := ToValue(lz.Instruction); ok {
				lz.Value = value
			}
		}
		if utils.IsNil(lz.User) {
			if user, ok := ToUser(lz.Instruction); ok {
				lz.User = user
			}
		}
	})

}

func (lz *LazyInstruction) ShouldSave() bool {
	if utils.IsNil(lz.Instruction) {
		return false
	}

	// TODO: use this flag to check if need save
	// return lz.Modify
	return lz.Instruction != nil
}

// just use lazy instruction
func (lz *LazyInstruction) GetId() int64 { return lz.id }

// just use IrCode
func (lz *LazyInstruction) GetName() string {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.Name
}

func (lz *LazyInstruction) GetVerboseName() string {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.VerboseName
}

func (lz *LazyInstruction) GetShortVerboseName() string {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.ShortVerboseName
}

func (lz *LazyInstruction) IsExtern() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.IsExternal
}

func (lz *LazyInstruction) GetOpcode() Opcode {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return SSAOpcodeUnKnow
	}
	return Opcode(lz.ir.Opcode)
}

func (lz *LazyInstruction) RefreshString() {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return
	}
	lz.Instruction.RefreshString()
}

func (lz *LazyInstruction) String() string {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.Instruction.String()
	// return "lz:" + lz.ir.String
}

func (lz *LazyInstruction) HasUsers() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return len(lz.ir.Users) == 0
}

func (lz *LazyInstruction) AddUser(user User) {
	lz.check()
	lz.Modify = true
	lz.Value.AddUser(user)
}

func (lz *LazyInstruction) HasValues() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	// return len(lz.ir.Defs) == 0
	return lz.ir.HasDefs
}

func (lz *LazyInstruction) IsMember() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.IsObjectMember
}

func (lz *LazyInstruction) IsObject() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.IsObject
}

func (lz *LazyInstruction) IsUndefined() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodeUndefined)
}

func (lz *LazyInstruction) IsParameter() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodeParameter)
}

func (lz *LazyInstruction) IsSideEffect() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodeSideEffect)
}

func (lz *LazyInstruction) IsPhi() bool {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return false
	}
	return lz.ir.Opcode == int64(SSAOpcodePhi)
}

func (lz *LazyInstruction) GetProgramName() string {
	if utils.IsNil(lz.ir) {
		log.Errorf("BUG: lazyInstruction IrCode is nil")
		return ""
	}
	return lz.ir.ProgramName
}

func (lz *LazyInstruction) GetFunc() *Function {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return nil
	}
	return lz.Instruction.GetFunc()
}

func (lz *LazyInstruction) SetFunc(f *Function) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetFunc(f)
}

func (lz *LazyInstruction) GetBlock() *BasicBlock {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return nil
	}
	return lz.Instruction.GetBlock()
}

func (lz *LazyInstruction) SetBlock(b *BasicBlock) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetBlock(b)
}

func (lz *LazyInstruction) GetProgram() *Program {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return nil
	}
	return lz.Instruction.GetProgram()
}

func (lz *LazyInstruction) SetProgram(p *Program) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetProgram(p)
}

func (lz *LazyInstruction) SetName(name string) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetName(name)
}

func (lz *LazyInstruction) SetVerboseName(name string) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetVerboseName(name)
}

func (lz *LazyInstruction) SetIsAnnotation(b bool) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetIsAnnotation(b)
}

func (lz *LazyInstruction) IsAnnotation() bool {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return false
	}
	return lz.Instruction.IsAnnotation()
}

func (lz *LazyInstruction) SetId(id int64) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetId(id)
}

func (lz *LazyInstruction) GetRange() *memedit.Range {
	lz.check()
	return lz.getRange(lz.Self())
}
func (lz *LazyInstruction) getRange(inst Instruction) *memedit.Range {
	if utils.IsNil(inst) {
		return nil
	}
	if inst.GetRange() == nil {
		editor, start, end, err := lz.ir.GetStartAndEndPositions()
		if err != nil {
			switch ret := inst.(type) {
			case *BasicBlock:
				// check if block has no instruction
				var startRng *memedit.Range
				if len(ret.Insts) > 0 {
					for _, startId := range ret.Insts {
						startInst, ok := lz.prog.GetInstructionById(startId)
						if !ok || startInst == nil {
							continue
						}
						if rng := startInst.GetRange(); rng != nil {
							startRng = rng
							break
						}
					}
				} else {
					for _, startId := range ret.Preds {
						startInst, ok := lz.prog.GetInstructionById(startId)
						if !ok || startInst == nil {
							continue
						}
						if rng := startInst.GetRange(); rng != nil {
							startRng = rng
							break
						}
					}
				}

				var endRng *memedit.Range
				if len(ret.Insts) > 0 {
					// Iterate from the last instruction to find the end range
					for i := len(ret.Insts) - 1; i >= 0; i-- {
						endInstId := ret.Insts[i]
						endInst, ok := lz.prog.GetInstructionById(endInstId)
						if !ok || endInst == nil {
							continue
						}
						if rng := endInst.GetRange(); rng != nil {
							endRng = rng
							break
						}
					}
				} else {
					for _, endId := range ret.Succs {
						endInst, ok := lz.prog.GetInstructionById(endId)
						if !ok || endInst == nil {
							continue
						}
						if rng := endInst.GetRange(); rng != nil {
							endRng = rng
							break
						}
					}
				}
				if startRng != nil && endRng != nil {
					log.Infof("use pred start range and succ end range for %v(%T)", inst.GetId(), inst)
					fallbackRange := memedit.NewRange(startRng.GetStart(), endRng.GetEnd())
					fallbackRange.SetEditor(startRng.GetEditor())
					inst.SetRange(fallbackRange)
					return fallbackRange
				}

				if startRng != nil {
					log.Infof("just use pred start range for %v(%T)", inst.GetId(), inst)
					inst.SetRange(startRng)
					return startRng
				}

				if endRng != nil {
					log.Infof("just use succ end range for %v(%T)", inst.GetId(), inst)
					inst.SetRange(endRng)
					return endRng
				}
			}
			log.Warnf("LazyInstruction(%T).GetRange failed: %v", inst, err)
			return nil
		}
		if editor != nil && start != nil && end != nil {
			inst.SetRange(editor.GetRangeByPosition(start, end))
		}
	}
	return inst.GetRange()
}

func (lz *LazyInstruction) SetRange(r *memedit.Range) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetRange(r)
}

func (lz *LazyInstruction) GetSourceCode() string {
	lz.check()
	if utils.IsNil(lz.Instruction) {
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
	if utils.IsNil(lz.Instruction) {
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
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SetExtern(extern)
}

func (lz *LazyInstruction) SelfDelete() {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return
	}
	lz.Instruction.SelfDelete()
}

func (lz *LazyInstruction) IsCFGEnterBlock() ([]Instruction, bool) {
	lz.check()
	if utils.IsNil(lz.Instruction) {
		return nil, false
	}
	return lz.Instruction.IsCFGEnterBlock()
}

func (lz *LazyInstruction) AddMask(v Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.AddMask(v)
}

func (lz *LazyInstruction) AddMember(v1 Value, v2 Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.AddMember(v1, v2)
}

func (lz *LazyInstruction) AddVariable(v *Variable) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.AddVariable(v)
}

func (lz *LazyInstruction) DeleteMember(v Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.DeleteMember(v)
}

func (lz *LazyInstruction) ForEachMember(fn func(Value, Value) bool) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.ForEachMember(fn)
}

func (lz *LazyInstruction) GetAllMember() map[Value]Value {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetAllMember()
}

func (lz *LazyInstruction) GetAllVariables() map[string]*Variable {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetAllVariables()
}

func (lz *LazyInstruction) GetIndexMember(i int) (Value, bool) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil, false
	}
	return lz.Value.GetIndexMember(i)
}

func (lz *LazyInstruction) GetKey() Value {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetKey()
}

func (lz *LazyInstruction) GetLastVariable() *Variable {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetLastVariable()
}

func (lz *LazyInstruction) GetMask() []Value {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetMask()
}

func (lz *LazyInstruction) GetMember(v Value) (Value, bool) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil, false
	}
	return lz.Value.GetMember(v)
}

func (lz *LazyInstruction) GetObject() Value {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetObject()
}

func (lz *LazyInstruction) GetStringMember(n string) (Value, bool) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil, false
	}
	return lz.Value.GetStringMember(n)
}

func (lz *LazyInstruction) GetType() Type {
	lz.check()
	if utils.IsNil(lz.Value) {
		log.Errorf("[BUG]: lazyInstruction value is nil,get type fail: %d", lz.id)
		return nil
	}
	return lz.Value.GetType()
}

func (lz *LazyInstruction) GetUsers() Users {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetUsers()
}

func (lz *LazyInstruction) GetValues() Values {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetValues()
}

func (lz *LazyInstruction) GetVariable(n string) *Variable {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	if v, ok := lz.variable[n]; ok {
		return v
	}
	{
		v := GetVariableFromDB(lz.id, n)
		v.Assign(lz)
		lz.variable[n] = v
		return v
	}
	// return lz.Value.GetVariable(n)
}

func (lz *LazyInstruction) Masked() bool {
	lz.check()
	if utils.IsNil(lz.Value) {
		return false
	}
	return lz.Value.Masked()
}

func (lz *LazyInstruction) NewError(e ErrorKind, t ErrorTag, msg string) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.NewError(e, t, msg)
}

func (lz *LazyInstruction) SetKey(v Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.SetKey(v)
}

func (lz *LazyInstruction) SetObject(v Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.SetObject(v)
}

func (lz *LazyInstruction) SetType(t Type) {
	lz.check()
	if utils.IsNil(lz.Value) {
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
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetPointer()
}

func (lz *LazyInstruction) AddPointer(v Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.AddPointer(v)
}
func (lz *LazyInstruction) GetReference() Value {
	lz.check()
	if utils.IsNil(lz.Value) {
		return nil
	}
	return lz.Value.GetReference()
}

func (lz *LazyInstruction) SetReference(v Value) {
	lz.check()
	if utils.IsNil(lz.Value) {
		return
	}
	lz.Value.SetReference(v)
}

func (lz *LazyInstruction) AddOccultation(p Value) {

}

func (lz *LazyInstruction) FlatOccultation() []Value {
	lz.check()
	var ret []Value
	var handler func(i *anValue)

	handler = func(i *anValue) {
		for _, vId := range i.occultation {
			// Corrected: Use GetValueById from the program's cache
			v, ok := lz.GetValueById(vId)
			if !ok || v == nil {
				continue
			}
			ret = append(ret, v)
			if p, ok := ToPhi(v); ok {
				handler(p.anValue)
			}
		}
	}
	if u, ok := ToUndefined(lz.Value); ok {
		handler(u.anValue)
	} else if e, ok := ToExternLib(lz.Value); ok {
		handler(e.anValue)
	}

	return ret
}

func (lz *LazyInstruction) getAnInstruction() *anInstruction {
	return lz.Instruction.getAnInstruction()
}

func (lz *LazyInstruction) getAnValue() *anValue {
	return lz.Value.getAnValue()
}
