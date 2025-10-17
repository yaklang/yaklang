package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

func (lz *LazyInstruction) GetInstructionById(id int64) (Instruction, bool) {
	lz.check()
	if lz.Instruction == nil || lz.GetProgram() == nil || lz.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[Instruction](lz.GetProgram().Cache, func(i Instruction) (Instruction, bool) {
		return i, true
	}, id)
}

func (lz *LazyInstruction) GetValueById(id int64) (Value, bool) {
	lz.check()
	if lz.Instruction == nil || lz.GetProgram() == nil || lz.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[Value](lz.GetProgram().Cache, ToValue, id)
}

func (lz *LazyInstruction) GetUsersByID(id int64) (User, bool) {
	lz.check()
	if lz.Instruction == nil || lz.GetProgram() == nil || lz.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[User](lz.GetProgram().Cache, ToUser, id)
}

func (lz *LazyInstruction) GetValuesByIDs(ids []int64) Values {
	lz.check()
	if lz.Instruction == nil || lz.GetProgram() == nil || lz.GetProgram().Cache == nil {
		return nil
	}
	return GetExs[Value](lz.GetProgram().Cache, ToValue, ids...)
}

func (lz *LazyInstruction) GetUsersByIDs(ids []int64) Users {
	lz.check()
	if lz.Instruction == nil || lz.GetProgram() == nil || lz.GetProgram().Cache == nil {
		return nil
	}
	return GetExs[User](lz.GetProgram().Cache, ToUser, ids...)
}

func (i *anInstruction) GetInstructionById(id int64) (Instruction, bool) {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[Instruction](i.GetProgram().Cache, func(i Instruction) (Instruction, bool) {
		return i, true
	}, id)
}

func (i *anInstruction) GetValueById(id int64) (Value, bool) {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[Value](i.GetProgram().Cache, ToValue, id)
}

func (i *anInstruction) GetUsersByID(id int64) (User, bool) {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[User](i.GetProgram().Cache, ToUser, id)
}

func (i *anInstruction) GetInstructionsByIDs(ids []int64) []Instruction {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil
	}
	return GetExs[Instruction](i.GetProgram().Cache, func(i Instruction) (Instruction, bool) {
		return i, true
	}, ids...)
}

func (i *anInstruction) GetValuesByIDs(ids []int64) Values {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil
	}
	return GetExs[Value](i.GetProgram().Cache, ToValue, ids...)
}

func (i *anInstruction) GetUsersByIDs(ids []int64) Users {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil
	}
	return GetExs[User](i.GetProgram().Cache, ToUser, ids...)
}

func (i *anInstruction) GetBasicBlockByID(id int64) (*BasicBlock, bool) {
	if i == nil || i.GetProgram() == nil || i.GetProgram().Cache == nil {
		return nil, false
	}
	return GetEx[*BasicBlock](i.GetProgram().Cache, ToBasicBlock, id)
}

func (v Values) GetIds() []int64 {
	ret := make([]int64, 0)
	for _, v := range v {
		ret = append(ret, v.GetId())
	}
	return ret
}
func GetEx[T any](c *ProgramCache, Cover func(Instruction) (T, bool), id int64) (T, bool) {
	var zero T
	if c == nil {
		return zero, false
	}
	slice := GetExs[T](c, Cover, id)
	if len(slice) == 0 {
		return zero, false
	}
	return slice[0], !utils.IsNil(slice[0])
}

func GetExs[T any](c *ProgramCache, Cover func(Instruction) (T, bool), ids ...int64) []T {
	if c == nil {
		return nil
	}

	ret := make([]T, 0)
	for _, id := range ids {
		if id == 0 {
			continue
		}
		inst := c.GetInstruction(id)
		v, ok := Cover(inst)
		if !ok {
			if utils.IsNil(inst) {
				log.Debugf("BUG::: nil instruction %v err: %d", inst, id)
			} else if IsControlInstruction(inst) {
				// log.Errorf("BUG::: control instruction %v err: %d", inst, id)
			} else {
				log.Debugf("BUG::: %v err: %d", inst, id)
			}
			continue
		}
		ret = append(ret, v)
	}
	return ret
}
